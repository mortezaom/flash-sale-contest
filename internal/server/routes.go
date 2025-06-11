package server

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	"flash_sale_contest/internal/cache"
	"flash_sale_contest/internal/database"
)

func (s *Server) RegisterRoutes() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("/", s.HelloWorldHandler)
	mux.HandleFunc("/health", s.healthHandler)
	mux.HandleFunc("/metrics", s.metricsHandler)

	mux.HandleFunc("/sale/current", s.currentSaleHandler)
	mux.HandleFunc("/sale/status", s.saleStatusHandler)
	mux.HandleFunc("/sale/info", s.saleInfoHandler)

	mux.HandleFunc("POST /checkout", s.checkoutHandler)
	mux.HandleFunc("POST /purchase", s.purchaseHandler)

	return s.corsMiddleware(mux)
}

func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, PATCH")
		w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-CSRF-Token")
		w.Header().Set("Access-Control-Allow-Credentials", "false")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func (s *Server) HelloWorldHandler(w http.ResponseWriter, r *http.Request) {
	resp := map[string]string{"message": "Flash Sale Contest API - Ready for High Load!"}
	jsonResp, _ := json.Marshal(resp)
	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonResp)
}

func (s *Server) metricsHandler(w http.ResponseWriter, r *http.Request) {
	stats := s.metrics.GetStats()
	jsonResp, _ := json.Marshal(stats)
	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonResp)
}

func (s *Server) saleStatusHandler(w http.ResponseWriter, r *http.Request) {
	activeSale := s.saleManager.GetCurrentSale()
	if activeSale == nil {
		http.Error(w, "No active sale", http.StatusNotFound)
		return
	}

	ctx := r.Context()
	remaining, err := s.cache.GetInventoryStatus(ctx, activeSale.SaleID)
	if err != nil {
		log.Printf("Failed to get inventory status: %v", err)
		remaining = -1
	}

	resp := map[string]interface{}{
		"sale_id":                activeSale.SaleID,
		"remaining_items":        remaining,
		"items_sold":             10000 - remaining,
		"sale_ends_at":           activeSale.EndTime,
		"time_remaining_seconds": int(time.Until(activeSale.EndTime).Seconds()),
	}

	jsonResp, _ := json.Marshal(resp)
	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonResp)
}

func (s *Server) currentSaleHandler(w http.ResponseWriter, r *http.Request) {
	activeSale := s.saleManager.GetCurrentSale()
	if activeSale == nil {
		http.Error(w, "No active sale", http.StatusNotFound)
		return
	}

	resp := map[string]interface{}{
		"sale_id":    activeSale.SaleID,
		"start_time": activeSale.StartTime,
		"end_time":   activeSale.EndTime,
	}

	jsonResp, _ := json.Marshal(resp)
	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonResp)
}

func (s *Server) checkoutHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	s.metrics.IncrementCheckoutRequests()

	userID := r.URL.Query().Get("user_id")
	itemID := r.URL.Query().Get("id")

	if userID == "" || itemID == "" {
		s.metrics.IncrementCheckoutFailed()
		http.Error(w, "user_id and id are required", http.StatusBadRequest)
		return
	}

	s.metrics.UpdateActiveUser(userID)

	activeSale := s.saleManager.GetCurrentSale()
	if activeSale == nil {
		s.metrics.IncrementCheckoutFailed()
		http.Error(w, "No active sale", http.StatusServiceUnavailable)
		return
	}

	ctx := r.Context()

	code, err := s.cache.ReserveItem(ctx, activeSale.SaleID, userID, itemID)
	if err != nil {
		s.metrics.IncrementCheckoutFailed()

		if err.Error() == "sold out" {
			s.metrics.IncrementSoldOutErrors()
			http.Error(w, "Item sold out", http.StatusConflict)
			return
		}
		if err.Error() == "user limit exceeded" {
			s.metrics.IncrementUserLimitErrors()
			http.Error(w, "Purchase limit exceeded", http.StatusForbidden)
			return
		}

		http.Error(w, "Failed to reserve item", http.StatusInternalServerError)
		return
	}

	s.metrics.IncrementCheckoutSuccess()
	s.metrics.RecordCheckoutLatency(time.Since(start))

	go func() {
		attempt := &database.CheckoutAttempt{
			SaleID: activeSale.SaleID,
			UserID: userID,
			ItemID: itemID,
			Code:   code,
			Status: false,
		}
		s.db.LogCheckoutAttempt(context.Background(), attempt)
	}()

	resp := map[string]string{"code": code}
	jsonResp, _ := json.Marshal(resp)
	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonResp)
}

