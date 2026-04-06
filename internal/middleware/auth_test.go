package middleware

import (
	"encoding/json"
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

func generateTestToken(t testing.TB, secret string, claims jwt.MapClaims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	if err != nil {
		t.Fatalf("failed to sign test token: %v", err)
	}
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
	validToken := generateTestToken(t, testJWTSecret, validClaims())
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
			name: "expired token from header",
			headerToken: generateTestToken(t, testJWTSecret, jwt.MapClaims{
				"user_id":  "u1",
				"username": "u",
				"role":     "viewer",
				"exp":      float64(time.Now().Add(-time.Hour).Unix()),
			}),
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
				t.Fatalf("expected status %d, got %d, body: %s",
					tt.expectedStatus, w.Code, w.Body.String())
			}

			if tt.expectedUserID != "" {
				var resp map[string]string
				if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
					t.Fatalf("failed to parse response: %v", err)
				}
				if resp["user_id"] != tt.expectedUserID {
					t.Errorf("expected user_id %s, got %s", tt.expectedUserID, resp["user_id"])
				}
			}
		})
	}
}
