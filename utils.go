package main

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
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

func SetupCors(mux *http.ServeMux, cfg *Config) http.Handler {

	c := cors.New(cors.Options{
		AllowedOrigins:   cfg.CorsAllowedOrigins,
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

func ValidateUrl(s string, cfg *Config) (string, bool) {
	if len(s) > cfg.MaxUrlLength {
		return fmt.Sprintf("Provided url exceeds max-length of %d", cfg.MaxUrlLength), false
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

func EnableGracefulShutdown(logger *slog.Logger, done chan struct{}, server *http.Server) {

	// enable Graceful Exit
	go func() {
		sigint := make(chan os.Signal, 1)
		signal.Notify(sigint, os.Interrupt, syscall.SIGTERM)
		<-sigint

		//when interrupt received
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			logger.Error("HTTP server Shutdown failed", "error", err)
		}
		close(done)
	}()

}

func StartTestServer(app *App) (*http.Server, *App, error) {

	testServer := &http.Server{Addr: app.cfg.TestServerAddr}

	//create global limiter & middleware
	rateLimitGlobally, globalRateLimiter, err := MakeGlobalRateLimitMiddleware(app.logger, InMemory, app.cfg.GlobalLimiterCount, app.cfg.GlobalLimiterCap, app.cfg.GlobalLimiterRate)

	if err != nil {
		return nil, nil, errors.New("Failed to create global rate limiter for stress test.")
	}

	//create per client limiter & middleware
	rateLimitPerClient, perClientRateLimiter, err := MakePerClientRateLimitMiddleware(app.logger, InMemory, app.cfg.PerClientLimiterCap, app.cfg.PerClientLimiterLimit, app.cfg.PerClientLimiterWindow, app.cfg.PerClientLimiterClientTtl)
	if err != nil {
		globalRateLimiter.Offline()
		return nil, nil, errors.New("Failed to create per client rate limiter for stress test.")
	}

	//middleware composer
	withMiddlewares := ComposeMiddlewares(rateLimitGlobally, rateLimitPerClient)

	//url shortener struct
	shortener, err := NewUrlShortener(InMemory, app.cfg.ShortenerCap, app.cfg.ShortenerTTL, app.cfg.ShortCodeLength)
	if err != nil {
		globalRateLimiter.Offline()
		perClientRateLimiter.Offline()
		return nil, nil, errors.New("Failed to create shortener instance for stress test.")
	}

	testApp := &App{app.cfg, app.logger, "Not Found", shortener, globalRateLimiter, perClientRateLimiter}

	//Route handlers
	mux := http.NewServeMux()
	mux.Handle("/", rateLimitGlobally(MakeIndexHandler()))
	mux.Handle("GET /{shortUrl}", rateLimitGlobally(http.HandlerFunc(testApp.RetrieveUrl)))
	mux.Handle("POST /api/shorten", withMiddlewares(http.HandlerFunc(testApp.ShortenUrl)))

	//No metrics Streaming for stress test server
	// mux.Handle("GET /api/metrics/stream", rateLimitGlobally(http.HandlerFunc(app.StreamMetrics)))
	testServer.Handler = SetupCors(mux, testApp.cfg)

	return testServer, app, nil

}

func MakeStressTestRouteMiddlewares(logger *slog.Logger) (Middleware, func(), error) {
	cfg := LoadStressTestRouteMiddlewareConfig()

	//create global limiter & middleware
	rateLimitGlobally, globalRateLimiter, err := MakeGlobalRateLimitMiddleware(logger, InMemory, cfg.GlobalLimiterCount, cfg.GlobalLimiterCap, cfg.GlobalLimiterRate)
	if err != nil {
		return nil, nil, errors.New("Failed to create global rate limiter for stress test route.")
	}

	//create per client limiter & middleware
	rateLimitPerClient, perClientRateLimiter, err := MakePerClientRateLimitMiddleware(logger, InMemory, cfg.PerClientLimiterCap, cfg.PerClientLimiterLimit, cfg.PerClientLimiterWindow, cfg.PerClientLimiterClientTtl)
	if err != nil {
		globalRateLimiter.Offline()
		return nil, nil, errors.New("Failed to create per client rate limiter for stress test route.")
	}

	//middleware composer
	return ComposeMiddlewares(rateLimitGlobally, rateLimitPerClient), func() {
		globalRateLimiter.Offline()
		perClientRateLimiter.Offline()
	}, nil

}

func SendSSEErrorEvent(w http.ResponseWriter, message string, f http.Flusher) {
	fmt.Fprintf(w, "event: error\ndata: {\"errorMessage\": \"%s\"}\n\n", message)

	f.Flush()
}

func Load404Page() (string, error) {
	page404HTMLText, err := os.ReadFile("frontend/out/404.html")

	if err != nil {
		return "", err
	}

	return string(page404HTMLText), nil
}
