package market

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"time"
)

// NewsArticle represents an ingested market news story.
type NewsArticle struct {
	Title       string    `json:"title"`
	Summary     string    `json:"summary"`
	URL         string    `json:"url"`
	Source      string    `json:"source"`
	PublishedAt time.Time `json:"published_at"`
}

// NewsDeduplicator tracks seen news headlines to prevent repetitive AI signal triggers.
type NewsDeduplicator struct {
	mu     sync.RWMutex
	hashes map[string]time.Time
}

// NewNewsDeduplicator instantiates a thread-safe deduplicator.
func NewNewsDeduplicator() *NewsDeduplicator {
	return &NewsDeduplicator{
		hashes: make(map[string]time.Time),
	}
}

// NormalizeTitle prepares a headline for similarity hashing.
func NormalizeTitle(title string) string {
	lower := strings.ToLower(strings.TrimSpace(title))
	var sb strings.Builder
	for _, r := range lower {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

// IsNew returns true if the news article has not been seen before.
func (d *NewsDeduplicator) IsNew(article NewsArticle) bool {
	norm := NormalizeTitle(article.Title)
	if norm == "" {
		return false
	}

	h := sha256.Sum256([]byte(norm))
	key := hex.EncodeToString(h[:])

	d.mu.Lock()
	defer d.mu.Unlock()

	if _, exists := d.hashes[key]; exists {
		return false
	}

	d.hashes[key] = time.Now()

	// Periodic cleanup of entries older than 48 hours
	if len(d.hashes) > 5000 {
		cutoff := time.Now().Add(-48 * time.Hour)
		for k, t := range d.hashes {
			if t.Before(cutoff) {
				delete(d.hashes, k)
			}
		}
	}

	return true
}
