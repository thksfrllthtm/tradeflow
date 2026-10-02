package tnvedagent

import (
	"strings"
	"testing"
)

func TestRussianNamePartDropsChineseTranslation(t *testing.T) {
	cases := map[string]string{
		"Перчатки рабочие трикотажные покрытые резиной / 浸胶针织劳保手套": "Перчатки рабочие трикотажные покрытые резиной",
		"Кружка керамическая для взрослых/成人陶瓷杯":                   "Кружка керамическая для взрослых",
		"  Ручной инструмент: Ключ 8/10 мм  ":                      "Ручной инструмент: Ключ 8/10 мм",
		"Перчатки хозяйственные":                                   "Перчатки хозяйственные",
	}
	for input, want := range cases {
		if got := russianNamePart(input); got != want {
			t.Errorf("russianNamePart(%q) = %q, want %q", input, got, want)
		}
	}
}

// Агент пишет только русскую часть, а перевод — отдельным запросом с образцами компании.
func TestNamingRuleWithoutChineseAndTranslatePromptWithSamples(t *testing.T) {
	if hasHan(DefaultTNVEDNamingRule) || !strings.Contains(DefaultTNVEDNamingRule, "Перчатки рабочие трикотажные покрытые резиной") {
		t.Fatal("naming rule for the agent must have Russian samples only")
	}
	if !strings.Contains(translateNamePrompt, "Перчатки рабочие трикотажные покрытые резиной / 浸胶针织劳保手套") {
		t.Fatal("translate prompt must have the company samples with translation")
	}
}
