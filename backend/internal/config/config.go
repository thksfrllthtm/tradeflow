package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
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
	// ИИ-агент подбора кода ТН ВЭД: OpenAI-совместимый API с инструментами (по умолчанию
	// DeepSeek Flash). Цены — USD за 1 млн токенов, только для учёта расхода.
	TNVEDAIAPIKey      string
	TNVEDAIBaseURL     string
	TNVEDAIModel       string
	TNVEDAIVision      bool
	TNVEDAIDailyLimit  int
	TNVEDAINamingRule  string
	TNVEDAIPriceInput  decimal.Decimal
	TNVEDAIPriceCached decimal.Decimal
	TNVEDAIPriceOutput decimal.Decimal
	// Модель для фото (TNVED_AI_VISION_MODEL, на проде gemini-3.8-flash через OpenAI-совместимый адрес
	// Gemini): описание фото, вопросы по фото, похожие товары, перевод наименования. Пусто — всё делает
	// модель TNVED_AI_MODEL, как раньше.
	TNVEDAIVisionModel       string
	TNVEDAIVisionAPIKey      string
	TNVEDAIVisionBaseURL     string
	TNVEDAIVisionPriceInput  decimal.Decimal
	TNVEDAIVisionPriceCached decimal.Decimal
	TNVEDAIVisionPriceOutput decimal.Decimal
	// TNVEDAIEmbedModel — модель эмбеддингов на адресе и ключе модели для фото (TNVED_AI_EMBED_MODEL,
	// gemini-embedding-001): поиск похожих товаров и записей базы решений по смыслу. Пусто — только по словам.
	TNVEDAIEmbedModel string
	// TNVEDAIEffort — уровень размышлений модели агента (TNVED_AI_EFFORT: low, high, max …; пусто — по умолчанию).
	TNVEDAIEffort string
	// TNVEDAIDocumentsEffort — уровень размышлений под-агентов «Документов для ввоза» (TNVED_AI_DOCUMENTS_EFFORT);
	// пусто — как TNVEDAIEffort.
	TNVEDAIDocumentsEffort string
	// Теневой подбор (TNVED_AI_SHADOW_*): вторая модель подбирает код параллельно с основной, сотрудник её не
	// видит, итоги сравнивает администратор в журнале агента. Без TNVED_AI_SHADOW_MODEL тени нет. Provider —
	// anthropic (Claude, ключ по умолчанию ANTHROPIC_API_KEY) или openai (OpenAI-совместимый адрес BaseURL).
	// Vision — модель смотрит фото сама; иначе получает описание от модели для фото, как основная.
	// BudgetUSD — потолок расхода тени за всё время (0 — тень выключена), DailyLimit — прогонов в сутки.
	TNVEDAIShadowProvider    string
	TNVEDAIShadowModel       string
	TNVEDAIShadowAPIKey      string
	TNVEDAIShadowBaseURL     string
	TNVEDAIShadowEffort      string
	TNVEDAIShadowVision      bool
	TNVEDAIShadowPriceInput  decimal.Decimal
	TNVEDAIShadowPriceCached decimal.Decimal
	TNVEDAIShadowPriceOutput decimal.Decimal
	TNVEDAIShadowBudgetUSD   decimal.Decimal
	TNVEDAIShadowDailyLimit  int
	// «Переводчик» (.docx): OpenAI-совместимый API, по умолчанию DeepSeek с ключом DEEPSEEK_API_KEY.
	TranslatorAIAPIKey  string
	TranslatorAIBaseURL string
	TranslatorAIModel   string
	// TavilyAPIKey — поиск в интернете для подбора кода ТН ВЭД ("" — без интернета).
	TavilyAPIKey string
	// RegistrationDisabled: запретить самостоятельную регистрацию (ALLOW_REGISTRATION=false).
	// На публичном сервере сотрудников заводит bootstrap-admin.
	RegistrationDisabled bool
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
	shadowVision, err := boolEnv("TNVED_AI_SHADOW_VISION", true)
	if err != nil {
		return Config{}, err
	}
	shadowDailyLimit, err := positiveIntEnv("TNVED_AI_SHADOW_DAILY_LIMIT", 50)
	if err != nil {
		return Config{}, err
	}
	prices := map[string]decimal.Decimal{}
	for name, fallback := range map[string]string{
		"TNVED_AI_PRICE_INPUT": "0.30", "TNVED_AI_PRICE_CACHED": "0.006", "TNVED_AI_PRICE_OUTPUT": "1.20",
		"TNVED_AI_VISION_PRICE_INPUT": "0.30", "TNVED_AI_VISION_PRICE_CACHED": "0.006", "TNVED_AI_VISION_PRICE_OUTPUT": "1.20",
		// Цены тени по умолчанию — Claude Sonnet 5.5 (прайс Anthropic, 2026-10).
		"TNVED_AI_SHADOW_PRICE_INPUT": "2", "TNVED_AI_SHADOW_PRICE_CACHED": "0.20", "TNVED_AI_SHADOW_PRICE_OUTPUT": "10",
		"TNVED_AI_SHADOW_BUDGET_USD": "0",
	} {
		value, err := nonNegativeDecimalEnv(name, fallback)
		if err != nil {
			return Config{}, err
		}
		prices[name] = value
	}
	tnvedAPIKey := stringEnv("TNVED_AI_API_KEY", strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY")))
	tnvedBaseURL := stringEnv("TNVED_AI_BASE_URL", "https://api.deepseek.com")

	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	if databaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}

	return Config{
		DatabaseURL: databaseURL,
		// PORT задают хостинги вроде Render; BACKEND_PORT — docker-compose.
		Port:                     stringEnv("BACKEND_PORT", stringEnv("PORT", "8080")),
		SessionCookieName:        stringEnv("SESSION_COOKIE_NAME", "import_calculator_session"),
		SessionTTL:               time.Duration(ttlHours) * time.Hour,
		CookieSecure:             cookieSecure,
		DeepSeekAPIKey:           strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY")),
		DeepSeekModel:            stringEnv("DEEPSEEK_MODEL", "deepseek-flash"),
		CeramicsAIAPIKey:         stringEnv("CERAMICS_AI_API_KEY", strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY"))),
		CeramicsAIBaseURL:        stringEnv("CERAMICS_AI_BASE_URL", "https://api.deepseek.com"),
		CeramicsAIModel:          stringEnv("CERAMICS_AI_MODEL", stringEnv("DEEPSEEK_MODEL", "deepseek-flash")),
		CeramicsAIVision:         ceramicsVision,
		TNVEDAIAPIKey:            tnvedAPIKey,
		TNVEDAIBaseURL:           tnvedBaseURL,
		TNVEDAIModel:             stringEnv("TNVED_AI_MODEL", "deepseek-flash"),
		TranslatorAIAPIKey:       stringEnv("TRANSLATOR_AI_API_KEY", strings.TrimSpace(os.Getenv("DEEPSEEK_API_KEY"))),
		TranslatorAIBaseURL:      stringEnv("TRANSLATOR_AI_BASE_URL", "https://api.deepseek.com"),
		TranslatorAIModel:        stringEnv("TRANSLATOR_AI_MODEL", stringEnv("DEEPSEEK_MODEL", "deepseek-flash")),
		TavilyAPIKey:             strings.TrimSpace(os.Getenv("TAVILY_API_KEY")),
		TNVEDAIEffort:            strings.ToLower(strings.TrimSpace(os.Getenv("TNVED_AI_EFFORT"))),
		TNVEDAIDocumentsEffort:   strings.ToLower(strings.TrimSpace(os.Getenv("TNVED_AI_DOCUMENTS_EFFORT"))),
		TNVEDAIVision:            tnvedVision,
		TNVEDAIDailyLimit:        tnvedDailyLimit,
		TNVEDAINamingRule:        strings.TrimSpace(os.Getenv("TNVED_AI_NAMING_RULE")),
		TNVEDAIPriceInput:        prices["TNVED_AI_PRICE_INPUT"],
		TNVEDAIPriceCached:       prices["TNVED_AI_PRICE_CACHED"],
		TNVEDAIPriceOutput:       prices["TNVED_AI_PRICE_OUTPUT"],
		TNVEDAIVisionModel:       strings.TrimSpace(os.Getenv("TNVED_AI_VISION_MODEL")),
		TNVEDAIVisionAPIKey:      stringEnv("TNVED_AI_VISION_API_KEY", tnvedAPIKey),
		TNVEDAIVisionBaseURL:     stringEnv("TNVED_AI_VISION_BASE_URL", tnvedBaseURL),
		TNVEDAIVisionPriceInput:  prices["TNVED_AI_VISION_PRICE_INPUT"],
		TNVEDAIVisionPriceCached: prices["TNVED_AI_VISION_PRICE_CACHED"],
		TNVEDAIVisionPriceOutput: prices["TNVED_AI_VISION_PRICE_OUTPUT"],
		TNVEDAIEmbedModel:        strings.TrimSpace(os.Getenv("TNVED_AI_EMBED_MODEL")),
		TNVEDAIShadowProvider:    strings.ToLower(stringEnv("TNVED_AI_SHADOW_PROVIDER", "anthropic")),
		TNVEDAIShadowModel:       strings.TrimSpace(os.Getenv("TNVED_AI_SHADOW_MODEL")),
		TNVEDAIShadowAPIKey:      stringEnv("TNVED_AI_SHADOW_API_KEY", strings.TrimSpace(os.Getenv("ANTHROPIC_API_KEY"))),
		TNVEDAIShadowBaseURL:     strings.TrimSpace(os.Getenv("TNVED_AI_SHADOW_BASE_URL")),
		TNVEDAIShadowEffort:      strings.ToLower(stringEnv("TNVED_AI_SHADOW_EFFORT", "high")),
		TNVEDAIShadowVision:      shadowVision,
		TNVEDAIShadowPriceInput:  prices["TNVED_AI_SHADOW_PRICE_INPUT"],
		TNVEDAIShadowPriceCached: prices["TNVED_AI_SHADOW_PRICE_CACHED"],
		TNVEDAIShadowPriceOutput: prices["TNVED_AI_SHADOW_PRICE_OUTPUT"],
		TNVEDAIShadowBudgetUSD:   prices["TNVED_AI_SHADOW_BUDGET_USD"],
		TNVEDAIShadowDailyLimit:  shadowDailyLimit,
		RegistrationDisabled:     !allowRegistration,
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

func nonNegativeDecimalEnv(name, fallback string) (decimal.Decimal, error) {
	parsed, err := decimal.NewFromString(strings.ReplaceAll(stringEnv(name, fallback), ",", "."))
	if err != nil || parsed.IsNegative() {
		return decimal.Zero, fmt.Errorf("%s must be a non-negative number", name)
	}
	return parsed, nil
}
