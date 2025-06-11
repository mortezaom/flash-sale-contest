package database

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	_ "github.com/joho/godotenv/autoload"
)

type Sale struct {
	ID         string    `json:"id"`
	SaleID     string    `json:"sale_id"`
	StartTime  time.Time `json:"start_time"`
	EndTime    time.Time `json:"end_time"`
	TotalItems int       `json:"total_items"`
	ItemsSold  int       `json:"items_sold"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

type Item struct {
	ID       string `json:"id"`
	ItemID   string `json:"item_id"`
	SaleID   string `json:"sale_id"`
	Name     string `json:"name"`
	ImageURL string `json:"image_url"`
}

type CheckoutAttempt struct {
	ID        string    `json:"id"`
	SaleID    string    `json:"sale_id"`
	UserID    string    `json:"user_id"`
	ItemID    string    `json:"item_id"`
	Code      string    `json:"code"`
	Status    bool    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}

type Purchase struct {
	ID           string    `json:"id"`
	SaleID       string    `json:"sale_id"`
	UserID       string    `json:"user_id"`
	ItemID       string    `json:"item_id"`
	PurchaseTime time.Time `json:"purchase_time"`
}

type Service interface {
	Health() map[string]string
	Close() error
	RunMigrations() error
	CreateSale(ctx context.Context, sale *Sale) error
	CreateItems(ctx context.Context, items []Item) error
	GetActiveSale(ctx context.Context) (*Sale, error)
	GetSaleItems(ctx context.Context, saleID string, limit int) ([]Item, error)
	LogCheckoutAttempt(ctx context.Context, attempt *CheckoutAttempt) error
	CreatePurchase(ctx context.Context, purchase *Purchase) error
	UpdateCheckoutStatus(ctx context.Context, code string, status bool) error
	GetShowcaseItemIDs(ctx context.Context, saleID string, limit int) (firstIDs, lastIDs []string, err error)
}

type service struct {
	db *sql.DB
}

var (
	database   = os.Getenv("BLUEPRINT_DB_DATABASE")
	password   = os.Getenv("BLUEPRINT_DB_PASSWORD")
	username   = os.Getenv("BLUEPRINT_DB_USERNAME")
	port       = os.Getenv("BLUEPRINT_DB_PORT")
	host       = os.Getenv("BLUEPRINT_DB_HOST")
	schema     = os.Getenv("BLUEPRINT_DB_SCHEMA")
	dbInstance *service
)

func New() Service {
	if dbInstance != nil {
		return dbInstance
	}
	connStr := fmt.Sprintf("postgres://%s:%s@%s:%s/%s?sslmode=disable&search_path=%s", username, password, host, port, database, schema)
	db, err := sql.Open("pgx", connStr)
	if err != nil {
		log.Fatal(err)
	}

	db.SetMaxOpenConns(100)
	db.SetMaxIdleConns(20)
	db.SetConnMaxLifetime(5 * time.Minute)

	dbInstance = &service{db: db}
	return dbInstance
}

func (s *service) Health() map[string]string {
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	stats := make(map[string]string)
	err := s.db.PingContext(ctx)
	if err != nil {
		stats["status"] = "down"
		stats["error"] = fmt.Sprintf("db down: %v", err)
		return stats
	}

	stats["status"] = "up"
	stats["message"] = "healthy"

	dbStats := s.db.Stats()
	stats["open_connections"] = strconv.Itoa(dbStats.OpenConnections)
	stats["in_use"] = strconv.Itoa(dbStats.InUse)
	stats["idle"] = strconv.Itoa(dbStats.Idle)

	return stats
}

func (s *service) Close() error {
	log.Printf("Disconnected from database: %s", database)
	return s.db.Close()
}

func (s *service) CreateSale(ctx context.Context, sale *Sale) error {
	query := `INSERT INTO sales (sale_id, start_time, end_time, total_items, status) VALUES ($1, $2, $3, $4, $5)`
	_, err := s.db.ExecContext(ctx, query, sale.SaleID, sale.StartTime, sale.EndTime, sale.TotalItems, sale.Status)
	return err
}

func (s *service) CreateItems(ctx context.Context, items []Item) error {
	if len(items) == 0 {
		return nil
	}

	batchSize := 1000
	for i := 0; i < len(items); i += batchSize {
		end := i + batchSize
		if end > len(items) {
			end = len(items)
		}

		if err := s.createItemsBatch(ctx, items[i:end]); err != nil {
			return err
		}
	}

	return nil
}

func (s *service) createItemsBatch(ctx context.Context, items []Item) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `INSERT INTO items (item_id, sale_id, name, image_url) VALUES ($1, $2, $3, $4)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, item := range items {
		_, err := stmt.ExecContext(ctx, item.ItemID, item.SaleID, item.Name, item.ImageURL)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *service) GetActiveSale(ctx context.Context) (*Sale, error) {
	query := `SELECT sale_id, start_time, end_time, total_items, items_sold, status FROM sales WHERE status = 'active' ORDER BY start_time DESC LIMIT 1`
	row := s.db.QueryRowContext(ctx, query)

	var sale Sale
	err := row.Scan(&sale.SaleID, &sale.StartTime, &sale.EndTime, &sale.TotalItems, &sale.ItemsSold, &sale.Status)
	if err != nil {
		return nil, err
	}
	return &sale, nil
}

func (s *service) GetSaleItems(ctx context.Context, saleID string, limit int) ([]Item, error) {
	query := `SELECT item_id, sale_id, name, image_url FROM items WHERE sale_id = $1 ORDER BY created_at LIMIT $2`
	rows, err := s.db.QueryContext(ctx, query, saleID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []Item
	for rows.Next() {
		var item Item
		err := rows.Scan(&item.ItemID, &item.SaleID, &item.Name, &item.ImageURL)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *service) LogCheckoutAttempt(ctx context.Context, attempt *CheckoutAttempt) error {
	query := `INSERT INTO checkout_attempts (sale_id, user_id, item_id, code, status) VALUES ($1, $2, $3, $4, $5)`
	_, err := s.db.ExecContext(ctx, query, attempt.SaleID, attempt.UserID, attempt.ItemID, attempt.Code, attempt.Status)
	return err
}

func (s *service) CreatePurchase(ctx context.Context, purchase *Purchase) error {
	query := `INSERT INTO purchases (sale_id, user_id, item_id) VALUES ($1, $2, $3)`
	_, err := s.db.ExecContext(ctx, query, purchase.SaleID, purchase.UserID, purchase.ItemID)
	return err
}

func (s *service) UpdateCheckoutStatus(ctx context.Context, code string, status bool) error {
	query := `UPDATE checkout_attempts SET status = $1 WHERE code = $2`
	_, err := s.db.ExecContext(ctx, query, status, code)
	return err
}
func (s *service) GetShowcaseItemIDs(ctx context.Context, saleID string, limit int) (firstIDs, lastIDs []string, err error) {
	// Get first N items
	firstQuery := `SELECT item_id FROM items WHERE sale_id = $1 ORDER BY item_id ASC LIMIT $2`
	rows, err := s.db.QueryContext(ctx, firstQuery, saleID, limit)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to query first items: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, nil, err
		}
		firstIDs = append(firstIDs, id)
	}

	// Get last N items
	lastQuery := `SELECT item_id FROM items WHERE sale_id = $1 ORDER BY item_id DESC LIMIT $2`
	rows, err = s.db.QueryContext(ctx, lastQuery, saleID, limit)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to query last items: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, nil, err
		}
		lastIDs = append(lastIDs, id)
	}

	return firstIDs, lastIDs, nil
}