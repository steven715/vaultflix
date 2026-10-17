package model

// Error codes of ErrorResponse. The frontend matches on them (keep
// web/src/api/errors.ts ErrorCode in step: a test in internal/handler checks it);
// messages are for API callers.
const (
	CodeBadRequest          = "bad_request"
	CodeUnauthorized        = "unauthorized"
	CodeForbidden           = "forbidden"
	CodeNotFound            = "not_found"
	CodeFileNotFound        = "file_not_found"
	CodePathNotAllowed      = "path_not_allowed"
	CodePathNotExist        = "path_not_exist"
	CodeAlreadyExists       = "already_exists"
	CodeUsernameTaken       = "username_taken"
	CodeConflict            = "conflict"
	CodeJobLaneBusy         = "job_lane_busy"
	CodeSourceDisabled      = "source_disabled"
	CodeCannotDisableAdmin  = "cannot_disable_admin"
	CodeNoCode              = "no_code"
	CodeKeyframeIndexFailed = "keyframe_index_failed"
	CodeStreamNotReady      = "stream_not_ready"
	CodeInternal            = "internal_error"
)
