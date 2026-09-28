package server

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/rqzbeh/simple-trader/internal/cache"
)

// SSEBroadcaster manages active HTTP connections for real-time Server-Sent Events.
// It streams events across multi-instance nodes via Redis Pub/Sub (Constitution Principle II).
type SSEBroadcaster struct {
	mu           sync.RWMutex
	clients      map[chan string]bool
	initPayloads func() []string
	redisClient  *cache.Client
	channel      string
}

// NewSSEBroadcaster creates a broadcaster.
func NewSSEBroadcaster() *SSEBroadcaster {
	return &SSEBroadcaster{
		clients: make(map[chan string]bool),
	}
}

// AttachRedis attaches a Redis client for distributed pub/sub SSE broadcasting.
func (b *SSEBroadcaster) AttachRedis(ctx context.Context, client *cache.Client, channel string) {
	b.mu.Lock()
	b.redisClient = client
	b.channel = channel
	b.mu.Unlock()

	if client == nil || client.Underlying() == nil {
		return
	}

	pubsub := client.Subscribe(ctx, channel)
	go func() {
		defer pubsub.Close()
		ch := pubsub.Channel()
		for msg := range ch {
			b.broadcastLocal(msg.Payload)
		}
	}()
}

// SetInitialPayloadProvider registers a callback returning initial SSE messages to send when a client connects.
func (b *SSEBroadcaster) SetInitialPayloadProvider(fn func() []string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.initPayloads = fn
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

	// Immediately push initial snapshots (e.g. all live cached asset ticks) so dashboard hydrates instantly with non-zero prices
	b.mu.RLock()
	provider := b.initPayloads
	b.mu.RUnlock()
	if provider != nil {
		for _, msg := range provider() {
			fmt.Fprint(w, msg)
		}
		flusher.Flush()
	}

	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	notify := r.Context().Done()
	for {
		select {
		case <-notify:
			return
		case <-heartbeat.C:
			fmt.Fprintf(w, "event: ping\ndata: keep-alive\n\n")
			flusher.Flush()
		case msg, ok := <-msgChan:
			if !ok {
				return
			}
			fmt.Fprint(w, msg)
			flusher.Flush()
		}
	}
}

// Broadcast dispatches an event to all connected SSE clients across nodes via Redis Pub/Sub.
func (b *SSEBroadcaster) Broadcast(event string, data string) {
	msg := fmt.Sprintf("event: %s\ndata: %s\n\n", event, data)

	b.mu.RLock()
	client := b.redisClient
	channel := b.channel
	b.mu.RUnlock()

	if client != nil && client.Underlying() != nil && channel != "" {
		if err := client.Publish(context.Background(), channel, msg); err == nil {
			return
		}
	}

	// Deliver locally if Redis unavailable or detached
	b.broadcastLocal(msg)
}

func (b *SSEBroadcaster) broadcastLocal(rawMsg string) {
	b.mu.RLock()
	defer b.mu.RUnlock()

	for client := range b.clients {
		select {
		case client <- rawMsg:
		default:
			// Client channel full or slow, skip non-blocking
		}
	}
}
