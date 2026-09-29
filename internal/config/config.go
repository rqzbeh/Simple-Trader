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
	TypesafeAPIKey             string
	RoutingConfidenceThreshold string // must parse >0 — startup error if missing (FR-016)
	Port                       string
	DatabaseURL                string
	RedisURL                   string
	AIBaseURL                  string
	AIAPIKey                   string
	AIModelID                  string
	AITemperature              float64
	AITimeoutSeconds           int
	AIReasoningEffort          string
	InitialCapital             float64
	CoreTargetPct              float64
	AlphaTargetPct             float64
	MaxDrawdownLimitPct        float64
	MaxRiskPerTradePct         float64
	MinRiskRewardRatio         float64 // Minimum R:R ratio required for trade execution
	KellyFraction              float64 // Fractional Kelly multiplier (e.g. 0.50 for Half-Kelly)
	MinRiskPerTradePct         float64 // Minimum risk floor per trade (e.g. 0.005 for 0.5%)
	MaxConcurrentSignals       int     // Maximum concurrent active futures signals allowed
	ImpactFactor               float64 // Friction market impact coefficient (e.g. 0.05)
	CalendarHaltMinutes        int     // Buffer window around high-impact macro releases (e.g. 15 minutes)
	EconomicCalendarURL        string  // Live institutional economic calendar feed endpoint
	ScreenerMin24hVolume       float64 // Minimum 24h volume for crypto screening ($50M USD)
	ScreenerMaxSpreadBps       float64 // Maximum spread for crypto screening (10 bps)
	DefaultLeverage            int     // Default isolated margin leverage factor
	MinStopLossPct             float64 // Minimum Stop Loss % from entry
	MaxStopLossPct             float64 // Maximum Stop Loss % from entry
	MinTakeProfitPct           float64 // Minimum Take Profit % from entry
	MaxTakeProfitPct           float64 // Maximum Take Profit % from entry
	MakerFeeRate               float64 // Institutional maker fee rate (e.g. 0.0002)
	TakerFeeRate               float64 // Institutional taker fee rate (e.g. 0.0005)
	MaxSlippagePct             float64 // Cap on slippage percentage (e.g. 0.05 for 5%)
	MaxTradeMarginPct          float64 // Max margin per trade as fraction of total equity (e.g. 0.20 for 20%)
	SignalMaxAgeMinutes        int     // Max lifetime of an intraday signal before time-exit (default 60 = 1h)
	IsProduction               bool
	AdminPassword              string
	TelegramBotToken           string
	TelegramChatID             string
	EarlyExit                  EarlyExitConfig

	// Optional Jev-Managed Parameter Overrides (spec-015)
	// Empty/unset in environment = decision core manages; set value = user override wins verbatim
	OverrideMinRiskRewardRatio *float64
	OverrideDefaultLeverage    *int
	OverrideMaxRiskPerTradePct *float64
	OverrideSLAtrMult          *float64
	OverrideTPAtrMult          *float64
	OverrideClusterDecayMode   string
	OverrideConfluenceMin      *float64
}

// RequiredEnvKeys: every key Load demands. NO in-code defaults (spec-017
// FR-401) — .env is the single source of truth. Order = boot report order.
func RequiredEnvKeys() []string {
	return []string{
		"PORT", "DATABASE_URL", "REDIS_URL", "ENV",
		"AI_BASE_URL", "AI_API_KEY", "AI_MODEL_ID", "AI_TEMPERATURE", "AI_TIMEOUT_SECONDS", "AI_REASONING_EFFORT",
		"JEV_BASE_URL", "JEV_MODEL", "TYPESAFE_API_KEY", "ROUTING_CONFIDENCE_THRESHOLD",
		"INITIAL_CAPITAL", "CORE_TARGET_PCT", "ALPHA_TARGET_PCT", "MAX_DRAWDOWN_LIMIT_PCT",
		"MIN_RISK_PER_TRADE_PCT", "KELLY_FRACTION", "MAX_CONCURRENT_SIGNALS",
		"IMPACT_FACTOR", "CALENDAR_HALT_MINUTES", "ECONOMIC_CALENDAR_URL",
		"SCREENER_MIN_24H_VOLUME", "SCREENER_MAX_SPREAD_BPS",
		"MIN_STOP_LOSS_PCT", "MAX_STOP_LOSS_PCT", "MIN_TAKE_PROFIT_PCT", "MAX_TAKE_PROFIT_PCT",
		"MAKER_FEE_RATE", "TAKER_FEE_RATE", "MAX_SLIPPAGE_PCT", "MAX_TRADE_MARGIN_PCT",
		"SIGNAL_MAX_AGE_MINUTES", "ADMIN_PASSWORD", "TELEGRAM_BOT_TOKEN", "TELEGRAM_CHAT_ID",
		"EARLY_EXIT_ENABLED", "EARLY_EXIT_MIN_HOLD_MIN", "EARLY_EXIT_MAX_PER_DAY",
		"EARLY_EXIT_COOLDOWN_MIN", "EARLY_EXIT_CONF_FLOOR",
		"TIMEFRAME_SET_ALPHA", "TIMEFRAME_SET_CORE",
		"DB_MAX_CONNS", "DB_MIN_CONNS",
	}
}

