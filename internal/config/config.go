package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// Config holds all environment settings for the Simple-Trader platform.
type Config struct {
	TypesafeAPIKey           string
	RoutingConfidenceThreshold string // must parse >0 — startup error if missing (FR-016)
	Port                string
	DatabaseURL         string
	RedisURL            string
	AIBaseURL           string
	AIAPIKey            string
	AIModelID           string
	AITemperature       float64
	AITimeoutSeconds    int
	AIReasoningEffort   string
	InitialCapital      float64
	CoreTargetPct       float64
	AlphaTargetPct      float64
	MaxDrawdownLimitPct float64
	MaxRiskPerTradePct  float64
	MinRiskRewardRatio  float64 // Minimum R:R ratio required for trade execution
	KellyFraction       float64 // Fractional Kelly multiplier (e.g. 0.50 for Half-Kelly)
	MinRiskPerTradePct  float64 // Minimum risk floor per trade (e.g. 0.005 for 0.5%)
	MaxConcurrentSignals int    // Maximum concurrent active futures signals allowed
	ImpactFactor        float64 // Friction market impact coefficient (e.g. 0.05)
	CalendarHaltMinutes int     // Buffer window around high-impact macro releases (e.g. 15 minutes)
	EconomicCalendarURL string  // Live institutional economic calendar feed endpoint
	ScreenerMin24hVolume float64 // Minimum 24h volume for crypto screening ($50M USD)
	ScreenerMaxSpreadBps float64 // Maximum spread for crypto screening (10 bps)
	DefaultLeverage     int     // Default isolated margin leverage factor
	MinStopLossPct      float64 // Minimum Stop Loss % from entry
	MaxStopLossPct      float64 // Maximum Stop Loss % from entry
	MinTakeProfitPct    float64 // Minimum Take Profit % from entry
	MaxTakeProfitPct    float64 // Maximum Take Profit % from entry
	MakerFeeRate        float64 // Institutional maker fee rate (e.g. 0.0002)
	TakerFeeRate        float64 // Institutional taker fee rate (e.g. 0.0005)
	MaxSlippagePct      float64 // Cap on slippage percentage (e.g. 0.05 for 5%)
	MaxTradeMarginPct   float64 // Max margin per trade as fraction of total equity (e.g. 0.20 for 20%)
	SignalMaxAgeMinutes int     // Max lifetime of an intraday signal before time-exit (default 60 = 1h)
	IsProduction        bool
	AdminPassword       string
	TelegramBotToken    string
	TelegramChatID      string
	EarlyExit           EarlyExitConfig

	// Optional Jev-Managed Parameter Overrides (spec-015)
	// Empty/unset in environment = decision core manages; set value = user override wins verbatim
	OverrideMinRiskRewardRatio *float64
	OverrideDefaultLeverage     *int
	OverrideMaxRiskPerTradePct  *float64
	OverrideSLAtrMult           *float64
	OverrideTPAtrMult           *float64
	OverrideClusterDecayMode    string
	OverrideConfluenceMin       *float64
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvFloat(key string, defaultVal float64) float64 {
	if val := os.Getenv(key); val != "" {
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			return f
		}
	}
	return defaultVal
}

func getEnvInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			return i
		}
	}
	return defaultVal
}

