# Phase 10 — Video Streaming Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace MinIO presigned URL streaming with `http.ServeFile` local-path streaming, enabling playback of Phase 9 imported videos.

**Architecture:** New `/api/videos/:id/stream` endpoint serves video files directly from disk via `http.ServeFile` (supports Range Request natively). Auth middleware gains `?token=` query param fallback for `<video src>`. Legacy MinIO videos get 307 redirect to presigned URL.

**Tech Stack:** Go 1.22, Gin, http.ServeFile, React 18, Nginx

---

## File Structure

| Action | File | Responsibility |
|--------|------|----------------|
| Modify | `internal/middleware/auth.go` | Add `?token=` query param fallback |
| Modify | `internal/middleware/auth_test.go` (create) | Test token from header, query param, precedence |
| Modify | `internal/handler/video_handler.go` | Add `Stream` method, simplify `GetByID` |
| Modify | `internal/handler/video_handler_test.go` | Add stream handler tests, update GetByID tests |
| Modify | `internal/service/video_service.go` | Remove expiry param from `GetByID`, return `/api/videos/{id}/stream` as `stream_url` |
| Modify | `internal/service/video_service_test.go` | Update `GetByID` tests for new signature |
| Modify | `web/src/pages/PlayerPage.tsx` | Append `?token=` to `stream_url` |
| Modify | `web/src/api/videos.ts` | Remove `urlExpiryMinutes` param |
| Modify | `web/nginx.conf` | Add stream-specific proxy location |
| Modify | `casbin/policy.csv` | Add viewer stream permission |
| Modify | `cmd/server/main.go` | Register stream route |

---

### Task 1: Auth Middleware — Query Param Token Fallback

**Files:**
- Modify: `internal/middleware/auth.go`
- Create: `internal/middleware/auth_test.go`

- [ ] **Step 1: Create auth middleware test file with table-driven tests**

```go
// internal/middleware/auth_test.go
package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func init() {
	gin.SetMode(gin.TestMode)
}

const testJWTSecret = "test-secret-key"

func generateTestToken(secret string, claims jwt.MapClaims) string {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, _ := token.SignedString([]byte(secret))
	return signed
}

func validClaims() jwt.MapClaims {
	return jwt.MapClaims{
		"user_id":  "user-1",
		"username": "testuser",
		"role":     "viewer",
		"exp":      float64(time.Now().Add(time.Hour).Unix()),
	}
}

func setupAuthRouter(secret string) *gin.Engine {
	r := gin.New()
	r.Use(JWTAuth(secret))
	r.GET("/protected", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{
			"user_id":  c.GetString("user_id"),
			"username": c.GetString("username"),
			"role":     c.GetString("role"),
		})
	})
	return r
}

func TestJWTAuth_TokenSources(t *testing.T) {
	validToken := generateTestToken(testJWTSecret, validClaims())
	invalidToken := "invalid.token.here"

	tests := []struct {
		name           string
		headerToken    string
		queryToken     string
		expectedStatus int
		expectedUserID string
	}{
		{
			name:           "valid token from header",
			headerToken:    validToken,
			queryToken:     "",
			expectedStatus: http.StatusOK,
			expectedUserID: "user-1",
		},
		{
			name:           "valid token from query param",
			headerToken:    "",
			queryToken:     validToken,
			expectedStatus: http.StatusOK,
			expectedUserID: "user-1",
		},
		{
			name:           "header takes precedence over query param",
			headerToken:    validToken,
			queryToken:     invalidToken,
			expectedStatus: http.StatusOK,
			expectedUserID: "user-1",
		},
		{
			name:           "invalid token from query param",
			headerToken:    "",
			queryToken:     invalidToken,
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "no token at all",
			headerToken:    "",
			queryToken:     "",
			expectedStatus: http.StatusUnauthorized,
		},
		{
			name:           "expired token from header",
			headerToken:    generateTestToken(testJWTSecret, jwt.MapClaims{"user_id": "u1", "username": "u", "role": "viewer", "exp": float64(time.Now().Add(-time.Hour).Unix())}),
			queryToken:     "",
			expectedStatus: http.StatusUnauthorized,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := setupAuthRouter(testJWTSecret)

			url := "/protected"
			if tt.queryToken != "" {
				url += "?token=" + tt.queryToken
			}
			req := httptest.NewRequest(http.MethodGet, url, nil)
			if tt.headerToken != "" {
				req.Header.Set("Authorization", "Bearer "+tt.headerToken)
			}

			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Fatalf("expected status %d, got %d, body: %s", tt.expectedStatus, w.Code, w.Body.String())
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `docker compose exec vaultflix-api go test ./internal/middleware/ -run TestJWTAuth_TokenSources -v`
Expected: FAIL — tests for query param token will fail because middleware doesn't support it yet.

- [ ] **Step 3: Modify auth middleware to support query param token**

Replace the entire `JWTAuth` function in `internal/middleware/auth.go`:

```go
package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"

	"github.com/steven/vaultflix/internal/model"
)

