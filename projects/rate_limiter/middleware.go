package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/rs/cors"
)

func MakeGlobalRateLimitMiddleware() (Middleware, func(), error) {
	limiter, err := NewGlobalLimiter(InMemory, 50000, 50000, 10000)
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

				response := ErrorResponse{"We're a bit busy right now. Please try again later."}
				json.NewEncoder(w).Encode(&response)
				return
			}
		})
	}, limiter.Offline, nil
}

func MakePerClientRateLimitMiddleware() (Middleware, func(), error) {
	limiter, err := NewPerClientLimiter(InMemory, 50000, 10, time.Minute)
	if err != nil {
		return nil, nil, err
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			response := ErrorResponse{ErrorMessage: ""}
			if r.URL.Path == "/shorten" {

				clientId, _, err := net.SplitHostPort(r.RemoteAddr)
				if err != nil {
					clientId = r.RemoteAddr
				}

				if clientId == "" {
					fmt.Println("Unable to extract client IP address")
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusUnauthorized)
					response = ErrorResponse{"You are not Authorized to used this site at the moment. Please try again later"}

				} else if storageFull, err := limiter.Allow(clientId); err != nil {
					var errorMessage string
					if storageFull {
						errorMessage = "We are a bit busy right now. Please try again later."
					} else {
						errorMessage = "Rate limit exceeded. Please try again later"
					}
					response = ErrorResponse{errorMessage}
					w.WriteHeader(http.StatusTooManyRequests)
					w.Header().Set("Content-Type", "application/json")
				}

				if response.ErrorMessage != "" {
					json.NewEncoder(w).Encode(&response)
				}
			}

			next.ServeHTTP(w, r)
		})
	}, limiter.Offline, nil
}

func SetupCors(mux *http.ServeMux) http.Handler {

	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"https://appurl.com", "http://localhost:8090", "http://localhost:3000"},
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type", "Authorization", "X-API-Key"},
		AllowCredentials: true,
		Debug:            false,
	})

	return c.Handler(mux)
}

// middleware utils

func ComposeMiddlewares(r ...Middleware) Middleware {

	return func(h http.Handler) http.Handler {
		acc := r[len(r)-1](h)

		for i := len(r) - 2; i >= 0; i-- {
			acc = r[i](acc)
		}
		return acc
	}
}
