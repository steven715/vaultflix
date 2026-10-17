package handler

import (
	"errors"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/steven/vaultflix/internal/model"
	"github.com/steven/vaultflix/internal/service"
)

// Error codes of ErrorResponse. The frontend matches on them (keep
// web/src/api/errors.ts ErrorCode in step); messages are for API callers.
const (
	codeBadRequest          = "bad_request"
	codeUnauthorized        = "unauthorized"
	codeForbidden           = "forbidden"
	codeNotFound            = "not_found"
	codeFileNotFound        = "file_not_found"
	codePathNotAllowed      = "path_not_allowed"
	codePathNotExist        = "path_not_exist"
	codeAlreadyExists       = "already_exists"
	codeUsernameTaken       = "username_taken"
	codeConflict            = "conflict"
	codeJobLaneBusy         = "job_lane_busy"
	codeSourceDisabled      = "source_disabled"
	codeCannotDisableAdmin  = "cannot_disable_admin"
	codeNoCode              = "no_code"
	codeKeyframeIndexFailed = "keyframe_index_failed"
	codeStreamNotReady      = "stream_not_ready"
	codeInternal            = "internal_error"
)

// httpError is how one domain error is answered.
type httpError struct {
	status int
	code   string
	reason string
}

// httpErrorFor maps a domain error to its answer; ok is false for an error no
// rule knows, which is a 500. The more specific errors come first: a busy
// Job Lane is also an ErrConflict, a NotFoundError also an ErrNotFound.
func httpErrorFor(err error) (h httpError, ok bool) {
	var busy *service.JobLaneBusyError
	switch {
	case errors.As(err, &busy):
		return httpError{http.StatusConflict, codeJobLaneBusy, jobLaneBusyMessage(err)}, true
	case errors.Is(err, service.ErrUsernameAlreadyExists):
		return httpError{http.StatusConflict, codeUsernameTaken, "username already exists"}, true
	case errors.Is(err, model.ErrAlreadyExists):
		return httpError{http.StatusConflict, codeAlreadyExists, "already exists"}, true
	case errors.Is(err, model.ErrMediaSourceDisabled):
		// 409, not 503: the player polls on 503 only (web/src/lib/hlsError.ts).
		return httpError{http.StatusConflict, codeSourceDisabled, "media source is disabled"}, true
	case errors.Is(err, model.ErrConflict):
		return httpError{http.StatusConflict, codeConflict, "conflicts with the current state"}, true
	case errors.Is(err, model.ErrPathNotExist):
		return httpError{http.StatusNotFound, codeFileNotFound, "file not found"}, true
	case errors.Is(err, model.ErrNotFound), errors.Is(err, model.ErrNotRemux):
		return httpError{http.StatusNotFound, codeNotFound, "not found"}, true
	case errors.Is(err, model.ErrPathNotAllowed):
		return httpError{http.StatusForbidden, codePathNotAllowed, "path outside the allowed area"}, true
	case errors.Is(err, model.ErrInvalidInput):
		return httpError{http.StatusBadRequest, codeBadRequest, "invalid input"}, true
	case errors.Is(err, model.ErrCannotDisableAdmin):
		return httpError{http.StatusBadRequest, codeCannotDisableAdmin, "cannot disable an admin account"}, true
	}
	return authOrStreamError(err)
}

func authOrStreamError(err error) (httpError, bool) {
	switch {
	case errors.Is(err, service.ErrInvalidCredentials):
		return httpError{http.StatusUnauthorized, codeUnauthorized, "invalid username or password"}, true
	case errors.Is(err, model.ErrAccountDisabled):
		return httpError{http.StatusForbidden, codeForbidden, "account is disabled"}, true
	case errors.Is(err, model.ErrCodeNotFound):
		return httpError{http.StatusUnprocessableEntity, codeNoCode, "no code in filename"}, true
	case errors.Is(err, model.ErrKeyframeIndexFailed):
		// 422, not 503: the player must stop polling and say why.
		return httpError{http.StatusUnprocessableEntity, codeKeyframeIndexFailed, "the keyframe index could not be built"}, true
	case errors.Is(err, model.ErrStreamPreparing):
		// The only 503: the player polls on it (docs/streaming.md).
		return httpError{http.StatusServiceUnavailable, codeStreamNotReady, "preparing the stream, retry shortly"}, true
	}
	return httpError{}, false
}

// writeError answers err for the operation op (e.g. "get user"). A domain
// error gets its status and code, with a message naming op; an invalid
// parameter keeps its own message; a busy Job Lane names the Job holding it.
// Anything else is logged, with logAttrs (slog key-value pairs such as
// "video_id", id), and answered 500 without its error chain.
func writeError(c *gin.Context, err error, op string, logAttrs ...any) {
	h, ok := httpErrorFor(err)
	if !ok {
		slog.Error(op+" failed", append([]any{"error", err, "path", c.FullPath()}, logAttrs...)...)
		c.JSON(http.StatusInternalServerError, model.ErrorResponse{Error: codeInternal, Message: op + " failed"})
		return
	}
	message := op + ": " + h.reason
	var invalid *model.InvalidParamError
	var missing *model.NotFoundError
	switch {
	case errors.As(err, &invalid):
		message = invalid.Message
	case errors.As(err, &missing):
		message = op + ": " + missing.Error()
	case h.code == codeJobLaneBusy:
		message = h.reason
	}
	c.JSON(h.status, model.ErrorResponse{Error: h.code, Message: message})
}

// badRequest answers a request the handler itself found invalid (a missing
// field, a malformed parameter), with a message saying what is wrong.
func badRequest(c *gin.Context, message string) {
	c.JSON(http.StatusBadRequest, model.ErrorResponse{Error: codeBadRequest, Message: message})
}
