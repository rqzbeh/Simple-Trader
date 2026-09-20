package market

import (
	"context"
	"testing"
	"time"
)

func TestNewsCrawlerDeduplicationAndSentiment(t *testing.T) {
	crawler := NewNewsCrawler(NewsFeedConfig{}, nil, nil)
	ctx := context.Background()

	// Ingest first headline
	art1, ok := crawler.IngestHeadline(ctx, "YahooFinance", "Federal Reserve announces unexpected interest rate cut", "https://example.com/1", time.Now())
	if !ok || art1 == nil {
		t.Fatalf("expected first headline to be successfully ingested")
	}

	if art1.Polarity != string(PolarityBullish) {
		t.Fatalf("expected bullish polarity for rate cut, got %s", art1.Polarity)
	}

	// Attempt to ingest duplicate headline (with minor capitalization/spacing difference)
	_, ok = crawler.IngestHeadline(ctx, "YahooFinance", "   Federal Reserve Announces Unexpected Interest Rate Cut!  ", "https://example.com/1-dup", time.Now())
	if ok {
		t.Fatalf("expected duplicate headline to be rejected by SHA-256 deduplication")
	}

	// Ingest bearish headline
	art2, ok := crawler.IngestHeadline(ctx, "CryptoPanic", "SEC files lawsuit against exchange causing crypto crash and selloff", "https://example.com/2", time.Now())
	if !ok || art2 == nil {
		t.Fatalf("expected second headline to be ingested")
	}

	if art2.Polarity != string(PolarityBearish) {
		t.Fatalf("expected bearish polarity for crash/selloff, got %s", art2.Polarity)
	}

	// Ingest Whale Alert accumulation headline
	art3, ok := crawler.IngestHeadline(ctx, "WhaleAlerts", "Whale alert: massive transfer to cold wallet spotted on chain with billionaire buy", "https://example.com/3", time.Now())
	if !ok || art3 == nil {
		t.Fatalf("expected whale headline to be ingested")
	}
	if art3.Polarity != string(PolarityBullish) {
		t.Fatalf("expected bullish polarity for whale accumulation, got %s", art3.Polarity)
	}

	// Ingest Politician insider trade headline
	art4, ok := crawler.IngestHeadline(ctx, "PoliticianTrades", "Congressional disclosure dump reveals politician sell and insider dump before regulatory review", "https://example.com/4", time.Now())
	if !ok || art4 == nil {
		t.Fatalf("expected politician trade headline to be ingested")
	}
	if art4.Polarity != string(PolarityBearish) {
		t.Fatalf("expected bearish polarity for politician sell / insider dump, got %s", art4.Polarity)
	}

	// Ingest Trump Crypto Ventures headline
	art5, ok := crawler.IngestHeadline(ctx, "TrumpCryptoVentures", "Trump crypto venture World Liberty Financial announces pro-crypto legislation backing", "https://example.com/5", time.Now())
	if !ok || art5 == nil {
		t.Fatalf("expected trump crypto headline to be ingested")
	}
	if art5.Polarity != string(PolarityBullish) {
		t.Fatalf("expected bullish polarity for trump endorsement / pro-crypto legislation, got %s", art5.Polarity)
	}

	// Check articles buffer
	articles := crawler.GetLatestArticles()
	if len(articles) != 5 {
		t.Fatalf("expected 5 unique articles in buffer, got %d", len(articles))
	}

	// Sentiment report
	report := crawler.GetAggregateSentiment()
	if report.HeadlineCount != 5 {
		t.Fatalf("expected 5 headlines in aggregate report, got %d", report.HeadlineCount)
	}
}
