package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	// PostgreSQL
	DBHost     string
	DBPort     int
	DBUser     string
	DBPassword string
	DBName     string

	// MinIO
	MinIOEndpoint        string
	MinIOPublicEndpoint  string
	MinIOAccessKey       string
	MinIOSecretKey       string
	MinIOUseSSL          bool
	MinIOVideoBucket     string
	MinIOThumbnailBucket string
	MinIOPreviewBucket   string

	// JWT
	JWTSecret      string
	JWTExpiryHours int
	// StreamTokenExpiryMinutes bounds the lifetime of the scope-limited token
	// minted for <video> streaming (passed in the URL where headers can't be
	// set). Keep it short; a viewing session that outlives it triggers a
	// transparent token refresh on the client.
	StreamTokenExpiryMinutes int

	// Server
	ServerPort string

	// VideoXAccelPrefix is a deployment-topology flag, not a business parameter:
	// it answers "is there an accel-capable proxy (nginx) in front of me?".
	// Empty (default) → Stream() serves bytes itself via http.ServeFile (dev,
	// unit tests, any path that does not go through nginx). Set to the internal
	// nginx location (e.g. "/internal-video/") in prod → Stream() returns an
	// X-Accel-Redirect header and lets nginx read the file off disk directly.
	VideoXAccelPrefix string

	// Admin defaults
	AdminDefaultUsername string
	AdminDefaultPassword string

	// Enrichment HTTP client (infra parameters; business params are per-request)
	EnrichHTTPTimeout  time.Duration
	EnrichUserAgent    string
	EnrichJavBusCookie string

	// HLS transcode cache directory. A writable directory where FFmpeg writes
	// per-video HLS segments. Mounted as a named Docker volume in production.
	TranscodeCacheDir string
}

func (c *Config) DatabaseDSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=disable",
		c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName,
	)
}

// Load reads configuration from environment variables. Infrastructure secrets
// (DB password, MinIO secret key, JWT signing secret, default admin password)
// are required and have no fallback: Load returns an error when any is unset or
// still set to a known insecure built-in placeholder. This is fail-closed —
// the binary must never run with a publicly-known signing key or password.
func Load() (*Config, error) {
	cfg := &Config{
		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnvInt("DB_PORT", 5432),
		DBUser:     getEnv("DB_USER", "vaultflix"),
		DBPassword: os.Getenv("DB_PASSWORD"),
		DBName:     getEnv("DB_NAME", "vaultflix"),

		MinIOEndpoint:        getEnv("MINIO_ENDPOINT", "localhost:9000"),
		MinIOPublicEndpoint:  getEnv("MINIO_PUBLIC_ENDPOINT", ""),
		MinIOAccessKey:       getEnv("MINIO_ACCESS_KEY", "minioadmin"),
		MinIOSecretKey:       os.Getenv("MINIO_SECRET_KEY"),
		MinIOUseSSL:          getEnvBool("MINIO_USE_SSL", false),
		MinIOVideoBucket:     getEnv("MINIO_VIDEO_BUCKET", "vaultflix-videos"),
		MinIOThumbnailBucket: getEnv("MINIO_THUMBNAIL_BUCKET", "vaultflix-thumbnails"),
		MinIOPreviewBucket:   getEnv("MINIO_PREVIEW_BUCKET", "vaultflix-previews"),

		JWTSecret:                os.Getenv("JWT_SECRET"),
		JWTExpiryHours:           getEnvInt("JWT_EXPIRY_HOURS", 24),
		StreamTokenExpiryMinutes: getEnvInt("STREAM_TOKEN_EXPIRY_MINUTES", 60),

		ServerPort:        getEnv("SERVER_PORT", "8080"),
		VideoXAccelPrefix: getEnv("VIDEO_XACCEL_PREFIX", ""),

		AdminDefaultUsername: getEnv("ADMIN_DEFAULT_USERNAME", "admin"),
		AdminDefaultPassword: os.Getenv("ADMIN_DEFAULT_PASSWORD"),

		EnrichHTTPTimeout:  getEnvDuration("ENRICH_HTTP_TIMEOUT", 15*time.Second),
		EnrichUserAgent:    getEnv("ENRICH_USER_AGENT", "Vaultflix/1.0"),
		EnrichJavBusCookie: getEnv("ENRICH_JAVBUS_COOKIE", "age=verified; existmag=all"),

		TranscodeCacheDir: getEnv("TRANSCODE_CACHE_DIR", "/var/cache/vaultflix/transcode"),
	}

	if err := cfg.validateSecrets(); err != nil {
		return nil, err
	}

	return cfg, nil
}

// insecureDefaults lists the historical built-in placeholder values that must
// never be accepted for the corresponding required secret.
var insecureDefaults = map[string]string{
	"JWT_SECRET":             "change-me-in-production",
	"DB_PASSWORD":            "vaultflix",
	"MINIO_SECRET_KEY":       "minioadmin",
	"ADMIN_DEFAULT_PASSWORD": "admin",
}

// validateSecrets ensures every required secret is set and not left at its
// known insecure default. All problems are reported together so a misconfigured
// deployment can fix them in one pass.
func (c *Config) validateSecrets() error {
	required := map[string]string{
		"JWT_SECRET":             c.JWTSecret,
		"DB_PASSWORD":            c.DBPassword,
		"MINIO_SECRET_KEY":       c.MinIOSecretKey,
		"ADMIN_DEFAULT_PASSWORD": c.AdminDefaultPassword,
	}

	var problems []string
	for _, name := range []string{"JWT_SECRET", "DB_PASSWORD", "MINIO_SECRET_KEY", "ADMIN_DEFAULT_PASSWORD"} {
		value := required[name]
		switch {
		case value == "":
			problems = append(problems, fmt.Sprintf("%s is required but not set", name))
		case value == insecureDefaults[name]:
			problems = append(problems, fmt.Sprintf("%s must not use the insecure built-in default %q", name, value))
		}
	}

	if len(problems) > 0 {
		return fmt.Errorf("insecure or missing required secrets: %s", strings.Join(problems, "; "))
	}
	return nil
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

func getEnvBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
