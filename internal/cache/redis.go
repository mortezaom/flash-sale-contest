package cache

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	codeExpiryTime  = 5 * time.Minute
	maxRetries      = 3
)

type CheckoutInfo struct {
	UserID    string    `json:"user_id"`
	ItemID    string    `json:"item_id"`
	SaleID    string    `json:"sale_id"`
	ExpiresAt time.Time `json:"expires_at"`
}

type Service interface {
	Health() map[string]string
	Close() error
	InitializeSale(ctx context.Context, saleID string, totalItems int) error
	ReserveItem(ctx context.Context, saleID, userID, itemID string) (string, error)
	VerifyAndPurchase(ctx context.Context, code string) (*CheckoutInfo, error)
	GetUserPurchaseCount(ctx context.Context, saleID, userID string) (int, error)
	IncrementUserPurchase(ctx context.Context, saleID, userID string) error
	GetInventoryStatus(ctx context.Context, saleID string) (int, error)
	CleanupExpiredCodes(ctx context.Context, saleID string) error
	SetShowcaseInfo(ctx context.Context, saleID string, info *ShowcaseInfo) error
	GetShowcaseInfo(ctx context.Context, saleID string) (*ShowcaseInfo, error)
	MarkItemAsSold(ctx context.Context, saleID string, itemNumber int) error
}

type ShowcaseInfo struct {
	FirstItemIDs []string `json:"first_item_ids"`
	LastItemIDs  []string `json:"last_item_ids"`
}

type service struct {
	client *redis.Client
}

var cacheInstance *service

func New() Service {
	if cacheInstance != nil {
		return cacheInstance
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:         os.Getenv("REDIS_ADDR"),
		Password:     os.Getenv("REDIS_PASSWORD"),
		DB:           0,
		PoolSize:     200, // Increased pool size
		MinIdleConns: 50,  // More idle connections
		MaxRetries:   maxRetries,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
		PoolTimeout:  4 * time.Second,
	})

	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Fatalf("Failed to connect to Redis: %v", err)
	}

	log.Println("Connected to Redis with optimized settings")
	cacheInstance = &service{client: rdb}
	return cacheInstance
}

func (s *service) Health() map[string]string {
	stats := make(map[string]string)
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	start := time.Now()
	if err := s.client.Ping(ctx).Err(); err != nil {
		stats["status"] = "down"
		stats["error"] = fmt.Sprintf("redis down: %v", err)
		return stats
	}

	latency := time.Since(start)
	poolStats := s.client.PoolStats()

	stats["status"] = "up"
	stats["message"] = "healthy"
	stats["ping_latency_ms"] = fmt.Sprintf("%.2f", float64(latency.Nanoseconds())/1e6)
	stats["pool_hits"] = strconv.Itoa(int(poolStats.Hits))
	stats["pool_misses"] = strconv.Itoa(int(poolStats.Misses))
	stats["pool_total_conns"] = strconv.Itoa(int(poolStats.TotalConns))
	stats["pool_idle_conns"] = strconv.Itoa(int(poolStats.IdleConns))

	return stats
}

func (s *service) Close() error {
	return s.client.Close()
}

func (s *service) InitializeSale(ctx context.Context, saleID string, totalItems int) error {
	pipe := s.client.Pipeline()
	inventoryKey := fmt.Sprintf("sale:%s:inventory", saleID)

	pipe.Set(ctx, inventoryKey, totalItems, time.Hour+10*time.Minute)

	pipe.Set(ctx, fmt.Sprintf("sale:%s:active", saleID), "1", time.Hour+10*time.Minute)
	pipe.Del(ctx, fmt.Sprintf("sale:%s:user_purchases", saleID))
	pipe.Del(ctx, fmt.Sprintf("sale:%s:sold_bitmap", saleID))

	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize sale: %w", err)
	}

	log.Printf("Initialized sale %s with %d items on a single key", saleID, totalItems)
	return nil
}

