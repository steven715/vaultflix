package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/mock"
	"github.com/steven/vaultflix/internal/model"
)

func serveBackfill(h *BackfillHandler, method, url, body string) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/api/admin/backfill-jobs", h.Start)
	r.GET("/api/admin/backfill-jobs/active", h.GetActive)
	r.POST("/api/admin/backfill-jobs/:id/cancel", h.Cancel)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(method, url, strings.NewReader(body)))
	return w
}

func TestBackfillStart_AcceptsKind(t *testing.T) {
	jobs := &mock.BackfillJobs{}
	w := serveBackfill(NewBackfillHandler(jobs), http.MethodPost, "/api/admin/backfill-jobs", `{"kind":"keyframe"}`)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want 202; body=%s", w.Code, w.Body.String())
	}
	if jobs.StartedKind != model.BackfillKeyframe {
		t.Errorf("started kind %q, want keyframe", jobs.StartedKind)
	}
	if !strings.Contains(w.Body.String(), `"job_id":"job-1"`) || !strings.Contains(w.Body.String(), `"kind":"keyframe"`) {
		t.Errorf("body = %s, want job_id and kind", w.Body.String())
	}
}

func TestBackfillStart_ErrorMapping(t *testing.T) {
	tests := []struct {
		name       string
		body       string
		startErr   error
		wantStatus int
	}{
		{"missing kind", `{}`, nil, http.StatusBadRequest},
		{"malformed body", `not json`, nil, http.StatusBadRequest},
		{"unknown kind", `{"kind":"thumbnail"}`, fmt.Errorf("x: %w", model.ErrInvalidInput), http.StatusBadRequest},
		{"another job running", `{"kind":"code"}`, model.ErrConflict, http.StatusConflict},
		{"unexpected failure", `{"kind":"code"}`, fmt.Errorf("boom"), http.StatusInternalServerError},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := serveBackfill(NewBackfillHandler(&mock.BackfillJobs{StartErr: tt.startErr}), http.MethodPost, "/api/admin/backfill-jobs", tt.body)
			if w.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d; body=%s", w.Code, tt.wantStatus, w.Body.String())
			}
		})
	}
}

func TestBackfillCancel_UnknownJob(t *testing.T) {
	h := NewBackfillHandler(&mock.BackfillJobs{CancelErr: model.ErrNotFound})
	if w := serveBackfill(h, http.MethodPost, "/api/admin/backfill-jobs/nope/cancel", ""); w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestBackfillGetActive_ReturnsLatestJob(t *testing.T) {
	h := NewBackfillHandler(&mock.BackfillJobs{Job: &model.BackfillJob{ID: "job-9", Kind: model.BackfillCodec, Status: "completed"}})
	w := serveBackfill(h, http.MethodGet, "/api/admin/backfill-jobs/active", "")
	if w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"kind":"codec"`) {
		t.Errorf("status = %d body = %s, want 200 with the codec job", w.Code, w.Body.String())
	}
}
