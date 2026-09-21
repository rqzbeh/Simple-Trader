package market

import "strings"

// AssetDefinition details a tradeable asset in Simple-Trader.
type AssetDefinition struct {
	Symbol        string  `json:"symbol"`         // Standardized internal symbol (e.g., "BTC/USDT", "COPPER/USDT")
	Name          string  `json:"name"`           // Display name (e.g. "Bitcoin", "Copper Futures")
	Bucket        string  `json:"bucket"`         // "CORE" or "ALPHA"
	ExposureGroup string  `json:"exposure_group"` // Bundle group (e.g. "GOLD", "SILVER", "COPPER", "OIL")
	FeedSource    string  `json:"feed_source"`    // "BINANCE", "KUCOIN", "COINEX", "YAHOO", "TRADINGVIEW"
	SourceParam   string  `json:"source_param"`   // Query parameter for exchange API (e.g. "BTCUSDT", "COPPERUSDT", "CL=F")
	MinSize       float64 `json:"min_size"`       // Minimum contract/token order quantity
	Decimals      int     `json:"decimals"`       // Price formatting decimals
}

// SupportedAssets returns the list of active Core and Alpha global crypto and commodity assets.
// Core assets are strictly commodities (Precious Metals, Industrial Metals, Energy).
// Alpha assets are liquid cryptocurrencies.
var SupportedAssets = []AssetDefinition{
	// --- CORE ASSETS (Commodities & Wealth Preservation / Inflation Hedges) ---
	// 1. Gold Exposure Group (Correlated Tokenized & Contract Assets)
	{
		Symbol:        "PAXG/USDT",
		Name:          "PAX Gold (Tokenized Gold)",
		Bucket:        "CORE",
		ExposureGroup: "GOLD",
		FeedSource:    "BINANCE",
		SourceParam:   "PAXGUSDT",
		MinSize:       0.001,
		Decimals:      2,
	},
	{
		Symbol:        "XAUT/USDT",
		Name:          "Tether Gold (Tokenized Gold)",
		Bucket:        "CORE",
		ExposureGroup: "GOLD",
		FeedSource:    "BINANCE",
		SourceParam:   "XAUTUSDT",
		MinSize:       0.001,
		Decimals:      2,
	},
	{
		Symbol:        "XAU/USDT",
		Name:          "Gold / Tether",
		Bucket:        "CORE",
		ExposureGroup: "GOLD",
		FeedSource:    "BINANCE",
		SourceParam:   "XAUUSDT",
		MinSize:       0.001,
		Decimals:      2,
	},

	// 2. Silver Exposure Group
	{
		Symbol:        "XAG/USDT",
		Name:          "Silver / Tether",
		Bucket:        "CORE",
		ExposureGroup: "SILVER",
		FeedSource:    "BINANCE",
		SourceParam:   "XAGUSDT",
		MinSize:       0.01,
		Decimals:      2,
	},

	// 3. Copper Exposure Group (Industrial Electrification)
	{
		Symbol:        "COPPER/USDT",
		Name:          "Copper Futures",
		Bucket:        "CORE",
		ExposureGroup: "COPPER",
		FeedSource:    "BINANCE",
		SourceParam:   "COPPERUSDT",
		MinSize:       0.1,
		Decimals:      3,
	},

	// 4. Platinum Exposure Group (Precious / Green Hydrogen Catalyst)
	{
		Symbol:        "XPT/USDT",
		Name:          "Platinum / Tether",
		Bucket:        "CORE",
		ExposureGroup: "PLATINUM",
		FeedSource:    "BINANCE",
		SourceParam:   "XPTUSDT",
		MinSize:       0.01,
		Decimals:      2,
	},

	// 5. Palladium Exposure Group (Precious / Automotive & Electronics)
	{
		Symbol:        "XPD/USDT",
		Name:          "Palladium / Tether",
		Bucket:        "CORE",
		ExposureGroup: "PALLADIUM",
		FeedSource:    "BINANCE",
		SourceParam:   "XPDUSDT",
		MinSize:       0.01,
		Decimals:      2,
	},

	// 6. Crude Oil Exposure Group (Macro Energy)
	{
		Symbol:        "OIL/USDT",
		Name:          "WTI Crude Oil",
		Bucket:        "CORE",
		ExposureGroup: "OIL",
		FeedSource:    "YAHOO",
		SourceParam:   "CL=F",
		MinSize:       0.1,
		Decimals:      2,
	},

	// 7. Aluminum Exposure Group (Industrial Lightweight Metals)
	{
		Symbol:        "ALU/USDT",
		Name:          "Aluminum Futures",
		Bucket:        "CORE",
		ExposureGroup: "ALUMINUM",
		FeedSource:    "YAHOO",
		SourceParam:   "ALI=F",
		MinSize:       0.1,
		Decimals:      2,
	},

	// --- ALPHA ASSETS (Universally Supported Top Tier Liquid Crypto) ---
	{
		Symbol:        "BTC/USDT",
		Name:          "Bitcoin",
		Bucket:        "ALPHA",
		ExposureGroup: "BTC",
		FeedSource:    "BINANCE",
		SourceParam:   "BTCUSDT",
		MinSize:       0.0001,
		Decimals:      2,
	},
	{
		Symbol:        "ETH/USDT",
		Name:          "Ethereum",
		Bucket:        "ALPHA",
		ExposureGroup: "ETH",
		FeedSource:    "BINANCE",
		SourceParam:   "ETHUSDT",
		MinSize:       0.001,
		Decimals:      2,
	},
	{
		Symbol:        "SOL/USDT",
		Name:          "Solana",
		Bucket:        "ALPHA",
		ExposureGroup: "SOL",
		FeedSource:    "BINANCE",
		SourceParam:   "SOLUSDT",
		MinSize:       0.01,
		Decimals:      2,
	},
	{
		Symbol:        "BNB/USDT",
		Name:          "BNB",
		Bucket:        "ALPHA",
		ExposureGroup: "BNB",
		FeedSource:    "BINANCE",
		SourceParam:   "BNBUSDT",
		MinSize:       0.01,
		Decimals:      2,
	},
	{
		Symbol:        "XRP/USDT",
		Name:          "XRP",
		Bucket:        "ALPHA",
		ExposureGroup: "XRP",
		FeedSource:    "BINANCE",
		SourceParam:   "XRPUSDT",
		MinSize:       1.0,
		Decimals:      4,
	},
	{
		Symbol:        "DOGE/USDT",
		Name:          "Dogecoin",
		Bucket:        "ALPHA",
		ExposureGroup: "DOGE",
		FeedSource:    "BINANCE",
		SourceParam:   "DOGEUSDT",
		MinSize:       1.0,
		Decimals:      4,
	},
	{
		Symbol:        "ADA/USDT",
		Name:          "Cardano",
		Bucket:        "ALPHA",
		ExposureGroup: "ADA",
		FeedSource:    "BINANCE",
		SourceParam:   "ADAUSDT",
		MinSize:       1.0,
		Decimals:      4,
	},
	{
		Symbol:        "AVAX/USDT",
		Name:          "Avalanche",
		Bucket:        "ALPHA",
		ExposureGroup: "AVAX",
		FeedSource:    "BINANCE",
		SourceParam:   "AVAXUSDT",
		MinSize:       0.1,
		Decimals:      3,
	},
	{
		Symbol:        "SUI/USDT",
		Name:          "Sui",
		Bucket:        "ALPHA",
		ExposureGroup: "SUI",
		FeedSource:    "BINANCE",
		SourceParam:   "SUIUSDT",
		MinSize:       1.0,
		Decimals:      4,
	},
	{
		Symbol:        "LINK/USDT",
		Name:          "Chainlink",
		Bucket:        "ALPHA",
		ExposureGroup: "LINK",
		FeedSource:    "BINANCE",
		SourceParam:   "LINKUSDT",
		MinSize:       0.1,
		Decimals:      3,
	},
	{
		Symbol:        "DOT/USDT",
		Name:          "Polkadot",
		Bucket:        "ALPHA",
		ExposureGroup: "DOT",
		FeedSource:    "BINANCE",
		SourceParam:   "DOTUSDT",
		MinSize:       0.1,
		Decimals:      3,
	},
	{
		Symbol:        "NEAR/USDT",
		Name:          "NEAR Protocol",
		Bucket:        "ALPHA",
		ExposureGroup: "NEAR",
		FeedSource:    "BINANCE",
		SourceParam:   "NEARUSDT",
		MinSize:       0.1,
		Decimals:      3,
	},
	{
		Symbol:        "LTC/USDT",
		Name:          "Litecoin",
		Bucket:        "ALPHA",
		ExposureGroup: "LTC",
		FeedSource:    "BINANCE",
		SourceParam:   "LTCUSDT",
		MinSize:       0.01,
		Decimals:      2,
	},
	{
		Symbol:        "BCH/USDT",
		Name:          "Bitcoin Cash",
		Bucket:        "ALPHA",
		ExposureGroup: "BCH",
		FeedSource:    "BINANCE",
		SourceParam:   "BCHUSDT",
		MinSize:       0.01,
		Decimals:      2,
	},
	{
		Symbol:        "UNI/USDT",
		Name:          "Uniswap",
		Bucket:        "ALPHA",
		ExposureGroup: "UNI",
		FeedSource:    "BINANCE",
		SourceParam:   "UNIUSDT",
		MinSize:       0.1,
		Decimals:      3,
	},
	{
		Symbol:        "APT/USDT",
		Name:          "Aptos",
		Bucket:        "ALPHA",
		ExposureGroup: "APT",
		FeedSource:    "BINANCE",
		SourceParam:   "APTUSDT",
		MinSize:       0.1,
		Decimals:      3,
	},
}

