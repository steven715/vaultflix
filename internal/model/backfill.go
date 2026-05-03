package model

import "time"

// BackfillJob 代表一次 preview 補生成作業的狀態（in-memory，不持久化）。
// 與 ImportJob 並列、語意各自獨立：import 處理新檔案，backfill 處理既有
// 影片的缺漏 preview。
type BackfillJob struct {
	ID             string          `json:"id"`
	Status         string          `json:"status"` // running | completed | failed | cancelled
	Total          int             `json:"total"`
	Processed      int             `json:"processed"`
	Succeeded      int             `json:"succeeded"`
	Failed         int             `json:"failed"`
	CurrentVideoID string          `json:"current_video_id,omitempty"`
	Errors         []BackfillError `json:"errors"`
	StartedAt      time.Time       `json:"started_at"`
	FinishedAt     *time.Time      `json:"finished_at,omitempty"`
}

// BackfillError 記錄單一影片補 preview 失敗資訊。
type BackfillError struct {
	VideoID          string `json:"video_id"`
	OriginalFilename string `json:"original_filename"`
	Error            string `json:"error"`
}

// BackfillProgress 是透過 WebSocket 推送的逐影片進度訊息。
type BackfillProgress struct {
	JobID            string `json:"job_id"`
	VideoID          string `json:"video_id"`
	OriginalFilename string `json:"original_filename"`
	Current          int    `json:"current"`
	Total            int    `json:"total"`
	Status           string `json:"status"` // processing | success | error
	Error            string `json:"error,omitempty"`
}
