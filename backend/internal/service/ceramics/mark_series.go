package ceramics

// Ceramics mark series: a size encoded in a mark, learned from known marks of the base.

import (
	"sort"
	"strconv"
	"strings"
)

// markSeries is a group of known marks that differ only by the size inside them:
// 705-7DJ, 705-9DJ, 705-10DJ → 705-{SIZE}DJ.
type markSeries struct {
	Template string
	Unit     string
	Sizes    map[string]bool
	TypeKeys map[string]int
	Examples []string
}

// MarkSeriesSuggestion is a size read from a mark by a series of known marks.
type MarkSeriesSuggestion struct {
	Template string      `json:"template"`
	Size     CeramicSize `json:"size"`
	Examples []string    `json:"examples"`
	TypeKeys []string    `json:"-"`
}

type digitRun struct{ start, end int }

func markDigitRuns(mark string) []digitRun {
	runs := []digitRun{}
	for index := 0; index < len(mark); {
		if mark[index] < '0' || mark[index] > '9' {
			index++
			continue
		}
		start := index
		for index < len(mark) && mark[index] >= '0' && mark[index] <= '9' {
			index++
		}
		runs = append(runs, digitRun{start, index})
	}
	return runs
}

// buildMarkSeries finds, for every known mark with an integer size, the digit runs equal to
// that size. A template counts as a series only when marks of at least two different sizes
// share it: one mark 607-7 of 7" does not prove that its 7 is the size.
func buildMarkSeries(rows []ceramicCatalogEntry) map[string]*markSeries {
	all := map[string]*markSeries{}
	for _, row := range rows {
		if row.Size == nil || !isWholeNumber(row.Size.Value) {
			continue
		}
		for _, run := range markDigitRuns(row.MarkKey) {
			if strings.TrimLeft(row.MarkKey[run.start:run.end], "0") != row.Size.Value {
				continue
			}
			template := row.MarkKey[:run.start] + "{SIZE}" + row.MarkKey[run.end:]
			key := template + "|" + row.Size.Unit
			series := all[key]
			if series == nil {
				series = &markSeries{Template: template, Unit: row.Size.Unit, Sizes: map[string]bool{}, TypeKeys: map[string]int{}}
				all[key] = series
			}
			series.Sizes[row.Size.Value] = true
			series.TypeKeys[row.TypeKey]++
			if len(series.Examples) < 6 {
				series.Examples = append(series.Examples, row.Mark+" → "+FormatCeramicSize(*row.Size))
			}
		}
	}
	for key, series := range all {
		if len(series.Sizes) < 2 {
			delete(all, key)
		}
	}
	return all
}

func isWholeNumber(value string) bool {
	_, err := strconv.Atoi(value)
	return err == nil
}

// inferSizeFromMarkPattern returns the sizes the mark gets from known series.
func inferSizeFromMarkPattern(markKey string, series map[string]*markSeries) []MarkSeriesSuggestion {
	suggestions := []MarkSeriesSuggestion{}
	for _, run := range markDigitRuns(markKey) {
		template := markKey[:run.start] + "{SIZE}" + markKey[run.end:]
		value := strings.TrimLeft(markKey[run.start:run.end], "0")
		if value == "" {
			continue
		}
		for _, unit := range []string{SizeUnitInch, SizeUnitCm, SizeUnitMm, SizeUnitMl, SizeUnitL} {
			found := series[template+"|"+unit]
			if found == nil {
				continue
			}
			typeKeys := make([]string, 0, len(found.TypeKeys))
			for key := range found.TypeKeys {
				typeKeys = append(typeKeys, key)
			}
			sort.Slice(typeKeys, func(a, b int) bool { return found.TypeKeys[typeKeys[a]] > found.TypeKeys[typeKeys[b]] })
			suggestions = append(suggestions, MarkSeriesSuggestion{Template: template, Size: CeramicSize{Value: value, Unit: unit}, Examples: found.Examples, TypeKeys: typeKeys})
		}
	}
	return suggestions
}
