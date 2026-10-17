package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/service"
)

func respond(t *testing.T, write func(c *gin.Context)) (int, model.ErrorResponse) {
	t.Helper()
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, "/", nil)
	write(c)
	var body model.ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("body %q: %v", w.Body.String(), err)
	}
	return w.Code, body
}

// The one table from domain errors to HTTP: the frontend (web/src/api/errors.ts
// ErrorCode) and the player (web/src/lib/hlsError.ts) depend on these.
func TestWriteError_MapsDomainErrors(t *testing.T) {
	tests := []struct {
		err        error
		wantStatus int
		wantCode   string
	}{
		{model.ErrNotFound, http.StatusNotFound, codeNotFound},
		{&model.NotFoundError{What: "tag"}, http.StatusNotFound, codeNotFound},
		{model.ErrNotRemux, http.StatusNotFound, codeNotFound},
		{model.ErrPathNotExist, http.StatusNotFound, codeFileNotFound},
		{model.ErrPathNotAllowed, http.StatusForbidden, codePathNotAllowed},
		{model.ErrInvalidInput, http.StatusBadRequest, codeBadRequest},
		{&model.InvalidParamError{Message: "page must be a positive integer"}, http.StatusBadRequest, codeBadRequest},
		{model.ErrAlreadyExists, http.StatusConflict, codeAlreadyExists},
		{service.ErrUsernameAlreadyExists, http.StatusConflict, codeUsernameTaken},
		{&service.JobLaneBusyError{RunningKind: service.JobKindBackfill}, http.StatusConflict, codeJobLaneBusy},
		{model.ErrConflict, http.StatusConflict, codeConflict},
		{model.ErrMediaSourceDisabled, http.StatusConflict, codeSourceDisabled},
		{model.ErrCannotDisableAdmin, http.StatusBadRequest, codeCannotDisableAdmin},
		{service.ErrInvalidCredentials, http.StatusUnauthorized, codeUnauthorized},
		{model.ErrAccountDisabled, http.StatusForbidden, codeForbidden},
		{model.ErrCodeNotFound, http.StatusUnprocessableEntity, codeNoCode},
		{model.ErrKeyframeIndexFailed, http.StatusUnprocessableEntity, codeKeyframeIndexFailed},
		{model.ErrStreamPreparing, http.StatusServiceUnavailable, codeStreamNotReady},
		{errors.New("db down"), http.StatusInternalServerError, codeInternal},
	}
	for _, tt := range tests {
		t.Run(tt.wantCode+" "+tt.err.Error(), func(t *testing.T) {
			status, body := respond(t, func(c *gin.Context) {
				writeError(c, fmt.Errorf("wrapped: %w", tt.err), "do the thing")
			})
			if status != tt.wantStatus || body.Error != tt.wantCode {
				t.Errorf("got %d %q, want %d %q", status, body.Error, tt.wantStatus, tt.wantCode)
			}
		})
	}
}

// Messages are for API callers: they name the operation, and never leak the
// wrapped chain of an unexpected error.
func TestWriteError_Messages(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"mapped error names the operation", model.ErrNotFound, "get user: not found"},
		{"a missing resource says which", &model.NotFoundError{What: "tag"}, "get user: tag not found"},
		{"an invalid parameter says which", &model.InvalidParamError{Message: "page must be a positive integer"}, "page must be a positive integer"},
		{"a busy lane names the Job holding it", &service.JobLaneBusyError{RunningKind: service.JobKindImport}, "匯入正在執行，完成或取消後再試"},
		{"an unexpected error hides its chain", errors.New("pq: connection refused at 10.0.0.5"), "get user failed"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, body := respond(t, func(c *gin.Context) { writeError(c, fmt.Errorf("wrapped: %w", tt.err), "get user") })
			if body.Message != tt.want {
				t.Errorf("message = %q, want %q", body.Message, tt.want)
			}
		})
	}
}

func TestBadRequest(t *testing.T) {
	status, body := respond(t, func(c *gin.Context) { badRequest(c, "source_id is required") })
	if status != http.StatusBadRequest || body.Error != codeBadRequest || body.Message != "source_id is required" {
		t.Errorf("got %d %+v", status, body)
	}
}
