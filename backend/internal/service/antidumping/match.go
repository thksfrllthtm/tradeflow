package antidumping

// Какая мера действует для кода строки и страны происхождения. Точный 10-значный код меры —
// мера применяется (ставка «для прочих производителей» страны). Код совпал только с товарной
// позицией меры («7210» без подробных кодов) — мера возможна, решает описание товара: ставка не
// начисляется, программа подсказывает проверить.

import (
	"fmt"
	"slices"
	"strings"
)

// Match — мера для кода: Exact — по точному коду, иначе по товарной позиции.
type Match struct {
	Measure Measure
	Exact   bool
}

// MatchCode — меры, под которые может попасть код (10 цифр).
func MatchCode(measures []Measure, code string) []Match {
	matches := []Match{}
	for _, measure := range measures {
		exact, heading := false, false
		for _, measureCode := range measure.Codes {
			switch {
			case measureCode == code:
				exact = true
			case len(measureCode) < 10 && strings.HasPrefix(code, measureCode) && !hasDetailedCodes(measure.Codes, measureCode):
				heading = true
			}
		}
		if exact || heading {
			matches = append(matches, Match{Measure: measure, Exact: exact})
		}
	}
	return matches
}

// hasDetailedCodes — у позиции в мере перечислены подробные коды: тогда сама позиция не значит
// «весь товар позиции» (в реестре позиция указывается рядом со своими кодами).
func hasDetailedCodes(codes []string, heading string) bool {
	for _, code := range codes {
		if len(code) > len(heading) && strings.HasPrefix(code, heading) {
			return true
		}
	}
	return false
}

// OtherCountry — страна указана, но мер против неё в списке стран нет (в том числе страны ЕАЭС).
const OtherCountry = "ZZ"

// OriginCountry — страна происхождения из поля «Страна» сметы: ISO, OtherCountry или "" (не указана).
func OriginCountry(text string) string {
	value := strings.ToLower(strings.TrimSpace(text))
	if value == "" {
		return ""
	}
	forms := map[string][]string{
		"CN": {"китай", "кнр", "china", "中国"}, "UA": {"украин", "ukrain"}, "MY": {"малайзи", "malaysia"},
		"TH": {"таиланд", "тайланд", "thailand"}, "TR": {"турци", "turkey", "türkiye", "turkiye"}, "JP": {"япони", "japan"},
		"GE": {"грузи", "georgia"}, "AZ": {"азербайджан", "azerbaijan"}, "IN": {"инди", "india"}, "UZ": {"узбекистан", "uzbekistan"},
		"KR": {"корея", "korea"}, "VN": {"вьетнам", "vietnam", "viet nam"}, "ID": {"индонези", "indonesia"}, "TW": {"тайвань", "taiwan"},
	}
	for _, country := range []string{"TW", "ID", "CN", "UA", "MY", "TH", "TR", "JP", "GE", "AZ", "IN", "UZ", "KR", "VN"} {
		for _, form := range forms[country] {
			if strings.Contains(value, form) {
				return country
			}
		}
	}
	if code := strings.ToUpper(value); len(code) == 2 && code != "ZZ" {
		return code
	}
	return OtherCountry
}

func countryList(codes []string) string {
	names := make([]string, 0, len(codes))
	for _, code := range codes {
		names = append(names, firstNonEmpty(CountryNames[code], code))
	}
	return strings.Join(names, ", ")
}

func percentLabel(value string) string { return strings.ReplaceAll(value, ".", ",") + " %" }

// Resolution — антидемпинг строки: Percent — начисляемая ставка ("" — не начисляется),
// Info — для показа в смете, Warning — нужно действие сотрудника (строка требует проверки).
type Resolution struct {
	Percent string
	Info    map[string]any
	Warning string
}

