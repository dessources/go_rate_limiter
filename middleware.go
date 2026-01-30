package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"
)

var routesLimitedPerClient []string = []string{"/api/shorten", "/api/stress-test/stream"}

func MakeGlobalRateLimitMiddleware(storageType StorageType, count int, cap int, rate int) (Middleware, *GlobalRateLimiter, error) {
	limiter, err := NewGlobalRateLimiter(storageType, count, cap, rate)
	if err != nil {
		return nil, nil, err
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if limiter.Allow(1) {
				next.ServeHTTP(w, r)
			} else {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				json.NewEncoder(w).Encode(ErrorResponse{"We are a bit busy right now. Please try again later."})
				return
			}
		})
	}, limiter, nil
}

func MakePerClientRateLimitMiddleware(storateType StorageType, cap int, limit int, window time.Duration) (Middleware, *PerClientRateLimiter, error) {
	limiter, err := NewPerClientRateLimiter(storateType, cap, limit, window)
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
						fmt.Println("Invalid API key provided")
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
						} else {
							errorMessage = "Rate limit exceeded. Please try again later"
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
