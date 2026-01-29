package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
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
	enableGracefulShutdown(idleConnsClosed, server)

	//create global limiter & middleware
	globalRateLimiter, globalRateLimiterCleanup, err := MakeGlobalRateLimitMiddleware()
	if err != nil {
		log.Fatal(err)
	}
	defer globalRateLimiterCleanup()

	//create per client limiter & middleware
	perClientRateLimiter, perClientRateLimiterCleanup, err := MakePerClientRateLimitMiddleware()
	if err != nil {
		log.Fatal(err)
	}
	defer perClientRateLimiterCleanup()

	//middleware composer
	withMiddlewares := ComposeMiddlewares(globalRateLimiter, perClientRateLimiter)

	//url shortener struct
	shortener, err := NewUrlShortener(InMemory, 100000, time.Hour)
	if err != nil {
		log.Fatal(err)
	}
	defer shortener.Offline()

	//create app struct with methods for api handler logic
	app := &App{shortener}

	//Route handlers
	mux := http.NewServeMux()
	mux.Handle("/", globalRateLimiter(MakeIndexHandler()))
	mux.Handle("GET /{shortUrl}", globalRateLimiter(http.HandlerFunc(app.RetrieveUrl)))
	mux.Handle("POST /api/shorten", withMiddlewares(http.HandlerFunc(app.ShortenUrl)))
	server.Handler = SetupCors(mux)

	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatal(err)
	}

	//wait for graceful shutdown
	<-idleConnsClosed

}

func enableGracefulShutdown(done chan struct{}, server *http.Server) {

	// enable Graceful Exit
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt, syscall.SIGTERM)
		<-sigint

		//when interrupt received
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			log.Printf("HTTP server Shutdown: %v", err)
		}
		close(done)
	}()

}
