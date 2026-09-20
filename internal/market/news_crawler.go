package market

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/rqzbeh/simple-trader/internal/cache"
	"github.com/rqzbeh/simple-trader/internal/db"
)

// RSSChannel and RSSItem represent basic RSS 2.0 XML structure.
type RSSItem struct {
	Title       string `xml:"title"`
	Link        string `xml:"link"`
	Description string `xml:"description"`
	PubDate     string `xml:"pubDate"`
}

type RSSFeed struct {
	XMLName xml.Name  `xml:"rss"`
	Channel struct {
		Title string    `xml:"title"`
		Items []RSSItem `xml:"item"`
	} `xml:"channel"`
}

// NewsFeedConfig contains configuration for the automated news crawler.
type NewsFeedConfig struct {
	PollInterval time.Duration
	FeedURLs     map[string]string // source name -> URL
}

// DefaultNewsFeedConfig provides standard public economic & crypto RSS feeds,
// including on-chain whale activity tracking, institutional inflows, and political/market-mover feeds.
func DefaultNewsFeedConfig() NewsFeedConfig {
	return NewsFeedConfig{
		PollInterval: 60 * time.Second,
		FeedURLs: map[string]string{
			"YahooFinance":            "https://finance.yahoo.com/news/rssindex",
			"CoinDesk":                "https://www.coindesk.com/arc/outboundfeeds/rss/",
			"CoinTelegraph":           "https://cointelegraph.com/rss",
			"Decrypt":                 "https://decrypt.co/feed",
			"WhaleAlerts":             "https://news.google.com/rss/search?q=crypto+whale+trades+OR+whale+alert+OR+%22large+transfer%22&hl=en-US&gl=US&ceid=US:en",
			"OnChainWhales":           "https://news.google.com/rss/search?q=arkham+crypto+whale+transfer+OR+lookonchain+alert&hl=en-US&gl=US&ceid=US:en",
			"PoliticianTrades":        "https://news.google.com/rss/search?q=%22congress+trading%22+OR+%22capitol+trades%22+OR+%22pelosi+trade%22+OR+%22senate+crypto%22&hl=en-US&gl=US&ceid=US:en",
			"TrumpCryptoVentures":     "https://news.google.com/rss/search?q=%22world+liberty+financial%22+OR+%22donald+trump+crypto%22+OR+%22trump+son%22+crypto&hl=en-US&gl=US&ceid=US:en",
		},
	}
}

// NewsCrawler continuously polls financial news feeds, deduplicates headlines with SHA-256,
// scores sentiment via financial NLP, and maintains the latest sentiment state.
type NewsCrawler struct {
	mu           sync.RWMutex
	cfg          NewsFeedConfig
	httpClient   *http.Client
	redisClient  *cache.Client
	dbStore      *db.Store
	seenHashes   map[string]time.Time
	articles     []db.NewsArticle
	running      bool
	stopChan     chan struct{}
}

// NewNewsCrawler creates a news crawler instance.
func NewNewsCrawler(cfg NewsFeedConfig, redisClient *cache.Client, dbStore *db.Store) *NewsCrawler {
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = 60 * time.Second
	}
	if len(cfg.FeedURLs) == 0 {
		cfg = DefaultNewsFeedConfig()
	}

	crawler := &NewsCrawler{
		cfg:         cfg,
		httpClient:  &http.Client{Timeout: 10 * time.Second},
		redisClient: redisClient,
		dbStore:     dbStore,
		seenHashes:  make(map[string]time.Time),
		articles:    make([]db.NewsArticle, 0, 100),
		stopChan:    make(chan struct{}),
	}

	// Hydrate from PostgreSQL database if available
	if dbStore != nil && dbStore.Pool != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		rows, err := dbStore.Pool.Query(ctx, `
			SELECT content_hash, title, source, url, sentiment_score, polarity, key_phrases, published_at, ingested_at
			FROM news_articles
			ORDER BY published_at DESC
			LIMIT 100
		`)
		if err == nil {
			defer rows.Close()
			for rows.Next() {
				var a db.NewsArticle
				if err := rows.Scan(&a.ContentHash, &a.Title, &a.Source, &a.URL, &a.SentimentScore, &a.Polarity, &a.KeyPhrases, &a.PublishedAt, &a.IngestedAt); err == nil {
					if a.KeyPhrases == nil {
						a.KeyPhrases = []string{}
					}
					crawler.articles = append(crawler.articles, a)
					crawler.seenHashes[a.ContentHash] = a.IngestedAt
				}
			}
		}
	}

	return crawler
}

// ComputeContentHash computes the SHA-256 fingerprint of a normalized headline title.
func ComputeContentHash(title string) string {
	norm := NormalizeTitle(title)
	h := sha256.Sum256([]byte(norm))
	return hex.EncodeToString(h[:])
}

