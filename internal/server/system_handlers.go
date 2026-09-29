package server

import (
	"net/url"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rqzbeh/simple-trader/internal/ai"
	"github.com/rqzbeh/simple-trader/internal/auth"
	"github.com/rqzbeh/simple-trader/internal/config"
	"github.com/rqzbeh/simple-trader/internal/trader"
)

type SystemStatsResponse struct {
	Version          string         `json:"version"`
	UptimeSeconds    int64          `json:"uptime_seconds"`
	RoutingThreshold *float64       `json:"routing_threshold,omitempty"`
	Gateway          StatsComponent `json:"gateway"`
	Jev              JevComponent   `json:"jev"`
}

type StatsComponent struct {
	Total         int64   `json:"total"`
	Success       int64   `json:"success"`
	Fail          int64   `json:"fail"`
	SuccessRate   float64 `json:"success_rate"`
	EMALatencyMs  float64 `json:"ema_latency_ms"`
	LastLatencyMs float64 `json:"last_latency_ms"`
	LastError     string  `json:"last_error"`
	LastOKAt      *string `json:"last_ok_at"`
}

type JevComponent struct {
	StatsComponent
	Model         string `json:"model"`
	KeyConfigured bool   `json:"key_configured"`
}

func (s *Server) handleSystemStats(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	uptime := int64(0)
	if !s.startTime.IsZero() {
		uptime = int64(time.Since(s.startTime).Seconds())
	}

	gw := ai.GetGatewayStats()
	var gwRate float64
	if gw.Total > 0 {
		gwRate = math.Round((float64(gw.Success)/float64(gw.Total))*10000) / 10000
	}
	var gwLastOK *string
	if !gw.LastOK.IsZero() {
		tStr := gw.LastOK.UTC().Format(time.RFC3339)
		gwLastOK = &tStr
	}
	gwErr := gw.LastError
	if len(gwErr) > 200 {
		gwErr = gwErr[:200]
	}

	jev := ai.GetJevStats()
	var jevRate float64
	if jev.Total > 0 {
		jevRate = math.Round((float64(jev.Success)/float64(jev.Total))*10000) / 10000
	}
	var jevLastOK *string
	if !jev.LastOK.IsZero() {
		tStr := jev.LastOK.UTC().Format(time.RFC3339)
		jevLastOK = &tStr
	}
	jevErr := jev.LastError
	if len(jevErr) > 200 {
		jevErr = jevErr[:200]
	}

	jevModel := "jev-latest"
	if s.decisionRouter != nil && s.decisionRouter.Jev != nil {
		jevModel = s.decisionRouter.Jev.Model()
	}

	typesafeKey := ""
	if s.cfg != nil {
		typesafeKey = s.cfg.TypesafeAPIKey
	}
	if typesafeKey == "" {
		typesafeKey = os.Getenv("TYPESAFE_API_KEY")
	}
	keyConfigured := typesafeKey != ""

	var routingThreshold *float64
	if s.decisionRouter != nil {
		thr := s.decisionRouter.GetThreshold()
		if thr > 0 {
			routingThreshold = &thr
		}
	}
	if routingThreshold == nil && s.cfg != nil {
		if thr, err := s.cfg.RoutingThreshold(); err == nil {
			routingThreshold = &thr
		}
	}

	resp := SystemStatsResponse{
		Version:          "3.0.0-decision-core",
		UptimeSeconds:    uptime,
		RoutingThreshold: routingThreshold,
		Gateway: StatsComponent{
			Total:         gw.Total,
			Success:       gw.Success,
			Fail:          gw.Fail,
			SuccessRate:   gwRate,
			EMALatencyMs:  math.Round(gw.EMA_LatencyMs*10) / 10,
			LastLatencyMs: math.Round(gw.LastLatencyMs*10) / 10,
			LastError:     gwErr,
			LastOKAt:      gwLastOK,
		},
		Jev: JevComponent{
			StatsComponent: StatsComponent{
				Total:         jev.Total,
				Success:       jev.Success,
				Fail:          jev.Fail,
				SuccessRate:   jevRate,
				EMALatencyMs:  math.Round(jev.EMA_LatencyMs*10) / 10,
				LastLatencyMs: math.Round(jev.LastLatencyMs*10) / 10,
				LastError:     jevErr,
				LastOKAt:      jevLastOK,
			},
			Model:         jevModel,
			KeyConfigured: keyConfigured,
		},
	}

	_ = json.NewEncoder(w).Encode(resp)
}

