package mock

import (
	"sync"

	"github.com/steven/vaultflix/internal/websocket"
)

// Notifier is a thread-safe mock for websocket.Notifier.
type Notifier struct {
	mu       sync.Mutex
	Messages []websocket.Message
}

func (m *Notifier) SendToUser(userID string, msg *websocket.Message) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Messages = append(m.Messages, *msg)
}

func (m *Notifier) Broadcast(msg *websocket.Message) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.Messages = append(m.Messages, *msg)
}

// GetMessages returns a snapshot of all captured messages.
func (m *Notifier) GetMessages() []websocket.Message {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := make([]websocket.Message, len(m.Messages))
	copy(cp, m.Messages)
	return cp
}
