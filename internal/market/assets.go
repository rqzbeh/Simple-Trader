package market

// AssetDefinition details a tradeable asset in Simple-Trader.
type AssetDefinition struct {
	Symbol      string  `json:"symbol"`       // Standardized internal symbol (e.g., "BTC/USD", "XAU/USD")
	Name        string  `json:"name"`         // Display name (e.g. "Bitcoin", "Gold Spot")
	Bucket      string  `json:"bucket"`       // "CORE" or "ALPHA"
	FeedSource  string  `json:"feed_source"`  // "BINANCE" or "YAHOO"
	SourceParam string  `json:"source_param"` // Query parameter (e.g. "BTCUSDT", "GC=F")
	MinSize     float64 `json:"min_size"`     // Minimum contract/token order quantity
	Decimals    int     `json:"decimals"`     // Price formatting decimals
}

// SupportedAssets returns the list of active Core and Alpha global assets.
// Iranian Bourse assets have been strictly excised.
var SupportedAssets = []AssetDefinition{
	// --- CORE ASSETS (Preservation / Hedging) ---
	{
		Symbol:      "XAU/USD",
		Name:        "Gold (Spot/Futures)",
		Bucket:      "CORE",
		FeedSource:  "YAHOO",
		SourceParam: "GC=F",
		MinSize:     0.01,
		Decimals:    2,
	},
	{
		Symbol:      "XAG/USD",
		Name:        "Silver",
		Bucket:      "CORE",
		FeedSource:  "YAHOO",
		SourceParam: "SI=F",
		MinSize:     0.1,
		Decimals:    3,
	},

	// --- ALPHA ASSETS (High Sharpe / Growth) ---
	{
		Symbol:      "BTC/USD",
		Name:        "Bitcoin",
		Bucket:      "ALPHA",
		FeedSource:  "BINANCE",
		SourceParam: "BTCUSDT",
		MinSize:     0.001,
		Decimals:    2,
	},
	{
		Symbol:      "ETH/USD",
		Name:        "Ethereum",
		Bucket:      "ALPHA",
		FeedSource:  "BINANCE",
		SourceParam: "ETHUSDT",
		MinSize:     0.01,
		Decimals:    2,
	},
	{
		Symbol:      "SOL/USD",
		Name:        "Solana",
		Bucket:      "ALPHA",
		FeedSource:  "BINANCE",
		SourceParam: "SOLUSDT",
		MinSize:     0.1,
		Decimals:    2,
	},
	{
		Symbol:      "EUR/USD",
		Name:        "Euro / US Dollar",
		Bucket:      "ALPHA",
		FeedSource:  "YAHOO",
		SourceParam: "EURUSD=X",
		MinSize:     100.0,
		Decimals:    5,
	},
	{
		Symbol:      "WTI/USD",
		Name:        "Crude Oil (WTI)",
		Bucket:      "ALPHA",
		FeedSource:  "YAHOO",
		SourceParam: "CL=F",
		MinSize:     1.0,
		Decimals:    2,
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
