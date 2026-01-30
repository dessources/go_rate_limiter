package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

var routesLimitedPerClient []string = []string{"/api/shorten", "/api/stress-test/stream"}

func MakeGlobalRateLimitMiddleware(logger *slog.Logger, storageType StorageType, count int, cap int, rate int) (Middleware, *GlobalRateLimiter, error) {
	limiter, err := NewGlobalRateLimiter(storageType, count, cap, rate)
	if err != nil {
		return nil, nil, err
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if limiter.Allow(1) {
				next.ServeHTTP(w, r)
			} else {
				logger.Warn("global rate limit exceeded", "remote_addr", r.RemoteAddr, "path", r.URL.Path)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				json.NewEncoder(w).Encode(ErrorResponse{"We are a bit busy right now. Please try again later."})
				return
			}
		})
	}, limiter, nil
}

func MakePerClientRateLimitMiddleware(logger *slog.Logger, storageType StorageType, cap int, limit int, window, ttl time.Duration) (Middleware, *PerClientRateLimiter, error) {
	limiter, err := NewPerClientRateLimiter(storageType, cap, limit, window, ttl)
	if err != nil {
		return nil, nil, err
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var errorMessage string

			//Clients identifed by combination of IP and API key
			for _, route := range routesLimitedPerClient {
				if r.URL.Path == route {
					ip, _, err := net.SplitHostPort(r.RemoteAddr)
					if err != nil {
						ip = r.RemoteAddr
					}

					apiKey := r.Header.Get("X-API-Key")
					if apiKey == "" {
						logger.Warn("invalid API key provided", "remote_addr", r.RemoteAddr, "path", r.URL.Path)
						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusUnauthorized)
						errorMessage = "Invalid API key provided."
						json.NewEncoder(w).Encode(&ErrorResponse{errorMessage})
						return
					}

					clientId := fmt.Sprintf("%s:%s", ip, apiKey)

					if storageFull, err := limiter.Allow(clientId); err != nil {
						if storageFull {
							errorMessage = "We are a bit busy right now. Please try again later."
							logger.Warn("per-client rate limiter storage full", "client_id", clientId, "path", r.URL.Path)
						} else {
							errorMessage = "Rate limit exceeded. Please try again later"
							logger.Warn("per-client rate limit exceeded", "client_id", clientId, "path", r.URL.Path)
						}

						w.Header().Set("Content-Type", "application/json")
						w.WriteHeader(http.StatusTooManyRequests)
						json.NewEncoder(w).Encode(&ErrorResponse{errorMessage})
						return
					}

				}
			}

			next.ServeHTTP(w, r)
		})
	}, limiter, nil
}