// Load parses environment variables and returns a validated Config.
func Load() (*Config, error) {
	// .env file is the single source of truth: load it into the process env
	// before any key is read (settings UI persists there; compose interpolates
	// only a subset of keys).
	ApplyEnvFile()

	// UPSTREAM_PROXY_URL is optional: unset = DIRECT connection. A set-but-
	// invalid value is a startup error — never a silent direct fallback.
	if p := strings.TrimSpace(os.Getenv("UPSTREAM_PROXY_URL")); p != "" {
		u, err := url.Parse(p)
		if err != nil || (u.Scheme != "socks5" && u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			return nil, fmt.Errorf("config: UPSTREAM_PROXY_URL invalid: need socks5://host:port or http://host:port, got %q", p)
		}
	}

	earlyExit, err := LoadEarlyExitConfig()
	if err != nil {
		return nil, err
	}

	// Optional Jev-Managed Parameter Overrides (spec-015)
	var overrideMinRR *float64
	if v := os.Getenv("MIN_RISK_TO_REWARD_RATIO"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f < 0.5 || f > 20.0 {
			return nil, fmt.Errorf("config: MIN_RISK_TO_REWARD_RATIO invalid: %q", v)
		}
		overrideMinRR = &f
	}

	var overrideDefaultLev *int
	if v := os.Getenv("DEFAULT_LEVERAGE"); v != "" {
		i, err := strconv.Atoi(v)
		if err != nil || i < 1 || i > 125 {
			return nil, fmt.Errorf("config: DEFAULT_LEVERAGE invalid: %q", v)
		}
		overrideDefaultLev = &i
	}

	var overrideMaxRisk *float64
	if v := os.Getenv("MAX_RISK_PER_TRADE_PCT"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f <= 0 || f > 1.0 {
			return nil, fmt.Errorf("config: MAX_RISK_PER_TRADE_PCT invalid: %q", v)
		}
		overrideMaxRisk = &f
	}

	var overrideSLAtrMult *float64
	if v := os.Getenv("SL_ATR_MULT"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f <= 0 || f > 20.0 {
			return nil, fmt.Errorf("config: SL_ATR_MULT invalid: %q", v)
		}
		overrideSLAtrMult = &f
	}

	var overrideTPAtrMult *float64
	if v := os.Getenv("TP_ATR_MULT"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f <= 0 || f > 20.0 {
			return nil, fmt.Errorf("config: TP_ATR_MULT invalid: %q", v)
		}
		overrideTPAtrMult = &f
	}

	overrideDecayMode := os.Getenv("CLUSTER_DECAY_MODE")
	if overrideDecayMode != "" {
		mode := strings.ToUpper(strings.TrimSpace(overrideDecayMode))
		if mode != "FAST_BREAKING" && mode != "MACRO_THEMATIC" {
			return nil, fmt.Errorf("config: CLUSTER_DECAY_MODE invalid: must be FAST_BREAKING or MACRO_THEMATIC, got %q", overrideDecayMode)
		}
		overrideDecayMode = mode
	}

	var overrideConfMin *float64
	if v := os.Getenv("CONFLUENCE_MIN"); v != "" {
		f, err := strconv.ParseFloat(v, 64)
		if err != nil || f < 0.0 || f > 1.0 {
			return nil, fmt.Errorf("config: CONFLUENCE_MIN invalid: %q", v)
		}
		overrideConfMin = &f
	}

	minRR := 2.5
	if overrideMinRR != nil {
		minRR = *overrideMinRR
	}
	defaultLev := 8
	if overrideDefaultLev != nil {
		defaultLev = *overrideDefaultLev
	}
	maxRisk := 0.02
	if overrideMaxRisk != nil {
		maxRisk = *overrideMaxRisk
	}

	return &Config{
		TypesafeAPIKey:             getEnv("TYPESAFE_API_KEY", ""),
		RoutingConfidenceThreshold: getEnv("ROUTING_CONFIDENCE_THRESHOLD", ""),
		Port:                getEnv("PORT", "8080"),
		DatabaseURL:         getEnv("DATABASE_URL", "postgres://trader:REDACTED_DB_PASSWORD@localhost:5432/simple_trader?sslmode=disable"),
		RedisURL:            getEnv("REDIS_URL", "redis://localhost:6379/0"),
		AIBaseURL:           getEnv("AI_BASE_URL", "https://api.openai.com/v1"),
		AIAPIKey:            getEnv("AI_API_KEY", ""),
		AIModelID:           getEnv("AI_MODEL_ID", ""),
		AITemperature:       getEnvFloat("AI_TEMPERATURE", 0.2),
		AITimeoutSeconds:    getEnvInt("AI_TIMEOUT_SECONDS", 30),
		AIReasoningEffort:   getEnv("AI_REASONING_EFFORT", "high"),
		InitialCapital:      getEnvFloat("INITIAL_CAPITAL", 10000.0),
		CoreTargetPct:       getEnvFloat("CORE_TARGET_PCT", 0.50),
		AlphaTargetPct:      getEnvFloat("ALPHA_TARGET_PCT", 0.50),
		MaxDrawdownLimitPct: getEnvFloat("MAX_DRAWDOWN_LIMIT_PCT", 0.08),
		MaxRiskPerTradePct:  maxRisk,
		MinRiskRewardRatio:  minRR,
		KellyFraction:       getEnvFloat("KELLY_FRACTION", 0.50),
		MinRiskPerTradePct:  getEnvFloat("MIN_RISK_PER_TRADE_PCT", 0.005),
		MaxConcurrentSignals: getEnvInt("MAX_CONCURRENT_SIGNALS", 5),
		ImpactFactor:        getEnvFloat("IMPACT_FACTOR", 0.05),
		CalendarHaltMinutes: getEnvInt("CALENDAR_HALT_MINUTES", 15),
		EconomicCalendarURL: getEnv("ECONOMIC_CALENDAR_URL", "https://nfs.faireconomy.media/ff_calendar_thisweek.json"),
		ScreenerMin24hVolume: getEnvFloat("SCREENER_MIN_24H_VOLUME", 50000000.0),
		ScreenerMaxSpreadBps: getEnvFloat("SCREENER_MAX_SPREAD_BPS", 10.0),
		DefaultLeverage:     defaultLev,
		MinStopLossPct:      getEnvFloat("MIN_STOP_LOSS_PCT", 0.6),
		MaxStopLossPct:      getEnvFloat("MAX_STOP_LOSS_PCT", 2.5),
		MinTakeProfitPct:    getEnvFloat("MIN_TAKE_PROFIT_PCT", 1.5),
		MaxTakeProfitPct:    getEnvFloat("MAX_TAKE_PROFIT_PCT", 8.0),
		MakerFeeRate:        getEnvFloat("MAKER_FEE_RATE", 0.0002),
		TakerFeeRate:        getEnvFloat("TAKER_FEE_RATE", 0.0005),
		MaxSlippagePct:      getEnvFloat("MAX_SLIPPAGE_PCT", 0.05),
		MaxTradeMarginPct:   getEnvFloat("MAX_TRADE_MARGIN_PCT", 0.20),
		SignalMaxAgeMinutes: getEnvInt("SIGNAL_MAX_AGE_MINUTES", 60),
		IsProduction:        getEnv("ENV", "development") == "production",
		AdminPassword:       getEnv("ADMIN_PASSWORD", ""),
		TelegramBotToken:    getEnv("TELEGRAM_BOT_TOKEN", ""),
		TelegramChatID:      getEnv("TELEGRAM_CHAT_ID", ""),
		EarlyExit:           earlyExit,

		OverrideMinRiskRewardRatio: overrideMinRR,
		OverrideDefaultLeverage:     overrideDefaultLev,
		OverrideMaxRiskPerTradePct:  overrideMaxRisk,
		OverrideSLAtrMult:           overrideSLAtrMult,
		OverrideTPAtrMult:           overrideTPAtrMult,
		OverrideClusterDecayMode:    overrideDecayMode,
		OverrideConfluenceMin:       overrideConfMin,
	}, nil
}

