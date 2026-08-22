package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/steven/vaultflix/internal/config"
	"github.com/steven/vaultflix/internal/repository"
	"github.com/steven/vaultflix/internal/service"
)

// runAdminPasswordReset resets the configured admin account's password to the
// current ADMIN_DEFAULT_PASSWORD, then returns so main can exit.
//
// This is the escape hatch for an existing database: initDefaultAdmin seeds an
// admin only while the users table is empty, so once any user exists no config
// change can recover a lost admin password. Deliberately a separate opt-in flag
// rather than startup behaviour — an env var silently taking over an existing
// account on every boot would be a security regression, not a convenience.
//
// Only the DB is needed, so this runs before MinIO/casbin are wired up.
func runAdminPasswordReset(ctx context.Context, pool *pgxpool.Pool, cfg *config.Config) {
	userRepo := repository.NewUserRepository(pool)
	authService := service.NewAuthService(userRepo, cfg.JWTSecret, cfg.JWTExpiryHours, cfg.StreamTokenExpiryMinutes)

	if err := authService.ResetPassword(ctx, cfg.AdminDefaultUsername, cfg.AdminDefaultPassword); err != nil {
		slog.Error("failed to reset admin password",
			"username", cfg.AdminDefaultUsername,
			"error", err,
		)
		os.Exit(1)
	}

	slog.Info("admin password reset to the current ADMIN_DEFAULT_PASSWORD",
		"username", cfg.AdminDefaultUsername,
	)
}