func JWTAuth(jwtSecret string) gin.HandlerFunc {
	secret := []byte(jwtSecret)

	return func(c *gin.Context) {
		var tokenString string

		// Priority 1: Authorization header
		authHeader := c.GetHeader("Authorization")
		if strings.HasPrefix(authHeader, "Bearer ") {
			tokenString = strings.TrimPrefix(authHeader, "Bearer ")
		}

		// Priority 2: query parameter fallback for contexts where custom headers
		// cannot be set (e.g. <video src>, WebSocket upgrade, SSE, file download).
		// Trade-off: token appears in server access logs and browser history.
		// Acceptable for self-hosted use; production-grade systems should use
		// short-lived tokens or a separate cookie-based auth for streaming.
		if tokenString == "" {
			tokenString = c.Query("token")
		}

		if tokenString == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, model.ErrorResponse{
				Error:   "unauthorized",
				Message: "missing token",
			})
			return
		}

		token, err := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return secret, nil
		})
		if err != nil || !token.Valid {
			c.AbortWithStatusJSON(http.StatusUnauthorized, model.ErrorResponse{
				Error:   "unauthorized",
				Message: "invalid or expired token",
			})
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			c.AbortWithStatusJSON(http.StatusUnauthorized, model.ErrorResponse{
				Error:   "unauthorized",
				Message: "invalid or expired token",
			})
			return
		}

		c.Set("user_id", claims["user_id"])
		c.Set("username", claims["username"])
		c.Set("role", claims["role"])

		c.Next()
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `docker compose exec vaultflix-api go test ./internal/middleware/ -run TestJWTAuth_TokenSources -v`
Expected: PASS — all 6 sub-tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/middleware/auth.go internal/middleware/auth_test.go
git commit -m "feat: support JWT token from query param in auth middleware"
```

---

### Task 2: Video Service — Remove Expiry Param, Return Stream Path

**Files:**
- Modify: `internal/service/video_service.go`
- Modify: `internal/service/video_service_test.go`

- [ ] **Step 1: Update existing tests for new `GetByID` signature**

In `internal/service/video_service_test.go`, update all `GetByID` calls to remove the `expiry` parameter, and update stream URL expectations:

For `TestVideoService_GetByID_Success`: Change call from `svc.GetByID(context.Background(), "vid-1", 2*time.Hour, "")` to `svc.GetByID(context.Background(), "vid-1", "")`. Change stream URL expectation from `"https://minio/stream-url"` to `"/api/videos/vid-1/stream"`. Remove `GeneratePresignedURLFunc` from mock setup (it should not be called). Remove the `time` import if unused after changes.

For `TestVideoService_GetByID_NotFound`: Change call from `svc.GetByID(context.Background(), "nonexistent", 2*time.Hour, "")` to `svc.GetByID(context.Background(), "nonexistent", "")`.

For `TestVideoService_GetByID_LocalPathMode`: Change call from `svc.GetByID(context.Background(), "vid-1", 2*time.Hour, "")` to `svc.GetByID(context.Background(), "vid-1", "")`. Change stream URL expectation from `""` to `"/api/videos/vid-1/stream"` (all videos now get the stream path).

- [ ] **Step 2: Run tests to verify they fail**

Run: `docker compose exec vaultflix-api go test ./internal/service/ -run TestVideoService_GetByID -v`
Expected: FAIL — compilation error because `GetByID` still expects 4 args.

- [ ] **Step 3: Update `GetByID` in video service**

In `internal/service/video_service.go`, replace the `GetByID` method:

```go
func (s *VideoService) GetByID(ctx context.Context, id string, userID string) (*model.VideoDetail, error) {
	video, err := s.videoRepo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get video %s: %w", id, err)
	}

	tags, err := s.tagRepo.GetByVideoID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get tags for video %s: %w", id, err)
	}

	streamURL := fmt.Sprintf("/api/videos/%s/stream", id)

	var thumbnailURL string
	if video.ThumbnailKey != "" {
		thumbnailURL, err = s.minioSvc.GenerateThumbnailPresignedURL(ctx, video.ThumbnailKey, 0)
		if err != nil {
			slog.Warn("failed to generate thumbnail url",
				"video_id", id,
				"error", err,
			)
		}
	}

	detail := &model.VideoDetail{
		VideoWithTags: model.VideoWithTags{
			Video:        *video,
			Tags:         tags,
			ThumbnailURL: thumbnailURL,
		},
		StreamURL: streamURL,
	}

	// Enrich with user-specific data if services are available and userID is provided
	if userID != "" && s.favoriteSvc != nil {
		favorited, err := s.favoriteSvc.IsFavorited(ctx, userID, id)
		if err != nil {
			slog.Warn("failed to check favorite status",
				"video_id", id,
				"user_id", userID,
				"error", err,
			)
		} else {
			detail.IsFavorited = favorited
		}
	}

	if userID != "" && s.historySvc != nil {
		progress, err := s.historySvc.GetProgress(ctx, userID, id)
		if err != nil {
			slog.Warn("failed to get watch progress",
				"video_id", id,
				"user_id", userID,
				"error", err,
			)
		} else {
			detail.WatchProgress = progress
		}
	}

	return detail, nil
}
```

Also remove the `"time"` import from this file if it's no longer used (check that no other function uses it).

- [ ] **Step 4: Run tests to verify they pass**

Run: `docker compose exec vaultflix-api go test ./internal/service/ -run TestVideoService_GetByID -v`
Expected: PASS — all GetByID tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/service/video_service.go internal/service/video_service_test.go
git commit -m "refactor: remove expiry param from VideoService.GetByID, return stream path"
```

