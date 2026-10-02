package antidumping

// Варианты ставки меры для выбора сотрудником: страна, производитель (или вид товара) и ставка —
// по строкам текста ставки реестра ЕЭК («— для Angang Steel Co., Ltd. — 11,87 %»).

import (
	"fmt"
	"regexp"
	"strings"
)

// Option — вариант ставки: Key — для выбора в строке сметы (номер меры, страна, строка текста).
type Option struct {
	Key         string `json:"key"`
	Measure     string `json:"measure"`
	ShortName   string `json:"shortName"`
	Country     string `json:"country"`
	CountryName string `json:"countryName"`
	Label       string `json:"label"`
	Percent     string `json:"percent"`
	Others      bool   `json:"others"`
}

// ChoiceNone — сотрудник решил антидемпинг не начислять (товар не подпадает под меру).
const ChoiceNone = "none"

var (
	labelBeforeDash = regexp.MustCompile(`для\s+(.+?)\s+[—–-]\s*\d`)
	labelBeforeRate = regexp.MustCompile(`для\s+(.+?)\s+в\s+(?:размере|следующих)`)
	listMark        = regexp.MustCompile(`^[\s\-–—*•]*(?:[а-яА-Яa-zA-Z]\)\s*)?`)
	labelNoise      = regexp.MustCompile(`^(?:компаний|компании|компания)\s+`)
)

func optionLabel(line string, country string) (string, bool) {
	label := ""
	if m := labelBeforeDash.FindStringSubmatch(line); m != nil {
		label = m[1]
	} else if m := labelBeforeRate.FindStringSubmatch(line); m != nil {
		label = m[1]
	}
	label = strings.TrimSpace(labelNoise.ReplaceAllString(strings.TrimSpace(listMark.ReplaceAllString(label, "")), ""))
	others := strings.Contains(line, "прочих производител")
	switch {
	case others:
		return "прочие производители", true
	case label == "" || strings.Contains(label, "происходящ") || (country != "" && countryOfLine(label, []string{country}) == country):
		// «в размере 29,39 %», «для ленты, происходящей из …», «для производителей Японии».
		return "все производители", false
	}
	return strings.TrimSuffix(label, ";"), false
}

// Options — варианты ставки меры (для приостановленной меры вариантов нет). Строка без страны
// в мере с несколькими странами относится к стране следующей строки, где страна названа
// («для компаний CMS Jant …» перед «для прочих производителей Турецкой Республики»); если такой
// строки нет — ко всем странам меры.
func Options(measure Measure) []Option {
	if measure.Suspended {
		return nil
	}
	options := []Option{}
	type line struct {
		index         int
		text, percent string
	}
	pending := []line{}
	add := func(item line, owner string) {
		label, others := optionLabel(item.text, owner)
		options = append(options, Option{
			Key: fmt.Sprintf("%s|%s|%d", measure.Number, owner, item.index), Measure: measure.Number, ShortName: measure.ShortName,
			Country: owner, CountryName: firstNonEmpty(CountryNames[owner], owner), Label: label, Percent: item.percent, Others: others,
		})
	}
	flush := func(owners []string) {
		for _, item := range pending {
			for _, owner := range owners {
				add(item, owner)
			}
		}
		pending = nil
	}
	section := ""
	for index, text := range strings.Split(measure.RateText, "\n") {
		matches := percentPattern.FindAllStringSubmatch(text, -1)
		country := countryOfLine(text, measure.Countries)
		if country != "" {
			flush([]string{country})
		}
		if len(matches) == 0 {
			if country != "" {
				section = country
			}
			continue
		}
		item := line{index: index, text: text, percent: percentText(matches[len(matches)-1][1])}
		switch owner := firstNonEmpty(country, section); {
		case owner != "":
			add(item, owner)
		case len(measure.Countries) == 1:
			add(item, measure.Countries[0])
		default:
			pending = append(pending, item)
		}
	}
	flush(measure.Countries)
	return options
}

// autoOptionKey — вариант, который программа берёт сама для страны: «прочие производители»,
// иначе единственный вариант страны.
func autoOptionKey(options []Option, country, percent string) string {
	for _, option := range options {
		if option.Country == country && option.Percent == percent && (option.Others || option.Label == "все производители") {
			return option.Key
		}
	}
	for _, option := range options {
		if option.Country == country && option.Percent == percent {
			return option.Key
		}
	}
	return ""
}
