package model

import "time"

// ImportJob 代表一次匯入作業的狀態（in-memory，不持久化）。
type ImportJob struct {
	ID          string        `json:"id"`
	SourceID    string        `json:"source_id"`
	SourceLabel string        `json:"source_label"`
	Status      string        `json:"status"`
	Total       int           `json:"total"`
	Processed   int           `json:"processed"`
	Imported    int           `json:"imported"`
	Skipped     int           `json:"skipped"`
	Failed      int           `json:"failed"`
	Errors      []ImportError `json:"errors"`
	StartedAt   time.Time     `json:"started_at"`
	FinishedAt  *time.Time    `json:"finished_at,omitempty"`
}

// ImportError 記錄單一檔案的匯入失敗資訊。
type ImportError struct {
	FileName string `json:"file_name"`
	Error    string `json:"error"`
}

// ImportProgress 是透過 WebSocket 推送的逐檔進度訊息。
type ImportProgress struct {
	JobID    string `json:"job_id"`
	FileName string `json:"file_name"`
	Current  int    `json:"current"`
	Total    int    `json:"total"`
	Status   string `json:"status"`
	Error    string `json:"error,omitempty"`
}