---

### Task 3: Video Handler — Simplify GetByID, Fix Compilation

**Files:**
- Modify: `internal/handler/video_handler.go`
- Modify: `internal/handler/video_handler_test.go`

- [ ] **Step 1: Update handler `GetByID` to match new service signature**

In `internal/handler/video_handler.go`, replace the `GetByID` method:

```go
func (h *VideoHandler) GetByID(c *gin.Context) {
	id := c.Param("id")

	userID := c.GetString("user_id")
	detail, err := h.videoService.GetByID(c.Request.Context(), id, userID)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{
				Error:   "not_found",
				Message: "video not found",
			})
			return
		}
		slog.Error("failed to get video", "error", err, "video_id", id)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to get video",
		})
		return
	}

	c.JSON(http.StatusOK, model.SuccessResponse{
		Data: detail,
	})
}
```

Also remove unused imports (`"strconv"`, `"time"`) from the handler file — but only if no other method in the file uses them. `"strconv"` is still used by `parseVideoFilter`, so keep it. `"time"` can be removed. `"strings"` is still used by `parseVideoFilter`, so keep it.

- [ ] **Step 2: Update handler tests for new GetByID**

In `internal/handler/video_handler_test.go`:

For `TestGetVideo_Success`: Update the mock `GeneratePresignedURLFunc` — it should no longer be needed for stream URL. Update the stream URL assertion from `"https://minio/stream"` to `"/api/videos/vid-1/stream"`.

Remove `TestGetVideo_InvalidExpiry` entirely — the `url_expiry_minutes` parameter no longer exists.

Remove unused imports (`"time"` if no longer used, keep `"strings"` for import handler test).

- [ ] **Step 3: Run all handler tests to verify they pass**

Run: `docker compose exec vaultflix-api go test ./internal/handler/ -run TestGetVideo -v`
Expected: PASS

- [ ] **Step 4: Run full test suite to catch any remaining compilation issues**

