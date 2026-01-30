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
	rateLimitGlobally, globalRateLimiter, err := MakeGlobalRateLimitMiddleware()
	if err != nil {
		log.Fatal(err)
	}
	defer globalRateLimiter.Offline()

	//create per client limiter & middleware
	rateLimitPerClient, perClientRateLimiter, err := MakePerClientRateLimitMiddleware()
	if err != nil {
		log.Fatal(err)
	}
	defer perClientRateLimiter.Offline()

	//middleware composer
	withMiddlewares := ComposeMiddlewares(rateLimitGlobally, rateLimitPerClient)

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
	server.Handler = SetupCors(mux)

	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatal(err)
	}

	//wait for graceful shutdown
	<-idleConnsClosed

}