// GetParamMode returns "user_override" or "core_managed" for the given parameter key.
func (c *Config) GetParamMode(param string) string {
	if c == nil {
		return "core_managed"
	}
	switch strings.ToLower(strings.TrimSpace(param)) {
	case "min_rr", "min_risk_to_reward_ratio":
		if c.OverrideMinRiskRewardRatio != nil {
			return "user_override"
		}
	case "leverage", "default_leverage":
		if c.OverrideDefaultLeverage != nil {
			return "user_override"
		}
	case "conviction", "max_risk_per_trade_pct":
		if c.OverrideMaxRiskPerTradePct != nil {
			return "user_override"
		}
	case "atr_regime", "sl_atr_mult", "tp_atr_mult":
		if c.OverrideSLAtrMult != nil || c.OverrideTPAtrMult != nil {
			return "user_override"
		}
	case "decay", "cluster_decay_mode", "cluster_decay":
		if c.OverrideClusterDecayMode != "" {
			return "user_override"
		}
	case "confluence", "confluence_min":
		if c.OverrideConfluenceMin != nil {
			return "user_override"
		}
	}
	return "core_managed"
}

// GetParamModes returns the per-param mode map for the 6 phase-1 parameters per contracts §5.
func (c *Config) GetParamModes() map[string]map[string]interface{} {
	modes := make(map[string]map[string]interface{})
	if c == nil {
		return modes
	}

	if c.OverrideMinRiskRewardRatio != nil {
		modes["min_rr"] = map[string]interface{}{"mode": "user_override", "value": *c.OverrideMinRiskRewardRatio}
	} else {
		modes["min_rr"] = map[string]interface{}{"mode": "core_managed"}
	}

	if c.OverrideDefaultLeverage != nil {
		modes["leverage"] = map[string]interface{}{"mode": "user_override", "value": *c.OverrideDefaultLeverage}
	} else {
		modes["leverage"] = map[string]interface{}{"mode": "core_managed"}
	}

	if c.OverrideMaxRiskPerTradePct != nil {
		modes["conviction"] = map[string]interface{}{"mode": "user_override", "value": *c.OverrideMaxRiskPerTradePct}
	} else {
		modes["conviction"] = map[string]interface{}{"mode": "core_managed"}
	}

	if c.OverrideSLAtrMult != nil || c.OverrideTPAtrMult != nil {
		vals := make(map[string]interface{})
		if c.OverrideSLAtrMult != nil {
			vals["sl_atr_mult"] = *c.OverrideSLAtrMult
		}
		if c.OverrideTPAtrMult != nil {
			vals["tp_atr_mult"] = *c.OverrideTPAtrMult
		}
		modes["atr_regime"] = map[string]interface{}{"mode": "user_override", "value": vals}
	} else {
		modes["atr_regime"] = map[string]interface{}{"mode": "core_managed"}
	}

	if c.OverrideClusterDecayMode != "" {
		modes["decay"] = map[string]interface{}{"mode": "user_override", "value": c.OverrideClusterDecayMode}
	} else {
		modes["decay"] = map[string]interface{}{"mode": "core_managed"}
	}

	if c.OverrideConfluenceMin != nil {
		modes["confluence"] = map[string]interface{}{"mode": "user_override", "value": *c.OverrideConfluenceMin}
	} else {
		modes["confluence"] = map[string]interface{}{"mode": "core_managed"}
	}

	return modes
}

// RoutingThreshold parses ROUTING_CONFIDENCE_THRESHOLD. Missing/invalid is a
// startup error — no silent default (spec-013 FR-016).
func (c *Config) RoutingThreshold() (float64, error) {
	if c.RoutingConfidenceThreshold == "" {
		return 0, fmt.Errorf("config: ROUTING_CONFIDENCE_THRESHOLD missing (no default allowed)")
	}
	v, err := strconv.ParseFloat(c.RoutingConfidenceThreshold, 64)
	if err != nil || v <= 0 || v > 1 {
		return 0, fmt.Errorf("config: ROUTING_CONFIDENCE_THRESHOLD invalid: %q", c.RoutingConfidenceThreshold)
	}
	return v, nil
}
