package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr string
		check   func(t *testing.T, cfg Config)
	}{
		{
			name:    "missing database",
			env:     map[string]string{"REDIS_ADDR": "localhost:6379", "JWT_SECRET": "s"},
			wantErr: "DATABASE_URL is required",
		},
		{
			name:    "missing redis",
			env:     map[string]string{"DATABASE_URL": "postgres://x", "JWT_SECRET": "s"},
			wantErr: "REDIS_ADDR is required",
		},
		{
			name:    "missing jwt",
			env:     map[string]string{"DATABASE_URL": "postgres://x", "REDIS_ADDR": "localhost:6379"},
			wantErr: "JWT_SECRET is required",
		},
		{
			name: "invalid duration",
			env: map[string]string{
				"DATABASE_URL": "postgres://x", "REDIS_ADDR": "localhost:6379", "JWT_SECRET": "s",
				"CACHE_TTL": "not-a-duration",
			},
			wantErr: "invalid duration",
		},
		{
			name: "defaults",
			env: map[string]string{
				"DATABASE_URL": "postgres://x", "REDIS_ADDR": "localhost:6379", "JWT_SECRET": "secret",
			},
			check: func(t *testing.T, cfg Config) {
				if cfg.HTTPAddr != ":8080" || cfg.CacheTTL != time.Hour || cfg.RateLimitPerMinute != 60 {
					t.Fatalf("unexpected defaults: %+v", cfg)
				}
			},
		},
		{
			name: "overrides and invalid ints fall back",
			env: map[string]string{
				"DATABASE_URL":          "postgres://x",
				"REDIS_ADDR":            "redis:6379",
				"JWT_SECRET":            "secret",
				"HTTP_ADDR":             ":9090",
				"BCRYPT_COST":           "nope",
				"RATE_LIMIT_PER_MINUTE": "30",
				"REDIS_DB":              "2",
				"CACHE_TTL":             "2h",
			},
			check: func(t *testing.T, cfg Config) {
				if cfg.HTTPAddr != ":9090" || cfg.BcryptCost != 12 || cfg.RedisDB != 2 || cfg.RateLimitPerMinute != 30 {
					t.Fatalf("unexpected overrides: %+v", cfg)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			keys := []string{
				"DATABASE_URL", "REDIS_ADDR", "JWT_SECRET", "HTTP_ADDR", "BCRYPT_COST",
				"RATE_LIMIT_PER_MINUTE", "REDIS_DB", "CACHE_TTL", "JWT_EXPIRY",
				"REQUEST_TIMEOUT", "SHUTDOWN_TIMEOUT", "REDIS_PASSWORD",
			}
			for _, k := range keys {
				t.Setenv(k, "")
			}
			for k, v := range tt.env {
				t.Setenv(k, v)
			}
			cfg, err := Load()
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %s", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			tt.check(t, cfg)
		})
	}
}
