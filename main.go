package main

import (
	"log"
	"net/http"
	"time"
)

type Middleware func(http.Handler) http.Handler

type StorageType int

const (
	InMemory StorageType = iota
	Redis
)

const maxUrlLength = 4096

func main() {

	server := &http.Server{
		Addr: ":8090",
	}

	idleConnsClosed := make(chan struct{})
	EnableGracefulShutdown(idleConnsClosed, server)

	//create global limiter & middleware
	rateLimitGlobally, globalRateLimiter, err := MakeGlobalRateLimitMiddleware(InMemory, 50000, 50000, 10000)
	if err != nil {
		log.Fatal(err)
	}
	defer globalRateLimiter.Offline()

	//create per client limiter & middleware
	rateLimitPerClient, perClientRateLimiter, err := MakePerClientRateLimitMiddleware(InMemory, 50000, 10, time.Minute)
	if err != nil {
		log.Fatal(err)
	}
	defer perClientRateLimiter.Offline()

	//middleware composers
	withMiddlewares := ComposeMiddlewares(rateLimitGlobally, rateLimitPerClient)
	//composed middleware for stress test route
	stressTestMiddlewares, cleanup, err := MakeTestRouteMiddlewares()
	if err != nil {
		log.Fatal(err)
	}
	defer cleanup()

	//url shortener struct
	shortener, err := NewUrlShortener(InMemory, 100000, time.Hour)
	if err != nil {
		log.Fatal(err)
	}
	defer shortener.Offline()

	//create app struct with methods for api handler logic
	app := &App{shortener, globalRateLimiter, perClientRateLimiter}

	//Route handlers
	mux := http.NewServeMux()
	mux.Handle("/", rateLimitGlobally(MakeIndexHandler()))
	mux.Handle("GET /{shortUrl}", rateLimitGlobally(http.HandlerFunc(app.RetrieveUrl)))
	mux.Handle("POST /api/shorten", withMiddlewares(http.HandlerFunc(app.ShortenUrl)))
	mux.Handle("GET /api/metrics/stream", rateLimitGlobally(http.HandlerFunc(app.StreamMetrics)))
	mux.Handle("GET /api/stress-test/stream", stressTestMiddlewares(http.HandlerFunc(StressTest)))
	server.Handler = SetupCors(mux)

	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatal(err)
	}

	//wait for graceful shutdown
	<-idleConnsClosed

}
