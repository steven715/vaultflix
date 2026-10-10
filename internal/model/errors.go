package model

import "errors"

var (
	ErrNotFound           = errors.New("resource not found")
	ErrAlreadyExists      = errors.New("resource already exists")
	ErrConflict           = errors.New("resource conflict")
	ErrAccountDisabled    = errors.New("account is disabled")
	ErrCannotDisableAdmin = errors.New("cannot disable admin account")
	ErrPathNotAllowed     = errors.New("path is not within allowed mount prefix")
	ErrPathNotExist       = errors.New("path does not exist on filesystem")
	ErrInvalidInput       = errors.New("invalid input")
)

// ErrStreamPreparing 表示 Video 的 Keyframe Index 尚未就緒、探測已在背景進行，
// 稍後重試即可播放。
var ErrStreamPreparing = errors.New("stream is preparing")

// ErrNotRemux 表示 Video 的 Play Mode 不是 remux，不經 HLS 播放。
var ErrNotRemux = errors.New("video play mode is not remux")

// ErrMediaSourceDisabled 表示 Video 所屬的 Media Source 已停用，不可讀取其檔案。
var ErrMediaSourceDisabled = errors.New("media source is disabled")

// ErrCodeNotFound 表示來源站找不到該番號的頁面。
var ErrCodeNotFound = errors.New("code not found at source")

// ErrScrapeBlocked 表示被 Cloudflare / JS challenge 擋下。
var ErrScrapeBlocked = errors.New("scrape blocked by challenge")

// ErrSourceUnavailable 表示來源站連線失敗 / 非預期狀態。
var ErrSourceUnavailable = errors.New("scrape source unavailable")
