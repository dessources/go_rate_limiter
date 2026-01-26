package main

import (
	"fmt"
	"log"
	"net/http"
	"time"
)

func LimitRateGlobal(limiter *GlobalLimiter, ctx HTTPContext, next http.Handler) {
	if limiter.Allow(1) {
		next.ServeHTTP(ctx.w, ctx.req)
	} else {
		ctx.w.WriteHeader(http.StatusTooManyRequests)
		_, _ = ctx.w.Write([]byte("Request rejected by rate limiter"))
		return
	}
}

func LimitRatePerClient(limiter *PerClientLimiter, ctx HTTPContext, next http.Handler) {

	clientId := ctx.req.Header.Get("X-API-Key")
	if clientId == "" {
		fmt.Println("Invalid API key provided")
		http.Error(ctx.w, "Invalid api key provided", http.StatusUnauthorized)
		return
	}

	if err := limiter.Allow(clientId); err != nil {
		http.Error(ctx.w, err.Error(), http.StatusTooManyRequests)
		return
	} else {
		next.ServeHTTP(ctx.w, ctx.req)
		return
	}
}

// middleware utils
func initializeGlobalLimiter() (*GlobalLimiter, Middleware) {

	l, err := NewLimiter(InMemory, 50000, 50000, 10000)
	if err != nil {
		log.Fatal(err)
	}

	return l, func(next http.Handler) http.Handler {
		return AsHandler(func(ctx HTTPContext) { LimitRateGlobal(l, ctx, next) })
	}
}

func initializePerClientLimiter() (*PerClientLimiter, Middleware) {

	l, err := NewPerClientLimiter(InMemory, 50000, 10, time.Minute)
	if err != nil {
		log.Fatal(err)
	}

	return l, func(next http.Handler) http.Handler {
		return AsHandler(func(ctx HTTPContext) { LimitRatePerClient(l, ctx, next) })
	}
}

// utility function to simplify route handler definitions
func AsHandler(handler RouteHandler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx := HTTPContext{w, req}
		handler(ctx)
	})
}

func ComposeMiddlewares(r ...Middleware) Middleware {
	if len(r) == 0 {
		log.Fatal("No middlewares provided")
	}

	return func(h http.Handler) http.Handler {
		var acc http.Handler

		for i, curr := range r {
			if i == 0 {
				acc = curr(h)
			} else {
				acc = curr(acc)
			}
		}
		return acc
	}
}
