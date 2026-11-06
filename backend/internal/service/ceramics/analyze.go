package ceramics

// Ceramics lookup and analysis: known marks come from the base, the rest are resolved here.

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
)

func foundByMark(key string, product repository.CeramicProduct) CeramicsResolution {
	dto := ceramicsProductDTO(product)
	size := catalogSize(product.SizeValue, product.SizeUnit)
	resolution := CeramicsResolution{
		Key: key, Mark: product.Mark, MarkKey: product.MarkKey, Status: CeramicsFoundByMark, Product: &dto,
		ProductType: dto.ProductType, TypeStatus: CeramicsPartMatched, TypeCandidates: []CeramicsAICandidate{},
		Size: size, SizeStatus: CeramicsPartMatched, SizeOptions: []CeramicsSizeOption{},
		Price:        CeramicsPriceResult{Status: CeramicsPriceMatched, Price: dto.Price, Options: []CeramicsPriceOption{}, AvailableSizes: []CeramicSize{}, Total: 1, Name: dto.Name},
		DisplayName:  dto.Name,
		ChineseTerms: []ChineseTerm{}, Warnings: []string{},
		Evidence: []CeramicsEvidence{
			{Field: "type", Source: SourceMarkExact, Text: fmt.Sprintf("Марка %s найдена в базе: «%s»", product.Mark, product.Name)},
			{Field: "price", Source: SourceMarkExact, Text: "Цена из записи базы для этой марки"},
		},
	}
	if size == nil {
		resolution.SizeStatus = CeramicsPartNotNeeded
	} else {
		resolution.Evidence = append(resolution.Evidence, CeramicsEvidence{Field: "size", Source: SourceMarkExact, Text: "Размер из наименования этой марки в базе"})
	}
	return resolution
}

// Lookup finds known marks without AI; unknown marks are simply absent from the answer.
func (s *CeramicsService) Lookup(ctx context.Context, marks []string) ([]CeramicsResolution, error) {
	if len(marks) > maxCeramicsLookupMarks {
		return nil, &service.ValidationError{Field: "marks", Message: fmt.Sprintf("Не больше %d марок за раз", maxCeramicsLookupMarks)}
	}
	keys := make([]string, 0, len(marks))
	seen := map[string]bool{}
	for _, mark := range marks {
		if key := NormalizeCeramicMark(mark); key != "" && len(key) <= 128 && !seen[key] {
			seen[key] = true
			keys = append(keys, key)
		}
	}
	products, err := s.repository.CeramicProductsByMarkKeys(ctx, keys)
	if err != nil {
		return nil, err
	}
	result := make([]CeramicsResolution, 0, len(products))
	for _, product := range products {
		result = append(result, foundByMark(product.MarkKey, product))
	}
	return result, nil
}

// validCeramicsPhoto accepts an empty value or a JPEG/PNG data URL (photo of the client's file
// for the model; photos are not stored).
func validCeramicsPhoto(photo string) (string, error) {
	photo = strings.TrimSpace(photo)
	if photo == "" {
		return "", nil
	}
	if !strings.HasPrefix(photo, "data:image/jpeg;base64,") && !strings.HasPrefix(photo, "data:image/png;base64,") {
		return "", &service.ValidationError{Field: "photo", Message: "Фото — JPG или PNG"}
	}
	if len(photo) > maxCeramicsPhotoBytes {
		return "", &service.ValidationError{Field: "photo", Message: "Фото слишком большое"}
	}
	return photo, nil
}

func validateCeramicsInputs(items []CeramicsInputItem) error {
	if len(items) == 0 {
		return &service.ValidationError{Field: "items", Message: "Нет товаров для подбора"}
	}
	if len(items) > maxCeramicsAnalyzeItems {
		return &service.ValidationError{Field: "items", Message: fmt.Sprintf("Не больше %d товаров за запрос", maxCeramicsAnalyzeItems)}
	}
	for index := range items {
		item := &items[index]
		item.Key = service.TruncateRunes(strings.TrimSpace(item.Key), 200)
		item.Mark = service.TruncateRunes(strings.TrimSpace(toHalfWidth(item.Mark)), 128)
		item.ChineseName = service.TruncateRunes(strings.TrimSpace(item.ChineseName), maxCeramicsTextRunes)
		item.RussianName = service.TruncateRunes(strings.TrimSpace(item.RussianName), maxCeramicsTextRunes)
		if item.Mark == "" && item.ChineseName == "" {
			return &service.ValidationError{Field: "items", Message: fmt.Sprintf("Товар %d: нет ни марки, ни китайского наименования", index+1)}
		}
		photo, err := validCeramicsPhoto(item.Photo)
		if err != nil {
			return err
		}
		item.Photo = photo
	}
	return nil
}

