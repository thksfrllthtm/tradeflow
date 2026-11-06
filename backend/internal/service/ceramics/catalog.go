package ceramics

// Ceramics base loaded once per request: types, sizes, mark series and prices.

import (
	"context"
	"sort"
	"unicode/utf8"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/shopspring/decimal"
)

type ceramicCatalogEntry struct {
	Mark    string
	MarkKey string
	TypeKey string
	Size    *CeramicSize
}

// ceramicsType is a product type of the base with its sizes and the way its names are written.
type ceramicsType struct {
	Name     string
	Key      string
	Count    int
	Template string
	Sizes    []CeramicSize
	Unsized  bool
}

// ceramicsContext is the base read once per request: types, marks, series and prices.
type ceramicsContext struct {
	types     []*ceramicsType
	typeByKey map[string]*ceramicsType
	typeNames []string
	series    map[string]*markSeries
	baseNames map[string]map[string][]string
	prices    map[string][]decimal.Decimal
	// sizeByChinese: size of the base records with exactly this Chinese name (numbers included).
	sizeByChinese map[string]*chineseSizeMatch
}

type chineseSizeMatch struct {
	Size        CeramicSize
	Mark        string
	Conflicting bool
}

func (s *CeramicsService) loadContext(ctx context.Context) (*ceramicsContext, error) {
	rows, err := s.repository.CeramicCatalog(ctx)
	if err != nil {
		return nil, err
	}
	return newCeramicsContext(rows), nil
}

func newCeramicsContext(rows []repository.CeramicProduct) *ceramicsContext {
	c := &ceramicsContext{typeByKey: map[string]*ceramicsType{}, baseNames: map[string]map[string][]string{}, prices: map[string][]decimal.Decimal{}, sizeByChinese: map[string]*chineseSizeMatch{}}
	entries := make([]ceramicCatalogEntry, 0, len(rows))
	sizeSeen := map[string]bool{}
	for _, row := range rows {
		item := c.typeByKey[row.ProductTypeKey]
		if item == nil {
			item = &ceramicsType{Name: row.ProductType, Key: row.ProductTypeKey}
			c.typeByKey[row.ProductTypeKey] = item
			c.types = append(c.types, item)
		}
		item.Count++
		size := catalogSize(row.SizeValue, row.SizeUnit)
		entries = append(entries, ceramicCatalogEntry{Mark: row.Mark, MarkKey: row.MarkKey, TypeKey: row.ProductTypeKey, Size: size})
		key := priceKey(row.ProductTypeKey, size)
		c.prices[key] = append(c.prices[key], row.Price)
		if size == nil {
			item.Unsized = true
		} else {
			if !sizeSeen[key] {
				sizeSeen[key] = true
				item.Sizes = append(item.Sizes, *size)
			}
			if item.Template == "" {
				item.Template = parseCeramicName(row.Name).Template
			}
		}
		if full := chineseFullKey(row.ChineseName); size != nil && utf8.RuneCountInString(full) >= 2 {
			if match := c.sizeByChinese[full]; match == nil {
				c.sizeByChinese[full] = &chineseSizeMatch{Size: *size, Mark: row.Mark}
			} else if match.Size.key() != size.key() {
				match.Conflicting = true
			}
		}
		if base := chineseBaseName(row.ChineseName); utf8.RuneCountInString(base) >= 2 {
			if c.baseNames[base] == nil {
				c.baseNames[base] = map[string][]string{}
			}
			c.baseNames[base][row.ProductTypeKey] = append(c.baseNames[base][row.ProductTypeKey], row.Mark)
		}
	}
	sort.Slice(c.types, func(a, b int) bool { return c.types[a].Name < c.types[b].Name })
	for _, item := range c.types {
		c.typeNames = append(c.typeNames, item.Name)
		sort.Slice(item.Sizes, func(a, b int) bool {
			if item.Sizes[a].Unit != item.Sizes[b].Unit {
				return item.Sizes[a].Unit < item.Sizes[b].Unit
			}
			left, _ := decimal.NewFromString(item.Sizes[a].Value)
			right, _ := decimal.NewFromString(item.Sizes[b].Value)
			return left.LessThan(right)
		})
	}
	c.series = buildMarkSeries(entries)
	return c
}
