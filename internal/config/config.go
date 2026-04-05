package config

import (
	"fmt"
	"os"
	"strconv"
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

	// JWT
	JWTSecret      string
	JWTExpiryHours int

	// Server
	ServerPort string

	// Admin defaults
	AdminDefaultUsername string
	AdminDefaultPassword string

}

func (c *Config) DatabaseDSN() string {
	return fmt.Sprintf(
		"postgres://%s:%s@%s:%d/%s?sslmode=disable",
		c.DBUser, c.DBPassword, c.DBHost, c.DBPort, c.DBName,
	)
}

func Load() *Config {
	return &Config{
		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnvInt("DB_PORT", 5432),
		DBUser:     getEnv("DB_USER", "vaultflix"),
		DBPassword: getEnv("DB_PASSWORD", "vaultflix"),
		DBName:     getEnv("DB_NAME", "vaultflix"),

		MinIOEndpoint:        getEnv("MINIO_ENDPOINT", "localhost:9000"),
		MinIOPublicEndpoint:  getEnv("MINIO_PUBLIC_ENDPOINT", ""),
		MinIOAccessKey:       getEnv("MINIO_ACCESS_KEY", "minioadmin"),
		MinIOSecretKey:       getEnv("MINIO_SECRET_KEY", "minioadmin"),
		MinIOUseSSL:          getEnvBool("MINIO_USE_SSL", false),
		MinIOVideoBucket:     getEnv("MINIO_VIDEO_BUCKET", "vaultflix-videos"),
		MinIOThumbnailBucket: getEnv("MINIO_THUMBNAIL_BUCKET", "vaultflix-thumbnails"),

		JWTSecret:      getEnv("JWT_SECRET", "change-me-in-production"),
		JWTExpiryHours: getEnvInt("JWT_EXPIRY_HOURS", 24),

		ServerPort: getEnv("SERVER_PORT", "8080"),

		AdminDefaultUsername: getEnv("ADMIN_DEFAULT_USERNAME", "admin"),
		AdminDefaultPassword: getEnv("ADMIN_DEFAULT_PASSWORD", "admin"),
	}
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
