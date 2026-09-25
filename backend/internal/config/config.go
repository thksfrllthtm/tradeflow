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
		DeepSeekModel:        stringEnv("DEEPSEEK_MODEL", "deepseek-v4-pro"),
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
