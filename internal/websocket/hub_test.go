package websocket

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"
)

// fakeConn satisfies the minimal interface needed by Client for hub-level tests.
// We don't actually read/write WebSocket frames here — we only test the Hub's
// channel-based routing and map management via the client's send channel.

func newTestClient(hub *Hub, userID string) *Client {
	return &Client{
		hub:    hub,
		conn:   nil, // not used in hub-level tests
		userID: userID,
		send:   make(chan []byte, sendBufferSize),
	}
}

func startHub(t *testing.T) (*Hub, context.CancelFunc) {
	t.Helper()
	hub := NewHub()
	ctx, cancel := context.WithCancel(context.Background())
	go hub.Run(ctx)
	return hub, cancel
}

func TestHub_RegisterAndSend(t *testing.T) {
	hub, cancel := startHub(t)
	defer cancel()

	client := newTestClient(hub, "user1")
	hub.Register(client)

	// Allow register to be processed
	time.Sleep(20 * time.Millisecond)

	msg := &Message{Type: TypeNotification, Payload: "hello"}
	hub.SendToUser("user1", msg)

	select {
	case data := <-client.send:
		var got Message
		if err := json.Unmarshal(data, &got); err != nil {
			t.Fatalf("failed to unmarshal: %v", err)
		}
		if got.Type != TypeNotification {
			t.Errorf("expected type %q, got %q", TypeNotification, got.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for message")
	}
}

func TestHub_SendToUser_MultipleConnections(t *testing.T) {
	hub, cancel := startHub(t)
	defer cancel()

	c1 := newTestClient(hub, "user1")
	c2 := newTestClient(hub, "user1")
	hub.Register(c1)
	hub.Register(c2)
	time.Sleep(20 * time.Millisecond)

	msg := &Message{Type: TypeNotification, Payload: "multi"}
	hub.SendToUser("user1", msg)

	for _, c := range []*Client{c1, c2} {
		select {
		case data := <-c.send:
			var got Message
			if err := json.Unmarshal(data, &got); err != nil {
				t.Fatalf("failed to unmarshal: %v", err)
			}
			if got.Type != TypeNotification {
				t.Errorf("expected type %q, got %q", TypeNotification, got.Type)
			}
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for message on one of the clients")
		}
	}
}

func TestHub_SendToUser_UserNotOnline(t *testing.T) {
	hub, cancel := startHub(t)
	defer cancel()

	// Should not panic or block
	msg := &Message{Type: TypeNotification, Payload: "nobody"}
	hub.SendToUser("nonexistent", msg)

	// Give time for the message to be processed
	time.Sleep(20 * time.Millisecond)
}

func TestHub_Unregister(t *testing.T) {
	hub, cancel := startHub(t)
	defer cancel()

	client := newTestClient(hub, "user1")
	hub.Register(client)
	time.Sleep(20 * time.Millisecond)

	hub.unregister <- client
	time.Sleep(20 * time.Millisecond)

	// send channel should be closed
	_, ok := <-client.send
	if ok {
		t.Error("expected send channel to be closed")
	}

	// Sending to this user should be a no-op (no panic)
	msg := &Message{Type: TypeNotification, Payload: "after-unreg"}
	hub.SendToUser("user1", msg)
	time.Sleep(20 * time.Millisecond)
}

func TestHub_Broadcast(t *testing.T) {
	hub, cancel := startHub(t)
	defer cancel()

	c1 := newTestClient(hub, "user1")
	c2 := newTestClient(hub, "user2")
	hub.Register(c1)
	hub.Register(c2)
	time.Sleep(20 * time.Millisecond)

	msg := &Message{Type: TypeNotification, Payload: "broadcast"}
	hub.Broadcast(msg)

	for _, c := range []*Client{c1, c2} {
		select {
		case <-c.send:
			// ok
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for broadcast")
		}
	}
}

func TestHub_ConcurrentAccess(t *testing.T) {
	hub, cancel := startHub(t)
	defer cancel()

	var wg sync.WaitGroup
	// Spawn goroutines that register, send, and unregister concurrently
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			userID := "user"
			if id%2 == 0 {
				userID = "userA"
			} else {
				userID = "userB"
			}
			c := newTestClient(hub, userID)
			hub.Register(c)
			time.Sleep(5 * time.Millisecond)

			msg := &Message{Type: TypeNotification, Payload: "concurrent"}
			hub.SendToUser(userID, msg)
			hub.Broadcast(msg)

			time.Sleep(5 * time.Millisecond)
			hub.unregister <- c
		}(i)
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// all goroutines completed without panic or deadlock
	case <-time.After(5 * time.Second):
		t.Fatal("timed out — possible deadlock in concurrent access test")
	}
}
