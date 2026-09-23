package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL       string
	Port              string
	SessionCookieName string
	SessionTTL        time.Duration
	CookieSecure      bool
	FileStorageDir    string
	AIProvider        string
	GeminiAPIKey      string
	GeminiModel       string
	AnthropicAPIKey   string
	AnthropicModel    string
}

func Load() (Config, error) {
	ttlHours, err := positiveIntEnv("SESSION_TTL_HOURS", 24)
	if err != nil {
		return Config{}, err
	}

	cookieSecure, err := boolEnv("COOKIE_SECURE", false)
	if err != nil {
		return Config{}, err
	}

	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}

	return Config{
		DatabaseURL:       databaseURL,
		Port:              stringEnv("BACKEND_PORT", "8080"),
		SessionCookieName: stringEnv("SESSION_COOKIE_NAME", "import_calculator_session"),
		SessionTTL:        time.Duration(ttlHours) * time.Hour,
		CookieSecure:      cookieSecure,
		FileStorageDir:    stringEnv("FILE_STORAGE_DIR", "./data/uploads"),
		AIProvider:        stringEnv("AI_PROVIDER", "anthropic"),
		GeminiAPIKey:      strings.TrimSpace(os.Getenv("GEMINI_API_KEY")),
		GeminiModel:       stringEnv("GEMINI_MODEL", "gemini-3.8-flash"),
		AnthropicAPIKey:   strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY")),
		AnthropicModel:    stringEnv("ANTHROPIC_MODEL", "claude-haiku-4-5-20251001"),
	}, nil
}

func stringEnv(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func positiveIntEnv(name string, fallback int) (int, error) {
	value := stringEnv(name, strconv.Itoa(fallback))
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", name)
	}
	return parsed, nil
}

func boolEnv(name string, fallback bool) (bool, error) {
	value := stringEnv(name, strconv.FormatBool(fallback))
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be true or false", name)
	}
	return parsed, nil
}
