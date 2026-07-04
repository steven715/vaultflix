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

// ErrCodeNotFound 表示來源站找不到該番號的頁面。
var ErrCodeNotFound = errors.New("code not found at source")

// ErrScrapeBlocked 表示被 Cloudflare / JS challenge 擋下。
var ErrScrapeBlocked = errors.New("scrape blocked by challenge")

// ErrSourceUnavailable 表示來源站連線失敗 / 非預期狀態。
var ErrSourceUnavailable = errors.New("scrape source unavailable")
