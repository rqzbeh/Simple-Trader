package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/rqzbeh/simple-trader/internal/db"
)

const (
	KeyInvestorPoolNAV  = "investor:pool:nav"
	KeyInvestorList     = "investor:list"
	KeyInvestorPrefix   = "investor:profile:"
	KeyNewsSeenHashes   = "news:seen_hashes"
	KeyNewsLatest       = "news:latest"
	KeyScreenerUniverse = "screener:active_universe"
	KeyAllocatorTiers   = "allocator:tier_status"
)

// SetPoolNAV caches the current master NAV and pool details.
func (c *Client) SetPoolNAV(ctx context.Context, nav float64, totalEquity, totalUnits float64, ttl time.Duration) error {
	if c.rdb == nil {
		return nil
	}
	payload := map[string]interface{}{
		"nav":          nav,
		"total_equity": totalEquity,
		"total_units":  totalUnits,
		"updated_at":   time.Now().Unix(),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return c.rdb.Set(ctx, KeyInvestorPoolNAV, data, ttl).Err()
}

// GetPoolNAV retrieves the cached master NAV.
func (c *Client) GetPoolNAV(ctx context.Context) (float64, error) {
	if c.rdb == nil {
		return 1.0, nil
	}
	data, err := c.rdb.Get(ctx, KeyInvestorPoolNAV).Bytes()
	if err != nil {
		return 1.0, err
	}
	var payload struct {
		NAV float64 `json:"nav"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return 1.0, err
	}
	if payload.NAV <= 0 {
		return 1.0, nil
	}
	return payload.NAV, nil
}

// SetInvestorProfile caches an investor's computed pro-rata profile.
func (c *Client) SetInvestorProfile(ctx context.Context, inv *db.Investor, ttl time.Duration) error {
	if c.rdb == nil || inv == nil {
		return nil
	}
	data, err := json.Marshal(inv)
	if err != nil {
		return err
	}
	key := fmt.Sprintf("%s%s", KeyInvestorPrefix, inv.ID)
	return c.rdb.Set(ctx, key, data, ttl).Err()
}

// IsNewsHashSeen checks if a news SHA-256 fingerprint was already ingested.
func (c *Client) IsNewsHashSeen(ctx context.Context, hash string) (bool, error) {
	if c.rdb == nil {
		return false, nil
	}
	return c.rdb.SIsMember(ctx, KeyNewsSeenHashes, hash).Result()
}

// AddNewsHash marks a news SHA-256 fingerprint as seen.
func (c *Client) AddNewsHash(ctx context.Context, hash string) error {
	if c.rdb == nil {
		return nil
	}
	return c.rdb.SAdd(ctx, KeyNewsSeenHashes, hash).Err()
}

// SetActiveCryptoUniverse caches admitted liquid crypto symbols.
func (c *Client) SetActiveCryptoUniverse(ctx context.Context, symbols []string) error {
	if c.rdb == nil {
		return nil
	}
	pipe := c.rdb.Pipeline()
	pipe.Del(ctx, KeyScreenerUniverse)
	if len(symbols) > 0 {
		var members []interface{}
		for _, s := range symbols {
			members = append(members, s)
		}
		pipe.SAdd(ctx, KeyScreenerUniverse, members...)
	}
	_, err := pipe.Exec(ctx)
	return err
}

// GetActiveCryptoUniverse returns currently qualified crypto pairs.
func (c *Client) GetActiveCryptoUniverse(ctx context.Context) ([]string, error) {
	if c.rdb == nil {
		return []string{"BTC/USD", "ETH/USD", "SOL/USD"}, nil
	}
	members, err := c.rdb.SMembers(ctx, KeyScreenerUniverse).Result()
	if err != nil || len(members) == 0 {
		return []string{"BTC/USD", "ETH/USD", "SOL/USD"}, nil
	}
	return members, nil
}
