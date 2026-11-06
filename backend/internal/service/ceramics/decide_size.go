package ceramics

import (
	"fmt"
	"strings"
)

// decideCeramicsSize: an explicit size in the Chinese name is the strong source, the mark
// series confirms it; a number without a unit, the client's Russian name and the series alone
// are assumptions. unitGuessed: the size came from a number without a unit (inch until the type
// is known).
func decideCeramicsSize(result *CeramicsResolution, item CeramicsInputItem, series []MarkSeriesSuggestion) (unitGuessed bool) {
	chinese, compound := extractSizeMentions(item.ChineseName)
	russian, _ := extractSizeMentions(item.RussianName)
	bare := []string{}
	if len(chinese) == 0 {
		bare = bareSizeNumbers(item.ChineseName)
	}
	for _, mention := range chinese {
		result.SizeOptions = append(result.SizeOptions, CeramicsSizeOption{Size: mention.Size, Source: SourceChineseName, Detail: mention.Raw})
	}
	for _, value := range bare {
		result.SizeOptions = append(result.SizeOptions, CeramicsSizeOption{Size: CeramicSize{Value: value, Unit: SizeUnitInch}, Source: SourceChineseNumber, Detail: value})
	}
	for _, mention := range russian {
		result.SizeOptions = append(result.SizeOptions, CeramicsSizeOption{Size: mention.Size, Source: SourceRussianName, Detail: mention.Raw})
	}
	seriesSizes := map[string]MarkSeriesSuggestion{}
	for _, suggestion := range series {
		if _, ok := seriesSizes[suggestion.Size.key()]; !ok {
			seriesSizes[suggestion.Size.key()] = suggestion
			result.SizeOptions = append(result.SizeOptions, CeramicsSizeOption{Size: suggestion.Size, Source: SourceMarkPattern, Detail: suggestion.Template})
		}
	}
	seriesText := func(suggestion MarkSeriesSuggestion) string {
		return fmt.Sprintf("%s (в базе: %s)", suggestion.Template, strings.Join(suggestion.Examples, ", "))
	}
	seriesWithValue := func(value string) (MarkSeriesSuggestion, bool) {
		for _, suggestion := range series {
			if suggestion.Size.Value == value {
				return suggestion, true
			}
		}
		return MarkSeriesSuggestion{}, false
	}
	addEvidence := func(source, text string) {
		result.Evidence = append(result.Evidence, CeramicsEvidence{Field: "size", Source: source, Text: text})
	}
	if compound {
		result.Warnings = append(result.Warnings, "В китайском наименовании несколько размеров через «×» (например 20×15 см) — размер нужно выбрать вручную")
	}

	switch {
	case len(chinese) == 1:
		size := chinese[0].Size
		result.Size = &size
		result.SizeStatus = CeramicsPartMatched
		addEvidence(SourceChineseName, fmt.Sprintf("Китайское название содержит %s → %s", chinese[0].Raw, FormatCeramicSize(size)))
		for key, suggestion := range seriesSizes {
			if key == size.key() {
				addEvidence(SourceMarkPattern, "Также совпадает с шаблоном марки "+seriesText(suggestion))
			} else {
				result.SizeStatus = CeramicsPartNeedReview
				addEvidence(SourceMarkPattern, fmt.Sprintf("Внимание: шаблон марки %s указывает другой размер — %s", seriesText(suggestion), FormatCeramicSize(suggestion.Size)))
			}
		}
		if len(russian) == 1 && russian[0].Size.key() != size.key() {
			result.Warnings = append(result.Warnings, fmt.Sprintf("В русском названии клиента другой размер: %s", russian[0].Raw))
		}
	case len(chinese) > 1:
		result.SizeStatus = CeramicsPartNeedReview
		raws := make([]string, 0, len(chinese))
		for _, mention := range chinese {
			raws = append(raws, mention.Raw)
		}
		addEvidence(SourceChineseName, "В китайском названии несколько размеров: "+strings.Join(raws, ", ")+" — выберите нужный")
	case len(bare) == 1 && !compound:
		size := CeramicSize{Value: bare[0], Unit: SizeUnitInch}
		unitGuessed = true
		result.Size = &size
		result.SizeStatus = CeramicsPartAssumed
		addEvidence(SourceChineseNumber, fmt.Sprintf("В китайском названии число %s без единицы — вероятно, размер. Проверьте", bare[0]))
		if suggestion, ok := seriesWithValue(bare[0]); ok {
			size.Unit, unitGuessed = suggestion.Size.Unit, false
			result.SizeStatus = CeramicsPartMatched
			addEvidence(SourceMarkPattern, "Подтверждается шаблоном марки "+seriesText(suggestion))
		} else if len(russian) == 1 && russian[0].Size.Value == bare[0] {
			size.Unit, unitGuessed = russian[0].Size.Unit, false
			result.SizeStatus = CeramicsPartMatched
			addEvidence(SourceRussianName, fmt.Sprintf("Подтверждается русским названием клиента: «%s»", russian[0].Raw))
		}
	case len(russian) == 1 && !compound:
		size := russian[0].Size
		result.Size = &size
		result.SizeStatus = CeramicsPartAssumed
		addEvidence(SourceRussianName, fmt.Sprintf("Русское название клиента содержит «%s» (перевод клиента, требует проверки)", russian[0].Raw))
		if suggestion, ok := seriesSizes[size.key()]; ok {
			result.SizeStatus = CeramicsPartMatched
			addEvidence(SourceMarkPattern, "Подтверждается шаблоном марки "+seriesText(suggestion))
		}
	case len(seriesSizes) == 1 && len(bare) == 0 && !compound:
		for _, suggestion := range seriesSizes {
			size := suggestion.Size
			result.Size = &size
			result.SizeStatus = CeramicsPartAssumed
			addEvidence(SourceMarkPattern, fmt.Sprintf("Предположение по серии марок: шаблон %s → %s. Проверьте", seriesText(suggestion), FormatCeramicSize(size)))
		}
	case len(seriesSizes) > 1 || len(russian) > 1 || len(bare) > 1 || compound:
		result.SizeStatus = CeramicsPartNeedReview
		addEvidence(SourceMarkPattern, "Размер неоднозначен — выберите из вариантов")
	default:
		result.SizeStatus = CeramicsPartNotFound
		addEvidence(SourceChineseName, "Размер не найден ни в китайском названии, ни в марке")
	}
	return unitGuessed
}
