package metrics

import (
	"sync"
	"sync/atomic"
	"time"
)

type Metrics struct {
	CheckoutRequests  int64
	CheckoutSuccess   int64
	CheckoutFailed    int64
	PurchaseRequests  int64
	PurchaseSuccess   int64
	PurchaseFailed    int64
	SoldOutErrors     int64
	UserLimitErrors   int64
	CodeInvalidErrors int64

	AvgCheckoutLatency int64 // nanoseconds
	AvgPurchaseLatency int64 // nanoseconds

	ActiveUsers    sync.Map // user_id -> last_activity_time
	TotalItemsSold int64

	mu                sync.RWMutex
	checkoutLatencies []time.Duration
	purchaseLatencies []time.Duration
}

type Service interface {
	IncrementCheckoutRequests()
	IncrementCheckoutSuccess()
	IncrementCheckoutFailed()
	IncrementPurchaseRequests()
	IncrementPurchaseSuccess()
	IncrementPurchaseFailed()
	IncrementSoldOutErrors()
	IncrementUserLimitErrors()
	IncrementCodeInvalidErrors()
	IncrementItemsSold()

	RecordCheckoutLatency(duration time.Duration)
	RecordPurchaseLatency(duration time.Duration)
	UpdateActiveUser(userID string)

	GetStats() map[string]interface{}
	Reset()
}

var metricsInstance *Metrics

func New() Service {
	if metricsInstance != nil {
		return metricsInstance
	}

	metricsInstance = &Metrics{
		checkoutLatencies: make([]time.Duration, 0, 1000),
		purchaseLatencies: make([]time.Duration, 0, 1000),
	}

	return metricsInstance
}

func (m *Metrics) IncrementCheckoutRequests() {
	atomic.AddInt64(&m.CheckoutRequests, 1)
}

func (m *Metrics) IncrementCheckoutSuccess() {
	atomic.AddInt64(&m.CheckoutSuccess, 1)
}

func (m *Metrics) IncrementCheckoutFailed() {
	atomic.AddInt64(&m.CheckoutFailed, 1)
}

func (m *Metrics) IncrementPurchaseRequests() {
	atomic.AddInt64(&m.PurchaseRequests, 1)
}

func (m *Metrics) IncrementPurchaseSuccess() {
	atomic.AddInt64(&m.PurchaseSuccess, 1)
}

func (m *Metrics) IncrementPurchaseFailed() {
	atomic.AddInt64(&m.PurchaseFailed, 1)
}

func (m *Metrics) IncrementSoldOutErrors() {
	atomic.AddInt64(&m.SoldOutErrors, 1)
}

func (m *Metrics) IncrementUserLimitErrors() {
	atomic.AddInt64(&m.UserLimitErrors, 1)
}

func (m *Metrics) IncrementCodeInvalidErrors() {
	atomic.AddInt64(&m.CodeInvalidErrors, 1)
}

func (m *Metrics) IncrementItemsSold() {
	atomic.AddInt64(&m.TotalItemsSold, 1)
}

func (m *Metrics) RecordCheckoutLatency(duration time.Duration) {
	atomic.StoreInt64(&m.AvgCheckoutLatency, int64(duration))

	m.mu.Lock()
	if len(m.checkoutLatencies) >= 1000 {
		m.checkoutLatencies = m.checkoutLatencies[1:]
	}
	m.checkoutLatencies = append(m.checkoutLatencies, duration)
	m.mu.Unlock()
}

func (m *Metrics) RecordPurchaseLatency(duration time.Duration) {
	atomic.StoreInt64(&m.AvgPurchaseLatency, int64(duration))

	m.mu.Lock()
	if len(m.purchaseLatencies) >= 1000 {
		m.purchaseLatencies = m.purchaseLatencies[1:]
	}
	m.purchaseLatencies = append(m.purchaseLatencies, duration)
	m.mu.Unlock()
}

func (m *Metrics) UpdateActiveUser(userID string) {
	m.ActiveUsers.Store(userID, time.Now())
}