// systemConfigResponse produces the masked runtime config payload.
func (s *Server) systemConfigResponse() map[string]interface{} {
	maskedAIKey := ""
	if s.cfg != nil {
		if len(s.cfg.AIAPIKey) > 8 {
			maskedAIKey = s.cfg.AIAPIKey[:4] + "..." + s.cfg.AIAPIKey[len(s.cfg.AIAPIKey)-4:]
		} else if len(s.cfg.AIAPIKey) > 0 {
			maskedAIKey = "***"
		}
	}

	typesafeKey := ""
	if s.cfg != nil {
		typesafeKey = s.cfg.TypesafeAPIKey
	}
	if typesafeKey == "" {
		typesafeKey = os.Getenv("TYPESAFE_API_KEY")
	}

	maskedTypesafeKey := ""
	if len(typesafeKey) > 8 {
		maskedTypesafeKey = "..." + typesafeKey[len(typesafeKey)-4:]
	} else if len(typesafeKey) > 0 {
		maskedTypesafeKey = "***"
	}

	var routingThreshold *float64
	if s.decisionRouter != nil {
		thr := s.decisionRouter.GetThreshold()
		if thr > 0 {
			routingThreshold = &thr
		}
	}
	if routingThreshold == nil && s.cfg != nil {
		if thr, err := s.cfg.RoutingThreshold(); err == nil {
			routingThreshold = &thr
		}
	}

	typesafeBaseURL := "https://api.typesafe.ai"
	if s.decisionRouter != nil && s.decisionRouter.Jev != nil {
		typesafeBaseURL = s.decisionRouter.Jev.BaseURL()
	}

	envFile := os.Getenv("ENV_FILE")
	if envFile == "" {
		envFile = config.DefaultEnvPath()
	}

	var aiBaseURLConfigured bool
	var aiModelID string
	var aiReasoningEffort string
	var aiAPIKeyConfigured bool
	var initialCapital float64
	var coreTargetPct float64
	var alphaTargetPct float64
	var telegramBotConfigured bool
	var telegramChatID string
	var defaultLeverage int
	var minRiskRewardRatio float64
	var maxConcurrentSignals int
	var maxRiskPerTradePct float64
	var maxDrawdownLimitPct float64
	var calendarHaltMinutes int
	var screenerMin24hVolume float64
	var aiTemperature float64
	var aiTimeoutSeconds int

	if s.cfg != nil {
		aiBaseURLConfigured = s.cfg.AIBaseURL != ""
		aiModelID = s.cfg.AIModelID
		aiReasoningEffort = s.cfg.AIReasoningEffort
		aiAPIKeyConfigured = s.cfg.AIAPIKey != ""
		initialCapital = s.cfg.InitialCapital
		coreTargetPct = s.cfg.CoreTargetPct
		alphaTargetPct = s.cfg.AlphaTargetPct
		telegramBotConfigured = s.cfg.TelegramBotToken != "" && s.cfg.TelegramChatID != ""
		telegramChatID = s.cfg.TelegramChatID
		defaultLeverage = s.cfg.DefaultLeverage
		minRiskRewardRatio = s.cfg.MinRiskRewardRatio
		maxConcurrentSignals = s.cfg.MaxConcurrentSignals
		maxRiskPerTradePct = s.cfg.MaxRiskPerTradePct
		maxDrawdownLimitPct = s.cfg.MaxDrawdownLimitPct
		calendarHaltMinutes = s.cfg.CalendarHaltMinutes
		screenerMin24hVolume = s.cfg.ScreenerMin24hVolume
		aiTemperature = s.cfg.AITemperature
		aiTimeoutSeconds = s.cfg.AITimeoutSeconds
	}

	earlyExitEnabled := true
	earlyExitMinHoldMin := 30
	earlyExitMaxPerDay := 3
	earlyExitCooldownMin := 60
	earlyExitConfFloor := 0.75
	if s.cfg != nil {
		earlyExitEnabled = s.cfg.EarlyExit.Enabled
		earlyExitMinHoldMin = s.cfg.EarlyExit.MinHoldMin
		earlyExitMaxPerDay = s.cfg.EarlyExit.MaxPerDay
		earlyExitCooldownMin = s.cfg.EarlyExit.CooldownMin
		earlyExitConfFloor = s.cfg.EarlyExit.ConfFloor
	}

	var paramModes map[string]map[string]interface{}
	var slAtrMult *float64
	var tpAtrMult *float64
	var clusterDecayMode string
	var confluenceMin *float64
	if s.cfg != nil {
		paramModes = s.cfg.GetParamModes()
		slAtrMult = s.cfg.OverrideSLAtrMult
		tpAtrMult = s.cfg.OverrideTPAtrMult
		clusterDecayMode = s.cfg.OverrideClusterDecayMode
		confluenceMin = s.cfg.OverrideConfluenceMin
	}

	return map[string]interface{}{
		"ai_base_url_configured":       aiBaseURLConfigured,
		"ai_model_id":                  aiModelID,
		"ai_reasoning_effort":          aiReasoningEffort,
		"ai_temperature":               aiTemperature,
		"ai_timeout_seconds":           aiTimeoutSeconds,
		"ai_api_key_configured":        aiAPIKeyConfigured,
		"ai_api_key_masked":            maskedAIKey,
		"typesafe_api_key_configured":  typesafeKey != "",
		"typesafe_api_key_masked":      maskedTypesafeKey,
		"typesafe_base_url":            typesafeBaseURL,
		"upstream_proxy_url":           os.Getenv("UPSTREAM_PROXY_URL"),
		"routing_confidence_threshold": routingThreshold,
		"default_leverage":             defaultLeverage,
		"min_risk_to_reward_ratio":     minRiskRewardRatio,
		"max_concurrent_signals":       maxConcurrentSignals,
		"max_risk_per_trade_pct":       maxRiskPerTradePct,
		"max_drawdown_limit_pct":       maxDrawdownLimitPct,
		"calendar_halt_minutes":        calendarHaltMinutes,
		"screener_min_24h_volume":      screenerMin24hVolume,
		"initial_capital":              initialCapital,
		"core_target_pct":              coreTargetPct,
		"alpha_target_pct":             alphaTargetPct,
		"telegram_bot_configured":      telegramBotConfigured,
		"telegram_chat_id":             telegramChatID,
		"early_exit_enabled":           earlyExitEnabled,
		"early_exit_min_hold_min":      earlyExitMinHoldMin,
		"early_exit_max_per_day":       earlyExitMaxPerDay,
		"early_exit_cooldown_min":      earlyExitCooldownMin,
		"early_exit_conf_floor":        earlyExitConfFloor,
		"env_file":                     envFile,
		"parameter_modes":              paramModes,
		"sl_atr_mult":                  slAtrMult,
		"tp_atr_mult":                  tpAtrMult,
		"cluster_decay_mode":           clusterDecayMode,
		"confluence_min":               confluenceMin,
	}
}