// Resolve — антидемпинг для кода строки, страны происхождения сметы и выбора сотрудника (choice —
// ключ варианта ставки, ChoiceNone — не начислять, "" — программа решает по стране). Варианты
// всех подходящих мер кладутся в Info["options"], чтобы сотрудник мог выбрать сам.
func Resolve(measures []Measure, code, origin, choice string) Resolution {
	matches := MatchCode(measures, code)
	if len(matches) == 0 {
		return Resolution{}
	}
	options := []Option{}
	for _, match := range matches {
		options = append(options, Options(match.Measure)...)
	}
	resolution := resolveAuto(matches, origin)
	if resolution.Info == nil {
		resolution.Info = map[string]any{}
	}
	resolution.Info["options"] = options
	switch choice {
	case "":
		return resolution
	case ChoiceNone:
		resolution.Info["status"], resolution.Info["selected"] = "declined", ChoiceNone
		resolution.Info["text"] = "Антидемпинг не начисляется: так выбрал сотрудник (товар не подпадает под меру)."
		return Resolution{Info: resolution.Info}
	}
	for _, option := range options {
		if option.Key != choice {
			continue
		}
		resolution.Info["status"], resolution.Info["selected"], resolution.Info["percent"] = "chosen", option.Key, option.Percent
		resolution.Info["number"], resolution.Info["shortName"], resolution.Info["country"] = option.Measure, option.ShortName, option.Country
		resolution.Info["text"] = fmt.Sprintf("Антидемпинг %s «%s»: %s — %s, %s (выбрано вручную).", option.Measure, option.ShortName, option.CountryName, option.Label, percentLabel(option.Percent))
		return Resolution{Percent: option.Percent, Info: resolution.Info}
	}
	// Выбор относится к мере, под которую код больше не попадает (код строки сменили), — не учитывается.
	chosenMeasure, _, _ := strings.Cut(choice, "|")
	if !slices.ContainsFunc(matches, func(match Match) bool { return match.Measure.Number == chosenMeasure }) {
		return resolution
	}
	// Вариант пропал после обновления реестра — ставку надо выбрать заново.
	resolution.Info["status"] = "stale"
	return Resolution{Info: resolution.Info, Warning: "Антидемпинг: выбранный вариант ставки больше не действует (реестр ЕЭК обновлён) — выберите вариант заново"}
}

// resolveAuto — без выбора сотрудника: точный код меры и страна происхождения из её списка.
func resolveAuto(matches []Match, origin string) Resolution {
	country := OriginCountry(origin)
	var exact *Match
	possible := []string{}
	for index := range matches {
		match := matches[index]
		if match.Exact && exact == nil {
			exact = &matches[index]
			continue
		}
		possible = append(possible, fmt.Sprintf("%s «%s» (%s)", match.Measure.Number, match.Measure.ShortName, countryList(match.Measure.Countries)))
	}
	if exact == nil {
		return Resolution{Info: map[string]any{
			"status": "possible", "possible": possible,
			"text": "Код входит в товарную позицию, на часть товаров которой есть антидемпинговая мера: " + strings.Join(possible, "; ") + ". Сверьте описание товара с решением ЕЭК и выберите вариант, если мера применяется.",
		}}
	}
	measure := exact.Measure
	info := map[string]any{
		"number": measure.Number, "shortName": measure.ShortName, "countries": measure.Countries, "countryNames": countryList(measure.Countries),
		"rates": measure.Rates, "endDate": measure.EndDate, "documentUrl": measure.DocumentURL, "needsCheck": measure.NeedsCheck,
	}
	if len(possible) > 0 {
		info["possible"] = possible
	}
	ratesText := []string{}
	for _, code := range measure.Countries {
		if rate := measure.Rates[code]; rate != "" {
			ratesText = append(ratesText, firstNonEmpty(CountryNames[code], code)+" — "+percentLabel(rate))
		}
	}
	label := fmt.Sprintf("%s «%s» (%s)", measure.Number, measure.ShortName, strings.Join(ratesText, ", "))
	switch {
	case measure.Suspended:
		info["status"], info["text"] = "suspended", "Антидемпинговая мера "+label+" временно не применяется."
		return Resolution{Info: info}
	case country == "":
		info["status"], info["text"] = "origin", "На код действует антидемпинговая мера "+label+". Выберите вариант ставки или укажите страну происхождения."
		return Resolution{Info: info, Warning: "Антидемпинг: на код действует мера " + label + " — выберите вариант ставки или укажите страну происхождения в параметрах расчёта"}
	case measure.Rates[country] == "":
		info["status"], info["text"] = "other-country", fmt.Sprintf("Мера %s действует только для: %s; для товара из страны «%s» не применяется.", label, countryList(measure.Countries), firstNonEmpty(CountryNames[country], strings.TrimSpace(origin)))
		return Resolution{Info: info}
	}
	percent := measure.Rates[country]
	info["status"], info["percent"], info["country"] = "applied", percent, country
	info["selected"] = autoOptionKey(Options(measure), country, percent)
	info["text"] = fmt.Sprintf("Антидемпинг %s «%s»: %s для прочих производителей (%s). Если производитель назван в решении ЕЭК, выберите его ставку.",
		measure.Number, measure.ShortName, percentLabel(percent), firstNonEmpty(CountryNames[country], country))
	resolution := Resolution{Percent: percent, Info: info}
	if measure.NeedsCheck != "" {
		resolution.Warning = "Антидемпинг " + measure.Number + ": " + measure.NeedsCheck
	}
	return resolution
}
