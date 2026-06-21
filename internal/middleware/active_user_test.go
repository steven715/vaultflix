package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
)

// setupActiveUserRouter wires a route behind RequireActiveUser, with a preceding
// middleware that injects userID into the context (as JWTAuth would).
func setupActiveUserRouter(userID string, checker UserStatusChecker) *gin.Engine {
	r := gin.New()
	r.Use(func(c *gin.Context) {
		if userID != "" {
			c.Set("user_id", userID)
		}
		c.Next()
	})
	r.Use(RequireActiveUser(checker))
	r.GET("/protected", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return r
}

func TestRequireActiveUser_ActivePasses(t *testing.T) {
	checker := &mock.UserRepository{
		GetByIDFunc: func(ctx context.Context, id string) (*model.User, error) {
			return &model.User{ID: id, Username: "u", Role: "viewer", DisabledAt: nil}, nil
		},
	}
	r := setupActiveUserRouter("user-1", checker)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/protected", nil))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestRequireActiveUser_DisabledRejected(t *testing.T) {
	now := time.Now()
	checker := &mock.UserRepository{
		GetByIDFunc: func(ctx context.Context, id string) (*model.User, error) {
			return &model.User{ID: id, Username: "u", Role: "viewer", DisabledAt: &now}, nil
		},
	}
	r := setupActiveUserRouter("user-1", checker)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/protected", nil))

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for disabled user, got %d", w.Code)
	}
}

func TestRequireActiveUser_DeletedRejected(t *testing.T) {
	checker := &mock.UserRepository{
		GetByIDFunc: func(ctx context.Context, id string) (*model.User, error) {
			return nil, model.ErrNotFound
		},
	}
	r := setupActiveUserRouter("user-1", checker)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/protected", nil))

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 for deleted user, got %d", w.Code)
	}
}

func TestRequireActiveUser_MissingSubjectRejected(t *testing.T) {
	checker := &mock.UserRepository{
		GetByIDFunc: func(ctx context.Context, id string) (*model.User, error) {
			t.Fatal("checker should not be called when user_id is missing")
			return nil, nil
		},
	}
	r := setupActiveUserRouter("", checker)

	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/protected", nil))

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 when user_id missing, got %d", w.Code)
	}
}
