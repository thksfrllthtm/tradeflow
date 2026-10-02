package antidumping

// Разбор реестра мер защиты внутреннего рынка ЕЭК (remedies.eaeunion.org): из записи реестра —
// товар, коды ТН ВЭД, страны, ставка «для прочих производителей» по каждой стране, сроки,
// решения. Реестр отдаёт ставки текстом («— для прочих производителей — 17,00 %»), поэтому
// ставка по стране вычисляется из текста; если однозначно не выходит — needsCheck.

import (
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// Measure — действующая мера: коды цифрами (10 — точный код, короче — товарная позиция),
// страны ISO, Rates — % для прочих производителей по стране.
type Measure struct {
	Number      string            `json:"number"`
	Type        string            `json:"type"`
	ShortName   string            `json:"shortName"`
	Description string            `json:"description"`
	Codes       []string          `json:"codes"`
	Countries   []string          `json:"countries"`
	Rates       map[string]string `json:"rates"`
	RateText    string            `json:"rateText"`
	MinPercent  string            `json:"minPercent"`
	MaxPercent  string            `json:"maxPercent"`
	StartDate   string            `json:"startDate"`
	EndDate     string            `json:"endDate"`
	Suspended   bool              `json:"suspended"`
	NeedsCheck  string            `json:"needsCheck"`
	Foundation  string            `json:"foundation"`
	DocumentURL string            `json:"documentUrl"`
	Note        string            `json:"note"`
}

type registryRecord struct {
	Number      string   `json:"investigationnumber"`
	Type        string   `json:"investigationtype"`
	ShortName   string   `json:"shortname"`
	Description string   `json:"description"`
	Action      string   `json:"actiondescription"`
	TNVED       []string `json:"tnved"`
	Positions   []string `json:"productposition"`
	Countries   []string `json:"exportingcountrycode"`
	StartDate   string   `json:"startdate"`
	EndDate     string   `json:"enddate"`
	Foundation  string   `json:"foundation"`
	Note        string   `json:"note"`
	Actual      bool     `json:"actual"`
}

// CountryNames — страны реестра по-русски.
var CountryNames = map[string]string{
	"CN": "Китай", "UA": "Украина", "MY": "Малайзия", "TH": "Таиланд", "TR": "Турция", "JP": "Япония",
	"GE": "Грузия", "AZ": "Азербайджан", "IN": "Индия", "UZ": "Узбекистан", "KR": "Республика Корея",
	"VN": "Вьетнам", "ID": "Индонезия", "TW": "Тайвань", "US": "США", "DE": "Германия", "IR": "Иран",
}

// Как страна пишется в тексте ставки («происходящего из Китайской Народной Республики»).
var countryInText = map[string][]string{
	"CN": {"Китайской Народной Республик", "КНР", "Китая"}, "UA": {"Украин"}, "MY": {"Малайзи"},
	"TH": {"Таиланд"}, "TR": {"Турецк", "Турци"}, "JP": {"Япони"}, "GE": {"Грузи"}, "AZ": {"Азербайджан"},
	"IN": {"Индии", "Индия"}, "UZ": {"Узбекистан"}, "KR": {"Корея", "Корей"}, "VN": {"Вьетнам"}, "ID": {"Индонези"},
}

var (
	percentPattern  = regexp.MustCompile(`(\d+(?:[.,]\d+)?)\s*%`)
	letteredPattern = regexp.MustCompile(`(?m)^\s*[а-я]\)\s`)
	hrefPattern     = regexp.MustCompile(`href="([^"]+)"`)
	tagPattern      = regexp.MustCompile(`<[^>]+>`)
	blockEnd        = regexp.MustCompile(`(?i)<br\s*/?>|</(p|li|tr|div)>`)
	blankLines      = regexp.MustCompile(`\n\s*\n+`)
)

// plainText — текст из HTML реестра: строки сохраняются, теги и невидимые символы убираются.
func plainText(value string) string {
	value = blockEnd.ReplaceAllString(value, "\n")
	value = html.UnescapeString(tagPattern.ReplaceAllString(value, ""))
	value = strings.NewReplacer("\u200b", "", "\ufeff", "", "\u00a0", " ").Replace(value)
	lines := strings.Split(value, "\n")
	for index, line := range lines {
		lines[index] = strings.TrimSpace(line)
	}
	return strings.TrimSpace(blankLines.ReplaceAllString(strings.Join(lines, "\n"), "\n"))
}

func digits(value string) string {
	var builder strings.Builder
	for _, r := range value {
		if r >= '0' && r <= '9' {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func registryDate(value string) string {
	if parsed, err := time.Parse(time.RFC3339, value); err == nil {
		return parsed.Format("2006-01-02")
	}
	if len(value) >= 10 {
		return value[:10]
	}
	return ""
}

func percentText(value string) string {
	parsed, err := decimal.NewFromString(strings.ReplaceAll(value, ",", "."))
	if err != nil {
		return ""
	}
	return parsed.String()
}

// countryOfLine — страна меры, упомянутая в строке текста ставки ("" — ни одной).
func countryOfLine(line string, countries []string) string {
	for _, country := range countries {
		for _, form := range countryInText[country] {
			if strings.Contains(line, form) {
				return country
			}
		}
	}
	return ""
}

// countryRates — ставка «для прочих производителей» по каждой стране меры из текста ставки.
func countryRates(text string, countries []string) (map[string]string, string) {
	rates := map[string]string{}
	others := map[string]bool{}
	section := ""
	all := []string{}
	for _, line := range strings.Split(text, "\n") {
		matches := percentPattern.FindAllStringSubmatch(line, -1)
		for _, match := range matches {
			all = append(all, percentText(match[1]))
		}
		country := countryOfLine(line, countries)
		last := ""
		if len(matches) > 0 {
			last = percentText(matches[len(matches)-1][1])
		}
		isOthers := strings.Contains(line, "прочих производител")
		switch {
		case country != "" && last == "":
			section = country
		case country != "" && last != "":
			if isOthers || !others[country] {
				rates[country], others[country] = last, isOthers
			}
		case isOthers && last != "":
			targets := countries
			if section != "" {
				targets = []string{section}
			}
			for _, target := range targets {
				if _, done := rates[target]; !done || !others[target] {
					rates[target], others[target] = last, true
				}
			}
		}
	}
	unique := map[string]bool{}
	for _, value := range all {
		unique[value] = true
	}
	needsCheck := ""
	for _, country := range countries {
		if _, ok := rates[country]; ok {
			continue
		}
		switch {
		case len(unique) == 1:
			rates[country] = all[0]
		case len(all) > 0:
			rates[country] = maxPercent(all)
			needsCheck = "Ставка зависит от производителя или вида товара — взята наибольшая, сверьте с решением ЕЭК"
		}
	}
	if needsCheck == "" && letteredPattern.MatchString(text) {
		needsCheck = "Ставка зависит от вида товара — сверьте описание товара с решением ЕЭК"
	}
	return rates, needsCheck
}

func maxPercent(values []string) string {
	best := decimal.Zero
	for _, value := range values {
		if parsed, err := decimal.NewFromString(value); err == nil && parsed.GreaterThan(best) {
			best = parsed
		}
	}
	return best.String()
}

func minPercent(values []string) string {
	var best *decimal.Decimal
	for _, value := range values {
		if parsed, err := decimal.NewFromString(value); err == nil && (best == nil || parsed.LessThan(*best)) {
			best = &parsed
		}
	}
	if best == nil {
		return ""
	}
	return best.String()
}

// ParseRegistry — действующие меры из ответа реестра ЕЭК ({"result":[...]}).
func ParseRegistry(raw []byte) ([]Measure, error) {
	var payload struct {
		Result []registryRecord `json:"result"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, fmt.Errorf("реестр ЕЭК: неожиданный ответ: %w", err)
	}
	measures := []Measure{}
	for _, record := range payload.Result {
		if !record.Actual || strings.TrimSpace(record.Number) == "" {
			continue
		}
		codes := []string{}
		seen := map[string]bool{}
		for _, code := range append(append([]string{}, record.TNVED...), record.Positions...) {
			if code = digits(code); len(code) >= 4 && len(code) <= 10 && !seen[code] {
				seen[code] = true
				codes = append(codes, code)
			}
		}
		sort.Strings(codes)
		countries := []string{}
		for _, country := range record.Countries {
			if country = strings.ToUpper(strings.TrimSpace(country)); country != "" {
				countries = append(countries, country)
			}
		}
		rateText := plainText(record.Action)
		rates, needsCheck := countryRates(rateText, countries)
		all := []string{}
		for _, match := range percentPattern.FindAllStringSubmatch(rateText, -1) {
			all = append(all, percentText(match[1]))
		}
		foundation := plainText(record.Foundation)
		lines := strings.Split(foundation, "\n")
		links := hrefPattern.FindAllStringSubmatch(record.Foundation+record.Note, -1)
		document := ""
		if len(links) > 0 {
			document = html.UnescapeString(links[len(links)-1][1])
		}
		measures = append(measures, Measure{
			Number: strings.TrimSpace(record.Number), Type: firstNonEmpty(record.Type, "AD"),
			ShortName: plainText(record.ShortName), Description: plainText(record.Description),
			Codes: codes, Countries: countries, Rates: rates, RateText: rateText,
			MinPercent: minPercent(all), MaxPercent: maxPercent(all),
			StartDate: registryDate(record.StartDate), EndDate: registryDate(record.EndDate),
			// Последнее решение по мере — о временном неприменении: пошлина сейчас не взимается.
			Suspended:  strings.Contains(strings.ToLower(lines[len(lines)-1]), "неприменени"),
			NeedsCheck: needsCheck, Foundation: foundation, DocumentURL: document, Note: plainText(record.Note),
		})
	}
	if len(measures) == 0 {
		return nil, fmt.Errorf("реестр ЕЭК: действующих мер не найдено")
	}
	return measures, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
