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
	DeepSeekAPIKey    string
	DeepSeekModel     string
	// Ceramics module AI: any OpenAI-compatible Chat Completions API. Empty key and model fall
	// back to DeepSeek; CeramicsAIVision sends product photos (deepseek-flash accepts images).
	CeramicsAIAPIKey  string
	CeramicsAIBaseURL string
	CeramicsAIModel   string
	CeramicsAIVision  bool
	// ИИ-агент подбора кода ТН ВЭД: OpenAI-совместимый API с инструментами и фото (по умолчанию
	// DeepSeek Flash). Цены — USD за 1 млн токенов, только для учёта расхода.
	TNVEDAIAPIKey      string
	TNVEDAIBaseURL     string
	TNVEDAIModel       string
	TNVEDAIVision      bool
	TNVEDAIDailyLimit  int
	TNVEDAINamingRule  string
	TNVEDAIPriceInput  float64
	TNVEDAIPriceCached float64
	TNVEDAIPriceOutput float64
	// RegistrationDisabled: запретить самостоятельную регистрацию (ALLOW_REGISTRATION=false).
	// На публичном сервере сотрудников заводит bootstrap-admin.
	RegistrationDisabled bool
	// StaticDir: папка собранного фронтенда. Если задана, сервер сам отдаёт сайт
	// (один адрес для сайта и API). На сервере сайт отдаёт Vercel, поэтому там не задаётся.
	StaticDir string
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

	allowRegistration, err := boolEnv("ALLOW_REGISTRATION", true)
	if err != nil {
		return Config{}, err
	}

	ceramicsVision, err := boolEnv("CERAMICS_AI_VISION", true)
	if err != nil {
		return Config{}, err
	}

	tnvedVision, err := boolEnv("TNVED_AI_VISION", true)
	if err != nil {
		return Config{}, err
	}
	tnvedDailyLimit, err := positiveIntEnv("TNVED_AI_DAILY_LIMIT", 100)
	if err != nil {
		return Config{}, err
	}
	prices := map[string]float64{"TNVED_AI_PRICE_INPUT": 0.30, "TNVED_AI_PRICE_CACHED": 0.006, "TNVED_AI_PRICE_OUTPUT": 1.20}
	for name, fallback := range prices {
		value, err := nonNegativeFloatEnv(name, fallback)
		if err != nil {
			return Config{}, err
		}
		prices[name] = value
	}

	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}

	return Config{
		DatabaseURL: databaseURL,
		// PORT задают хостинги вроде Render; BACKEND_PORT — docker-compose.
		Port:                 stringEnv("BACKEND_PORT", stringEnv("PORT", "8080")),
		SessionCookieName:    stringEnv("SESSION_COOKIE_NAME", "import_calculator_session"),
		SessionTTL:           time.Duration(ttlHours) * time.Hour,
		CookieSecure:         cookieSecure,
		DeepSeekAPIKey:       strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY")),
		DeepSeekModel:        stringEnv("DEEPSEEK_MODEL", "deepseek-flash"),
		CeramicsAIAPIKey:     stringEnv("CERAMICS_AI_API_KEY", strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY"))),
		CeramicsAIBaseURL:    stringEnv("CERAMICS_AI_BASE_URL", "https://api.deepseek.com"),
		CeramicsAIModel:      stringEnv("CERAMICS_AI_MODEL", stringEnv("DEEPSEEK_MODEL", "deepseek-flash")),
		CeramicsAIVision:     ceramicsVision,
		TNVEDAIAPIKey:        stringEnv("TNVED_AI_API_KEY", strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY"))),
		TNVEDAIBaseURL:       stringEnv("TNVED_AI_BASE_URL", "https://api.deepseek.com"),
		TNVEDAIModel:         stringEnv("TNVED_AI_MODEL", "deepseek-flash"),
		TNVEDAIVision:        tnvedVision,
		TNVEDAIDailyLimit:    tnvedDailyLimit,
		TNVEDAINamingRule:    strings.TrimSpace(os.Getenv("TNVED_AI_NAMING_RULE")),
		TNVEDAIPriceInput:    prices["TNVED_AI_PRICE_INPUT"],
		TNVEDAIPriceCached:   prices["TNVED_AI_PRICE_CACHED"],
		TNVEDAIPriceOutput:   prices["TNVED_AI_PRICE_OUTPUT"],
		RegistrationDisabled: !allowRegistration,
		StaticDir:            strings.TrimSpace(os.Getenv("STATIC_DIR")),
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

func nonNegativeFloatEnv(name string, fallback float64) (float64, error) {
	value := stringEnv(name, strconv.FormatFloat(fallback, 'f', -1, 64))
	parsed, err := strconv.ParseFloat(strings.ReplaceAll(value, ",", "."), 64)
	if err != nil || parsed < 0 {
		return 0, fmt.Errorf("%s must be a non-negative number", name)
	}
	return parsed, nil
}
