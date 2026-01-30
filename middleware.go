package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"
)

func MakeGlobalRateLimitMiddleware() (Middleware, *GlobalRateLimiter, error) {
	limiter, err := NewGlobalRateLimiter(InMemory, 50000, 50000, 10000)
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

func MakePerClientRateLimitMiddleware() (Middleware, *PerClientRateLimiter, error) {
	limiter, err := NewPerClientRateLimiter(InMemory, 50000, 10, time.Minute)
	if err != nil {
		return nil, nil, err
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			errorMessage := "An unknown error occured"

			//TODO: Clients should be identifed by combination of IP and API key

			if r.URL.Path == "/api/shorten" {
				clientId, _, err := net.SplitHostPort(r.RemoteAddr)
				if err != nil {
					clientId = r.RemoteAddr
				}

				if clientId == "" {
					fmt.Println("Unable to extract client IP address")
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusUnauthorized)
					errorMessage = "You are not Authorized to used this site at the moment. Please try again later"
					json.NewEncoder(w).Encode(&ErrorResponse{errorMessage})
					return

				} else if storageFull, err := limiter.Allow(clientId); err != nil {

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

			next.ServeHTTP(w, r)
		})
	}, limiter, nil
}