// GET /api/v1/system/config
func (s *Server) handleGetSystemConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.systemConfigResponse())
}

var allowedConfigKeys = map[string]bool{
	"ROUTING_CONFIDENCE_THRESHOLD": true,
	"AI_MODEL_ID":                  true,
	"AI_TEMPERATURE":               true,
	"AI_REASONING_EFFORT":          true,
	"AI_TIMEOUT_SECONDS":           true,
	"DEFAULT_LEVERAGE":             true,
	"MIN_RISK_TO_REWARD_RATIO":     true,
	"MAX_CONCURRENT_SIGNALS":       true,
	"MAX_RISK_PER_TRADE_PCT":       true,
	"MAX_DRAWDOWN_LIMIT_PCT":       true,
	"CALENDAR_HALT_MINUTES":        true,
	"TYPESAFE_API_KEY":             true,
	"UPSTREAM_PROXY_URL":           true,
	"SCREENER_MIN_24H_VOLUME":      true,
	"TIMEFRAME_SET_ALPHA":          true,
	"TIMEFRAME_SET_CORE":           true,
	"EARLY_EXIT_ENABLED":           true,
	"EARLY_EXIT_MIN_HOLD_MIN":      true,
	"EARLY_EXIT_MAX_PER_DAY":       true,
	"EARLY_EXIT_COOLDOWN_MIN":      true,
	"EARLY_EXIT_CONF_FLOOR":        true,
	"SL_ATR_MULT":                  true,
	"TP_ATR_MULT":                  true,
	"CLUSTER_DECAY_MODE":           true,
	"CONFLUENCE_MIN":               true,
}

func parseNumericFloat(val interface{}) (float64, error) {
	switch v := val.(type) {
	case float64:
		return v, nil
	case int:
		return float64(v), nil
	case string:
		return strconv.ParseFloat(strings.TrimSpace(v), 64)
	default:
		return 0, fmt.Errorf("invalid numeric value")
	}
}

func parseNumericInt(val interface{}) (int, error) {
	switch v := val.(type) {
	case float64:
		if v != float64(int(v)) {
			return 0, fmt.Errorf("must be an integer")
		}
		return int(v), nil
	case int:
		return v, nil
	case string:
		return strconv.Atoi(strings.TrimSpace(v))
	default:
		return 0, fmt.Errorf("invalid integer value")
	}
}

