package config

import (
	"strings"
	"testing"
)

// setSecureSecrets sets all four required secrets to safe non-default values.
func setSecureSecrets(t *testing.T) {
	t.Helper()
	t.Setenv("JWT_SECRET", "a-real-secret")
	t.Setenv("DB_PASSWORD", "a-real-db-password")
	t.Setenv("MINIO_SECRET_KEY", "a-real-minio-key")
	t.Setenv("ADMIN_DEFAULT_PASSWORD", "a-real-admin-password")
}

func TestLoad_AllSecretsSet(t *testing.T) {
	setSecureSecrets(t)

	cfg, err := Load()
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if cfg.JWTSecret != "a-real-secret" {
		t.Errorf("unexpected JWTSecret: %q", cfg.JWTSecret)
	}
}

func TestLoad_MissingSecretFails(t *testing.T) {
	setSecureSecrets(t)
	t.Setenv("JWT_SECRET", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when JWT_SECRET is unset, got nil")
	}
	if !strings.Contains(err.Error(), "JWT_SECRET is required") {
		t.Errorf("expected JWT_SECRET-required error, got %v", err)
	}
}

func TestLoad_InsecureDefaultRejected(t *testing.T) {
	setSecureSecrets(t)
	t.Setenv("JWT_SECRET", "change-me-in-production")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for known insecure JWT_SECRET, got nil")
	}
	if !strings.Contains(err.Error(), "insecure built-in default") {
		t.Errorf("expected insecure-default error, got %v", err)
	}
}

func TestLoad_AllProblemsReportedTogether(t *testing.T) {
	setSecureSecrets(t)
	t.Setenv("DB_PASSWORD", "vaultflix")        // insecure default
	t.Setenv("MINIO_SECRET_KEY", "")            // missing
	t.Setenv("ADMIN_DEFAULT_PASSWORD", "admin") // insecure default

	_, err := Load()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	for _, want := range []string{"DB_PASSWORD", "MINIO_SECRET_KEY", "ADMIN_DEFAULT_PASSWORD"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("expected error to mention %s, got %v", want, err)
		}
	}
}
