package websocket

// Message is the unified envelope for all WebSocket messages.
// Type determines the Payload format; the frontend dispatches on Type.
type Message struct {
	Type    string      `json:"type"`
	Payload interface{} `json:"payload"`
}

// Predefined message types.
const (
	// Server → client: per-file progress during import.
	TypeImportProgress = "import_progress"
	// Server → client: import job completed.
	TypeImportComplete = "import_complete"
	// Server → client: import job fatal error.
	TypeImportError = "import_error"
	// Server → client: per-video progress during preview backfill.
	TypeBackfillProgress = "backfill_progress"
	// Server → client: backfill job completed (status reflects completed/failed/cancelled).
	TypeBackfillComplete = "backfill_complete"
	// Server → client: backfill job fatal error.
	TypeBackfillError = "backfill_error"
	// Server → client: generic notification.
	TypeNotification = "notification"
	// Client → server: heartbeat keepalive.
	TypePing = "ping"
	// Server → client: per-video progress during enrichment.
	TypeEnrichProgress = "enrich_progress"
	// Server → client: enrichment completed (suggestion staged).
	TypeEnrichComplete = "enrich_complete"
	// Server → client: enrichment fatal error (no code / all sources failed).
	TypeEnrichError = "enrich_error"
)

// NotificationPayload is the payload for TypeNotification messages.
type NotificationPayload struct {
	Title   string `json:"title"`
	Message string `json:"message"`
	Level   string `json:"level"` // "info", "warn", "error"
}