// SampleRequiredEnv: bootstrap values (former in-code defaults, now living in
// .env). Used by .env.example generation and tests — never by Load.
func SampleRequiredEnv() map[string]string {
	return map[string]string{
		"PORT":         "8080",
		"DATABASE_URL": "postgres://trader:REDACTED_DB_PASSWORD@localhost:5432/simple_trader?sslmode=disable",
		"REDIS_URL":    "redis://localhost:6379/0", "ENV": "development",
		"AI_BASE_URL": "https://api.openai.com/v1", "AI_API_KEY": "", "AI_MODEL_ID": "",
		"AI_TEMPERATURE": "0.2", "AI_TIMEOUT_SECONDS": "30", "AI_REASONING_EFFORT": "high",
		"JEV_BASE_URL": "https://api.typesafe.ai", "JEV_MODEL": "jev-latest", "TYPESAFE_API_KEY": "", "ROUTING_CONFIDENCE_THRESHOLD": "0.75",
		"INITIAL_CAPITAL": "10000", "CORE_TARGET_PCT": "0.5", "ALPHA_TARGET_PCT": "0.5",
		"MAX_DRAWDOWN_LIMIT_PCT": "0.08", "MIN_RISK_PER_TRADE_PCT": "0.005", "KELLY_FRACTION": "0.5",
		"MAX_CONCURRENT_SIGNALS": "5", "IMPACT_FACTOR": "0.05", "CALENDAR_HALT_MINUTES": "15",
		"ECONOMIC_CALENDAR_URL":   "https://nfs.faireconomy.media/ff_calendar_thisweek.json",
		"SCREENER_MIN_24H_VOLUME": "50000000", "SCREENER_MAX_SPREAD_BPS": "10",
		"MIN_STOP_LOSS_PCT": "0.6", "MAX_STOP_LOSS_PCT": "2.5",
		"MIN_TAKE_PROFIT_PCT": "1.5", "MAX_TAKE_PROFIT_PCT": "8",
		"MAKER_FEE_RATE": "0.0002", "TAKER_FEE_RATE": "0.0005",
		"MAX_SLIPPAGE_PCT": "0.05", "MAX_TRADE_MARGIN_PCT": "0.2",
		"SIGNAL_MAX_AGE_MINUTES": "60", "ADMIN_PASSWORD": "",
		"TELEGRAM_BOT_TOKEN": "", "TELEGRAM_CHAT_ID": "",
		"EARLY_EXIT_ENABLED": "true", "EARLY_EXIT_MIN_HOLD_MIN": "30", "EARLY_EXIT_MAX_PER_DAY": "3",
		"EARLY_EXIT_COOLDOWN_MIN": "60", "EARLY_EXIT_CONF_FLOOR": "0.75",
		"TIMEFRAME_SET_ALPHA": "15m,1h,4h", "TIMEFRAME_SET_CORE": "1h,4h,12h",
		"DB_MAX_CONNS": "25", "DB_MIN_CONNS": "5",
	}
}

// floatKeys/ints/bool: keys whose value must parse (presence alone is not
// enough) — validated together so one boot error lists EVERY problem.
var (
	requiredFloatKeys = []string{
		"AI_TEMPERATURE", "INITIAL_CAPITAL", "CORE_TARGET_PCT", "ALPHA_TARGET_PCT",
		"MAX_DRAWDOWN_LIMIT_PCT", "MIN_RISK_PER_TRADE_PCT", "KELLY_FRACTION",
		"IMPACT_FACTOR", "SCREENER_MIN_24H_VOLUME", "SCREENER_MAX_SPREAD_BPS",
		"MIN_STOP_LOSS_PCT", "MAX_STOP_LOSS_PCT", "MIN_TAKE_PROFIT_PCT", "MAX_TAKE_PROFIT_PCT",
		"MAKER_FEE_RATE", "TAKER_FEE_RATE", "MAX_SLIPPAGE_PCT", "MAX_TRADE_MARGIN_PCT",
		"EARLY_EXIT_CONF_FLOOR",
	}
	requiredIntKeys = []string{
		"AI_TIMEOUT_SECONDS", "MAX_CONCURRENT_SIGNALS", "CALENDAR_HALT_MINUTES",
		"SIGNAL_MAX_AGE_MINUTES", "EARLY_EXIT_MIN_HOLD_MIN", "EARLY_EXIT_MAX_PER_DAY",
		"EARLY_EXIT_COOLDOWN_MIN", "DB_MAX_CONNS", "DB_MIN_CONNS",
	}
	requiredBoolKeys = []string{"EARLY_EXIT_ENABLED"}
)

