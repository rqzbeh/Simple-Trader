package server

import (
	"fmt"
	"net/http"
	"sync"
)

// SSEBroadcaster manages active HTTP connections for real-time Server-Sent Events.
type SSEBroadcaster struct {
	mu      sync.RWMutex
	clients map[chan string]bool
}

// NewSSEBroadcaster creates a broadcaster.
func NewSSEBroadcaster() *SSEBroadcaster {
	return &SSEBroadcaster{
		clients: make(map[chan string]bool),
	}
}

// ServeHTTP handles incoming SSE client connections.
func (b *SSEBroadcaster) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	msgChan := make(chan string, 32)

	b.mu.Lock()
	b.clients[msgChan] = true
	b.mu.Unlock()

	defer func() {
		b.mu.Lock()
		delete(b.clients, msgChan)
		close(msgChan)
		b.mu.Unlock()
	}()

	// Send initial ping event
	fmt.Fprintf(w, "event: ping\ndata: connected\n\n")
	flusher.Flush()

	notify := r.Context().Done()
	for {
		select {
		case <-notify:
			return
		case msg, ok := <-msgChan:
			if !ok {
				return
			}
			fmt.Fprint(w, msg)
			flusher.Flush()
		}
	}
}

// Broadcast dispatches an event to all connected SSE clients.
func (b *SSEBroadcaster) Broadcast(event string, data string) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	msg := fmt.Sprintf("event: %s\ndata: %s\n\n", event, data)
	for client := range b.clients {
		select {
		case client <- msg:
		default:
			// Client channel full or slow, skip non-blocking
		}
	}
}
