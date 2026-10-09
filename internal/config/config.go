// Package config loads process configuration from environment variables.
// Nothing here is hardcoded: a missing required value is a startup error.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config is the full runtime configuration for the API process.
type Config struct {
	// HTTP
	HTTPAddr        string
	ShutdownTimeout time.Duration
	RequestTimeout  time.Duration

	// Postgres (pgxpool connection string).
	DatabaseURL string

	// Redis
	RedisAddr     string
	RedisPassword string
	RedisDB       int
	CacheTTL      time.Duration

	// Auth (used in later phases; loaded now so env is the single source of truth).
	JWTSecret          string
	JWTExpiry          time.Duration
	BcryptCost         int
	RateLimitPerMinute int
}

// Load reads configuration from the environment.
// Required: DATABASE_URL, REDIS_ADDR, JWT_SECRET.
func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:           getenv("HTTP_ADDR", ":8080"),
		DatabaseURL:        os.Getenv("DATABASE_URL"),
		RedisAddr:          os.Getenv("REDIS_ADDR"),
		RedisPassword:      os.Getenv("REDIS_PASSWORD"),
		JWTSecret:          os.Getenv("JWT_SECRET"),
		RateLimitPerMinute: getenvInt("RATE_LIMIT_PER_MINUTE", 60),
		BcryptCost:         getenvInt("BCRYPT_COST", 12),
		RedisDB:            getenvInt("REDIS_DB", 0),
	}

	var err error
	if cfg.ShutdownTimeout, err = getenvDuration("SHUTDOWN_TIMEOUT", 10*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.RequestTimeout, err = getenvDuration("REQUEST_TIMEOUT", 5*time.Second); err != nil {
		return Config{}, err
	}
	if cfg.JWTExpiry, err = getenvDuration("JWT_EXPIRY", 24*time.Hour); err != nil {
		return Config{}, err
	}
	if cfg.CacheTTL, err = getenvDuration("CACHE_TTL", time.Hour); err != nil {
		return Config{}, err
	}

	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.RedisAddr == "" {
		return Config{}, fmt.Errorf("REDIS_ADDR is required")
	}
	if cfg.JWTSecret == "" {
		return Config{}, fmt.Errorf("JWT_SECRET is required")
	}

	return cfg, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getenvInt(key string, fallback int) int {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return n
}

func getenvDuration(key string, fallback time.Duration) (time.Duration, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid duration for %s: %w", key, err)
	}
	return d, nil
}