// validateRequiredEnv: presence of every required key + type parse of typed
// keys, aggregated into ONE explicit error (FR-401 zero-fallback).
func validateRequiredEnv() error {
	var problems []string
	for _, k := range RequiredEnvKeys() {
		if _, ok := os.LookupEnv(k); !ok {
			problems = append(problems, k+" (missing)")
		}
	}
	for _, k := range requiredFloatKeys {
		v, ok := os.LookupEnv(k)
		if !ok {
			continue
		}
		if _, err := strconv.ParseFloat(v, 64); err != nil {
			problems = append(problems, k+" (not a number: "+v+")")
		}
	}
	for _, k := range requiredIntKeys {
		v, ok := os.LookupEnv(k)
		if !ok {
			continue
		}
		if _, err := strconv.Atoi(v); err != nil {
			problems = append(problems, k+" (not an integer: "+v+")")
		}
	}
	for _, k := range requiredBoolKeys {
		v, ok := os.LookupEnv(k)
		if !ok {
			continue
		}
		if _, err := strconv.ParseBool(v); err != nil {
			problems = append(problems, k+" (not a bool: "+v+")")
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("config: required env keys invalid — set them in .env (no in-code defaults, spec-017): %s", strings.Join(problems, ", "))
	}
	return nil
}

// validateRequiredSemantics: range checks that used to be masked by in-code
// defaults (e.g. impact<=0 silently became 0.05). Explicit error now.
func validateRequiredSemantics() error {
	posF := func(key string) error {
		v := os.Getenv(key)
		if f, err := strconv.ParseFloat(v, 64); err != nil || f <= 0 {
			return fmt.Errorf("config: %s must be > 0, got %q", key, v)
		}
		return nil
	}
	nonNegF := func(key string) error {
		v := os.Getenv(key)
		if f, err := strconv.ParseFloat(v, 64); err != nil || f < 0 {
			return fmt.Errorf("config: %s must be >= 0, got %q", key, v)
		}
		return nil
	}
	posI := func(key string) error {
		v := os.Getenv(key)
		if i, err := strconv.Atoi(v); err != nil || i <= 0 {
			return fmt.Errorf("config: %s must be an integer > 0, got %q", key, v)
		}
		return nil
	}
	checks := []func() error{
		func() error { return posF("INITIAL_CAPITAL") },
		func() error { return posF("KELLY_FRACTION") },
		func() error { return posF("MIN_RISK_PER_TRADE_PCT") },
		func() error { return posF("IMPACT_FACTOR") },
		func() error { return posF("SCREENER_MIN_24H_VOLUME") },
		func() error { return posF("SCREENER_MAX_SPREAD_BPS") },
		func() error { return nonNegF("MAKER_FEE_RATE") },
		func() error { return nonNegF("TAKER_FEE_RATE") },
		func() error { return posI("MAX_CONCURRENT_SIGNALS") },
		func() error { return posI("SIGNAL_MAX_AGE_MINUTES") },
		func() error { return posI("AI_TIMEOUT_SECONDS") },
		func() error { return posI("CALENDAR_HALT_MINUTES") },
		func() error { return posI("DB_MAX_CONNS") },
		func() error { return posI("DB_MIN_CONNS") },
	}
	var problems []string
	for _, c := range checks {
		if err := c(); err != nil {
			problems = append(problems, err.Error())
		}
	}
	// stop/tp band sanity: min <= max
	band := func(minK, maxK string) error {
		lo, _ := strconv.ParseFloat(os.Getenv(minK), 64)
		hi, _ := strconv.ParseFloat(os.Getenv(maxK), 64)
		if lo > hi {
			return fmt.Errorf("config: %s (%s) must be <= %s (%s)", minK, os.Getenv(minK), maxK, os.Getenv(maxK))
		}
		return nil
	}
	for _, b := range [][2]string{{"MIN_STOP_LOSS_PCT", "MAX_STOP_LOSS_PCT"}, {"MIN_TAKE_PROFIT_PCT", "MAX_TAKE_PROFIT_PCT"}} {
		if err := band(b[0], b[1]); err != nil {
			problems = append(problems, err.Error())
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("config: invalid values in .env (spec-017): %s", strings.Join(problems, "; "))
	}
	return nil
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

	if verr := validateRequiredEnv(); verr != nil {
		return nil, verr
	}
	// Semantic sanity (spec-017): values must be usable, not just present.
	if verr := validateRequiredSemantics(); verr != nil {
		return nil, verr
	}
	// Typed accessors — values pre-validated by validateRequiredEnv.
	mustF := func(key string) float64 { f, _ := strconv.ParseFloat(os.Getenv(key), 64); return f }
	mustI := func(key string) int { i, _ := strconv.Atoi(os.Getenv(key)); return i }

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
		TypesafeAPIKey:             os.Getenv("TYPESAFE_API_KEY"),
		RoutingConfidenceThreshold: os.Getenv("ROUTING_CONFIDENCE_THRESHOLD"),
		Port:                       os.Getenv("PORT"),
		DatabaseURL:                os.Getenv("DATABASE_URL"),
		RedisURL:                   os.Getenv("REDIS_URL"),
		AIBaseURL:                  os.Getenv("AI_BASE_URL"),
		AIAPIKey:                   os.Getenv("AI_API_KEY"),
		AIModelID:                  os.Getenv("AI_MODEL_ID"),
		AITemperature:              mustF("AI_TEMPERATURE"),
		AITimeoutSeconds:           mustI("AI_TIMEOUT_SECONDS"),
		AIReasoningEffort:          os.Getenv("AI_REASONING_EFFORT"),
		InitialCapital:             mustF("INITIAL_CAPITAL"),
		CoreTargetPct:              mustF("CORE_TARGET_PCT"),
		AlphaTargetPct:             mustF("ALPHA_TARGET_PCT"),
		MaxDrawdownLimitPct:        mustF("MAX_DRAWDOWN_LIMIT_PCT"),
		MaxRiskPerTradePct:         maxRisk,
		MinRiskRewardRatio:         minRR,
		KellyFraction:              mustF("KELLY_FRACTION"),
		MinRiskPerTradePct:         mustF("MIN_RISK_PER_TRADE_PCT"),
		MaxConcurrentSignals:       mustI("MAX_CONCURRENT_SIGNALS"),
		ImpactFactor:               mustF("IMPACT_FACTOR"),
		CalendarHaltMinutes:        mustI("CALENDAR_HALT_MINUTES"),
		EconomicCalendarURL:        os.Getenv("ECONOMIC_CALENDAR_URL"),
		ScreenerMin24hVolume:       mustF("SCREENER_MIN_24H_VOLUME"),
		ScreenerMaxSpreadBps:       mustF("SCREENER_MAX_SPREAD_BPS"),
		DefaultLeverage:            defaultLev,
		MinStopLossPct:             mustF("MIN_STOP_LOSS_PCT"),
		MaxStopLossPct:             mustF("MAX_STOP_LOSS_PCT"),
		MinTakeProfitPct:           mustF("MIN_TAKE_PROFIT_PCT"),
		MaxTakeProfitPct:           mustF("MAX_TAKE_PROFIT_PCT"),
		MakerFeeRate:               mustF("MAKER_FEE_RATE"),
		TakerFeeRate:               mustF("TAKER_FEE_RATE"),
		MaxSlippagePct:             mustF("MAX_SLIPPAGE_PCT"),
		MaxTradeMarginPct:          mustF("MAX_TRADE_MARGIN_PCT"),
		SignalMaxAgeMinutes:        mustI("SIGNAL_MAX_AGE_MINUTES"),
		IsProduction:               os.Getenv("ENV") == "production",
		AdminPassword:              os.Getenv("ADMIN_PASSWORD"),
		TelegramBotToken:           os.Getenv("TELEGRAM_BOT_TOKEN"),
		TelegramChatID:             os.Getenv("TELEGRAM_CHAT_ID"),
		EarlyExit:                  earlyExit,

		OverrideMinRiskRewardRatio: overrideMinRR,
		OverrideDefaultLeverage:    overrideDefaultLev,
		OverrideMaxRiskPerTradePct: overrideMaxRisk,
		OverrideSLAtrMult:          overrideSLAtrMult,
		OverrideTPAtrMult:          overrideTPAtrMult,
		OverrideClusterDecayMode:   overrideDecayMode,
		OverrideConfluenceMin:      overrideConfMin,
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
	// Env is the source of truth (spec-017); the struct field mirrors it for
	// GET display and is only a fallback when env is unset.
	raw := strings.TrimSpace(os.Getenv("ROUTING_CONFIDENCE_THRESHOLD"))
	if raw == "" && c != nil {
		raw = strings.TrimSpace(c.RoutingConfidenceThreshold)
	}
	if raw == "" {
		return 0, fmt.Errorf("config: ROUTING_CONFIDENCE_THRESHOLD missing (no default allowed)")
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil || v <= 0 || v > 1 {
		return 0, fmt.Errorf("config: ROUTING_CONFIDENCE_THRESHOLD invalid: %q", raw)
	}
	return v, nil
}
