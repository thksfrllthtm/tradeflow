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

func TestCleanItemKeepsOtherNameColumns(t *testing.T) {
	item := cleanItem(AgentItemInput{Name: "桶红本色头，0.7公斤", OtherNames: []string{" 0,7 Лопата оран штыковой B-1 ", "桶红本色头，0.7公斤", "", "третье", "четвёртое"}})
	if len(item.OtherNames) != 2 || item.OtherNames[0] != "0,7 Лопата оран штыковой B-1" || item.OtherNames[1] != "третье" {
		t.Fatalf("other names: %q", item.OtherNames)
	}
	if names := item.names(); len(names) != 4 || names[2] != "0,7 Лопата оран штыковой B-1" {
		t.Fatalf("all names for the base search: %q", names)
	}
	message := initialAgentMessage(item, 1, nil)
	text := message["content"].([]map[string]any)[0]["text"].(string)
	if !strings.Contains(text, "Лопата оран штыковой") {
		t.Fatal("the agent must get the other name columns")
	}
}
