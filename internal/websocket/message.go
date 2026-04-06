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
	// Server → client: generic notification.
	TypeNotification = "notification"
	// Client → server: heartbeat keepalive.
	TypePing = "ping"
)

// NotificationPayload is the payload for TypeNotification messages.
type NotificationPayload struct {
	Title   string `json:"title"`
	Message string `json:"message"`
	Level   string `json:"level"` // "info", "warn", "error"
}
