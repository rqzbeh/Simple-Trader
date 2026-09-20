package server_test

import (
	"bufio"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rqzbeh/simple-trader/internal/config"
	"github.com/rqzbeh/simple-trader/internal/server"
)

func TestHealthAndAssetsEndpoints(t *testing.T) {
	cfg := &config.Config{
		Port:         "8080",
		AIModelID:    "gpt-4o-mini",
		IsProduction: false,
	}

	srv := server.NewServer(cfg, nil, nil, nil, nil, nil)
	ts := httptest.NewServer(srv.Router())
	defer ts.Close()

	// 1. Health endpoint
	resp, err := http.Get(ts.URL + "/health")
	if err != nil {
		t.Fatalf("failed calling /health: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	// 2. Assets endpoint
	respAssets, err := http.Get(ts.URL + "/api/v1/assets")
	if err != nil {
		t.Fatalf("failed calling /api/v1/assets: %v", err)
	}
	if respAssets.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", respAssets.StatusCode)
	}
}

func TestSSEEventBroadcaster(t *testing.T) {
	broadcaster := server.NewSSEBroadcaster()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		broadcaster.ServeHTTP(w, r)
	}))
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("failed connecting to SSE: %v", err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Errorf("expected text/event-stream content type, got %s", resp.Header.Get("Content-Type"))
	}

	// Broadcast test event in background
	go func() {
		time.Sleep(50 * time.Millisecond)
		broadcaster.Broadcast("ticker", `{"symbol":"BTC/USD","price":68500.0}`)
	}()

	reader := bufio.NewReader(resp.Body)
	receivedEvent := false

	done := make(chan bool)
	go func() {
		for {
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			if strings.Contains(line, "BTC/USD") {
				receivedEvent = true
				done <- true
				return
			}
		}
	}()

	select {
	case <-done:
		if !receivedEvent {
			t.Errorf("expected to receive SSE ticker payload")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for SSE message")
	}
}
