package server

import (
	"context"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/rqzbeh/simple-trader/internal/db"
	"github.com/rqzbeh/simple-trader/internal/market"
)

// NewsEventDebounce is how long a symbol rests after an evaluation before a
// new news event may trigger it again (spec-020 US-A1).
const NewsEventDebounce = 60 * time.Second

// ScanIntervalMinutes parses the OPTIONAL backstop scan interval
// (spec-020 US-A2): empty ⇒ 0 = event-driven only; positive integer minutes;
// anything else is an explicit boot error (no silent default).
func ScanIntervalMinutes(raw string) (time.Duration, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("SCAN_INTERVAL_MINUTES invalid: %q (positive integer minutes, or empty for event-driven only)", raw)
	}
	return time.Duration(n) * time.Minute, nil
}

// lockSymbol guards per-symbol evaluations against overlap (spec-020 US-A3):
// returns an unlock func, or nil when the symbol is already being evaluated.
func (s *Server) lockSymbol(sym string) func() {
	if _, loaded := s.evalBusy.LoadOrStore(sym, struct{}{}); loaded {
		return nil
	}
	return func() { s.evalBusy.Delete(sym) }
}

func (s *Server) markEvaluated(sym string) {
	s.lastEval.Store(sym, time.Now())
}

// debounced reports whether a fresh evaluation happened inside the window.
func (s *Server) debounced(sym string, now time.Time, window time.Duration) bool {
	if v, ok := s.lastEval.Load(sym); ok {
		if t, is := v.(time.Time); is && now.Sub(t) < window {
			return true
		}
	}
	return false
}

// EnqueueNewsArticle reacts to a freshly ingested headline (spec-020 US-A1):
// every universe symbol the headline is relevant to is queued for an
// immediate evaluation. Queue overflow is logged explicitly, never silent.
func (s *Server) EnqueueNewsArticle(article *db.NewsArticle) {
	if s == nil || article == nil || s.newsQueue == nil {
		return
	}
	universe := s.scanUniverse()
	if len(universe) == 0 {
		return
	}
	for _, sym := range universe {
		if len(market.HeadlinesForSymbol([]string{article.Title}, sym)) == 0 {
			continue
		}
		// Dedupe BEFORE enqueue (spec-020): a market-wide headline hits every
		// symbol at once — queueing the same symbol twice only fills the queue
		// with duplicates and evicts real events. Pending = already queued.
		if _, loaded := s.newsPending.LoadOrStore(sym, struct{}{}); loaded {
			continue
		}
		select {
		case s.newsQueue <- sym:
			log.Printf("[NewsEvent] queued %s for evaluation (%.60s)", sym, article.Title)
		default:
			s.newsPending.Delete(sym)
			log.Printf("[NewsEvent] queue full — dropped %s (explicit)", sym)
		}
	}
}

// StartNewsDrivenScanner consumes the news queue: debounce → overlap guard →
// single-symbol evaluation through the SAME pipeline as the periodic scan.
func (s *Server) StartNewsDrivenScanner(ctx context.Context) {
	if s == nil {
		return
	}
	if s.newsQueue == nil {
		s.newsQueue = make(chan string, 512)
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("[ERROR] news-driven scanner recovered from panic: %v", r)
			}
		}()
		for {
			select {
			case <-ctx.Done():
				return
			case sym := <-s.newsQueue:
				s.newsPending.Delete(sym) // dequeued — eligible for future events
				func() {
					defer func() {
						if r := recover(); r != nil {
							log.Printf("[ERROR] evalSymbolFromNews %s recovered from panic: %v", sym, r)
						}
					}()
					s.evalSymbolFromNews(ctx, sym)
				}()
			}
		}
	}()
}

// evalSymbolFromNews runs one news-triggered evaluation with the guards.
func (s *Server) evalSymbolFromNews(ctx context.Context, sym string) {
	if s.aiClient == nil {
		return
	}
	if s.debounced(sym, time.Now(), NewsEventDebounce) {
		log.Printf("[NewsEvent] %s debounced (evaluated < %s ago)", sym, NewsEventDebounce)
		return
	}
	unlock := s.lockSymbol(sym)
	if unlock == nil {
		log.Printf("[NewsEvent] %s already evaluating — skip (overlap guard)", sym)
		return
	}
	defer unlock()

	// Active signal ⇒ nothing to decide for entry (same rule as the scan).
	if s.dbStore != nil {
		if existing, err := s.dbStore.GetActiveFuturesSignalBySymbol(ctx, sym); err == nil && existing != nil {
			return
		}
	}
	var headlines []string
	if s.newsCrawler != nil {
		latest := s.newsCrawler.GetLatestArticles()
		for i := 0; i < len(latest) && i < 15; i++ {
			headlines = append(headlines, latest[i].Title)
		}
	}
	s.markEvaluated(sym)
	_, holdReason, err := s.EvaluateSymbolSignal(ctx, sym, headlines)
	if err != nil {
		log.Printf("[NewsEvent] %s error: %v", sym, err)
	} else if holdReason != "" {
		log.Printf("[NewsEvent] %s HOLD: %s", sym, holdReason)
	} else {
		log.Printf("[NewsEvent] %s evaluated (signal created or active)", sym)
	}
}