Run: `docker compose exec vaultflix-api go test ./...`
Expected: PASS — all tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/handler/video_handler.go internal/handler/video_handler_test.go
git commit -m "refactor: simplify GetByID handler, remove url_expiry_minutes param"
```

---

### Task 4: Stream Handler

**Files:**
- Modify: `internal/handler/video_handler.go`
- Modify: `internal/handler/video_handler_test.go`

- [ ] **Step 1: Write stream handler tests**

The stream handler calls `videoService.GetPresignedURL` for legacy redirect (added in Step 3). The handler accesses MinIO through `VideoService`, not directly.

Append to `internal/handler/video_handler_test.go` (add `"os"` to the import block):

```go
func setupStreamRouter(videoSvc *service.VideoService, mediaSourceSvc *service.MediaSourceService) (*gin.Engine, *VideoHandler) {
	r := gin.New()
	h := NewVideoHandler(nil, videoSvc, mediaSourceSvc)
	r.GET("/api/videos/:id/stream", h.Stream)
	return r, h
}

func TestStreamVideo(t *testing.T) {
	// Create a temp file to serve as video content
	tmpDir := t.TempDir()
	videoContent := []byte("fake video content for testing")
	videoFile := tmpDir + "/test.mp4"
	if err := os.WriteFile(videoFile, videoContent, 0644); err != nil {
		t.Fatalf("failed to write temp video file: %v", err)
	}

	sourceID := "src-1"
	// filePath is relative to mount path
	filePath := "test.mp4"

	tests := []struct {
		name           string
		videoID        string
		video          *model.Video
		videoErr       error
		source         *model.MediaSource
		sourceErr      error
		expectedStatus int
		expectedBody   string
		rangeHeader    string
	}{
		{
			name:    "success - new mode",
			videoID: "vid-1",
			video: &model.Video{
				ID:       "vid-1",
				MimeType: "video/mp4",
				SourceID: &sourceID,
				FilePath: &filePath,
			},
			source: &model.MediaSource{
				ID:        "src-1",
				MountPath: tmpDir,
				Enabled:   true,
			},
			expectedStatus: http.StatusOK,
			expectedBody:   "fake video content for testing",
		},
		{
			name:    "range request",
			videoID: "vid-1",
			video: &model.Video{
				ID:       "vid-1",
				MimeType: "video/mp4",
				SourceID: &sourceID,
				FilePath: &filePath,
			},
			source: &model.MediaSource{
				ID:        "src-1",
				MountPath: tmpDir,
				Enabled:   true,
			},
			expectedStatus: http.StatusPartialContent,
			rangeHeader:    "bytes=0-9",
		},
		{
			name:           "video not found in DB",
			videoID:        "nonexistent",
			videoErr:       model.ErrNotFound,
			expectedStatus: http.StatusNotFound,
		},
		{
			name:    "source disabled",
			videoID: "vid-1",
			video: &model.Video{
				ID:       "vid-1",
				MimeType: "video/mp4",
				SourceID: &sourceID,
				FilePath: &filePath,
			},
			source: &model.MediaSource{
				ID:        "src-1",
				MountPath: tmpDir,
				Enabled:   false,
			},
			expectedStatus: http.StatusServiceUnavailable,
		},
		{
			name:    "file not on disk",
			videoID: "vid-1",
			video: func() *model.Video {
				fp := "nonexistent.mp4"
				return &model.Video{
					ID:       "vid-1",
					MimeType: "video/mp4",
					SourceID: &sourceID,
					FilePath: &fp,
				}
			}(),
			source: &model.MediaSource{
				ID:        "src-1",
				MountPath: tmpDir,
				Enabled:   true,
			},
			expectedStatus: http.StatusNotFound,
		},
		{
			name:    "path traversal attempt",
			videoID: "vid-1",
			video: func() *model.Video {
				fp := "../../etc/passwd"
				return &model.Video{
					ID:       "vid-1",
					MimeType: "video/mp4",
					SourceID: &sourceID,
					FilePath: &fp,
				}
			}(),
			source: &model.MediaSource{
				ID:        "src-1",
				MountPath: "/mnt/host/videos",
				Enabled:   true,
			},
			expectedStatus: http.StatusForbidden,
		},
		{
			name:    "legacy mode - redirect to presigned URL",
			videoID: "vid-legacy",
			video: &model.Video{
				ID:             "vid-legacy",
				MinIOObjectKey: "videos/vid-legacy/test.mp4",
				MimeType:       "video/mp4",
			},
			expectedStatus: http.StatusTemporaryRedirect,
		},
		{
			name:    "source_id is nil and no minio key",
			videoID: "vid-broken",
			video: &model.Video{
				ID:       "vid-broken",
				MimeType: "video/mp4",
			},
			expectedStatus: http.StatusInternalServerError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			videoRepo := &mock.VideoRepository{
				GetByIDFunc: func(ctx context.Context, id string) (*model.Video, error) {
					if tt.videoErr != nil {
						return nil, tt.videoErr
					}
					return tt.video, nil
				},
			}
			tagRepo := &mock.TagRepository{
				GetByVideoIDFunc: func(ctx context.Context, videoID string) ([]model.Tag, error) {
					return []model.Tag{}, nil
				},
			}
			minioSvc := &mock.MinIOClient{
				GeneratePresignedURLFunc: func(ctx context.Context, objectKey string, expiry time.Duration) (string, error) {
					return "https://minio/legacy-stream", nil
				},
				GenerateThumbnailPresignedURLFunc: func(ctx context.Context, objectKey string, expiry time.Duration) (string, error) {
					return "", nil
				},
			}

			videoSvc := service.NewVideoService(videoRepo, tagRepo, minioSvc)

			var mediaSourceSvc *service.MediaSourceService
			if tt.source != nil || tt.sourceErr != nil {
				mediaSourceRepo := &mock.MediaSourceRepository{
					FindByIDFunc: func(ctx context.Context, id string) (*model.MediaSource, error) {
						if tt.sourceErr != nil {
							return nil, tt.sourceErr
						}
						return tt.source, nil
					},
				}
				mediaSourceSvc = service.NewMediaSourceService(mediaSourceRepo, "/mnt/host/")
			}

			r, _ := setupStreamRouter(videoSvc, mediaSourceSvc)

			url := "/api/videos/" + tt.videoID + "/stream"
			req := httptest.NewRequest(http.MethodGet, url, nil)
			if tt.rangeHeader != "" {
				req.Header.Set("Range", tt.rangeHeader)
			}
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Fatalf("expected status %d, got %d, body: %s", tt.expectedStatus, w.Code, w.Body.String())
			}

			if tt.expectedBody != "" && w.Body.String() != tt.expectedBody {
				t.Errorf("expected body %q, got %q", tt.expectedBody, w.Body.String())
			}

			if tt.expectedStatus == http.StatusTemporaryRedirect {
				loc := w.Header().Get("Location")
				if loc != "https://minio/legacy-stream" {
					t.Errorf("expected redirect to presigned URL, got Location: %s", loc)
				}
			}
		})
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `docker compose exec vaultflix-api go test ./internal/handler/ -run TestStreamVideo -v`
Expected: FAIL — `Stream` method does not exist, and `VideoService` has no `GetPresignedURL`.

