// Package config loads Golo's runtime configuration from environment variables.
// It has zero dependencies on other internal packages so anything may import it.
package config

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config holds all environment-derived settings for the golo process.
type Config struct {
	// Provider selects which data provider implementation to run: "mock", "replay", or "sportmonks".
	Provider string

	SportMonksAPIKey  string
	SportMonksBaseURL string

	Port      string
	DBPath    string
	ModelPath string

	FirebaseDatabaseURL   string
	FirebaseAuth          string
	OddsAPIKey            string
	OddsAPIBaseURL        string
	OddsProvider          string
	OddsBookmaker         string
	OddsSportKeys         []string
	OddsPollInterval      time.Duration
	AlertEngineEnabled    bool
	TelegramEnabled       bool
	TelegramBotToken      string
	TelegramWebhookSecret string
	AdminPasswordHash     string
	AdminSessionSecret    string
	AllowedWebOrigin      string
	HistoricalDatasetPath string

	PollInterval time.Duration

	// PriorityCompetitions holds provider competition IDs (e.g. SportMonks
	// league IDs) that should be surfaced first when multiple matches are
	// live at once. Order matters: earlier entries are higher priority.
	// Empty means no reordering — matches are returned in provider order.
	// This does NOT restrict which leagues the provider fetches; SportMonks
	// scopes that at the account level (you pick leagues in their dashboard
	// when subscribing to a plan), not via an API parameter.
	PriorityCompetitions []string
}

// Load reads configuration from the environment, applying defaults for anything unset.
// It never fails — missing provider credentials are only a problem once that provider
// is actually selected, which is validated by the caller wiring up the provider.
//
// Before reading env vars, it loads ./.env if present (real environment
// variables always take priority over the file, matching standard dotenv
// semantics) — without this, the README's "cp .env.example .env" quickstart
// would silently do nothing for local runs.
func Load() Config {
	loadDotEnv(".env")

	return Config{
		Provider: getEnv("PROVIDER", "mock"),

		SportMonksAPIKey:  getEnv("SPORTMONKS_API_KEY", ""),
		SportMonksBaseURL: getEnv("SPORTMONKS_BASE_URL", "https://api.sportmonks.com/v3/football"),

		Port:      getEnv("PORT", "8080"),
		DBPath:    getEnv("DB_PATH", "./golo.db"),
		ModelPath: getEnv("MODEL_PATH", "./models/hazard_v1.json"),

		FirebaseDatabaseURL:   getEnv("FIREBASE_DATABASE_URL", ""),
		FirebaseAuth:          getEnv("FIREBASE_AUTH", ""),
		OddsAPIKey:            getEnv("ODDS_API_KEY", ""),
		OddsAPIBaseURL:        getEnv("ODDS_API_BASE_URL", "https://api.the-odds-api.com/v4"),
		OddsProvider:          getEnv("ODDS_PROVIDER", "the-odds-api"),
		OddsBookmaker:         getEnv("ODDS_BOOKMAKER", "Betsson"),
		OddsSportKeys:         getEnvListWithDefault("ODDS_SPORT_KEYS", []string{"soccer_brazil_campeonato", "soccer_conmebol_copa_libertadores", "soccer_usa_mls", "soccer_mexico_ligamx"}),
		OddsPollInterval:      getEnvDuration("ODDS_POLL_INTERVAL_SECONDS", 60*time.Second),
		AlertEngineEnabled:    getEnvBool("ALERT_ENGINE_ENABLED", false),
		TelegramEnabled:       getEnvBool("TELEGRAM_ENABLED", false),
		TelegramBotToken:      getEnv("TELEGRAM_BOT_TOKEN", ""),
		TelegramWebhookSecret: getEnv("TELEGRAM_WEBHOOK_SECRET", ""),
		AdminPasswordHash:     getEnv("ADMIN_PASSWORD_HASH", ""),
		AdminSessionSecret:    getEnv("ADMIN_SESSION_SECRET", ""),
		AllowedWebOrigin:      getEnv("ALLOWED_WEB_ORIGIN", ""),
		HistoricalDatasetPath: getEnv("HISTORICAL_DATASET_PATH", "./ml/data/fixtures.jsonl"),

		PollInterval: getEnvDuration("PROVIDER_POLL_INTERVAL_SECONDS", 3*time.Second),

		PriorityCompetitions: getEnvList("PRIORITY_COMPETITION_IDS"),
	}
}

func getEnvBool(key string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	switch value {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

// getEnvList splits a comma-separated env var into a trimmed, non-empty list.
func getEnvList(key string) []string {
	raw := os.Getenv(key)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func getEnvListWithDefault(key string, fallback []string) []string {
	if values := getEnvList(key); len(values) > 0 {
		return values
	}
	return append([]string(nil), fallback...)
}

// loadDotEnv parses a simple KEY=VALUE file (one per line, '#' comments,
// blank lines ignored) and calls os.Setenv for any key not already set in
// the real environment. Missing file is not an error — .env is optional.
func loadDotEnv(path string) {
	f, err := os.Open(path)
	if err != nil {
		return
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)

		if _, alreadySet := os.LookupEnv(key); !alreadySet {
			os.Setenv(key, value)
		}
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvDuration(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	seconds, err := strconv.Atoi(v)
	if err != nil || seconds <= 0 {
		return fallback
	}
	return time.Duration(seconds) * time.Second
}