func parseString(val interface{}) string {
	switch v := val.(type) {
	case string:
		return strings.TrimSpace(v)
	case float64:
		if v == float64(int64(v)) {
			return strconv.FormatInt(int64(v), 10)
		}
		return strconv.FormatFloat(v, 'f', -1, 64)
	case int:
		return strconv.Itoa(v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func parseBool(val interface{}) (bool, error) {
	switch v := val.(type) {
	case bool:
		return v, nil
	case string:
		return strconv.ParseBool(strings.TrimSpace(v))
	case int:
		if v == 1 {
			return true, nil
		} else if v == 0 {
			return false, nil
		}
		return false, fmt.Errorf("invalid boolean integer")
	case float64:
		if v == 1.0 {
			return true, nil
		} else if v == 0.0 {
			return false, nil
		}
		return false, fmt.Errorf("invalid boolean float")
	default:
		return false, fmt.Errorf("invalid boolean value")
	}
}

func isMaskedPlaceholder(val string) bool {
	v := strings.TrimSpace(val)
	if v == "" || v == "***" || strings.Contains(v, "••") {
		return true
	}
	if strings.Contains(v, "...") && len(v) <= 16 {
		return true
	}
	return false
}

func isEmptyValue(val interface{}) bool {
	if val == nil {
		return true
	}
	if s, ok := val.(string); ok && strings.TrimSpace(s) == "" {
		return true
	}
	return false
}

// ensureDecisionRouter makes sure decision router is live.
func (s *Server) ensureDecisionRouter(thr float64) {
	if s.decisionRouter != nil {
		s.decisionRouter.SetThreshold(thr)
		return
	}
	typesafeKey := ""
	if s.cfg != nil {
		typesafeKey = s.cfg.TypesafeAPIKey
	}
	if typesafeKey == "" {
		typesafeKey = os.Getenv("TYPESAFE_API_KEY")
	}
	s.decisionRouter = &trader.DecisionRouter{
		Jev:       ai.NewJevClient("https://api.typesafe.ai", typesafeKey, 12*time.Second),
		Threshold: thr,
		Escalate: func(ctx context.Context, payload interface{}) (trader.DecisionOutcome, error) {
			if s.aiClient == nil {
				return trader.DecisionOutcome{}, fmt.Errorf("ai client unavailable for escalation")
			}
			// Same contract as the boot-time closure: judge the REAL entry
			// payload — an empty body made 9Router answer HOLD for every
			// escalated candidate (2026-09-29 no-signal defect).
			req, perr := trader.EscalationPayload(payload)
			if perr != nil {
				return trader.DecisionOutcome{}, perr
			}
			resp, err := s.aiClient.Analyze(ctx, req)
			if err != nil {
				return trader.DecisionOutcome{}, fmt.Errorf("escalation failed: %w", err)
			}
			out := trader.DecisionOutcome{Confidence: resp.Confidence}
			switch resp.Decision {
			case "BUY":
				out.Choice = "LONG"
			case "SELL":
				out.Choice = "SHORT"
			default:
				out.Choice = "NO_TRADE"
			}
			return out, nil
		},
	}
	if s.shadow != nil {
		s.shadow.Router = s.decisionRouter
	}
}

// PUT /api/v1/system/config
func (s *Server) handlePutSystemConfig(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	token := auth.ExtractToken(r)
	if token == "" || s.authenticator == nil || !s.authenticator.ValidateTokenContext(r.Context(), token) {
		w.WriteHeader(http.StatusUnauthorized)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized: valid session token required"})
		return
	}

	var rawMap map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&rawMap); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid JSON payload: " + err.Error()})
		return
	}

	// 1. Whitelist validation
	for k := range rawMap {
		if !allowedConfigKeys[k] {
			w.WriteHeader(http.StatusBadRequest)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": fmt.Sprintf("key %q is not allowed", k),
			})
			return
		}
	}

	// 2. Validate values and ranges
	envUpdates, err := validateAndBuildEnvUpdates(rawMap)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
		return
	}

	// 3. Persist to .env file
	envFilePath := config.DefaultEnvPath()
	if len(envUpdates) > 0 {
		if err := config.UpsertEnv(envFilePath, envUpdates); err != nil {
			w.WriteHeader(http.StatusInternalServerError)
			_ = json.NewEncoder(w).Encode(map[string]string{
				"error": fmt.Sprintf("failed persisting to %s: %v", envFilePath, err),
			})
			return
		}
	}

	// 4. Apply in-process to live config and runtime services
	s.applyLiveConfigUpdates(envUpdates)

	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(s.systemConfigResponse())
}

