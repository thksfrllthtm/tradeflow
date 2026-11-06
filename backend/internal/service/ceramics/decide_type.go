package ceramics

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service/ai"
)

var ceramicsSourceLabels = map[string]string{
	"chinese_name": "китайское название", "image": "фото", "mark": "марка", "russian_name": "русское название клиента",
	"hints": "совпадения в базе",
}

func describeSources(sources []string) string {
	labels := make([]string, 0, len(sources))
	for _, source := range sources {
		if label := ceramicsSourceLabels[source]; label != "" {
			labels = append(labels, label)
		}
	}
	if len(labels) == 0 {
		return ""
	}
	return " (по: " + strings.Join(labels, ", ") + ")"
}

// decideCeramicsType: the same Chinese name in the base settles the type without AI;
// otherwise stage A proposes candidates from the base types, similar types are added and
// stage B compares them. The model can only pick a type of the base.
func (s *CeramicsService) decideCeramicsType(ctx context.Context, cc *ceramicsContext, result *CeramicsResolution, item CeramicsInputItem, series []MarkSeriesSuggestion) {
	addEvidence := func(source, text string) {
		result.Evidence = append(result.Evidence, CeramicsEvidence{Field: "type", Source: source, Text: text})
	}
	for _, term := range result.ChineseTerms {
		addEvidence(SourceChineseName, fmt.Sprintf("Китайское название содержит %s — %s", term.Text, term.Meaning))
	}

	// Types of base records with the same Chinese name (without sizes and numbers).
	nameTypes := []string{}
	base := chineseBaseName(item.ChineseName)
	if matches := cc.baseNames[base]; utf8.RuneCountInString(base) >= 2 && len(matches) > 0 {
		for typeKey := range matches {
			nameTypes = append(nameTypes, typeKey)
		}
		sort.Slice(nameTypes, func(a, b int) bool { return len(matches[nameTypes[a]]) > len(matches[nameTypes[b]]) })
	}
	seriesTypes := []string{}
	for _, suggestion := range series {
		for _, typeKey := range suggestion.TypeKeys {
			if !slices.Contains(seriesTypes, typeKey) {
				seriesTypes = append(seriesTypes, typeKey)
			}
		}
	}

	if len(nameTypes) == 1 && (len(seriesTypes) == 0 || (len(seriesTypes) == 1 && seriesTypes[0] == nameTypes[0])) {
		typeItem := cc.typeByKey[nameTypes[0]]
		marks := cc.baseNames[base][nameTypes[0]]
		if len(marks) > 5 {
			marks = marks[:5]
		}
		result.ProductType, result.TypeStatus = typeItem.Name, CeramicsPartMatched
		result.TypeCandidates = []CeramicsAICandidate{{ProductType: typeItem.Name, Confidence: 1, Reason: "Такое же китайское название в базе", Sources: []string{"chinese_name"}}}
		addEvidence(SourceChineseNameMatch, fmt.Sprintf("Китайское название без размера «%s» совпадает с записями базы: %s", base, strings.Join(marks, ", ")))
		if len(seriesTypes) == 1 {
			addEvidence(SourceMarkPattern, "Серия марки в базе — тот же тип")
		}
		return
	}

	localNames := []string{}
	for _, typeKey := range append(append([]string{}, nameTypes...), seriesTypes...) {
		if typeItem := cc.typeByKey[typeKey]; typeItem != nil && !slices.Contains(localNames, typeItem.Name) {
			localNames = append(localNames, typeItem.Name)
		}
	}
	if len(nameTypes) > 1 {
		addEvidence(SourceChineseNameMatch, "Такое же китайское название в базе у нескольких типов: "+strings.Join(localNames, ", "))
	}
	localCandidates := func(reason string) {
		for _, name := range localNames {
			result.TypeCandidates = append(result.TypeCandidates, CeramicsAICandidate{ProductType: name, Reason: reason, Sources: []string{"hints"}})
		}
		if len(result.TypeCandidates) > 0 {
			result.TypeStatus = CeramicsPartNeedReview
		} else {
			result.TypeStatus = CeramicsPartNotFound
		}
	}
	if !s.ai.Configured() {
		result.Warnings = append(result.Warnings, "ИИ не настроен на сервере — тип определяется только по совпадениям в базе")
		localCandidates("Совпадение в базе")
		return
	}

	product := CeramicsAIProduct{Mark: item.Mark, ChineseName: item.ChineseName, RussianName: item.RussianName, Photo: item.Photo, Hints: CeramicsAIHints{ChineseTerms: result.ChineseTerms}}
	if result.Size != nil {
		product.Hints.SizeInName = FormatCeramicSize(*result.Size)
	}
	for _, typeKey := range nameTypes {
		product.Hints.TypesOfSameName = append(product.Hints.TypesOfSameName, cc.typeByKey[typeKey].Name)
	}
	for _, typeKey := range seriesTypes {
		product.Hints.TypesOfMarkSeries = append(product.Hints.TypesOfMarkSeries, cc.typeByKey[typeKey].Name)
	}
	if item.Photo != "" && !s.ai.VisionEnabled() {
		result.Warnings = append(result.Warnings, "Фото не анализировалось: модель ИИ настроена без распознавания изображений")
	}

	result.AIUsed = true
	stageA, err := s.ai.FindProductCandidates(ctx, product, cc.typeNames)
	if err != nil {
		result.Warnings = append(result.Warnings, "ИИ: "+ai.AIErrorMessage(err))
		localCandidates("Совпадение в базе")
		return
	}
	if len(stageA.ChineseAnalysis) > 0 {
		parts := make([]string, 0, len(stageA.ChineseAnalysis))
		for _, term := range stageA.ChineseAnalysis {
			parts = append(parts, term.Text+" — "+term.Meaning)
		}
		addEvidence(SourceAI, "Разбор китайского названия ИИ: "+strings.Join(parts, "; "))
	}

	// Candidates: stage A, then types from base matches, then more general or more specific
	// forms of the top candidates, so that «тарелка» is always compared with «тарелка салатница».
	pool := []CeramicsAICandidate{}
	add := func(candidate CeramicsAICandidate) {
		if !containsCandidate(pool, candidate.ProductType) && len(pool) < maxCeramicsCandidates {
			pool = append(pool, candidate)
		}
	}
	for _, candidate := range stageA.Candidates {
		add(candidate)
	}
	for _, name := range localNames {
		add(CeramicsAICandidate{ProductType: name, Reason: "Совпадение в базе", Sources: []string{"hints"}})
	}
	top := append([]CeramicsAICandidate{}, pool...)
	if len(top) > 2 {
		top = top[:2]
	}
	for _, candidate := range top {
		for _, name := range cc.typeNames {
			if similarCeramicTypes(candidate.ProductType, name) {
				add(CeramicsAICandidate{ProductType: name, Reason: "Похожий тип: проверяется сравнением", Sources: []string{}})
			}
		}
	}
	if len(stageA.Candidates) > 0 {
		parts := make([]string, 0, len(stageA.Candidates))
		for _, candidate := range stageA.Candidates {
			parts = append(parts, fmt.Sprintf("%s (%.0f%%)", candidate.ProductType, candidate.Confidence*100))
		}
		addEvidence(SourceAI, "ИИ, этап 1 — кандидаты из типов базы: "+strings.Join(parts, ", "))
	}

	switch {
	case len(pool) == 0:
		result.TypeStatus = CeramicsPartNotFound
		addEvidence(SourceAI, "ИИ не нашёл подходящего типа среди типов базы")
		return
	case len(pool) == 1:
		result.TypeCandidates = pool
		if pool[0].Confidence >= ceramicsMinConfidence {
			result.ProductType, result.TypeStatus = pool[0].ProductType, CeramicsPartMatched
			addEvidence(SourceAI, fmt.Sprintf("Выбран «%s»%s: %s", pool[0].ProductType, describeSources(pool[0].Sources), pool[0].Reason))
		} else {
			result.TypeStatus = CeramicsPartNeedReview
			addEvidence(SourceAI, "ИИ не уверен в единственном кандидате — нужна проверка")
		}
		return
	}

	names := make([]string, 0, len(pool))
	for _, candidate := range pool {
		names = append(names, candidate.ProductType)
	}
	comparison, err := s.ai.CompareCandidates(ctx, product, names)
	if err != nil {
		result.Warnings = append(result.Warnings, "ИИ (сравнение): "+ai.AIErrorMessage(err))
		result.TypeCandidates, result.TypeStatus = pool, CeramicsPartNeedReview
		return
	}
	// Candidates in the order of the comparison, then the rest of the pool.
	ordered := append([]CeramicsAICandidate{}, comparison.Ranking...)
	for _, candidate := range pool {
		if !containsCandidate(ordered, candidate.ProductType) {
			ordered = append(ordered, candidate)
		}
	}
	result.TypeCandidates = ordered
	if comparison.Matched {
		result.ProductType, result.TypeStatus = comparison.ProductType, CeramicsPartMatched
		addEvidence(SourceAI, fmt.Sprintf("ИИ, этап 2 — сравнение «%s»: выбран «%s» (%.0f%%)%s. %s",
			strings.Join(names, "» / «"), comparison.ProductType, comparison.Confidence*100, describeSources(comparison.Sources), comparison.Reason))
		return
	}
	result.TypeStatus = CeramicsPartNeedReview
	addEvidence(SourceAI, fmt.Sprintf("ИИ, этап 2 — не удалось уверенно выбрать между «%s». %s", strings.Join(names, "» / «"), comparison.Reason))
}

func containsCandidate(candidates []CeramicsAICandidate, name string) bool {
	for _, candidate := range candidates {
		if candidate.ProductType == name {
			return true
		}
	}
	return false
}
