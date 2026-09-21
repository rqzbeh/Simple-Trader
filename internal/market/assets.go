package market

// AssetDefinition details a tradeable asset in Simple-Trader.
type AssetDefinition struct {
	Symbol      string  `json:"symbol"`       // Standardized internal symbol (e.g., "BTC/USDT", "ETH/USDT")
	Name        string  `json:"name"`         // Display name (e.g. "Bitcoin", "PAX Gold")
	Bucket      string  `json:"bucket"`       // "CORE" or "ALPHA"
	FeedSource  string  `json:"feed_source"`  // "BINANCE"
	SourceParam string  `json:"source_param"` // Query parameter for exchange API (e.g. "BTCUSDT")
	MinSize     float64 `json:"min_size"`     // Minimum contract/token order quantity
	Decimals    int     `json:"decimals"`     // Price formatting decimals
}

// SupportedAssets returns the list of active Core and Alpha global pure crypto assets.
// All assets trade live online against real exchange order books and tickers (USDT/USDC).
var SupportedAssets = []AssetDefinition{
	// --- CORE ASSETS (Preservation / Hedging / Large Cap Anchor) ---
	{
		Symbol:      "PAXG/USDT",
		Name:        "PAX Gold (Tokenized Gold)",
		Bucket:      "CORE",
		FeedSource:  "BINANCE",
		SourceParam: "PAXGUSDT",
		MinSize:     0.001,
		Decimals:    2,
	},
	{
		Symbol:      "BNB/USDT",
		Name:        "BNB",
		Bucket:      "CORE",
		FeedSource:  "BINANCE",
		SourceParam: "BNBUSDT",
		MinSize:     0.01,
		Decimals:    2,
	},

	// --- ALPHA ASSETS (High Sharpe / High Liquidity / Growth) ---
	{
		Symbol:      "BTC/USDT",
		Name:        "Bitcoin",
		Bucket:      "ALPHA",
		FeedSource:  "BINANCE",
		SourceParam: "BTCUSDT",
		MinSize:     0.0001,
		Decimals:    2,
	},
	{
		Symbol:      "ETH/USDT",
		Name:        "Ethereum",
		Bucket:      "ALPHA",
		FeedSource:  "BINANCE",
		SourceParam: "ETHUSDT",
		MinSize:     0.001,
		Decimals:    2,
	},
	{
		Symbol:      "SOL/USDT",
		Name:        "Solana",
		Bucket:      "ALPHA",
		FeedSource:  "BINANCE",
		SourceParam: "SOLUSDT",
		MinSize:     0.01,
		Decimals:    2,
	},
	{
		Symbol:      "XRP/USDT",
		Name:        "XRP",
		Bucket:      "ALPHA",
		FeedSource:  "BINANCE",
		SourceParam: "XRPUSDT",
		MinSize:     1.0,
		Decimals:    4,
	},
	{
		Symbol:      "LINK/USDT",
		Name:        "Chainlink",
		Bucket:      "ALPHA",
		FeedSource:  "BINANCE",
		SourceParam: "LINKUSDT",
		MinSize:     0.1,
		Decimals:    3,
	},
	{
		Symbol:      "EUR/USDT",
		Name:        "Euro / Tether",
		Bucket:      "ALPHA",
		FeedSource:  "BINANCE",
		SourceParam: "EURUSDT",
		MinSize:     1.0,
		Decimals:    4,
	},
}

// GetSupportedAssets returns the list of all supported assets.
func GetSupportedAssets() []AssetDefinition {
	return SupportedAssets
}

// FindAsset locates an asset definition by symbol.
func FindAsset(symbol string) (AssetDefinition, bool) {
	for _, a := range SupportedAssets {
		if a.Symbol == symbol {
			return a, true
		}
	}
	return AssetDefinition{}, false
}