func (m *Metrics) GetStats() map[string]interface{} {
	activeUserCount := 0
	cutoff := time.Now().Add(-5 * time.Minute)

	m.ActiveUsers.Range(func(key, value interface{}) bool {
		if lastActivity, ok := value.(time.Time); ok && lastActivity.After(cutoff) {
			activeUserCount++
		}
		return true
	})

	m.mu.RLock()
	avgCheckoutMs := float64(0)
	if len(m.checkoutLatencies) > 0 {
		total := time.Duration(0)
		for _, lat := range m.checkoutLatencies {
			total += lat
		}
		avgCheckoutMs = float64(total.Nanoseconds()) / float64(len(m.checkoutLatencies)) / 1e6
	}

	avgPurchaseMs := float64(0)
	if len(m.purchaseLatencies) > 0 {
		total := time.Duration(0)
		for _, lat := range m.purchaseLatencies {
			total += lat
		}
		avgPurchaseMs = float64(total.Nanoseconds()) / float64(len(m.purchaseLatencies)) / 1e6
	}
	m.mu.RUnlock()

	checkoutSuccessRate := float64(0)
	if totalCheckouts := atomic.LoadInt64(&m.CheckoutRequests); totalCheckouts > 0 {
		checkoutSuccessRate = float64(atomic.LoadInt64(&m.CheckoutSuccess)) / float64(totalCheckouts) * 100
	}

	purchaseSuccessRate := float64(0)
	if totalPurchases := atomic.LoadInt64(&m.PurchaseRequests); totalPurchases > 0 {
		purchaseSuccessRate = float64(atomic.LoadInt64(&m.PurchaseSuccess)) / float64(totalPurchases) * 100
	}

	return map[string]interface{}{
		"checkout_requests":       atomic.LoadInt64(&m.CheckoutRequests),
		"checkout_success":        atomic.LoadInt64(&m.CheckoutSuccess),
		"checkout_failed":         atomic.LoadInt64(&m.CheckoutFailed),
		"checkout_success_rate":   checkoutSuccessRate,
		"purchase_requests":       atomic.LoadInt64(&m.PurchaseRequests),
		"purchase_success":        atomic.LoadInt64(&m.PurchaseSuccess),
		"purchase_failed":         atomic.LoadInt64(&m.PurchaseFailed),
		"purchase_success_rate":   purchaseSuccessRate,
		"sold_out_errors":         atomic.LoadInt64(&m.SoldOutErrors),
		"user_limit_errors":       atomic.LoadInt64(&m.UserLimitErrors),
		"code_invalid_errors":     atomic.LoadInt64(&m.CodeInvalidErrors),
		"total_items_sold":        atomic.LoadInt64(&m.TotalItemsSold),
		"active_users_5min":       activeUserCount,
		"avg_checkout_latency_ms": avgCheckoutMs,
		"avg_purchase_latency_ms": avgPurchaseMs,
	}
}

func (m *Metrics) Reset() {
	atomic.StoreInt64(&m.CheckoutRequests, 0)
	atomic.StoreInt64(&m.CheckoutSuccess, 0)
	atomic.StoreInt64(&m.CheckoutFailed, 0)
	atomic.StoreInt64(&m.PurchaseRequests, 0)
	atomic.StoreInt64(&m.PurchaseSuccess, 0)
	atomic.StoreInt64(&m.PurchaseFailed, 0)
	atomic.StoreInt64(&m.SoldOutErrors, 0)
	atomic.StoreInt64(&m.UserLimitErrors, 0)
	atomic.StoreInt64(&m.CodeInvalidErrors, 0)
	atomic.StoreInt64(&m.TotalItemsSold, 0)

	m.ActiveUsers = sync.Map{}

	m.mu.Lock()
	m.checkoutLatencies = m.checkoutLatencies[:0]
	m.purchaseLatencies = m.purchaseLatencies[:0]
	m.mu.Unlock()
}
