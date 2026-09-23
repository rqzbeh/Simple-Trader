package indicators

// MACDResult encapsulates the MACD line, signal line, and histogram series.
type MACDResult struct {
	MACD      []float64
	Signal    []float64
	Histogram []float64
}

// BollingerResult holds Upper, Middle (SMA), and Lower Bollinger bands.
type BollingerResult struct {
	Upper  []float64
	Middle []float64
	Lower  []float64
}

// SuperTrendResult holds the SuperTrend stop line and trend regime ("BULL" or "BEAR").
type SuperTrendResult struct {
	Value []float64
	Trend []string
}

// Snapshot contains the latest indicator state for evaluating confluence and LLM prompt context.
type Snapshot struct {
	Symbol          string
	Price           float64
	RSI             float64
	MACD            float64
	MACDSignal      float64
	MACDHistogram   float64
	UpperBand       float64
	MiddleBand      float64
	LowerBand       float64
	ATR             float64
	SuperTrendVal   float64
	SuperTrendTrend string
	VWAP            float64
	OBI             float64
	CVD             float64
	Divergence      DivergenceType
	Regime          MarketRegime
	VolRatio        float64
	GarmanKlass     float64
	Parkinson       float64
	KaufmanER          float64
	CMF                float64
	NATR               float64
	ConfluenceScore    float64
	SuggestedDirection string
}
