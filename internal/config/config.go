package config

import (
	"os"
	"strconv"
)

// Config holds all environment settings for the Simple-Trader platform.
type Config struct {
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
	LogLevel            string
	IsProduction        bool
	AdminPassword       string
	AppSecret           string
	TelegramBotToken    string
	TelegramChatID      string
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
	return &Config{
		Port:                getEnv("PORT", "8080"),
		DatabaseURL:         getEnv("DATABASE_URL", "postgres://trader:trader_secret@localhost:5432/simple_trader?sslmode=disable"),
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
		MaxRiskPerTradePct:  getEnvFloat("MAX_RISK_PER_TRADE_PCT", 0.02),
		MinRiskRewardRatio:  getEnvFloat("MIN_RISK_TO_REWARD_RATIO", 2.5),
		KellyFraction:       getEnvFloat("KELLY_FRACTION", 0.50),
		MinRiskPerTradePct:  getEnvFloat("MIN_RISK_PER_TRADE_PCT", 0.005),
		MaxConcurrentSignals: getEnvInt("MAX_CONCURRENT_SIGNALS", 5),
		ImpactFactor:        getEnvFloat("IMPACT_FACTOR", 0.05),
		CalendarHaltMinutes: getEnvInt("CALENDAR_HALT_MINUTES", 15),
		EconomicCalendarURL: getEnv("ECONOMIC_CALENDAR_URL", "https://nfs.faireconomy.media/ff_calendar_thisweek.json"),
		ScreenerMin24hVolume: getEnvFloat("SCREENER_MIN_24H_VOLUME", 50000000.0),
		ScreenerMaxSpreadBps: getEnvFloat("SCREENER_MAX_SPREAD_BPS", 10.0),
		DefaultLeverage:     getEnvInt("DEFAULT_LEVERAGE", 8),
		MinStopLossPct:      getEnvFloat("MIN_STOP_LOSS_PCT", 0.6),
		MaxStopLossPct:      getEnvFloat("MAX_STOP_LOSS_PCT", 2.5),
		MinTakeProfitPct:    getEnvFloat("MIN_TAKE_PROFIT_PCT", 1.5),
		MaxTakeProfitPct:    getEnvFloat("MAX_TAKE_PROFIT_PCT", 8.0),
		MakerFeeRate:        getEnvFloat("MAKER_FEE_RATE", 0.0002),
		TakerFeeRate:        getEnvFloat("TAKER_FEE_RATE", 0.0005),
		MaxSlippagePct:      getEnvFloat("MAX_SLIPPAGE_PCT", 0.05),
		LogLevel:            getEnv("LOG_LEVEL", "info"),
		IsProduction:        getEnv("ENV", "development") == "production",
		AdminPassword:       getEnv("ADMIN_PASSWORD", ""),
		AppSecret:           getEnv("APP_SECRET", ""),
		TelegramBotToken:    getEnv("TELEGRAM_BOT_TOKEN", ""),
		TelegramChatID:      getEnv("TELEGRAM_CHAT_ID", ""),
	}, nil
}
