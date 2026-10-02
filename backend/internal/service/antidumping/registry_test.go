package antidumping

import (
	"strings"
	"testing"
)

func seedMeasures(t *testing.T) map[string]Measure {
	t.Helper()
	measures, err := ParseRegistry(seedRegistry)
	if err != nil {
		t.Fatal(err)
	}
	byNumber := map[string]Measure{}
	for _, measure := range measures {
		byNumber[measure.Number] = measure
	}
	return byNumber
}

// Ставки «для прочих производителей» по странам — как в тексте реестра ЕЭК (сверено вручную).
func TestParseRegistryCountryRates(t *testing.T) {
	measures := seedMeasures(t)
	if len(measures) != 28 {
		t.Fatalf("действующих мер: %d, ожидалось 28", len(measures))
	}
	cases := map[string]map[string]string{
		"AD-41": {"CN": "17.23"},
		"AD-26": {"CN": "17", "UA": "23.9"},
		"AD-28": {"AZ": "16.18", "CN": "13.14"},
		"AD-37": {"TR": "35.29", "JP": "36.54", "TH": "42.82", "MY": "9.38"},
		"AD-11": {"CN": "29.39", "MY": "29.39"},
		"AD-16": {"CN": "31"},
		"AD-18": {"CN": "35.35"},
		"AD-38": {"CN": "16.25"},
		"AD-32": {"CN": "21.89"},
	}
	for number, want := range cases {
		measure := measures[number]
		for country, rate := range want {
			if measure.Rates[country] != rate {
				t.Errorf("%s %s: ставка %q, ожидалась %q (все: %v)", number, country, measure.Rates[country], rate, measure.Rates)
			}
		}
	}
	if !measures["AD-38"].Suspended || measures["AD-41"].Suspended {
		t.Fatal("диоксид титана временно не применяется, свечи — применяются")
	}
	if measures["AD-1"].NeedsCheck == "" {
		t.Fatal("у стальных труб ставка зависит от вида трубы — нужна проверка")
	}
	if measures["AD-41"].DocumentURL == "" || measures["AD-41"].EndDate != "2031-05-09" {
		t.Fatalf("документ и срок: %q %q", measures["AD-41"].DocumentURL, measures["AD-41"].EndDate)
	}
}

func TestResolveByCodeAndOrigin(t *testing.T) {
	all := []Measure{}
	for _, measure := range seedMeasures(t) {
		all = append(all, measure)
	}
	applied := Resolve(all, "8511100001", "Китай")
	if applied.Percent != "17.23" || applied.Warning != "" || applied.Info["status"] != "applied" {
		t.Fatalf("свечи из Китая: %+v", applied)
	}
	if noOrigin := Resolve(all, "8511100001", ""); noOrigin.Percent != "" || !strings.Contains(noOrigin.Warning, "укажите страну") {
		t.Fatalf("без страны — не начислять и попросить страну: %+v", noOrigin)
	}
	if other := Resolve(all, "8511100001", "Германия"); other.Percent != "" || other.Warning != "" || other.Info["status"] != "other-country" {
		t.Fatalf("свечи не из Китая: %+v", other)
	}
	if wheels := Resolve(all, "8708701009", "Таиланд"); wheels.Percent != "" && wheels.Percent != "42.82" {
		t.Fatalf("диски из Таиланда: %+v", wheels)
	}
	if titanium := Resolve(all, "3206111000", "КНР"); titanium.Percent != "" {
		t.Fatalf("диоксид титана временно не облагается: %+v", titanium)
	}
	if gloves := Resolve(all, "6116108000", "Китай"); gloves.Percent != "" || gloves.Info != nil {
		t.Fatalf("на перчатки мер нет: %+v", gloves)
	}
	// Металлопрокат с полимерным покрытием указан позициями 7210, 7212, 7225 — только подсказка.
	if coated := Resolve(all, "7210708000", "Китай"); coated.Percent != "" || coated.Info["status"] != "possible" {
		t.Fatalf("позиция без точного кода — только проверить: %+v", coated)
	}
}

func TestOriginCountry(t *testing.T) {
	for text, want := range map[string]string{"Китай": "CN", "КНР": "CN", "China": "CN", "Республика Казахстан": OtherCountry, "Германия": OtherCountry, "Турция": "TR", "": "", "Тайвань (Китай)": "TW"} {
		if got := OriginCountry(text); got != want {
			t.Errorf("%q: %q, ожидалось %q", text, got, want)
		}
	}
}
