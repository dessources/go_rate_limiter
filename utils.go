package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/rs/cors"
)

//---------------Middleware utils ----------------

func SetupCors(mux *http.ServeMux) http.Handler {

	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"https://pety.to", "http://localhost:8090", "http://localhost:3000"},
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type", "Authorization", "X-API-Key"},
		AllowCredentials: true,
		Debug:            false,
	})

	return c.Handler(mux)
}

func ComposeMiddlewares(r ...Middleware) Middleware {
	return func(h http.Handler) http.Handler {
		//start by wrapping handler with last middleware
		acc := r[len(r)-1](h)

		//then compose from last to first
		for i := len(r) - 2; i >= 0; i-- {
			acc = r[i](acc)
		}
		return acc
	}
}

//------------- Handler utils----------------------

func containsTxtFile(name string) bool {
	parts := strings.Split(name, "/")
	for _, part := range parts {
		if strings.HasSuffix(part, ".txt") {
			return true
		}
	}
	return false
}

type FileHidingFileSystem struct {
	http.FileSystem
}

type FileHidingFile struct {
	http.File
}

func (fsys FileHidingFileSystem) Open(name string) (http.File, error) {
	if containsTxtFile(name) {
		// If txt file, return 403 error
		return nil, fs.ErrPermission
	}

	file, err := fsys.FileSystem.Open(name)
	if err != nil {
		return nil, err
	}
	return FileHidingFile{file}, nil
}

func ValidateUrl(s string) (string, bool) {
	if len(s) > maxUrlLength {
		return fmt.Sprintf("Provided url exceeds max-length of %d", maxUrlLength), false
	}

	u, err := url.Parse(s)

	if err != nil || u.Host == "" {
		return "Invalid url provided", false
	}

	if u.Scheme != "http" && u.Scheme != "https" {
		return "Invalid protocol provided. Only http:// or https:// allowed", false
	}

	return "", true
}

func EnableGracefulShutdown(done chan struct{}, server *http.Server) {

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

func StartTestServer() (*http.Server, *App, error) {

	testServer := &http.Server{Addr: ":8091"}

	//create global limiter & middleware
	rateLimitGlobally, globalRateLimiter, err := MakeGlobalRateLimitMiddleware()
	if err != nil {
		return nil, nil, errors.New("Failed to create global rate limiter for stress test.")
	}

	//create per client limiter & middleware
	rateLimitPerClient, perClientRateLimiter, err := MakePerClientRateLimitMiddleware()
	if err != nil {
		globalRateLimiter.Offline()
		return nil, nil, errors.New("Failed to create per client rate limiter for stress test.")
	}

	//middleware composer
	withMiddlewares := ComposeMiddlewares(rateLimitGlobally, rateLimitPerClient)

	//url shortener struct
	shortener, err := NewUrlShortener(InMemory, 100000, time.Hour)
	if err != nil {
		globalRateLimiter.Offline()
		perClientRateLimiter.Offline()
		return nil, nil, errors.New("Failed to create shortener instance for stress test.")
	}

	app := &App{shortener, globalRateLimiter, perClientRateLimiter}

	//Route handlers
	mux := http.NewServeMux()
	mux.Handle("/", rateLimitGlobally(MakeIndexHandler()))
	mux.Handle("GET /{shortUrl}", rateLimitGlobally(http.HandlerFunc(app.RetrieveUrl)))
	mux.Handle("POST /api/shorten", withMiddlewares(http.HandlerFunc(app.ShortenUrl)))
	// mux.Handle("GET /api/metrics/stream", rateLimitGlobally(http.HandlerFunc(app.StreamMetrics)))
	testServer.Handler = SetupCors(mux)

	return testServer, app, nil

}
