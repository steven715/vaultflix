package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"

	"github.com/casbin/casbin/v2"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/steven/vaultflix/internal/config"
	"github.com/steven/vaultflix/internal/handler"
	"github.com/steven/vaultflix/internal/middleware"
	"github.com/steven/vaultflix/internal/repository"
	"github.com/steven/vaultflix/internal/service"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg := config.Load()

	// Connect to PostgreSQL
	pool, err := pgxpool.New(context.Background(), cfg.DatabaseDSN())
	if err != nil {
		slog.Error("failed to connect to postgresql", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(context.Background()); err != nil {
		slog.Error("failed to ping postgresql", "error", err)
		os.Exit(1)
	}
	slog.Info("connected to postgresql")

	// Connect to MinIO
	minioClient, err := minio.New(cfg.MinIOEndpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.MinIOAccessKey, cfg.MinIOSecretKey, ""),
		Secure: cfg.MinIOUseSSL,
	})
	if err != nil {
		slog.Error("failed to create minio client", "error", err)
		os.Exit(1)
	}

	exists, err := minioClient.BucketExists(context.Background(), cfg.MinIOVideoBucket)
	if err != nil {
		slog.Error("failed to connect to minio", "error", err)
		os.Exit(1)
	}
	if exists {
		slog.Info("minio connected, bucket exists", "bucket", cfg.MinIOVideoBucket)
	} else {
		slog.Warn("minio connected, bucket not found", "bucket", cfg.MinIOVideoBucket)
	}

	// Create a separate MinIO client for presigned URL generation using the public endpoint.
	// Uses BucketLookupPath to avoid location lookup calls to the unreachable public endpoint.
	var presignClient *minio.Client
	if cfg.MinIOPublicEndpoint != "" && cfg.MinIOPublicEndpoint != cfg.MinIOEndpoint {
		presignClient, err = minio.New(cfg.MinIOPublicEndpoint, &minio.Options{
			Creds:        credentials.NewStaticV4(cfg.MinIOAccessKey, cfg.MinIOSecretKey, ""),
			Secure:       cfg.MinIOUseSSL,
			BucketLookup: minio.BucketLookupPath,
			Region:       "us-east-1",
		})
		if err != nil {
			slog.Error("failed to create minio presign client", "error", err)
			os.Exit(1)
		}
		slog.Info("minio presign client created", "public_endpoint", cfg.MinIOPublicEndpoint)
	}

	// Initialize Casbin enforcer
	enforcer, err := casbin.NewEnforcer("casbin/model.conf", "casbin/policy.csv")
	if err != nil {
		slog.Error("failed to initialize casbin enforcer", "error", err)
		os.Exit(1)
	}
	slog.Info("casbin enforcer initialized")

	// Initialize layers
	userRepo := repository.NewUserRepository(pool)
	videoRepo := repository.NewVideoRepository(pool)
	tagRepo := repository.NewTagRepository(pool)
	historyRepo := repository.NewWatchHistoryRepository(pool)
	favoriteRepo := repository.NewFavoriteRepository(pool)
	recRepo := repository.NewRecommendationRepository(pool)
	mediaSourceRepo := repository.NewMediaSourceRepository(pool)

	minioService := service.NewMinIOService(minioClient, presignClient, cfg.MinIOVideoBucket, cfg.MinIOThumbnailBucket)
	authService := service.NewAuthService(userRepo, cfg.JWTSecret, cfg.JWTExpiryHours)
	userService := service.NewUserService(userRepo)
	importService := service.NewImportService(videoRepo, minioService)
	videoService := service.NewVideoService(videoRepo, tagRepo, minioService)
	historyService := service.NewWatchHistoryService(historyRepo, videoRepo, minioService)
	favoriteService := service.NewFavoriteService(favoriteRepo, minioService)

	recService := service.NewRecommendationService(recRepo, videoRepo, minioService)
	mediaSourceService := service.NewMediaSourceService(mediaSourceRepo, service.AllowedMountPrefix)

	// Inject user-interaction services into video service for enriching detail responses
	videoService.SetUserServices(favoriteService, historyService)

	authHandler := handler.NewAuthHandler(authService)
	videoHandler := handler.NewVideoHandler(importService, videoService, mediaSourceService)
	tagHandler := handler.NewTagHandler(tagRepo, videoRepo)
	historyHandler := handler.NewHistoryHandler(historyService)
	favoriteHandler := handler.NewFavoriteHandler(favoriteService)
	recHandler := handler.NewRecommendationHandler(recService)
	userHandler := handler.NewUserHandler(userService)
	mediaSourceHandler := handler.NewMediaSourceHandler(mediaSourceService)

	// Initialize default admin account
	initDefaultAdmin(context.Background(), userRepo, authService, cfg)

	// Setup Gin router
	r := gin.Default()

	// Public routes
	r.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	r.POST("/api/auth/register", authHandler.Register)
	r.POST("/api/auth/login", authHandler.Login)

	// Protected routes
	api := r.Group("/api")
	api.Use(middleware.JWTAuth(cfg.JWTSecret))
	api.Use(middleware.CasbinRBAC(enforcer))
	{
		api.GET("/me", authHandler.Me)

		// Video endpoints
		api.GET("/videos", videoHandler.List)
		api.GET("/videos/:id", videoHandler.GetByID)
		api.PUT("/videos/:id", videoHandler.Update)
		api.DELETE("/videos/:id", videoHandler.Delete)
		api.POST("/videos/import", videoHandler.Import)
		api.POST("/videos/:id/tags", tagHandler.AddVideoTag)
		api.DELETE("/videos/:id/tags/:tagId", tagHandler.RemoveVideoTag)

		// Tag endpoints
		api.GET("/tags", tagHandler.List)
		api.POST("/tags", tagHandler.Create)

		// Watch history endpoints
		api.POST("/watch-history", historyHandler.SaveProgress)
		api.GET("/watch-history", historyHandler.List)

		// Favorite endpoints
		api.GET("/favorites", favoriteHandler.List)
		api.POST("/favorites", favoriteHandler.Add)
		api.DELETE("/favorites/:videoId", favoriteHandler.Remove)

		// User management endpoints (admin only, enforced by Casbin)
		api.GET("/users", userHandler.List)
		api.POST("/users", userHandler.Create)
		api.DELETE("/users/:id", userHandler.Delete)
		api.PUT("/users/:id/enable", userHandler.Enable)
		api.PUT("/users/:id/password", userHandler.ResetPassword)

		// Recommendation endpoints
		api.GET("/recommendations/today", recHandler.GetToday)
		api.GET("/recommendations", recHandler.ListByDate)
		api.POST("/recommendations", recHandler.Create)
		api.PUT("/recommendations/:id", recHandler.UpdateSortOrder)
		api.DELETE("/recommendations/:id", recHandler.Delete)

		// Media source endpoints (admin only, enforced by Casbin)
		api.GET("/media-sources", mediaSourceHandler.List)
		api.POST("/media-sources", mediaSourceHandler.Create)
		api.PUT("/media-sources/:id", mediaSourceHandler.Update)
		api.DELETE("/media-sources/:id", mediaSourceHandler.Delete)
	}

	slog.Info("starting server", "port", cfg.ServerPort)
	if err := r.Run(":" + cfg.ServerPort); err != nil {
		slog.Error("failed to start server", "error", err)
		os.Exit(1)
	}
}

func initDefaultAdmin(ctx context.Context, userRepo repository.UserRepository, authService *service.AuthService, cfg *config.Config) {
	count, err := userRepo.CountUsers(ctx)
	if err != nil {
		slog.Error("failed to count users for admin init", "error", err)
		os.Exit(1)
	}

	if count > 0 {
		slog.Info("users table not empty, skipping admin init", "user_count", count)
		return
	}

	_, err = authService.Register(ctx, cfg.AdminDefaultUsername, cfg.AdminDefaultPassword, "admin")
	if err != nil {
		slog.Error("failed to create default admin account", "error", err)
		os.Exit(1)
	}

	slog.Info("default admin account created", "username", cfg.AdminDefaultUsername)
}
