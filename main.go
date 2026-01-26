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

type HTTPContext struct {
	w   http.ResponseWriter
	req *http.Request
}

type RouteHandler func(ctx HTTPContext)
type Middleware func(http.Handler) http.Handler

type UrlShortenerPayload struct {
	Original string `json:"original"`
}

type StorageType int

const (
	InMemory StorageType = iota
	Redis
)

const maxUrlLength = 2048

func main() {
	server := &http.Server{
		Addr: ":8090",
	}

	idleConnsClosed := make(chan struct{})
	enableGracefulShutdown(idleConnsClosed, server)

	//create global limiter & middleware
	globalLimiter, globalRateLimit := initializeGlobalLimiter()

	//create per client limiter & middleware
	perClientLimiter, perClientRateLimit := initializePerClientLimiter()

	//middlware composer
	withMiddlewares := ComposeMiddlewares(globalRateLimit, perClientRateLimit)

	//create url shortener
	shortener, shorten, retrieve := createUrlShortener()

	//create server
	mux := http.NewServeMux()
	mux.Handle("/", globalRateLimit(AsHandler(Index)))
	mux.Handle("GET /{shortUrl}", withMiddlewares(AsHandler(retrieve)))
	mux.Handle("POST /shorten", withMiddlewares(AsHandler(shorten)))
	server.Handler = mux

	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		log.Fatal(err)
	}

	//wait for graceful shutdown
	<-idleConnsClosed
	globalLimiter.Offline()
	shortener.Offline()
	perClientLimiter.Offline()

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
