package main

import (
	"context"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/steven/vaultflix/internal/config"
	"github.com/steven/vaultflix/internal/handler"
	"github.com/steven/vaultflix/internal/middleware"
	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/repository"
	"github.com/steven/vaultflix/internal/scraper"
	"github.com/steven/vaultflix/internal/service"
	"github.com/steven/vaultflix/internal/streaming"
	"github.com/steven/vaultflix/internal/websocket"
)

// version is injected at build time via -ldflags "-X main.version=<sha>".
// Defaults to "dev" for `go run` / local builds.
var version = "dev"

func main() {
	resetAdminPassword := flag.Bool("reset-admin-password", false,
		"reset the admin account's password to ADMIN_DEFAULT_PASSWORD, then exit")
	flag.Parse()

	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)
	slog.Info("starting vaultflix", "version", version)

	cfg, err := config.Load()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

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

	// Maintenance mode: needs the DB only, so short-circuit before MinIO.
	if *resetAdminPassword {
		runAdminPasswordReset(context.Background(), pool, cfg)
		return
	}

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

	for _, bucket := range []string{cfg.MinIOThumbnailBucket, cfg.MinIOPreviewBucket} {
		ok, err := minioClient.BucketExists(context.Background(), bucket)
		if err != nil {
			slog.Error("failed to check minio bucket", "error", err, "bucket", bucket)
			os.Exit(1)
		}
		if ok {
			slog.Info("minio bucket exists", "bucket", bucket)
		} else {
			slog.Warn("minio bucket not found", "bucket", bucket)
		}
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

	// Initialize WebSocket hub (before services, since ImportService depends on it)
	hub := websocket.NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go hub.Run(ctx)
	slog.Info("websocket hub started")

	// Initialize layers
	userRepo := repository.NewUserRepository(pool)
	videoRepo := repository.NewVideoRepository(pool)
	tagRepo := repository.NewTagRepository(pool)
	historyRepo := repository.NewWatchHistoryRepository(pool)
	favoriteRepo := repository.NewFavoriteRepository(pool)
	recRepo := repository.NewRecommendationRepository(pool)
	mediaSourceRepo := repository.NewMediaSourceRepository(pool)
	watchSessionRepo := repository.NewWatchSessionRepository(pool)
	analyticsRepo := repository.NewAnalyticsRepository(pool)
	playbackTelemetryRepo := repository.NewPlaybackTelemetryRepository(pool)

	minioService := service.NewMinIOService(minioClient, presignClient, cfg.MinIOVideoBucket, cfg.MinIOThumbnailBucket, cfg.MinIOPreviewBucket, service.NewInMemoryURLCache())
	authService := service.NewAuthService(userRepo, cfg.JWTSecret, cfg.JWTExpiryHours, cfg.StreamTokenExpiryMinutes)
	userService := service.NewUserService(userRepo)
	mediaProcessor := service.NewMediaProcessor(service.NewFFmpegTool(), minioService)
	importService := service.NewImportService(videoRepo, mediaProcessor, hub)
	mediaSourceService := service.NewMediaSourceService(mediaSourceRepo, service.AllowedMountPrefix)
	videoService := service.NewVideoService(videoRepo, mediaSourceService, tagRepo, minioService)
	historyService := service.NewWatchHistoryService(historyRepo, videoRepo, minioService)
	favoriteService := service.NewFavoriteService(favoriteRepo, minioService)

	recService := service.NewRecommendationService(recRepo, videoRepo, minioService)
	watchSessionService := service.NewWatchSessionService(watchSessionRepo)
	analyticsService := service.NewAnalyticsService(analyticsRepo)
	playbackTelemetryService := service.NewPlaybackTelemetryService(playbackTelemetryRepo)

	// Enrichment: scraper clients, service, and handler
	suggestionRepo := repository.NewSuggestionRepository(pool)
	enrichHTTPClient := scraper.NewClient(scraper.ClientOptions{
		Timeout:     cfg.EnrichHTTPTimeout,
		UserAgent:   cfg.EnrichUserAgent,
		MinInterval: 2 * time.Second,
		MaxRetries:  2,
		Cookies:     scraper.ParseCookieHeader(cfg.EnrichJavBusCookie),
	})
	scrapers := []scraper.MetadataScraper{
		scraper.NewJavBusScraper(enrichHTTPClient, ""),
		// JavLibrary deferred to Phase 1.x (Cloudflare-blocked); see spec §14.
	}
	enrichService := service.NewEnrichmentService(scrapers, videoRepo, suggestionRepo, minioService, hub)
	enrichHandler := handler.NewEnrichmentHandler(enrichService)

	// Inject user-interaction services into video service for enriching detail responses
	videoService.SetUserServices(favoriteService, historyService)

	segmentGen := streaming.NewFFmpegSegmentGenerator()
	segmentCache, err := streaming.NewSegmentCache(segmentGen, cfg.TranscodeCacheDir, 60*time.Second, cfg.TranscodeCacheMaxBytes)
	if err != nil {
		slog.Error("failed to init segment cache", "error", err)
		os.Exit(1)
	}
	segmentCache.StartSweeper(ctx)

	keyframeIndexRepo := repository.NewKeyframeIndexRepository(pool)
	keyframeService := service.NewKeyframeService(keyframeIndexRepo)
	hlsService := service.NewHLSService(videoService, keyframeService, segmentCache)
	hlsHandler := handler.NewHLSHandler(hlsService)
	importService.SetKeyframeProber(keyframeService)

	authHandler := handler.NewAuthHandler(authService)
	videoHandler := handler.NewVideoHandler(importService, videoService, mediaSourceService, cfg.VideoXAccelPrefix)
	tagHandler := handler.NewTagHandler(tagRepo, videoRepo)
	historyHandler := handler.NewHistoryHandler(historyService)
	favoriteHandler := handler.NewFavoriteHandler(favoriteService)
	recHandler := handler.NewRecommendationHandler(recService)
	userHandler := handler.NewUserHandler(userService)
	mediaSourceHandler := handler.NewMediaSourceHandler(mediaSourceService)
	watchSessionHandler := handler.NewWatchSessionHandler(watchSessionService)
	analyticsHandler := handler.NewAnalyticsHandler(analyticsService)
	playbackTelemetryHandler := handler.NewPlaybackTelemetryHandler(playbackTelemetryService)

	backfillRunner := service.NewBackfillRunner(hub, map[model.BackfillKind]service.BackfillTask{
		model.BackfillPreview:  service.NewPreviewBackfill(videoRepo, mediaSourceService, mediaProcessor),
		model.BackfillCodec:    service.NewCodecBackfill(videoRepo, mediaSourceService, mediaProcessor),
		model.BackfillKeyframe: service.NewKeyframeBackfill(keyframeService, videoRepo, mediaSourceService),
		model.BackfillCode:     service.NewCodeBackfill(videoRepo),
	})
	backfillHandler := handler.NewBackfillHandler(backfillRunner)

	wsHandler := handler.NewWSHandler(hub)

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
	api.Use(middleware.RequireActiveUser(userRepo))
	middleware.RegisterRoutes(api, apiRoutes(apiHandlers{
		auth:              authHandler,
		video:             videoHandler,
		hls:               hlsHandler,
		tag:               tagHandler,
		history:           historyHandler,
		watchSession:      watchSessionHandler,
		playbackTelemetry: playbackTelemetryHandler,
		favorite:          favoriteHandler,
		user:              userHandler,
		recommendation:    recHandler,
		mediaSource:       mediaSourceHandler,
		backfill:          backfillHandler,
		analytics:         analyticsHandler,
		enrichment:        enrichHandler,
		ws:                wsHandler,
	}))

	// Graceful shutdown: cancel Hub context on SIGINT/SIGTERM
	go func() {
		quit := make(chan os.Signal, 1)
		signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
		<-quit
		slog.Info("shutting down server")
		cancel()
	}()

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