- [ ] **Step 3: Add `GetPresignedURL` to VideoService**

In `internal/service/video_service.go`, add after the `GetByID` method:

```go
// GetPresignedURL generates a presigned URL for legacy MinIO-stored videos.
func (s *VideoService) GetPresignedURL(ctx context.Context, objectKey string) (string, error) {
	return s.minioSvc.GeneratePresignedURL(ctx, objectKey, 0)
}
```

- [ ] **Step 4: Implement Stream handler**

In `internal/handler/video_handler.go`, add the following imports to the import block (if not already present): `"net/http"`, `"os"`, `"path/filepath"`. Add `"net/http"` is already imported via `"net/http"`. Add `"os"` and `"path/filepath"`.

Add the `Stream` method:

```go
// Stream serves a video file directly from disk using http.ServeFile.
// Supports HTTP Range Request (seeking), Content-Length, and If-Modified-Since (304).
// Authentication: JWT from Authorization header or ?token= query param.
func (h *VideoHandler) Stream(c *gin.Context) {
	ctx := c.Request.Context()
	videoID := c.Param("id")

	video, err := h.videoService.GetByID(ctx, videoID, "")
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			c.JSON(http.StatusNotFound, model.ErrorResponse{
				Error:   "not_found",
				Message: "video not found",
			})
			return
		}
		slog.Error("failed to get video for streaming", "error", err, "video_id", videoID)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to get video",
		})
		return
	}

	// Legacy mode: video stored in MinIO (no source_id, has minio_object_key)
	if video.SourceID == nil && video.MinIOObjectKey != "" {
		presignedURL, err := h.videoService.GetPresignedURL(ctx, video.MinIOObjectKey)
		if err != nil {
			slog.Error("failed to generate presigned url for legacy video",
				"error", err, "video_id", videoID)
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{
				Error:   "internal_error",
				Message: "failed to generate stream url",
			})
			return
		}
		c.Redirect(http.StatusTemporaryRedirect, presignedURL)
		return
	}

	// New mode: video stored on local disk via media source
	if video.SourceID == nil || video.FilePath == nil {
		slog.Error("video has no source and no minio key", "video_id", videoID)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "video has no playable source",
		})
		return
	}

	source, err := h.mediaSourceService.GetByID(ctx, *video.SourceID)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			slog.Error("media source not found for video",
				"video_id", videoID, "source_id", *video.SourceID)
			c.JSON(http.StatusInternalServerError, model.ErrorResponse{
				Error:   "internal_error",
				Message: "media source not found",
			})
			return
		}
		slog.Error("failed to get media source", "error", err, "source_id", *video.SourceID)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{
			Error:   "internal_error",
			Message: "failed to get media source",
		})
		return
	}

	if !source.Enabled {
		c.JSON(http.StatusServiceUnavailable, model.ErrorResponse{
			Error:   "source_unavailable",
			Message: "media source is currently disabled",
		})
		return
	}

	fullPath := filepath.Join(source.MountPath, *video.FilePath)
	cleanPath := filepath.Clean(fullPath)

	// Path traversal protection: resolved path must stay within the source's mount path.
	// The mount path itself was validated against AllowedMountPrefix when the source was created.
	cleanMount := filepath.Clean(source.MountPath)
	if !strings.HasPrefix(cleanPath, cleanMount) {
		c.JSON(http.StatusForbidden, model.ErrorResponse{
			Error:   "path_not_allowed",
			Message: "resolved file path is outside allowed area",
		})
		return
	}

	if _, err := os.Stat(cleanPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, model.ErrorResponse{
			Error:   "file_not_found",
			Message: "video file not found on disk (may have been moved or drive unmounted)",
		})
		return
	}

	c.Header("Content-Type", video.MimeType)
	http.ServeFile(c.Writer, c.Request, cleanPath)
}
```

