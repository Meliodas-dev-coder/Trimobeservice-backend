package config

import (
	"strings"
	"testing"
)

func TestLoadRejectsWeakJWTSecret(t *testing.T) {
	t.Setenv("JWT_SECRET", "short")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "at least 32") {
		t.Fatalf("expected a min-length error, got %v", err)
	}
}

func TestLoadRejectsMissingJWTSecret(t *testing.T) {
	t.Setenv("JWT_SECRET", "")
	if _, err := Load(); err == nil {
		t.Fatal("expected an error for missing JWT_SECRET")
	}
}

func TestLoadRejectsPlaceholderSecretInProduction(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("JWT_SECRET", "dev-only-change-me-to-a-long-random-string-xxxx")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "placeholder") {
		t.Fatalf("expected a placeholder-secret error, got %v", err)
	}
}

func TestLoadAcceptsStrongSecret(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	t.Setenv("JWT_SECRET", "0123456789abcdef0123456789abcdef0123456789")
	cfg, err := Load()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HTTP.TrustProxy {
		t.Fatal("TrustProxy should default to false")
	}
}