// GetSupportedAssets returns the list of all supported assets.
func GetSupportedAssets() []AssetDefinition {
	return SupportedAssets
}

// FindAsset locates an asset definition by symbol (supports "BTC/USDT" or "BTCUSDT").
func FindAsset(symbol string) (AssetDefinition, bool) {
	cleanSym := strings.ToUpper(symbol)
	for _, a := range SupportedAssets {
		if a.Symbol == cleanSym {
			return a, true
		}
		if strings.ReplaceAll(a.Symbol, "/", "") == strings.ReplaceAll(cleanSym, "/", "") {
			return a, true
		}
	}
	return AssetDefinition{}, false
}

// GetBucket returns "CORE" or "ALPHA" dynamically based on the asset definition.
func GetBucket(symbol string) string {
	if def, ok := FindAsset(symbol); ok && def.Bucket != "" {
		return def.Bucket
	}
	return "ALPHA"
}

// GetExposureGroup returns the economic exposure bundle for the symbol.
// Gold commodities ("PAXG/USDT", "XAUT/USDT", "XAU/USDT") share the "GOLD" group
// to bundle identical physical/tokenized exposure and prevent splitting or duplicating cash.
func GetExposureGroup(symbol string) string {
	clean := strings.ToUpper(symbol)
	switch clean {
	case "PAXG/USDT", "XAUT/USDT", "XAU/USDT", "PAXGUSDT", "XAUTUSDT", "XAUUSDT":
		return "GOLD"
	case "XAG/USDT", "XAGUSDT":
		return "SILVER"
	case "COPPER/USDT", "COPPERUSDT":
		return "COPPER"
	case "XPT/USDT", "XPTUSDT":
		return "PLATINUM"
	case "XPD/USDT", "XPDUSDT":
		return "PALLADIUM"
	case "OIL/USDT", "OILUSDT", "CL=F", "BZ=F":
		return "OIL"
	case "ALU/USDT", "ALUUSDT", "ALI=F":
		return "ALUMINUM"
	default:
		if def, ok := FindAsset(clean); ok && def.ExposureGroup != "" {
			return def.ExposureGroup
		}
		// Fall back to base coin (e.g. "BTC" for "BTC/USDT")
		parts := strings.Split(clean, "/")
		if len(parts) > 0 {
			return parts[0]
		}
		return clean
	}
}

// AreCorrelatedCommodities returns true if two symbols share the same underlying commodity exposure (e.g. PAXG & XAUT).
func AreCorrelatedCommodities(symA, symB string) bool {
	if symA == symB {
		return true
	}
	grpA := GetExposureGroup(symA)
	grpB := GetExposureGroup(symB)
	if grpA == "" || grpB == "" {
		return false
	}
	// Only commodity groups are subject to correlated bundling constraints
	switch grpA {
	case "GOLD", "SILVER", "COPPER", "PLATINUM", "PALLADIUM", "OIL", "ALUMINUM":
		return grpA == grpB
	default:
		return false
	}
}

// GetCorrelatedSymbols returns all symbols in the supported assets list that share the same exposure group.
func GetCorrelatedSymbols(symbol string) []string {
	targetGroup := GetExposureGroup(symbol)
	if targetGroup == "" {
		return []string{symbol}
	}
	var matches []string
	for _, a := range SupportedAssets {
		if a.ExposureGroup == targetGroup {
			matches = append(matches, a.Symbol)
		}
	}
	if len(matches) == 0 {
		return []string{symbol}
	}
	return matches
}