// Analyze resolves a small batch of marks: known marks come from the base, the rest are
// analysed once per mark.
func (s *CeramicsService) Analyze(ctx context.Context, items []CeramicsInputItem) ([]CeramicsResolution, error) {
	if err := validateCeramicsInputs(items); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(items))
	for _, item := range items {
		if key := NormalizeCeramicMark(item.Mark); key != "" {
			keys = append(keys, key)
		}
	}
	known, err := s.repository.CeramicProductsByMarkKeys(ctx, keys)
	if err != nil {
		return nil, err
	}
	knownByKey := map[string]repository.CeramicProduct{}
	for _, product := range known {
		knownByKey[product.MarkKey] = product
	}
	cc, err := s.loadContext(ctx)
	if err != nil {
		return nil, err
	}

	results := make([]CeramicsResolution, len(items))
	// The same mark in one batch is analysed once.
	firstByKey := map[string]int{}
	var wait sync.WaitGroup
	slots := make(chan struct{}, ceramicsAnalyzeParallel)
	for index, item := range items {
		markKey := NormalizeCeramicMark(item.Mark)
		if product, ok := knownByKey[markKey]; ok && markKey != "" {
			results[index] = foundByMark(item.Key, product)
			continue
		}
		dedupKey := markKey + "|" + item.ChineseName
		if _, ok := firstByKey[dedupKey]; ok {
			continue
		}
		firstByKey[dedupKey] = index
		wait.Add(1)
		go func(index int, item CeramicsInputItem) {
			defer wait.Done()
			slots <- struct{}{}
			defer func() { <-slots }()
			results[index] = s.analyzeOne(ctx, cc, item)
		}(index, item)
	}
	wait.Wait()
	for index, item := range items {
		if results[index].Status != "" {
			continue
		}
		copied := results[firstByKey[NormalizeCeramicMark(item.Mark)+"|"+item.ChineseName]]
		copied.Key = item.Key
		results[index] = copied
	}
	return results, nil
}

func (s *CeramicsService) analyzeOne(ctx context.Context, cc *ceramicsContext, item CeramicsInputItem) CeramicsResolution {
	markKey := NormalizeCeramicMark(item.Mark)
	result := CeramicsResolution{
		Key: item.Key, Mark: item.Mark, MarkKey: markKey,
		TypeCandidates: []CeramicsAICandidate{}, SizeOptions: []CeramicsSizeOption{}, Evidence: []CeramicsEvidence{}, Warnings: []string{},
		ChineseTerms: chineseTerms(toHalfWidth(item.ChineseName)),
		Price:        CeramicsPriceResult{Status: CeramicsPriceNotChecked, Options: []CeramicsPriceOption{}, AvailableSizes: []CeramicSize{}},
	}
	if len(cc.types) == 0 {
		result.Status, result.TypeStatus, result.SizeStatus = CeramicsNotDetermined, CeramicsPartNotFound, CeramicsPartNotFound
		result.Warnings = append(result.Warnings, "База керамики пуста: сначала загрузите известные марки на вкладке «База марок»")
		return result
	}
	seriesSuggestions := inferSizeFromMarkPattern(markKey, cc.series)
	unitGuessed := decideCeramicsSize(&result, item, seriesSuggestions)
	s.decideCeramicsType(ctx, cc, &result, item, seriesSuggestions)

	typeItem := cc.typeByKey[normalizeCeramicTypeKey(result.ProductType)]
	// A number without a unit gets the unit the base uses for this type.
	if typeItem != nil && unitGuessed && result.Size != nil {
		units := map[string]bool{}
		for _, size := range typeItem.Sizes {
			units[size.Unit] = true
		}
		if len(units) == 1 {
			for unit := range units {
				result.Size.Unit = unit
			}
			for index := range result.SizeOptions {
				if result.SizeOptions[index].Source == SourceChineseNumber {
					result.SizeOptions[index].Size.Unit = result.Size.Unit
				}
			}
		}
	}
	// The same Chinese name with the same number is in the base: the number is its size.
	if result.SizeStatus == CeramicsPartAssumed && result.Size != nil {
		if match := cc.sizeByChinese[chineseFullKey(item.ChineseName)]; match != nil && !match.Conflicting && match.Size.Value == result.Size.Value {
			result.Size.Unit = match.Size.Unit
			result.SizeStatus = CeramicsPartMatched
			result.Evidence = append(result.Evidence, CeramicsEvidence{Field: "size", Source: SourceChineseNameMatch, Text: fmt.Sprintf("Такое же китайское название в базе у марки %s — размер %s", match.Mark, FormatCeramicSize(match.Size))})
		}
	}
	if typeItem != nil && result.SizeStatus == CeramicsPartNotFound && len(typeItem.Sizes) == 0 && typeItem.Unsized {
		result.SizeStatus = CeramicsPartNotNeeded
		result.Evidence = append(result.Evidence, CeramicsEvidence{Field: "size", Source: SourcePriceMode, Text: fmt.Sprintf("В базе «%s» записан без размера", typeItem.Name)})
	}
	if typeItem != nil && result.TypeStatus == CeramicsPartMatched && (result.Size != nil || result.SizeStatus == CeramicsPartNotNeeded) {
		result.Price = cc.priceFor(typeItem, result.Size)
		result.Evidence = append(result.Evidence, CeramicsEvidence{Field: "price", Source: SourcePriceMode, Text: result.Price.Message})
	} else if typeItem != nil && result.TypeStatus == CeramicsPartMatched {
		result.Price.AvailableSizes = append([]CeramicSize{}, typeItem.Sizes...)
		result.Price.HasUnsized = typeItem.Unsized
		result.Price.Message = "Цена не подобрана: сначала нужно определить размер. В базе есть: " + describeAvailableSizes(result.Price.AvailableSizes, result.Price.HasUnsized) + "."
	}
	if typeItem != nil && result.TypeStatus == CeramicsPartMatched {
		result.DisplayName = ceramicRussianName(typeItem.Template, typeItem.Name, result.Size)
	}

	switch {
	case result.TypeStatus == CeramicsPartNotFound:
		result.Status = CeramicsNotDetermined
	case result.TypeStatus == CeramicsPartMatched &&
		(result.SizeStatus == CeramicsPartMatched || result.SizeStatus == CeramicsPartNotNeeded) &&
		result.Price.Status == CeramicsPriceMatched:
		result.Status = CeramicsAuto
	default:
		result.Status = CeramicsNeedReview
	}
	return result
}
