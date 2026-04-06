package websocket

import (
	"log/slog"
	"time"

	"github.com/gorilla/websocket"
)

const (
	// writeWait is the deadline for a write to the peer.
	writeWait = 10 * time.Second

	// pongWait is the deadline for reading the next pong from the peer.
	pongWait = 60 * time.Second

	// pingPeriod is the interval for sending pings. Must be less than pongWait.
	pingPeriod = (pongWait * 9) / 10

	// maxMessageSize is the maximum message size allowed from the peer.
	maxMessageSize = 4096

	// sendBufferSize is the buffer size of the client send channel.
	sendBufferSize = 256
)

// Client represents a single WebSocket connection.
type Client struct {
	hub    *Hub
	conn   *websocket.Conn
	userID string
	send   chan []byte
}

// NewClient creates a Client bound to the given Hub and connection.
func NewClient(hub *Hub, conn *websocket.Conn, userID string) *Client {
	return &Client{
		hub:    hub,
		conn:   conn,
		userID: userID,
		send:   make(chan []byte, sendBufferSize),
	}
}

// WritePump pumps messages from the send channel to the WebSocket connection.
// It also sends periodic ping frames to keep the connection alive.
//
// A single goroutine per connection must call this method; it serialises all
// writes to the underlying websocket.Conn.
func (c *Client) WritePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.conn.Close()
	}()

	for {
		select {
		case data, ok := <-c.send:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				// Hub closed the channel — send a close frame and exit.
				c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, data); err != nil {
				slog.Debug("websocket write failed",
					"error", err,
					"user_id", c.userID,
				)
				return
			}

		case <-ticker.C:
			c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				slog.Debug("websocket ping failed",
					"error", err,
					"user_id", c.userID,
				)
				return
			}
		}
	}
}

// ReadPump pumps messages from the WebSocket connection to the Hub.
// It sets read limits and deadlines, handles pong frames, and unregisters
// the client on any read error (disconnect, timeout, etc.).
//
// A single goroutine per connection must call this method.
func (c *Client) ReadPump() {
	defer func() {
		c.hub.unregister <- c
		c.conn.Close()
	}()

	c.conn.SetReadLimit(maxMessageSize)
	c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		c.conn.SetReadDeadline(time.Now().Add(pongWait))
		return nil
	})

	for {
		_, message, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err,
				websocket.CloseGoingAway,
				websocket.CloseNormalClosure,
			) {
				slog.Warn("websocket unexpected close",
					"error", err,
					"user_id", c.userID,
				)
			}
			return
		}
		slog.Debug("websocket message received",
			"user_id", c.userID,
			"size", len(message),
		)
	}
}
