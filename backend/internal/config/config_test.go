package config

import "testing"

// Основная модель — Claude, тень — DeepSeek: ключи по умолчанию берутся у своего провайдера, тень без зрения
// получает описание фото.
func TestProviderDefaults(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://localhost/test")
	t.Setenv("ANTHROPIC_API_KEY", "claude-key")
	t.Setenv("DEEPSEEK_API_KEY", "deepseek-key")
	t.Setenv("TNVED_AI_PROVIDER", "Anthropic")
	t.Setenv("TNVED_AI_SHADOW_PROVIDER", "deepseek")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.TNVEDAIProvider != "anthropic" || cfg.TNVEDAIAPIKey != "claude-key" {
		t.Fatalf("основная: %q %q", cfg.TNVEDAIProvider, cfg.TNVEDAIAPIKey)
	}
	if cfg.TNVEDAIShadowAPIKey != "deepseek-key" || cfg.TNVEDAIShadowVision {
		t.Fatalf("тень: ключ %q, фото %v", cfg.TNVEDAIShadowAPIKey, cfg.TNVEDAIShadowVision)
	}
	t.Setenv("TNVED_AI_PROVIDER", "")
	t.Setenv("TNVED_AI_SHADOW_PROVIDER", "")
	if cfg, _ = Load(); cfg.TNVEDAIAPIKey != "deepseek-key" || cfg.TNVEDAIShadowAPIKey != "claude-key" || !cfg.TNVEDAIShadowVision {
		t.Fatalf("как раньше: %q %q %v", cfg.TNVEDAIAPIKey, cfg.TNVEDAIShadowAPIKey, cfg.TNVEDAIShadowVision)
	}
}