// IngestHeadline processes a single article headline, performing SHA-256 deduplication and sentiment scoring.
// Returns (article, true) if it was fresh and successfully ingested.
func (c *NewsCrawler) IngestHeadline(ctx context.Context, source, title, url string, pubTime time.Time) (*db.NewsArticle, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	norm := NormalizeTitle(title)
	if norm == "" {
		return nil, false
	}

	hash := ComputeContentHash(title)
	if _, exists := c.seenHashes[hash]; exists {
		return nil, false
	}

	// Check Redis deduplication set if available
	if c.redisClient != nil {
		if seen, err := c.redisClient.IsNewsHashSeen(ctx, hash); err == nil && seen {
			c.seenHashes[hash] = time.Now()
			return nil, false
		}
	}

	c.seenHashes[hash] = time.Now()
	if c.redisClient != nil {
		_ = c.redisClient.AddNewsHash(ctx, hash)
	}

	// Score sentiment for this headline
	report := AnalyzeNewsSentiment([]string{title})

	if pubTime.IsZero() {
		pubTime = time.Now()
	}

	article := db.NewsArticle{
		ContentHash:    hash,
		Title:          title,
		Source:         source,
		URL:            url,
		SentimentScore: report.Score,
		Polarity:       string(report.Polarity),
		KeyPhrases:     report.KeyPhrases,
		PublishedAt:    pubTime,
		IngestedAt:     time.Now(),
	}

	// Prepend to internal buffer (keep up to 100 recent articles)
	c.articles = append([]db.NewsArticle{article}, c.articles...)
	if len(c.articles) > 100 {
		c.articles = c.articles[:100]
	}

	// Persist to Postgres if available
	if c.dbStore != nil && c.dbStore.Pool != nil {
		_, _ = c.dbStore.Pool.Exec(ctx, `
			INSERT INTO news_articles (content_hash, title, source, url, sentiment_score, polarity, key_phrases, published_at, ingested_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
			ON CONFLICT (content_hash) DO NOTHING
		`, article.ContentHash, article.Title, article.Source, article.URL, article.SentimentScore, article.Polarity, article.KeyPhrases, article.PublishedAt, article.IngestedAt)
	}

	return &article, true
}

// GetLatestArticles returns recent news articles sorted newest first.
func (c *NewsCrawler) GetLatestArticles() []db.NewsArticle {
	c.mu.RLock()
	defer c.mu.RUnlock()

	result := make([]db.NewsArticle, len(c.articles))
	copy(result, c.articles)
	return result
}

// GetAggregateSentiment calculates sentiment over all currently tracked articles.
func (c *NewsCrawler) GetAggregateSentiment() NewsSentimentReport {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if len(c.articles) == 0 {
		return NewsSentimentReport{
			Score:         0.0,
			Polarity:      PolarityNeutral,
			HeadlineCount: 0,
		}
	}

	headlines := make([]string, len(c.articles))
	for i, a := range c.articles {
		headlines[i] = a.Title
	}

	return AnalyzeNewsSentiment(headlines)
}

// FetchFeed pulls and parses a single RSS feed endpoint.
func (c *NewsCrawler) FetchFeed(ctx context.Context, source, feedURL string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, feedURL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", "SimpleTrader-NewsCrawler/2.0")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("unexpected status %d from %s", resp.StatusCode, feedURL)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var rss RSSFeed
	if err := xml.Unmarshal(body, &rss); err != nil {
		return 0, fmt.Errorf("failed to parse XML from %s: %w", feedURL, err)
	}

	ingested := 0
	for _, item := range rss.Channel.Items {
		var pubTime time.Time
		if item.PubDate != "" {
			pubTime, _ = time.Parse(time.RFC1123Z, item.PubDate)
			if pubTime.IsZero() {
				pubTime, _ = time.Parse(time.RFC1123, item.PubDate)
			}
		}
		if _, ok := c.IngestHeadline(ctx, source, item.Title, item.Link, pubTime); ok {
			ingested++
		}
	}

	return ingested, nil
}

// PollOnce sweeps all configured RSS feeds once.
func (c *NewsCrawler) PollOnce(ctx context.Context) {
	for source, url := range c.cfg.FeedURLs {
		count, err := c.FetchFeed(ctx, source, url)
		if err != nil {
			log.Printf("[NewsCrawler] warning: failed fetching %s: %v", source, err)
		} else if count > 0 {
			log.Printf("[NewsCrawler] Ingested %d fresh articles from %s", count, source)
		}
	}
}

// Start begins continuous background polling.
func (c *NewsCrawler) Start(ctx context.Context) {
	c.mu.Lock()
	if c.running {
		c.mu.Unlock()
		return
	}
	c.running = true
	c.stopChan = make(chan struct{})
	c.mu.Unlock()

	go func() {
		// Run initial sweep
		c.PollOnce(ctx)

		ticker := time.NewTicker(c.cfg.PollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				c.Stop()
				return
			case <-c.stopChan:
				return
			case <-ticker.C:
				c.PollOnce(ctx)
			}
		}
	}()
}

// Stop halts the news crawler loop.
func (c *NewsCrawler) Stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.running {
		return
	}
	c.running = false
	close(c.stopChan)
}