Add `"os"` and `"path/filepath"` to the import block of `video_handler.go`.

- [ ] **Step 5: Run tests to verify they pass**

Run: `docker compose exec vaultflix-api go test ./internal/handler/ -run TestStreamVideo -v`
Expected: PASS — all 7 sub-tests pass.

- [ ] **Step 6: Run full test suite**

Run: `docker compose exec vaultflix-api go test ./...`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add internal/handler/video_handler.go internal/handler/video_handler_test.go internal/service/video_service.go
git commit -m "feat: add video stream handler with http.ServeFile and legacy redirect"
```

---

### Task 5: Casbin Policy + Route Registration

**Files:**
- Modify: `casbin/policy.csv`
- Modify: `cmd/server/main.go`

- [ ] **Step 1: Add viewer stream permission to Casbin policy**

Append to `casbin/policy.csv` (before the trailing newline):

```
p, viewer, /api/videos/:id/stream, GET
```

- [ ] **Step 2: Register stream route**

In `cmd/server/main.go`, add the stream route after the existing video routes (after line `api.POST("/videos/import", videoHandler.Import)`):

```go
api.GET("/videos/:id/stream", videoHandler.Stream)
```

- [ ] **Step 3: Run full test suite**

Run: `docker compose exec vaultflix-api go test ./...`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add casbin/policy.csv cmd/server/main.go
git commit -m "feat: register stream route and add viewer Casbin permission"
```

---

### Task 6: Nginx Configuration for Streaming

**Files:**
- Modify: `web/nginx.conf`

- [ ] **Step 1: Add stream-specific proxy location**

In `web/nginx.conf`, add a new location block **before** the existing `/api/` block:

