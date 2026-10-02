package antidumping

import "testing"

// Варианты ставок из текста реестра: производители, «прочие», страны; выбор сотрудника.
func TestOptionsAndChoice(t *testing.T) {
	measures := seedMeasures(t)
	find := func(number, country, label string) Option {
		for _, option := range Options(measures[number]) {
			if option.Country == country && option.Label == label {
				return option
			}
		}
		t.Fatalf("%s: нет варианта %s «%s» среди %+v", number, country, label, Options(measures[number]))
		return Option{}
	}
	if find("AD-8", "CN", "Angang Steel Co., Ltd.").Percent != "11.87" || find("AD-8", "CN", "прочие производители").Percent != "20.2" {
		t.Fatal("AD-8: ставки производителей")
	}
	if find("AD-26", "UA", "прочие производители").Percent != "23.9" || find("AD-26", "CN", "прочие производители").Percent != "17" {
		t.Fatal("AD-26: ставки по странам")
	}
	// Турецкие заводы CMS Jant — только Турция, не все страны меры.
	for _, option := range Options(measures["AD-37"]) {
		if option.Label != "все производители" && option.Label != "прочие производители" && option.Country != "TR" {
			t.Fatalf("AD-37: завод %s отнесён к %s", option.Label, option.Country)
		}
	}
	if len(Options(measures["AD-11"])) != 2 || Options(measures["AD-38"]) != nil {
		t.Fatal("AD-11: две страны одной ставкой; AD-38 приостановлена — вариантов нет")
	}

	all := []Measure{}
	for _, measure := range measures {
		all = append(all, measure)
	}
	angang := find("AD-8", "CN", "Angang Steel Co., Ltd.")
	// Код из позиции 7210 (мера указана позицией): сотрудник выбрал завод — ставка начисляется.
	if chosen := Resolve(all, "7210708000", "", angang.Key); chosen.Percent != "11.87" || chosen.Warning != "" || chosen.Info["status"] != "chosen" {
		t.Fatalf("выбор завода: %+v", chosen)
	}
	// Свечи из Китая: программа берёт «все производители», сотрудник может отказаться.
	if auto := Resolve(all, "8511100001", "Китай", ""); auto.Percent != "17.23" || auto.Info["selected"] == "" || len(auto.Info["options"].([]Option)) != 1 {
		t.Fatalf("авто: %+v", auto)
	}
	if declined := Resolve(all, "8511100001", "Китай", ChoiceNone); declined.Percent != "" || declined.Warning != "" || declined.Info["status"] != "declined" {
		t.Fatalf("не начислять: %+v", declined)
	}
	if stale := Resolve(all, "8511100001", "Китай", "AD-41|CN|99"); stale.Percent != "" || stale.Warning == "" {
		t.Fatalf("устаревший выбор: %+v", stale)
	}
}

func TestChoiceOfOtherMeasureIsIgnored(t *testing.T) {
	all := []Measure{}
	for _, measure := range seedMeasures(t) {
		all = append(all, measure)
	}
	// Выбор от меры AD-8 (код сменили на свечи): программа решает сама по стране.
	if result := Resolve(all, "8511100001", "Китай", "AD-8|CN|1"); result.Percent != "17.23" || result.Warning != "" {
		t.Fatalf("чужой выбор должен игнорироваться: %+v", result)
	}
}
