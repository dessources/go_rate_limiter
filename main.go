package main

import (
	"log/slog"
	"net/http"
	"os"
)

type Middleware func(http.Handler) http.Handler

type StorageType int

const (
	InMemory StorageType = iota
	Redis
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := LoadConfig()
	if err != nil {
		logger.Error("failed to load configuration", "error", err)
		return
	}
	server := &http.Server{
		Addr: cfg.ServerAddr,
	}

	idleConnsClosed := make(chan struct{})
	EnableGracefulShutdown(idleConnsClosed, server)

	//create global limiter & middleware
	rateLimitGlobally, globalRateLimiter, err := MakeGlobalRateLimitMiddleware(logger, InMemory, cfg.GlobalLimiterCount, cfg.GlobalLimiterCap, cfg.GlobalLimiterRate)
	if err != nil {
		logger.Error("failed to create global rate limiter middleware", "error", err)
		return
	}
	defer globalRateLimiter.Offline()

	//create per client limiter & middleware
	rateLimitPerClient, perClientRateLimiter, err := MakePerClientRateLimitMiddleware(logger, InMemory, cfg.PerClientLimiterCap, cfg.PerClientLimiterLimit, cfg.PerClientLimiterWindow, cfg.PerClientLimiterClientTtl)

	if err != nil {
		logger.Error("failed to create per-client rate limiter middleware", "error", err)
		return
	}
	defer perClientRateLimiter.Offline()

	//middleware composers
	withMiddlewares := ComposeMiddlewares(rateLimitGlobally, rateLimitPerClient)
	//composed middleware for stress test route
	stressTestMiddlewares, cleanup, err := MakeStressTestRouteMiddlewares()
	if err != nil {
		logger.Error("failed to create stress test route middlewares", "error", err)
		return
	}
	defer cleanup()

	//url shortener struct
	shortener, err := NewUrlShortener(InMemory, cfg.ShortenerCap, cfg.ShortenerTTL, cfg.ShortCodeLength)
	if err != nil {
		logger.Error("failed to create URL shortener", "error", err)
		return
	}
	defer shortener.Offline()

	//create app struct with methods for api handler logic
	app := &App{cfg, logger, shortener, globalRateLimiter, perClientRateLimiter}

	//Route handlers
	mux := http.NewServeMux()
	mux.Handle("/", rateLimitGlobally(MakeIndexHandler()))
	mux.Handle("GET /{shortUrl}", rateLimitGlobally(http.HandlerFunc(app.RetrieveUrl)))
	mux.Handle("POST /api/shorten", withMiddlewares(http.HandlerFunc(app.ShortenUrl)))
	mux.Handle("GET /api/metrics/stream", rateLimitGlobally(http.HandlerFunc(app.StreamMetrics)))
	mux.Handle("GET /api/stress-test/stream", stressTestMiddlewares(http.HandlerFunc(app.StressTest)))
	server.Handler = SetupCors(mux, cfg)

	logger.Info("server starting", "addr", cfg.ServerAddr)
	if err := server.ListenAndServe(); err != http.ErrServerClosed {
		logger.Error("server failed to start", "error", err)
		return
	}

	//wait for graceful shutdown
	<-idleConnsClosed
	logger.Info("server stopped gracefully")
}
