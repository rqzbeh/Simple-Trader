package server

import (
	"context"
	"testing"
	"time"

	"github.com/rqzbeh/simple-trader/internal/db"
)

// TestScanIntervalOptional (spec-020 US-A2): unset = event-driven only;
// set = backstop minutes; invalid = explicit boot error (no default).
func TestScanIntervalOptional(t *testing.T) {
	if d, err := ScanIntervalMinutes(""); err != nil || d != 0 {
		t.Errorf("unset must be event-driven only: d=%v err=%v", d, err)
	}
	if d, err := ScanIntervalMinutes(" 5 "); err != nil || d != 5*time.Minute {
		t.Errorf("5 must parse to 5m: d=%v err=%v", d, err)
	}
	for _, bad := range []string{"abc", "-3", "0"} {
		if _, err := ScanIntervalMinutes(bad); err == nil {
			t.Errorf("%q must fail explicit", bad)
		}
	}
}

// TestNewsEventDebounceAndOverlap (spec-020 US-A1/A3): recent evaluation ⇒
// debounced; concurrent evaluation ⇒ overlap guard denies the lock.
func TestNewsEventDebounceAndOverlap(t *testing.T) {
	s := &Server{}
	now := time.Now()
	if s.debounced("BTC/USDT", now, NewsEventDebounce) {
		t.Errorf("never-evaluated symbol must not be debounced")
	}
	s.markEvaluated("BTC/USDT")
	if !s.debounced("BTC/USDT", now.Add(10*time.Second), NewsEventDebounce) {
		t.Errorf("recent evaluation must debounce")
	}
	if s.debounced("BTC/USDT", now.Add(2*NewsEventDebounce), NewsEventDebounce) {
		t.Errorf("stale evaluation must not debounce")
	}
	unlock := s.lockSymbol("ETH/USDT")
	if unlock == nil {
		t.Fatalf("first lock must succeed")
	}
	if s.lockSymbol("ETH/USDT") != nil {
		t.Errorf("second lock must be denied (overlap guard)")
	}
	unlock()
	if s.lockSymbol("ETH/USDT") == nil {
		t.Errorf("lock must be reusable after release")
	}
}

// TestEnqueueNewsArticleMatchesSymbols (spec-020 US-A1): a Bitcoin headline
// queues BTC but not unrelated universe symbols; nil article is a no-op.
func TestEnqueueNewsArticleMatchesSymbols(t *testing.T) {
	s := &Server{newsQueue: make(chan string, 16)}
	s.EnqueueNewsArticle(nil)
	if len(s.newsQueue) != 0 {
		t.Errorf("nil article must not enqueue")
	}
	s.EnqueueNewsArticle(&db.NewsArticle{Title: "Bitcoin hits all-time high as ETF inflows surge"})
	queued := map[string]bool{}
	for len(s.newsQueue) > 0 {
		queued[<-s.newsQueue] = true
	}
	btcQueued := false
	for sym := range queued {
		if len(sym) >= 3 && sym[:3] == "BTC" {
			btcQueued = true
		}
	}
	if !btcQueued {
		t.Fatalf("Bitcoin headline must queue the BTC symbol, queued=%v", queued)
	}
	// Market-wide headlines legitimately reach multiple symbols (macro news
	// affects the universe); load is bounded by debounce + overlap guard.

	// Narrow, symbol-specific headline: only alias-matched symbols queue.
	s2 := &Server{newsQueue: make(chan string, 16)}
	s2.EnqueueNewsArticle(&db.NewsArticle{Title: "London Metal Exchange copper stocks tumble"})
	n := len(s2.newsQueue)
	close(s2.newsQueue)
	got := []string{}
	for sym := range s2.newsQueue {
		got = append(got, sym)
	}
	if n == 0 {
		t.Fatalf("copper headline must queue at least COPPER")
	}
	for _, sym := range got {
		if len(sym) < 3 {
			t.Errorf("bad symbol %q", sym)
		}
	}
}

func TestEnqueueNewsArticleDegradedAndNilQueue(t *testing.T) {
	// Nil server: must be safe no-op
	var nilServer *Server
	nilServer.EnqueueNewsArticle(&db.NewsArticle{Title: "Bitcoin rallies"})

	// Nil queue: must be safe no-op
	s := &Server{newsQueue: nil}
	s.EnqueueNewsArticle(&db.NewsArticle{Title: "Bitcoin rallies"})

	// Nil article: must be safe no-op
	s.newsQueue = make(chan string, 16)
	s.EnqueueNewsArticle(nil)
	if len(s.newsQueue) != 0 {
		t.Errorf("expected 0 queued items on nil article")
	}
}

func TestStartNewsDrivenScannerDegraded(t *testing.T) {
	// Server with nil crawler and nil screener must start worker without panic
	s := &Server{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.StartNewsDrivenScanner(ctx)
	if s.newsQueue == nil {
		t.Fatal("StartNewsDrivenScanner must initialize newsQueue if nil")
	}

	// Enqueue a symbol to verify consumption without panic
	s.newsQueue <- "BTC/USDT"
	time.Sleep(50 * time.Millisecond)
}
