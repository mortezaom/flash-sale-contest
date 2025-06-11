// internal/server/middleware.go

package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"runtime/debug"
	"time"
)

func (s *Server) rateLimitMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simple rate limiting using Redis
		if r.URL.Path == "/checkout" || r.URL.Path == "/purchase" {
			userID := r.URL.Query().Get("user_id")
			if userID != "" {
				key := fmt.Sprintf("rate_limit:%s", userID)
				count, err := s.cache.GetClient().Incr(r.Context(), key).Result()
				if err == nil {
					if count == 1 {
						s.cache.GetClient().Expire(r.Context(), key, time.Minute)
					}
					if count > 100 { // 100 requests per minute per user
						http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
						return
					}
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				log.Printf("Panic recovered: %v\n%s", err, debug.Stack())

				s.metrics.IncrementPanic()

				http.Error(w, "Internal server error", http.StatusInternalServerError)
			}
		}()

		next.ServeHTTP(w, r)
	})
}

func (s *Server) timeoutMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		defer cancel()

		r = r.WithContext(ctx)
		next.ServeHTTP(w, r)
	})
}