func validateAndBuildEnvUpdates(rawMap map[string]interface{}) (map[string]string, error) {
	envUpdates := make(map[string]string)

	if v, exists := rawMap["ROUTING_CONFIDENCE_THRESHOLD"]; exists {
		f, err := parseNumericFloat(v)
		if err != nil || f <= 0 || f > 1.0 {
			return nil, fmt.Errorf("ROUTING_CONFIDENCE_THRESHOLD invalid: must be between 0 (exclusive) and 1 (inclusive)")
		}
		envUpdates["ROUTING_CONFIDENCE_THRESHOLD"] = strconv.FormatFloat(f, 'f', -1, 64)
	}

	if v, exists := rawMap["AI_MODEL_ID"]; exists {
		s := parseString(v)
		if s == "" {
			return nil, fmt.Errorf("AI_MODEL_ID invalid: cannot be empty")
		}
		envUpdates["AI_MODEL_ID"] = s
	}

	if v, exists := rawMap["AI_TEMPERATURE"]; exists {
		f, err := parseNumericFloat(v)
		if err != nil || f < 0.0 || f > 2.0 {
			return nil, fmt.Errorf("AI_TEMPERATURE invalid: must be between 0.0 and 2.0")
		}
		envUpdates["AI_TEMPERATURE"] = strconv.FormatFloat(f, 'f', -1, 64)
	}

	if v, exists := rawMap["AI_REASONING_EFFORT"]; exists {
		s := strings.ToLower(parseString(v))
		if s != "low" && s != "medium" && s != "high" {
			return nil, fmt.Errorf("AI_REASONING_EFFORT invalid: must be one of 'low', 'medium', 'high'")
		}
		envUpdates["AI_REASONING_EFFORT"] = s
	}

	if v, exists := rawMap["AI_TIMEOUT_SECONDS"]; exists {
		i, err := parseNumericInt(v)
		if err != nil || i < 1 || i > 300 {
			return nil, fmt.Errorf("AI_TIMEOUT_SECONDS invalid: must be between 1 and 300")
		}
		envUpdates["AI_TIMEOUT_SECONDS"] = strconv.Itoa(i)
	}

	if v, exists := rawMap["DEFAULT_LEVERAGE"]; exists {
		if isEmptyValue(v) {
			envUpdates["DEFAULT_LEVERAGE"] = ""
		} else {
			i, err := parseNumericInt(v)
			if err != nil || i < 1 || i > 125 {
				return nil, fmt.Errorf("DEFAULT_LEVERAGE invalid: must be between 1 and 125")
			}
			envUpdates["DEFAULT_LEVERAGE"] = strconv.Itoa(i)
		}
	}

	if v, exists := rawMap["MIN_RISK_TO_REWARD_RATIO"]; exists {
		if isEmptyValue(v) {
			envUpdates["MIN_RISK_TO_REWARD_RATIO"] = ""
		} else {
			f, err := parseNumericFloat(v)
			if err != nil || f < 0.5 || f > 20.0 {
				return nil, fmt.Errorf("MIN_RISK_TO_REWARD_RATIO invalid: must be between 0.5 and 20.0")
			}
			envUpdates["MIN_RISK_TO_REWARD_RATIO"] = strconv.FormatFloat(f, 'f', -1, 64)
		}
	}

	if v, exists := rawMap["MAX_CONCURRENT_SIGNALS"]; exists {
		i, err := parseNumericInt(v)
		if err != nil || i < 1 || i > 100 {
			return nil, fmt.Errorf("MAX_CONCURRENT_SIGNALS invalid: must be between 1 and 100")
		}
		envUpdates["MAX_CONCURRENT_SIGNALS"] = strconv.Itoa(i)
	}

	if v, exists := rawMap["MAX_RISK_PER_TRADE_PCT"]; exists {
		if isEmptyValue(v) {
			envUpdates["MAX_RISK_PER_TRADE_PCT"] = ""
		} else {
			f, err := parseNumericFloat(v)
			if err != nil || f <= 0 || f > 1.0 {
				return nil, fmt.Errorf("MAX_RISK_PER_TRADE_PCT invalid: must be between 0 (exclusive) and 1 (inclusive)")
			}
			envUpdates["MAX_RISK_PER_TRADE_PCT"] = strconv.FormatFloat(f, 'f', -1, 64)
		}
	}

	if v, exists := rawMap["MAX_DRAWDOWN_LIMIT_PCT"]; exists {
		f, err := parseNumericFloat(v)
		if err != nil || f <= 0 || f > 1.0 {
			return nil, fmt.Errorf("MAX_DRAWDOWN_LIMIT_PCT invalid: must be between 0 (exclusive) and 1 (inclusive)")
		}
		envUpdates["MAX_DRAWDOWN_LIMIT_PCT"] = strconv.FormatFloat(f, 'f', -1, 64)
	}

	if v, exists := rawMap["CALENDAR_HALT_MINUTES"]; exists {
		i, err := parseNumericInt(v)
		if err != nil || i < 0 || i > 1440 {
			return nil, fmt.Errorf("CALENDAR_HALT_MINUTES invalid: must be between 0 and 1440")
		}
		envUpdates["CALENDAR_HALT_MINUTES"] = strconv.Itoa(i)
	}

	if v, exists := rawMap["UPSTREAM_PROXY_URL"]; exists {
		s := parseString(v)
		if s != "" {
			u, err := url.Parse(s)
			if err != nil || (u.Scheme != "socks5" && u.Scheme != "http" && u.Scheme != "https") {
				return nil, fmt.Errorf("UPSTREAM_PROXY_URL invalid: need socks5://host:port or http://host:port")
			}
		}
		envUpdates["UPSTREAM_PROXY_URL"] = s
	}

	if v, exists := rawMap["TYPESAFE_API_KEY"]; exists {
		s := parseString(v)
		// Masked writes: only overwrite if value non-empty AND not the masked placeholder
		if !isMaskedPlaceholder(s) && s != "" {
			// Zero-fallback: probe TypeSafe with the candidate key BEFORE saving.
			// A bad key must fail loudly here, not at the next trade cycle (401).
			if err := probeTypeSafe(context.Background(), s); err != nil {
				return nil, err
			}
			envUpdates["TYPESAFE_API_KEY"] = s
		}
	}

	if v, exists := rawMap["SCREENER_MIN_24H_VOLUME"]; exists {
		f, err := parseNumericFloat(v)
		if err != nil || f < 0 {
			return nil, fmt.Errorf("SCREENER_MIN_24H_VOLUME invalid: must be >= 0")
		}
		envUpdates["SCREENER_MIN_24H_VOLUME"] = strconv.FormatFloat(f, 'f', -1, 64)
	}

	if v, exists := rawMap["TIMEFRAME_SET_ALPHA"]; exists {
		s := parseString(v)
		set, err := config.ParseTimeframeSet("TIMEFRAME_SET_ALPHA", s)
		if err != nil {
			return nil, err
		}
		envUpdates["TIMEFRAME_SET_ALPHA"] = strings.Join(set, ",")
	}

	if v, exists := rawMap["TIMEFRAME_SET_CORE"]; exists {
		s := parseString(v)
		set, err := config.ParseTimeframeSet("TIMEFRAME_SET_CORE", s)
		if err != nil {
			return nil, err
		}
		envUpdates["TIMEFRAME_SET_CORE"] = strings.Join(set, ",")
	}

	if v, exists := rawMap["EARLY_EXIT_ENABLED"]; exists {
		b, err := parseBool(v)
		if err != nil {
			return nil, fmt.Errorf("EARLY_EXIT_ENABLED invalid: must be a boolean")
		}
		envUpdates["EARLY_EXIT_ENABLED"] = strconv.FormatBool(b)
	}

	if v, exists := rawMap["EARLY_EXIT_MIN_HOLD_MIN"]; exists {
		i, err := parseNumericInt(v)
		if err != nil || i < 0 {
			return nil, fmt.Errorf("EARLY_EXIT_MIN_HOLD_MIN invalid: must be >= 0")
		}
		envUpdates["EARLY_EXIT_MIN_HOLD_MIN"] = strconv.Itoa(i)
	}

	if v, exists := rawMap["EARLY_EXIT_MAX_PER_DAY"]; exists {
		i, err := parseNumericInt(v)
		if err != nil || i < 1 {
			return nil, fmt.Errorf("EARLY_EXIT_MAX_PER_DAY invalid: must be >= 1")
		}
		envUpdates["EARLY_EXIT_MAX_PER_DAY"] = strconv.Itoa(i)
	}

	if v, exists := rawMap["EARLY_EXIT_COOLDOWN_MIN"]; exists {
		i, err := parseNumericInt(v)
		if err != nil || i < 0 {
			return nil, fmt.Errorf("EARLY_EXIT_COOLDOWN_MIN invalid: must be >= 0")
		}
		envUpdates["EARLY_EXIT_COOLDOWN_MIN"] = strconv.Itoa(i)
	}

	if v, exists := rawMap["EARLY_EXIT_CONF_FLOOR"]; exists {
		f, err := parseNumericFloat(v)
		if err != nil || f < 0.0 || f > 1.0 {
			return nil, fmt.Errorf("EARLY_EXIT_CONF_FLOOR invalid: must be between 0.0 and 1.0")
		}
		envUpdates["EARLY_EXIT_CONF_FLOOR"] = strconv.FormatFloat(f, 'f', -1, 64)
	}

	if v, exists := rawMap["SL_ATR_MULT"]; exists {
		if isEmptyValue(v) {
			envUpdates["SL_ATR_MULT"] = ""
		} else {
			f, err := parseNumericFloat(v)
			if err != nil || f <= 0 || f > 20.0 {
				return nil, fmt.Errorf("SL_ATR_MULT invalid: must be between 0 (exclusive) and 20.0")
			}
			envUpdates["SL_ATR_MULT"] = strconv.FormatFloat(f, 'f', -1, 64)
		}
	}

	if v, exists := rawMap["TP_ATR_MULT"]; exists {
		if isEmptyValue(v) {
			envUpdates["TP_ATR_MULT"] = ""
		} else {
			f, err := parseNumericFloat(v)
			if err != nil || f <= 0 || f > 20.0 {
				return nil, fmt.Errorf("TP_ATR_MULT invalid: must be between 0 (exclusive) and 20.0")
			}
			envUpdates["TP_ATR_MULT"] = strconv.FormatFloat(f, 'f', -1, 64)
		}
	}

	if v, exists := rawMap["CLUSTER_DECAY_MODE"]; exists {
		if isEmptyValue(v) {
			envUpdates["CLUSTER_DECAY_MODE"] = ""
		} else {
			s := strings.ToUpper(strings.TrimSpace(parseString(v)))
			if s != "FAST_BREAKING" && s != "MACRO_THEMATIC" {
				return nil, fmt.Errorf("CLUSTER_DECAY_MODE invalid: must be FAST_BREAKING or MACRO_THEMATIC")
			}
			envUpdates["CLUSTER_DECAY_MODE"] = s
		}
	}

	if v, exists := rawMap["CONFLUENCE_MIN"]; exists {
		if isEmptyValue(v) {
			envUpdates["CONFLUENCE_MIN"] = ""
		} else {
			f, err := parseNumericFloat(v)
			if err != nil || f < 0.0 || f > 1.0 {
				return nil, fmt.Errorf("CONFLUENCE_MIN invalid: must be between 0.0 and 1.0")
			}
			envUpdates["CONFLUENCE_MIN"] = strconv.FormatFloat(f, 'f', -1, 64)
		}
	}

	return envUpdates, nil
}

