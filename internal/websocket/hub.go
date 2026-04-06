package websocket

import (
	"context"
	"encoding/json"
	"log/slog"
)

// Notifier is the interface that services use to push messages to connected
// clients. Depend on this interface rather than concrete *Hub to keep the
// service layer decoupled and testable.
type Notifier interface {
	// SendToUser pushes msg to every active connection for the given userID.
	// If the user is not online the call is silently ignored.
	SendToUser(userID string, msg *Message)

	// Broadcast pushes msg to every connected client.
	Broadcast(msg *Message)
}

// targetedMessage carries data destined for a specific user.
type targetedMessage struct {
	userID string
	data   []byte
}

// Hub is the central registry of active WebSocket connections.
// All map mutations are serialised through a single goroutine (Run) using
// channels, following the gorilla/websocket chat-example pattern.
type Hub struct {
	// clients maps userID → slice of *Client (one user may have many tabs).
	clients map[string][]*Client

	register   chan *Client
	unregister chan *Client
	broadcast  chan []byte
	targeted   chan targetedMessage
}

// NewHub creates a Hub instance. Call go hub.Run(ctx) to start the event loop.
func NewHub() *Hub {
	return &Hub{
		clients:    make(map[string][]*Client),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		broadcast:  make(chan []byte, 256),
		targeted:   make(chan targetedMessage, 256),
	}
}

// Run starts the Hub event loop. It blocks until ctx is cancelled.
func (h *Hub) Run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			slog.Info("websocket hub shutting down")
			return

		case client := <-h.register:
			h.clients[client.userID] = append(h.clients[client.userID], client)
			slog.Info("websocket client registered",
				"user_id", client.userID,
				"connections", len(h.clients[client.userID]),
			)

		case client := <-h.unregister:
			h.removeClient(client)

		case msg := <-h.targeted:
			h.deliverToUser(msg.userID, msg.data)

		case data := <-h.broadcast:
			for userID, clients := range h.clients {
				h.deliverToClients(userID, clients, data)
			}
		}
	}
}

// Register queues a client for registration. Called from the HTTP handler
// goroutine after a successful WebSocket upgrade.
func (h *Hub) Register(client *Client) {
	h.register <- client
}

// SendToUser pushes msg to all connections belonging to userID.
func (h *Hub) SendToUser(userID string, msg *Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		slog.Error("failed to marshal websocket message",
			"error", err,
			"user_id", userID,
		)
		return
	}
	h.targeted <- targetedMessage{userID: userID, data: data}
}

// Broadcast pushes msg to all connected clients.
func (h *Hub) Broadcast(msg *Message) {
	data, err := json.Marshal(msg)
	if err != nil {
		slog.Error("failed to marshal websocket broadcast message", "error", err)
		return
	}
	h.broadcast <- data
}

// deliverToUser sends data to all clients of a given user, dropping clients
// whose send buffer is full.
func (h *Hub) deliverToUser(userID string, data []byte) {
	clients := h.clients[userID]
	if len(clients) == 0 {
		return
	}
	h.deliverToClients(userID, clients, data)
}

// deliverToClients attempts to write data to each client's send channel.
// Clients with full buffers are removed.
func (h *Hub) deliverToClients(userID string, clients []*Client, data []byte) {
	for _, c := range clients {
		select {
		case c.send <- data:
		default:
			slog.Warn("websocket send buffer full, dropping client",
				"user_id", userID,
			)
			h.removeClient(c)
		}
	}
}

// removeClient removes a single client from the registry and closes its
// send channel. Safe to call if client is already removed.
func (h *Hub) removeClient(client *Client) {
	clients := h.clients[client.userID]
	for i, c := range clients {
		if c == client {
			h.clients[client.userID] = append(clients[:i], clients[i+1:]...)
			close(client.send)
			slog.Info("websocket client unregistered",
				"user_id", client.userID,
				"remaining", len(h.clients[client.userID]),
			)
			if len(h.clients[client.userID]) == 0 {
				delete(h.clients, client.userID)
			}
			return
		}
	}
}