func (s *service) ReserveItem(ctx context.Context, saleID, userID, itemID string) (string, error) {
	// The Lua script is now simpler as it doesn't need shard logic.
	luaScript := `
		local inventory_key = KEYS[1]
		local user_key = KEYS[2]
		local user_id = ARGV[1]
		local max_per_user = tonumber(ARGV[2])

		-- Check user limit first
		local user_count = redis.call('HGET', user_key, user_id)
		if user_count and tonumber(user_count) >= max_per_user then
			return "user_limit_exceeded"
		end

		-- Try to reserve inventory
		local remaining = redis.call('DECR', inventory_key)
		if remaining < 0 then
			redis.call('INCR', inventory_key)
			return "sold_out"
		end

		return "success"
	`
	// The inventory key is now simple and singular.
	inventoryKey := fmt.Sprintf("sale:%s:inventory", saleID)
	userKey := fmt.Sprintf("sale:%s:user_purchases", saleID)

	// We no longer pass a shard_id to the script.
	result, err := s.client.Eval(ctx, luaScript, []string{inventoryKey, userKey}, userID, 10).Result()
	if err != nil {
		return "", err
	}

	status := result.(string)
	if status == "user_limit_exceeded" {
		return "", fmt.Errorf("user limit exceeded")
	}
	if status == "sold_out" {
		return "", fmt.Errorf("sold out")
	}

	// This part remains the same.
	code := s.generateCode()
	checkoutInfo := CheckoutInfo{
		UserID:    userID,
		ItemID:    itemID,
		SaleID:    saleID,
		ExpiresAt: time.Now().Add(codeExpiryTime),
	}

	data, _ := json.Marshal(checkoutInfo)
	codeKey := fmt.Sprintf("checkout_code:%s", code)

	err = s.client.Set(ctx, codeKey, data, codeExpiryTime).Err()
	if err != nil {
		// If setting the code fails, we must return the inventory.
		s.client.Incr(ctx, inventoryKey)
		return "", err
	}

	return code, nil
}

func (s *service) VerifyAndPurchase(ctx context.Context, code string) (*CheckoutInfo, error) {
	codeKey := fmt.Sprintf("checkout_code:%s", code)

	data, err := s.client.GetDel(ctx, codeKey).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, fmt.Errorf("invalid or expired code")
		}
		return nil, err
	}

	var checkoutInfo CheckoutInfo
	if err := json.Unmarshal([]byte(data), &checkoutInfo); err != nil {
		return nil, err
	}

	if time.Now().After(checkoutInfo.ExpiresAt) {
		return nil, fmt.Errorf("code expired")
	}

	return &checkoutInfo, nil
}

func (s *service) GetUserPurchaseCount(ctx context.Context, saleID, userID string) (int, error) {
	key := fmt.Sprintf("sale:%s:user_purchases", saleID)
	result := s.client.HGet(ctx, key, userID)

	if result.Err() == redis.Nil {
		return 0, nil
	}
	if result.Err() != nil {
		return 0, result.Err()
	}

	count, err := strconv.Atoi(result.Val())
	if err != nil {
		return 0, err
	}
	return count, nil
}

func (s *service) IncrementUserPurchase(ctx context.Context, saleID, userID string) error {
	key := fmt.Sprintf("sale:%s:user_purchases", saleID)
	return s.client.HIncrBy(ctx, key, userID, 1).Err()
}

func (s *service) CleanupExpiredCodes(ctx context.Context, saleID string) error {
	pattern := "checkout_code:*"
	iter := s.client.Scan(ctx, 0, pattern, 100).Iterator()

	var expiredKeys []string
	for iter.Next(ctx) {
		key := iter.Val()
		ttl := s.client.TTL(ctx, key).Val()
		if ttl < 0 { // Key exists but has no TTL or is expired
			expiredKeys = append(expiredKeys, key)
		}
	}

	if len(expiredKeys) > 0 {
		return s.client.Del(ctx, expiredKeys...).Err()
	}

	return iter.Err()
}

func (s *service) GetInventoryStatus(ctx context.Context, saleID string) (int, error) {
	inventoryKey := fmt.Sprintf("sale:%s:inventory", saleID)

	val, err := s.client.Get(ctx, inventoryKey).Int()
	if err != nil {
		if err == redis.Nil {
			return 0, nil
		}
		return 0, err
	}

	return val, nil
}

func (s *service) generateCode() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

func (s *service) MarkItemAsSold(ctx context.Context, saleID string, itemNumber int) error {
	if itemNumber <= 0 {
		return fmt.Errorf("itemNumber must be positive")
	}
	key := fmt.Sprintf("sale:%s:sold_bitmap", saleID)
	// Redis bitmaps are 0-indexed, so we subtract 1 from the item number.
	offset := int64(itemNumber - 1)
	return s.client.SetBit(ctx, key, offset, 1).Err()
}

func (s *service) SetShowcaseInfo(ctx context.Context, saleID string, info *ShowcaseInfo) error {
	key := fmt.Sprintf("sale:%s:showcase_ids", saleID)
	data, err := json.Marshal(info)
	if err != nil {
		return err
	}
	return s.client.Set(ctx, key, data, time.Hour+10*time.Minute).Err()
}

// GetShowcaseInfo retrieves the minimal showcase data.
func (s *service) GetShowcaseInfo(ctx context.Context, saleID string) (*ShowcaseInfo, error) {
	key := fmt.Sprintf("sale:%s:showcase_ids", saleID)
	data, err := s.client.Get(ctx, key).Result()
	if err != nil {
		return nil, err
	}

	var info ShowcaseInfo
	if err := json.Unmarshal([]byte(data), &info); err != nil {
		return nil, err
	}
	return &info, nil
}