func (s *Server) applyLiveConfigUpdates(envUpdates map[string]string) {
	if s.cfg == nil {
		return
	}
	if v, ok := envUpdates["ROUTING_CONFIDENCE_THRESHOLD"]; ok {
		f, _ := strconv.ParseFloat(v, 64)
		s.cfg.RoutingConfidenceThreshold = v
		_ = os.Setenv("ROUTING_CONFIDENCE_THRESHOLD", v)
		s.ensureDecisionRouter(f)
	}
	if v, ok := envUpdates["AI_MODEL_ID"]; ok {
		s.cfg.AIModelID = v
		_ = os.Setenv("AI_MODEL_ID", v)
	}
	if v, ok := envUpdates["AI_TEMPERATURE"]; ok {
		f, _ := strconv.ParseFloat(v, 64)
		s.cfg.AITemperature = f
		_ = os.Setenv("AI_TEMPERATURE", v)
	}
	if v, ok := envUpdates["AI_REASONING_EFFORT"]; ok {
		s.cfg.AIReasoningEffort = v
		_ = os.Setenv("AI_REASONING_EFFORT", v)
	}
	if v, ok := envUpdates["AI_TIMEOUT_SECONDS"]; ok {
		i, _ := strconv.Atoi(v)
		s.cfg.AITimeoutSeconds = i
		_ = os.Setenv("AI_TIMEOUT_SECONDS", v)
	}
	if v, ok := envUpdates["DEFAULT_LEVERAGE"]; ok {
		if v == "" {
			s.cfg.OverrideDefaultLeverage = nil
			s.cfg.DefaultLeverage = 8
			_ = os.Unsetenv("DEFAULT_LEVERAGE")
		} else {
			i, _ := strconv.Atoi(v)
			s.cfg.OverrideDefaultLeverage = &i
			s.cfg.DefaultLeverage = i
			_ = os.Setenv("DEFAULT_LEVERAGE", v)
		}
	}
	if v, ok := envUpdates["MIN_RISK_TO_REWARD_RATIO"]; ok {
		if v == "" {
			s.cfg.OverrideMinRiskRewardRatio = nil
			s.cfg.MinRiskRewardRatio = 2.5
			_ = os.Unsetenv("MIN_RISK_TO_REWARD_RATIO")
		} else {
			f, _ := strconv.ParseFloat(v, 64)
			s.cfg.OverrideMinRiskRewardRatio = &f
			s.cfg.MinRiskRewardRatio = f
			_ = os.Setenv("MIN_RISK_TO_REWARD_RATIO", v)
		}
	}
	if v, ok := envUpdates["MAX_CONCURRENT_SIGNALS"]; ok {
		i, _ := strconv.Atoi(v)
		s.cfg.MaxConcurrentSignals = i
		_ = os.Setenv("MAX_CONCURRENT_SIGNALS", v)
	}
	if v, ok := envUpdates["MAX_RISK_PER_TRADE_PCT"]; ok {
		if v == "" {
			s.cfg.OverrideMaxRiskPerTradePct = nil
			s.cfg.MaxRiskPerTradePct = 0.02
			_ = os.Unsetenv("MAX_RISK_PER_TRADE_PCT")
		} else {
			f, _ := strconv.ParseFloat(v, 64)
			s.cfg.OverrideMaxRiskPerTradePct = &f
			s.cfg.MaxRiskPerTradePct = f
			_ = os.Setenv("MAX_RISK_PER_TRADE_PCT", v)
		}
	}
	if v, ok := envUpdates["SL_ATR_MULT"]; ok {
		if v == "" {
			s.cfg.OverrideSLAtrMult = nil
			_ = os.Unsetenv("SL_ATR_MULT")
		} else {
			f, _ := strconv.ParseFloat(v, 64)
			s.cfg.OverrideSLAtrMult = &f
			_ = os.Setenv("SL_ATR_MULT", v)
		}
	}
	if v, ok := envUpdates["TP_ATR_MULT"]; ok {
		if v == "" {
			s.cfg.OverrideTPAtrMult = nil
			_ = os.Unsetenv("TP_ATR_MULT")
		} else {
			f, _ := strconv.ParseFloat(v, 64)
			s.cfg.OverrideTPAtrMult = &f
			_ = os.Setenv("TP_ATR_MULT", v)
		}
	}
	if v, ok := envUpdates["CLUSTER_DECAY_MODE"]; ok {
		if v == "" {
			s.cfg.OverrideClusterDecayMode = ""
			_ = os.Unsetenv("CLUSTER_DECAY_MODE")
		} else {
			s.cfg.OverrideClusterDecayMode = v
			_ = os.Setenv("CLUSTER_DECAY_MODE", v)
		}
	}
	if v, ok := envUpdates["CONFLUENCE_MIN"]; ok {
		if v == "" {
			s.cfg.OverrideConfluenceMin = nil
			_ = os.Unsetenv("CONFLUENCE_MIN")
		} else {
			f, _ := strconv.ParseFloat(v, 64)
			s.cfg.OverrideConfluenceMin = &f
			_ = os.Setenv("CONFLUENCE_MIN", v)
		}
	}
	if v, ok := envUpdates["MAX_DRAWDOWN_LIMIT_PCT"]; ok {
		f, _ := strconv.ParseFloat(v, 64)
		s.cfg.MaxDrawdownLimitPct = f
		_ = os.Setenv("MAX_DRAWDOWN_LIMIT_PCT", v)
	}
	if v, ok := envUpdates["CALENDAR_HALT_MINUTES"]; ok {
		i, _ := strconv.Atoi(v)
		s.cfg.CalendarHaltMinutes = i
		_ = os.Setenv("CALENDAR_HALT_MINUTES", v)
	}
	if v, ok := envUpdates["UPSTREAM_PROXY_URL"]; ok {
		_ = os.Setenv("UPSTREAM_PROXY_URL", v)
	}
	if v, ok := envUpdates["TYPESAFE_API_KEY"]; ok {
		s.cfg.TypesafeAPIKey = v
		_ = os.Setenv("TYPESAFE_API_KEY", v)
		if s.decisionRouter != nil && s.decisionRouter.Jev != nil {
			s.decisionRouter.Jev.SetAPIKey(v)
		}
	}
	if v, ok := envUpdates["SCREENER_MIN_24H_VOLUME"]; ok {
		f, _ := strconv.ParseFloat(v, 64)
		s.cfg.ScreenerMin24hVolume = f
		_ = os.Setenv("SCREENER_MIN_24H_VOLUME", v)
	}
	if v, ok := envUpdates["TIMEFRAME_SET_ALPHA"]; ok {
		_ = os.Setenv("TIMEFRAME_SET_ALPHA", v)
		_ = config.UpdateTimeframeSet("TIMEFRAME_SET_ALPHA", v)
	}
	if v, ok := envUpdates["TIMEFRAME_SET_CORE"]; ok {
		_ = os.Setenv("TIMEFRAME_SET_CORE", v)
		_ = config.UpdateTimeframeSet("TIMEFRAME_SET_CORE", v)
	}
	if v, ok := envUpdates["EARLY_EXIT_ENABLED"]; ok {
		b, _ := strconv.ParseBool(v)
		s.cfg.EarlyExit.Enabled = b
		_ = os.Setenv("EARLY_EXIT_ENABLED", v)
	}
	if v, ok := envUpdates["EARLY_EXIT_MIN_HOLD_MIN"]; ok {
		i, _ := strconv.Atoi(v)
		s.cfg.EarlyExit.MinHoldMin = i
		_ = os.Setenv("EARLY_EXIT_MIN_HOLD_MIN", v)
	}
	if v, ok := envUpdates["EARLY_EXIT_MAX_PER_DAY"]; ok {
		i, _ := strconv.Atoi(v)
		s.cfg.EarlyExit.MaxPerDay = i
		_ = os.Setenv("EARLY_EXIT_MAX_PER_DAY", v)
	}
	if v, ok := envUpdates["EARLY_EXIT_COOLDOWN_MIN"]; ok {
		i, _ := strconv.Atoi(v)
		s.cfg.EarlyExit.CooldownMin = i
		_ = os.Setenv("EARLY_EXIT_COOLDOWN_MIN", v)
	}
	if v, ok := envUpdates["EARLY_EXIT_CONF_FLOOR"]; ok {
		f, _ := strconv.ParseFloat(v, 64)
		s.cfg.EarlyExit.ConfFloor = f
		_ = os.Setenv("EARLY_EXIT_CONF_FLOOR", v)
	}
	if s.earlyExitManager != nil {
		s.earlyExitManager.UpdateConfig(s.cfg.EarlyExit)
	}
	if s.aiClient != nil {
		s.aiClient.SetParams(s.cfg.AIModelID, s.cfg.AIReasoningEffort, s.cfg.AITemperature, s.cfg.AITimeoutSeconds)
	}
}


// probeTypeSafe validates a candidate API key with one cheap live call so a
// rejected key never lands in .env (explicit save-time error, FR-007).
func probeTypeSafe(ctx context.Context, key string) error {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	body := `{"state":"key probe","model":"jev-latest","questions":{"ok":{"type":"noul","instructions":"ok?"}}}`
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.typesafe.ai/v1/systemone", strings.NewReader(body))
	if err != nil {
		return fmt.Errorf("TYPESAFE_API_KEY validation could not run: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+key)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("TYPESAFE_API_KEY validation unreachable: %v", err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case 200:
		return nil
	case 401, 403:
		return fmt.Errorf("TYPESAFE_API_KEY rejected by TypeSafe (http %d): key is invalid — fix the value and save again", resp.StatusCode)
	default:
		return fmt.Errorf("TYPESAFE_API_KEY validation failed: TypeSafe http %d", resp.StatusCode)
	}
}