func (s *Server) purchaseHandler(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	s.metrics.IncrementPurchaseRequests()

	code := r.URL.Query().Get("code")
	if code == "" {
		s.metrics.IncrementPurchaseFailed()
		http.Error(w, "code is required", http.StatusBadRequest)
		return
	}

	ctx := r.Context()
	checkoutInfo, err := s.cache.VerifyAndPurchase(ctx, code)
	if err != nil {
		s.metrics.IncrementPurchaseFailed()
		s.metrics.IncrementCodeInvalidErrors()
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := s.cache.IncrementUserPurchase(ctx, checkoutInfo.SaleID, checkoutInfo.UserID); err != nil {
		s.metrics.IncrementPurchaseFailed()
		http.Error(w, "Failed to complete purchase", http.StatusInternalServerError)
		return
	}

	s.metrics.IncrementPurchaseSuccess()
	s.metrics.IncrementItemsSold()
	s.metrics.RecordPurchaseLatency(time.Since(start))

	go func(info *cache.CheckoutInfo) {
		parts := strings.Split(info.ItemID, "_item_")
		if len(parts) == 2 {
			if itemNumber, err := strconv.Atoi(parts[1]); err == nil {
				s.cache.MarkItemAsSold(context.Background(), info.SaleID, itemNumber)
			}
		}

		purchase := &database.Purchase{
			SaleID: info.SaleID,
			UserID: info.UserID,
			ItemID: info.ItemID,
		}
		if err := s.db.CreatePurchase(context.Background(), purchase); err != nil {
			log.Printf("FATAL: Failed to log purchase to DB for code %s: %v", code, err)
		}
		s.db.UpdateCheckoutStatus(context.Background(), code, true)
	}(checkoutInfo)

	resp := map[string]interface{}{
		"success": true,
		"user_id": checkoutInfo.UserID,
		"item_id": checkoutInfo.ItemID,
		"sale_id": checkoutInfo.SaleID,
	}
	jsonResp, _ := json.Marshal(resp)
	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonResp)
}

func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	health := map[string]interface{}{
		"database": s.db.Health(),
		"cache":    s.cache.Health(),
		"metrics":  s.metrics.GetStats(),
		"status":   "ok",
	}

	resp, _ := json.Marshal(health)
	w.Header().Set("Content-Type", "application/json")
	w.Write(resp)
}

func (s *Server) saleInfoHandler(w http.ResponseWriter, r *http.Request) {
	activeSale := s.saleManager.GetCurrentSale()
	if activeSale == nil {
		http.Error(w, "No active sale", http.StatusServiceUnavailable)
		return
	}

	ctx := r.Context()
	var showcase *cache.ShowcaseInfo

	showcase, err := s.cache.GetShowcaseInfo(ctx, activeSale.SaleID)
	if err != nil {
		log.Printf("Cache miss for showcase on sale %s. Fetching from DB.", activeSale.SaleID)
		firstIDs, lastIDs, dbErr := s.db.GetShowcaseItemIDs(ctx, activeSale.SaleID, 10)
		if dbErr != nil {
			http.Error(w, "Failed to retrieve sale info", http.StatusInternalServerError)
			return
		}
		showcase = &cache.ShowcaseInfo{FirstItemIDs: firstIDs, LastItemIDs: lastIDs}

		go s.cache.SetShowcaseInfo(context.Background(), activeSale.SaleID, showcase)
	}

	info := map[string]interface{}{
		"sale_id":     activeSale.SaleID,
		"total_items": 10000,
		"first_items": showcase.FirstItemIDs,
		"last_items":  showcase.LastItemIDs,
	}

	jsonResp, _ := json.Marshal(info)
	w.Header().Set("Content-Type", "application/json")
	w.Write(jsonResp)
}
