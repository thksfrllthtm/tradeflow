package decisions

// Тот же товар в базе решений: по фото или наименованию, до подбора кода ИИ.

import (
	"context"
	"encoding/json"
	"strings"
	"unicode"
)

// normalizeDecisionName: регистр, ё/е, знаки препинания и лишние пробелы не важны.
func NormalizeDecisionName(value string) string {
	var builder strings.Builder
	space := false
	for _, r := range strings.ReplaceAll(strings.ToLower(value), "ё", "е") {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if space && builder.Len() > 0 {
				builder.WriteByte(' ')
			}
			builder.WriteRune(r)
			space = false
			continue
		}
		space = true
	}
	return builder.String()
}

// SameProductMatch — запись базы о том же товаре и по чему он узнан.
type SameProductMatch struct {
	Decision CodeDecisionView `json:"decision"`
	ByPhoto  bool             `json:"byPhoto"`
	ByName   bool             `json:"byName"`
	// PhotoDiffers — совпало наименование, но фото у обоих есть и не похожи: фото главнее,
	// поэтому такая запись не пропускает ИИ и показывается с предупреждением.
	PhotoDiffers bool `json:"photoDiffers"`
}

// FindSame ищет в базе тот же товар: то же фото или то же наименование (из инвойса или поставщика).
// Сначала проверенные записи.
func (s *CodeDecisionService) FindSame(ctx context.Context, names []string, photoHashes []string) ([]SameProductMatch, error) {
	keys, err := s.repository.CodeDecisionKeys(ctx)
	if err != nil {
		return nil, err
	}
	wanted := map[string]bool{}
	for _, name := range names {
		for _, variant := range decisionNameVariants(name) {
			wanted[variant] = true
		}
	}
	type hit struct{ byPhoto, byName, photoDiffers bool }
	hits := map[int64]hit{}
	ids := []int64{}
	for _, key := range keys {
		var hashes []string
		_ = json.Unmarshal(key.PhotoHashes, &hashes)
		byPhoto := len(photoHashes) > 0 && samePhoto(photoHashes, hashes)
		byName := false
		for _, name := range []string{key.InvoiceName, key.SupplierName, key.FinalName} {
			for _, variant := range decisionNameVariants(name) {
				byName = byName || wanted[variant]
			}
		}
		if byPhoto || byName {
			_, similar := photoSimilarity(photoHashes, hashes)
			differs := !byPhoto && len(photoHashes) > 0 && len(hashes) > 0 && !similar
			hits[key.ID] = hit{byPhoto, byName, differs}
			ids = append(ids, key.ID)
		}
	}
	rows, err := s.repository.CodeDecisionsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	views, err := s.views(ctx, rows)
	if err != nil {
		return nil, err
	}
	matches := make([]SameProductMatch, 0, len(views))
	for _, view := range views {
		matches = append(matches, SameProductMatch{Decision: view, ByPhoto: hits[view.ID].byPhoto, ByName: hits[view.ID].byName, PhotoDiffers: hits[view.ID].photoDiffers})
	}
	// Проверенные — первыми, затем по фото, затем новые.
	sortMatches(matches)
	if len(matches) > 5 {
		matches = matches[:5]
	}
	return matches, nil
}

// decisionNameVariants — по чему наименование узнаётся: целиком и по частям «русское / китайское»
// (так записаны наименования компании). Слишком короткое («перчатки», «钳子») слишком общее.
func decisionNameVariants(name string) []string {
	parts := []string{name}
	if left, right, ok := strings.Cut(name, "/"); ok {
		parts = append(parts, left, right)
	}
	variants := []string{}
	for _, part := range parts {
		normalized := NormalizeDecisionName(part)
		minimum := 8
		if strings.IndexFunc(normalized, func(r rune) bool { return unicode.Is(unicode.Han, r) }) >= 0 {
			minimum = 4
		}
		if len([]rune(normalized)) >= minimum {
			variants = append(variants, normalized)
		}
	}
	return variants
}

// AgreedVerifiedCode — код, на котором сходятся все проверенные записи о том же товаре; без ИИ
// обходимся, только если такой код один (иначе одно и то же наименование оформлялось по-разному).
func AgreedVerifiedCode(matches []SameProductMatch) (string, bool) {
	code := ""
	for _, match := range matches {
		if match.Decision.Status != "verified" || match.PhotoDiffers {
			continue
		}
		if code != "" && code != match.Decision.Code {
			return "", false
		}
		code = match.Decision.Code
	}
	return code, code != ""
}

func sortMatches(matches []SameProductMatch) {
	rank := func(match SameProductMatch) int {
		value := 0
		if match.Decision.Status != "verified" {
			value += 2
		}
		if !match.ByPhoto {
			value++
		}
		return value
	}
	for i := 1; i < len(matches); i++ {
		for j := i; j > 0; j-- {
			left, right := matches[j-1], matches[j]
			if rank(left) < rank(right) || (rank(left) == rank(right) && !left.Decision.UpdatedAt.Before(right.Decision.UpdatedAt)) {
				break
			}
			matches[j-1], matches[j] = right, left
		}
	}
}
