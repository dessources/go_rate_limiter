package main

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	//General config
	baseUrl string

	// Server configuration
	ServerAddr         string
	TestServerAddr     string
	CorsAllowedOrigins []string

	// Global Rate Limiter
	GlobalLimiterCount int
	GlobalLimiterCap   int
	GlobalLimiterRate  int

	// Per-Client Rate Limiter
	PerClientLimiterCap       int
	PerClientLimiterLimit     int
	PerClientLimiterWindow    time.Duration
	PerClientLimiterClientTtl time.Duration

	// URL Shortener
	ShortenerCap    int
	ShortenerTTL    time.Duration
	ShortCodeLength int
	MaxUrlLength    int

	//others
	Fallback404HTML string
}

func LoadConfig() (*Config, error) {
	// Use strconv.Atoi for integers, strconv.ParseInt for more complex numbers
	globalCap := getEnvAsInt("GLOBAL_LIMITER_CAP", 50000)
	baseUrl := getEnv("BASE_URL", "https://pety.to")
	corsAllowedOrigins := append([]string{baseUrl},
		getEnvAsSlice("CORS_ALLOWED_ORIGINS", []string{"http://localhost:3000", "http://localhost:8090"})...)

	return &Config{
		baseUrl:            baseUrl,
		ServerAddr:         getEnv("SERVER_ADDR", ":8090"),
		TestServerAddr:     getEnv("TEST_SERVER_ADDR", ":8091"),
		CorsAllowedOrigins: corsAllowedOrigins,

		GlobalLimiterCount: globalCap, // Often the same as cap at start
		GlobalLimiterCap:   globalCap,
		GlobalLimiterRate:  getEnvAsInt("GLOBAL_LIMITER_RATE", 10000*60),

		PerClientLimiterCap:    getEnvAsInt("PER_CLIENT_LIMITER_CAP", 50000),
		PerClientLimiterLimit:  getEnvAsInt("PER_CLIENT_LIMITER_LIMIT", 10),
		PerClientLimiterWindow: getEnvAsDuration("PER_CLIENT_WINDOW_SECONDS", 60*time.Second),

		PerClientLimiterClientTtl: getEnvAsDuration("PER_CLIENT_LIMITER_CLIENT_TTL", time.Minute*30),

		ShortenerCap:    getEnvAsInt("SHORTENER_CAP", 100000),
		ShortenerTTL:    getEnvAsDuration("SHORTENER_TTL_HOURS", time.Hour),
		ShortCodeLength: getEnvAsInt("SHORT_CODE_LENGTH", 4),
		MaxUrlLength:    getEnvAsInt("MAX_URL_LENGTH", 4096),

		Fallback404HTML: getEnv("FALLBACK_404_HTML", "<h1>Short link not found</h1><p>It seems this short link has expired or never existed.</p><a href='/'>Go to homepage</a>"),
	}, nil
}

type StressTestRouteMiddlewareConfig struct {
	// Global Rate Limiter
	GlobalLimiterCount int
	GlobalLimiterCap   int
	GlobalLimiterRate  int

	// Per-Client Rate Limiter
	PerClientLimiterCap       int
	PerClientLimiterLimit     int
	PerClientLimiterWindow    time.Duration
	PerClientLimiterClientTtl time.Duration
}

func LoadStressTestRouteMiddlewareConfig() *StressTestRouteMiddlewareConfig {
	return &StressTestRouteMiddlewareConfig{
		GlobalLimiterCount: getEnvAsInt("STRESS_TEST_GLOBAL_LIMITER_COUNT", 10),
		GlobalLimiterCap:   getEnvAsInt("STRESS_TEST_GLOBAL_LIMITER_CAP", 10),
		GlobalLimiterRate:  getEnvAsInt("STRESS_TEST_GLOBAL_LIMITER_RATE", 1),

		// Per-Client Rate Limiter
		PerClientLimiterCap:    getEnvAsInt("STRESS_TEST_PER_CLIENT_LIMITER_COUNT", 50),
		PerClientLimiterLimit:  getEnvAsInt("STRESS_TEST_PER_CLIENT_LIMITER_LIMIT", 1),
		PerClientLimiterWindow: getEnvAsDuration("STRESS_TEST_PER_CLIENT_LIMITER_WINDOW", time.Minute*5),

		PerClientLimiterClientTtl: getEnvAsDuration("PER_CLIENT_LIMITER_CLIENT_TTL", time.Minute*30),
	}
}

// --- Helper functions to read env vars with defaults ---

func getEnv(key string, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func getEnvAsInt(key string, fallback int) int {
	if valueStr, ok := os.LookupEnv(key); ok {
		value, err := strconv.Atoi(valueStr)
		if err == nil {
			return value
		}
	}
	return fallback
}

func getEnvAsDuration(key string, fallback time.Duration) time.Duration {
	if valueStr, ok := os.LookupEnv(key); ok {
		seconds, err := strconv.Atoi(valueStr)
		if err == nil {
			return time.Second * time.Duration(seconds)
		}
	}
	return fallback
}

func getEnvAsSlice(key string, fallback []string) []string {
	if value, ok := os.LookupEnv(key); ok {
		return strings.Split(value, ",")
	}
	return fallback
}
