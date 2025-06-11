package sale

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	"flash_sale_contest/internal/cache"
	"flash_sale_contest/internal/database"
)

var (
	adjectives = []string{
		"Amazing", "Brilliant", "Cosmic", "Dynamic", "Epic", "Fantastic", "Golden", "Heroic",
		"Incredible", "Legendary", "Magical", "Nuclear", "Omega", "Premium", "Quantum", "Royal",
		"Supreme", "Titanium", "Ultra", "Vivid", "Wondrous", "Xtreme", "Youthful", "Zestful",
		"Awesome", "Blazing", "Crimson", "Dazzling", "Electric", "Fiery", "Glorious", "Hypnotic",
		"Infinite", "Jade", "Kinetic", "Luminous", "Mystic", "Neon", "Obsidian", "Platinum",
		"Radiant", "Stellar", "Thunderous", "Unbreakable", "Volcanic", "Wicked", "Xenial", "Zealous",
	}

	nouns = []string{
		"Blade", "Crystal", "Dragon", "Eagle", "Falcon", "Gem", "Hammer", "Island", "Jewel",
		"Knight", "Lion", "Mountain", "Ninja", "Orb", "Phoenix", "Quest", "Ring", "Star",
		"Thunder", "Universe", "Vortex", "Warrior", "Yacht", "Zenith", "Armor", "Bow", "Crown",
		"Dagger", "Engine", "Fire", "Guardian", "Heart", "Ice", "Justice", "Key", "Light",
		"Mirror", "Nexus", "Onyx", "Prism", "Quiver", "Relic", "Sword", "Temple", "Uplink",
		"Victory", "Wings", "Xerus", "Yoke", "Zone", "Arrow", "Banner", "Cipher", "Dome",
	}
)

type Manager struct {
	db     database.Service
	cache  cache.Service
	mu     sync.RWMutex
	active *ActiveSale
}

type ActiveSale struct {
	SaleID    string
	StartTime time.Time
	EndTime   time.Time
}

func NewManager(db database.Service, cache cache.Service) *Manager {
	return &Manager{
		db:    db,
		cache: cache,
	}
}

func (m *Manager) Start(ctx context.Context) error {
	if err := m.db.RunMigrations(); err != nil {
		return fmt.Errorf("failed to run migrations: %w", err)
	}

	if err := m.startNewSale(ctx); err != nil {
		return fmt.Errorf("failed to start initial sale: %w", err)
	}

	ticker := time.NewTicker(time.Hour)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if err := m.startNewSale(ctx); err != nil {
					log.Printf("Failed to start new sale: %v", err)
				}
			}
		}
	}()

	log.Println("Sale manager started")
	return nil
}

func (m *Manager) GetCurrentSale() *ActiveSale {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.active
}

func (m *Manager) startNewSale(ctx context.Context) error {
	now := time.Now()
	saleID := fmt.Sprintf("sale_%d", now.Unix())
	log.Printf("Starting new sale: %s", saleID)

	// ... (CreateSale in DB) ...
	if err := m.db.CreateSale(ctx, &database.Sale{
		SaleID:     saleID,
		StartTime:  now,
		EndTime:    now.Add(time.Hour),
		TotalItems: 10000,
		Status:     "active",
	}); err != nil {
		return fmt.Errorf("failed to create sale: %w", err)
	}

	items := m.generateItems(saleID, 10000)
	if err := m.db.CreateItems(ctx, items); err != nil {
		return fmt.Errorf("failed to create items: %w", err)
	}

	firstIDs, lastIDs, err := m.db.GetShowcaseItemIDs(ctx, saleID, 10)
	if err != nil {
		log.Printf("Warning: could not get showcase IDs for cache warming: %v", err)
	} else {
		showcaseInfo := &cache.ShowcaseInfo{
			FirstItemIDs: firstIDs,
			LastItemIDs:  lastIDs,
		}
		if err := m.cache.SetShowcaseInfo(ctx, saleID, showcaseInfo); err != nil {
			log.Printf("Warning: failed to cache showcase info: %v", err)
		}
	}

	if err := m.cache.InitializeSale(ctx, saleID, 10000); err != nil {
		return fmt.Errorf("failed to initialize cache: %w", err)
	}

	m.mu.Lock()
	m.active = &ActiveSale{
		SaleID:    saleID,
		StartTime: now,
		EndTime:   now.Add(time.Hour),
	}
	m.mu.Unlock()

	log.Printf("Sale %s is active.", saleID)
	return nil
}

func (m *Manager) generateItems(saleID string, count int) []database.Item {
	items := make([]database.Item, count)

	for i := 0; i < count; i++ {
		adj := adjectives[rand.Intn(len(adjectives))]
		noun := nouns[rand.Intn(len(nouns))]
		name := fmt.Sprintf("%s %s", adj, noun)

		itemID := fmt.Sprintf("%s_item_%06d", saleID, i+1)
		imageURL := fmt.Sprintf("https://via.placeholder.com/400x400/%06x/FFFFFF?text=%s+%s",
			rand.Intn(0xFFFFFF), adj, noun)

		items[i] = database.Item{
			ItemID:   itemID,
			SaleID:   saleID,
			Name:     name,
			ImageURL: imageURL,
		}
	}

	return items
}
