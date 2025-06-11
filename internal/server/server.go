package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	_ "github.com/joho/godotenv/autoload"

	"flash_sale_contest/internal/cache"
	"flash_sale_contest/internal/database"
	"flash_sale_contest/internal/metrics"
	"flash_sale_contest/internal/sale"
)

type Server struct {
	port        int
	db          database.Service
	cache       cache.Service
	saleManager *sale.Manager
	metrics     metrics.Service
}

func NewServer() *http.Server {
	port, _ := strconv.Atoi(os.Getenv("PORT"))

	dbService := database.New()
	cacheService := cache.New()
	metricsService := metrics.New()
	saleManager := sale.NewManager(dbService, cacheService)

	NewServer := &Server{
		port:        port,
		db:          dbService,
		cache:       cacheService,
		saleManager: saleManager,
		metrics:     metricsService,
	}

	ctx := context.Background()
	if err := saleManager.Start(ctx); err != nil {
		log.Fatalf("Failed to start sale manager: %v", err)
	}

	server := &http.Server{
		Addr:           fmt.Sprintf(":%d", NewServer.port),
		Handler:        NewServer.RegisterRoutes(),
		IdleTimeout:    time.Minute,
		ReadTimeout:    10 * time.Second,
		WriteTimeout:   30 * time.Second,
	}

	return server
}
