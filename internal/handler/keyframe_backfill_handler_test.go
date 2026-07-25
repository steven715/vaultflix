package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

type fakeKeyframeBackfiller struct {
	processed, failed int
	err               error
}

func (f *fakeKeyframeBackfiller) RunBackfill(ctx context.Context) (int, int, error) {
	return f.processed, f.failed, f.err
}

func TestKeyframeBackfillRun_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewKeyframeBackfillHandler(&fakeKeyframeBackfiller{processed: 3, failed: 1})
	r := gin.New()
	r.POST("/admin/videos/backfill-keyframes", h.Run)

	req := httptest.NewRequest(http.MethodPost, "/admin/videos/backfill-keyframes", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"processed":3`) || !strings.Contains(w.Body.String(), `"failed":1`) {
		t.Errorf("body = %s", w.Body.String())
	}
}

func TestKeyframeBackfillRun_Error(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewKeyframeBackfillHandler(&fakeKeyframeBackfiller{err: errors.New("db down")})
	r := gin.New()
	r.POST("/admin/videos/backfill-keyframes", h.Run)

	req := httptest.NewRequest(http.MethodPost, "/admin/videos/backfill-keyframes", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", w.Code)
	}
}
