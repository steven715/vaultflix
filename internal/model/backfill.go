package model

import "time"

// BackfillKind 是 Backfill 的種類（見 CONTEXT.md「Backfill」）。
type BackfillKind string

const (
	BackfillPreview  BackfillKind = "preview"  // 補 Preview
	BackfillCodec    BackfillKind = "codec"    // 補 Media Info 的 video/audio codec
	BackfillKeyframe BackfillKind = "keyframe" // 補 remux Video 的 Keyframe Index
	BackfillCode     BackfillKind = "code"     // 由檔名補 Code
)

// BackfillJob 是一次 Backfill 的執行狀態（in-memory，不持久化，ADR-0003）。
// 與 ImportJob 並列、語意各自獨立：Import 登錄新檔案，Backfill 對既有
// Video 補算衍生資料。同一時間全域只有一個 BackfillJob 在跑。
type BackfillJob struct {
	ID             string          `json:"id"`
	Kind           BackfillKind    `json:"kind"`
	Status         string          `json:"status"` // running | completed | failed | cancelled
	Total          int             `json:"total"`
	Processed      int             `json:"processed"`
	Succeeded      int             `json:"succeeded"`
	Failed         int             `json:"failed"`
	Skipped        int             `json:"skipped"` // Media Source 停用而略過
	CurrentVideoID string          `json:"current_video_id,omitempty"`
	Errors         []BackfillError `json:"errors"`
	StartedAt      time.Time       `json:"started_at"`
	FinishedAt     *time.Time      `json:"finished_at,omitempty"`
}

// BackfillError 記錄單一 Video 在 Backfill 中失敗的資訊。
type BackfillError struct {
	VideoID          string `json:"video_id"`
	OriginalFilename string `json:"original_filename"`
	Error            string `json:"error"`
}

// BackfillProgress 是透過 WebSocket 推送的逐影片進度訊息。
type BackfillProgress struct {
	JobID            string       `json:"job_id"`
	Kind             BackfillKind `json:"kind"`
	VideoID          string       `json:"video_id"`
	OriginalFilename string       `json:"original_filename"`
	Current          int          `json:"current"`
	Total            int          `json:"total"`
	Status           string       `json:"status"` // processing | success | skipped | error
	Error            string       `json:"error,omitempty"`
}
