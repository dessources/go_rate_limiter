package main

import (
	"fmt"
	"net/http"
	"time"
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
				http.Error(w, "We're a bit busy right now. Please try again later.", http.StatusTooManyRequests)
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
			clientId := r.Header.Get("X-API-Key")

			if clientId == "" {
				fmt.Println("Invalid API key provided")
				http.Error(w, "Invalid api key provided", http.StatusUnauthorized)
				return
			}

			if err := limiter.Allow(clientId); err != nil {
				http.Error(w, err.Error(), http.StatusTooManyRequests)
				return
			} else {
				next.ServeHTTP(w, r)
				return
			}
		})
	}, limiter.Offline, nil
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