```nginx
    # Video streaming: disable buffering for large file pass-through
    location ~ ^/api/videos/[^/]+/stream$ {
        proxy_pass http://vaultflix-api:8080;
        proxy_buffering off;
        client_max_body_size 0;
        proxy_set_header Range $http_range;
        proxy_set_header If-Range $http_if_range;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
```

The full `web/nginx.conf` should be:

```nginx
server {
    listen 80;
    server_name _;
    root /usr/share/nginx/html;
    index index.html;

    # Video streaming: disable buffering for large file pass-through
    location ~ ^/api/videos/[^/]+/stream$ {
        proxy_pass http://vaultflix-api:8080;
        proxy_buffering off;
        client_max_body_size 0;
        proxy_set_header Range $http_range;
        proxy_set_header If-Range $http_if_range;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    # API reverse proxy
    location /api/ {
        proxy_pass http://vaultflix-api:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }

    # Static assets with long-term cache
    location /assets/ {
        expires 1y;
        add_header Cache-Control "public, immutable";
    }

    # SPA fallback: index.html must not be cached
    location / {
        try_files $uri $uri/ /index.html;
        add_header Cache-Control "no-cache";
    }
}
```

- [ ] **Step 2: Commit**

```bash
git add web/nginx.conf
git commit -m "feat: add nginx streaming proxy config with buffering disabled"
```

---

### Task 7: Frontend — Append Token to Stream URL

**Files:**
- Modify: `web/src/pages/PlayerPage.tsx`
- Modify: `web/src/api/videos.ts`

- [ ] **Step 1: Update `getVideo` API function to remove expiry param**

Replace `web/src/api/videos.ts`:

```typescript
import client from './client'
import type { PaginatedResponse, VideoWithTags, VideoDetail, VideoListParams } from '../types'

export async function listVideos(params: VideoListParams): Promise<PaginatedResponse<VideoWithTags>> {
  const res = await client.get<PaginatedResponse<VideoWithTags>>('/videos', { params })
  return res.data
}

export async function getVideo(id: string): Promise<VideoDetail> {
  const res = await client.get<VideoDetail>(`/videos/${id}`)
  return res.data
}
```

- [ ] **Step 2: Update PlayerPage to append token to stream URL**

In `web/src/pages/PlayerPage.tsx`, make these changes:

Add `useAuth` import:

```typescript
import { useAuth } from '../contexts/AuthContext'
```

Inside `PlayerPage` function, add after the refs declarations (after line `const videoIDRef = useRef<string>('')`):

```typescript
const { token } = useAuth()
```

Replace the `<video>` element's `src` attribute. Change from:

```tsx
src={video.stream_url}
```

To:

```tsx
src={token ? `${video.stream_url}?token=${token}` : undefined}
```

Update `handleVideoError` — the retry logic re-fetches the video detail. Since stream URL is now a stable path (not an expiring presigned URL), the error is less likely to be URL expiry. Keep the retry logic (1 retry) but update the `getVideo` call to remove the second argument (already done since we removed the param).

Check that `getVideo(video.id)` call on line 138 has no second argument — it should already be `getVideo(video.id)` without `urlExpiryMinutes`.

- [ ] **Step 3: Commit**

```bash
git add web/src/pages/PlayerPage.tsx web/src/api/videos.ts
git commit -m "feat: append JWT token to stream URL in player page"
```

---

### Task 8: Cleanup and Final Verification

**Files:**
- Various files for cleanup

- [ ] **Step 1: Run full Go test suite**

Run: `docker compose exec vaultflix-api go test ./...`
Expected: PASS — all tests pass.

- [ ] **Step 2: Build and verify frontend compiles**

Run: `docker compose exec vaultflix-web npm run build`
Expected: Build succeeds with no TypeScript errors.

- [ ] **Step 3: Restart containers and do a smoke test**

Run: `docker compose down && docker compose up -d --build`

Verify manually:
1. Login to the app
2. Navigate to a video detail page
3. Video should play (for Phase 9 imported videos)
4. Seeking (dragging progress bar) should work
5. Check browser dev tools Network tab — requests to `/api/videos/:id/stream` should return 200 (or 206 for range requests)

- [ ] **Step 4: Final commit if any cleanup needed**

Only if there are remaining uncommitted changes from fixups during verification.

```bash
git add -A
git commit -m "chore: Phase 10 final cleanup"
```
