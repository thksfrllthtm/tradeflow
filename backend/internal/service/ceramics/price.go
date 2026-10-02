package ceramics

// Ceramics module: the price of a product type and size is the most frequent price of the
// base records with exactly this type and size (mode, never an average). A tie or a size
// missing from the base goes to the operator.

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"
)

const (
	CeramicsPriceMatched    = "MATCHED"
	CeramicsPriceNeedReview = "PRICE_NEED_REVIEW"
	CeramicsPriceNotFound   = "NOT_FOUND"
	// The type or size is not determined yet, so the price is not looked up.
	CeramicsPriceNotChecked = "NOT_CHECKED"
)

type CeramicsPriceOption struct {
	Price string `json:"price"`
	Count int    `json:"count"`
}

type CeramicsPriceResult struct {
	Status  string                `json:"status"`
	Price   string                `json:"price,omitempty"`
	Options []CeramicsPriceOption `json:"options"`
	Total   int                   `json:"total"`
	// AvailableSizes of the type in the base, shown when the exact size has no price.
	AvailableSizes []CeramicSize `json:"availableSizes"`
	HasUnsized     bool          `json:"hasUnsized"`
	Message        string        `json:"message"`
	// Name is the Russian part of the name for this type and size, written like the base writes
	// names of this type («Керамическая фруктовница 9 дюймов для взрослых»).
	Name string `json:"name"`
}

// priceMode counts equal prices (4.5 and 4.50 are the same) and returns them most
// frequent first; mode is empty when the top count is shared.
func priceMode(prices []decimal.Decimal) (mode string, options []CeramicsPriceOption) {
	counts := map[string]int{}
	values := map[string]decimal.Decimal{}
	for _, price := range prices {
		key := price.String()
		counts[key]++
		values[key] = price
	}
	options = make([]CeramicsPriceOption, 0, len(counts))
	for key, count := range counts {
		options = append(options, CeramicsPriceOption{Price: key, Count: count})
	}
	sort.Slice(options, func(a, b int) bool {
		if options[a].Count != options[b].Count {
			return options[a].Count > options[b].Count
		}
		return values[options[a].Price].LessThan(values[options[b].Price])
	})
	if len(options) == 1 || (len(options) > 1 && options[0].Count > options[1].Count) {
		mode = options[0].Price
	}
	return mode, options
}

// formatCeramicSizeLong writes the unit as a word: 1 дюйм, 3 дюйма, 7 дюймов, 7.5 дюйма.
func formatCeramicSizeLong(size CeramicSize) string {
	if size.Unit != SizeUnitInch {
		return FormatCeramicSize(size)
	}
	number, err := strconv.Atoi(size.Value)
	word := "дюймов"
	switch {
	case err != nil:
		word = "дюйма"
	case number%100 >= 11 && number%100 <= 14:
	case number%10 == 1:
		word = "дюйм"
	case number%10 >= 2 && number%10 <= 4:
		word = "дюйма"
	}
	return size.Value + " " + word
}

// priceFor finds the price of a type and size in the loaded base; a nil size selects
// records stored without a size.
func (c *ceramicsContext) priceFor(item *ceramicsType, size *CeramicSize) CeramicsPriceResult {
	typeName := item.Name
	result := CeramicsPriceResult{Options: []CeramicsPriceOption{}, AvailableSizes: append([]CeramicSize{}, item.Sizes...), HasUnsized: item.Unsized, Name: ceramicRussianName(item.Template, item.Name, size)}
	prices := c.prices[priceKey(item.Key, size)]
	result.Total = len(prices)
	label := typeName
	if size != nil {
		label = typeName + " " + FormatCeramicSize(*size)
	}
	if len(prices) == 0 {
		result.Status = CeramicsPriceNotFound
		switch {
		case size != nil:
			result.Message = fmt.Sprintf("Для размера %s точная цена в базе не найдена. Требуется проверка.", FormatCeramicSize(*size))
		default:
			result.Message = fmt.Sprintf("Для «%s» без размера цены в базе нет. Требуется проверка.", typeName)
		}
		if available := describeAvailableSizes(result.AvailableSizes, result.HasUnsized); available != "" {
			result.Message += " В базе есть: " + available + "."
		}
		return result
	}
	mode, options := priceMode(prices)
	result.Options = options
	if mode == "" {
		result.Status = CeramicsPriceNeedReview
		parts := make([]string, 0, len(options))
		for _, option := range options {
			if option.Count == options[0].Count {
				parts = append(parts, fmt.Sprintf("%s — %d %s", option.Price, option.Count, timesWord(option.Count)))
			}
		}
		result.Message = fmt.Sprintf("Цена требует проверки: для «%s» нет одной самой частой цены (%s).", label, strings.Join(parts, "; "))
		return result
	}
	result.Status = CeramicsPriceMatched
	result.Price = mode
	if len(prices) == 1 {
		result.Message = fmt.Sprintf("Единственная цена в базе для «%s».", label)
	} else {
		result.Message = fmt.Sprintf("Наиболее часто встречающаяся цена в базе для «%s»: %d из %d записей.", label, options[0].Count, len(prices))
	}
	return result
}

func timesWord(count int) string {
	if count%10 >= 2 && count%10 <= 4 && (count%100 < 12 || count%100 > 14) {
		return "раза"
	}
	return "раз"
}

func describeAvailableSizes(sizes []CeramicSize, unsized bool) string {
	parts := make([]string, 0, len(sizes)+1)
	for _, size := range sizes {
		parts = append(parts, FormatCeramicSize(size))
	}
	if unsized {
		parts = append(parts, "без размера")
	}
	return strings.Join(parts, ", ")
}

func priceKey(typeKey string, size *CeramicSize) string {
	if size == nil {
		return typeKey + "|"
	}
	return typeKey + "|" + size.key()
}
