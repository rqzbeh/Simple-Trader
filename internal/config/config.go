package config

import (
	"os"
	"strconv"
)

// Config holds all environment settings for the Simple-Trader platform.
type Config struct {
	Port               string
	DatabaseURL        string
	RedisURL           string
	AIBaseURL          string
	AIAPIKey           string
	AIModelID          string
	AITemperature      float64
	AITimeoutSeconds   int
	InitialCapital     float64
	CoreTargetPct      float64
	AlphaTargetPct     float64
	MaxDrawdownLimitPct float64
	MaxRiskPerTradePct float64
	LogLevel           string
	IsProduction       bool
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
		AIModelID:           getEnv("AI_MODEL_ID", "gpt-4o"),
		AITemperature:       getEnvFloat("AI_TEMPERATURE", 0.2),
		AITimeoutSeconds:    getEnvInt("AI_TIMEOUT_SECONDS", 30),
		InitialCapital:      getEnvFloat("INITIAL_CAPITAL", 10000.0),
		CoreTargetPct:       getEnvFloat("CORE_TARGET_PCT", 0.50),
		AlphaTargetPct:      getEnvFloat("ALPHA_TARGET_PCT", 0.50),
		MaxDrawdownLimitPct: getEnvFloat("MAX_DRAWDOWN_LIMIT_PCT", 0.08),
		MaxRiskPerTradePct:  getEnvFloat("MAX_RISK_PER_TRADE_PCT", 0.02),
		LogLevel:            getEnv("LOG_LEVEL", "info"),
		IsProduction:        getEnv("ENV", "development") == "production",
	}, nil
}
