package ceramics

// Ceramics module: name and price of ceramic tableware by mark, Chinese name and photo.
// Separate from estimates and the customs calculation.
//
// A base record is the company's name, mark and price. Product type, size and Chinese part are
// read from the name («Керамическая фруктовница 7 дюймов для взрослых/ 成年人陶瓷盘7高脚水果盘»).
//
// Order: exact mark in the base → size from the Chinese name / mark series → product type
// (same Chinese name in the base, otherwise AI choosing only from the base types) → the most
// frequent price of this type and size. Anything uncertain is NEED_REVIEW for the operator;
// nothing is saved without the operator's confirmation.

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/ai"
	"github.com/shopspring/decimal"
)

// Result statuses of a row.
const (
	CeramicsFoundByMark   = "FOUND_BY_MARK"
	CeramicsAuto          = "AUTO"
	CeramicsNeedReview    = "NEED_REVIEW"
	CeramicsNotDetermined = "NOT_DETERMINED"
)

// Statuses of the product type and the size.
const (
	CeramicsPartMatched    = "MATCHED"
	CeramicsPartAssumed    = "ASSUMED"
	CeramicsPartNeedReview = "NEED_REVIEW"
	CeramicsPartNotFound   = "NOT_FOUND"
	// The type is stored in the base without sizes.
	CeramicsPartNotNeeded = "NOT_NEEDED"
)

// Evidence sources shown to the operator.
const (
	SourceMarkExact        = "mark_exact"
	SourceChineseName      = "chinese_name"
	SourceChineseNumber    = "chinese_number"
	SourceChineseNameMatch = "chinese_name_match"
	SourceMarkPattern      = "mark_pattern"
	SourceRussianName      = "russian_name"
	SourceAI               = "ai"
	SourcePriceMode        = "price_mode"
)

const (
	maxCeramicsLookupMarks  = 5000
	maxCeramicsAnalyzeItems = 5
	maxCeramicsImportItems  = 5000
	maxCeramicsPhotoBytes   = 400 << 10
	maxCeramicsTextRunes    = 500
	ceramicsAnalyzeParallel = 3
)

type CeramicsService struct {
	repository *repository.Repository
	ai         *CeramicsAIClient
}

func NewCeramicsService(repo *repository.Repository, ai *CeramicsAIClient) *CeramicsService {
	return &CeramicsService{repository: repo, ai: ai}
}

// CeramicsProduct is a base record: name, mark and price, with the type, size and Chinese part
// read from the name.
type CeramicsProduct struct {
	ID          int64  `json:"id"`
	Mark        string `json:"mark"`
	Name        string `json:"name"`
	Price       string `json:"price"`
	ProductType string `json:"productType"`
	SizeValue   string `json:"sizeValue"`
	SizeUnit    string `json:"sizeUnit"`
	ChineseName string `json:"chineseName"`
	CreatedAt   string `json:"createdAt"`
	UpdatedAt   string `json:"updatedAt"`
}

type CeramicsProductType struct {
	Name         string `json:"name"`
	ProductCount int64  `json:"productCount"`
}

// CeramicsProductInput is everything the base stores.
type CeramicsProductInput struct {
	Mark  string `json:"mark"`
	Name  string `json:"name"`
	Price string `json:"price"`
}

type CeramicsEvidence struct {
	// Field: "type", "size" or "price".
	Field  string `json:"field"`
	Source string `json:"source"`
	Text   string `json:"text"`
}

type CeramicsSizeOption struct {
	Size   CeramicSize `json:"size"`
	Source string      `json:"source"`
	Detail string      `json:"detail"`
}

// CeramicsResolution is the answer for one mark.
type CeramicsResolution struct {
	Key            string                `json:"key"`
	Mark           string                `json:"mark"`
	MarkKey        string                `json:"markKey"`
	Status         string                `json:"status"`
	Product        *CeramicsProduct      `json:"product,omitempty"`
	ProductType    string                `json:"productType"`
	TypeStatus     string                `json:"typeStatus"`
	TypeCandidates []CeramicsAICandidate `json:"typeCandidates"`
	Size           *CeramicSize          `json:"size"`
	SizeStatus     string                `json:"sizeStatus"`
	SizeOptions    []CeramicsSizeOption  `json:"sizeOptions"`
	Price          CeramicsPriceResult   `json:"price"`
	// DisplayName: the base name for a found mark, otherwise the Russian part of the name
	// written like the other names of this type.
	DisplayName  string             `json:"displayName"`
	ChineseTerms []ChineseTerm      `json:"chineseTerms"`
	Evidence     []CeramicsEvidence `json:"evidence"`
	Warnings     []string           `json:"warnings"`
	AIUsed       bool               `json:"aiUsed"`
}

type CeramicsInputItem struct {
	Key         string `json:"key"`
	Mark        string `json:"mark"`
	ChineseName string `json:"chineseName"`
	RussianName string `json:"russianName"`
	Photo       string `json:"photo"`
}

type CeramicsImportResult struct {
	Created int `json:"created"`
	Updated int `json:"updated"`
	Skipped int `json:"skipped"`
}

type CeramicsAIStatus struct {
	Configured bool `json:"configured"`
	Vision     bool `json:"vision"`
}

func (s *CeramicsService) AIStatus() CeramicsAIStatus {
	return CeramicsAIStatus{Configured: s.ai.Configured(), Vision: s.ai.VisionEnabled()}
}

// ── Base ──

func ceramicsProductDTO(product repository.CeramicProduct) CeramicsProduct {
	dto := CeramicsProduct{
		ID: product.ID, Mark: product.Mark, Name: product.Name, Price: product.Price.String(),
		ProductType: product.ProductType, ChineseName: product.ChineseName,
		CreatedAt: product.CreatedAt.UTC().Format("2006-01-02T15:04:05Z"),
		UpdatedAt: product.UpdatedAt.UTC().Format("2006-01-02T15:04:05Z"),
	}
	if size := catalogSize(product.SizeValue, product.SizeUnit); size != nil {
		dto.SizeValue, dto.SizeUnit = size.Value, size.Unit
	}
	return dto
}

func catalogSize(value *decimal.Decimal, unit *string) *CeramicSize {
	if value == nil || unit == nil {
		return nil
	}
	size := newCeramicSize(*value, *unit)
	return &size
}

func (s *CeramicsService) Types(ctx context.Context) ([]CeramicsProductType, error) {
	types, err := s.repository.CeramicProductTypes(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]CeramicsProductType, 0, len(types))
	for _, item := range types {
		result = append(result, CeramicsProductType{Name: item.Name, ProductCount: item.ProductCount})
	}
	return result, nil
}

func (s *CeramicsService) List(ctx context.Context, query, productType string, limit, offset int) ([]CeramicsProduct, int64, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	filter := repository.CeramicProductFilter{Query: query, TypeKey: normalizeCeramicTypeKey(productType), Limit: limit, Offset: offset}
	products, total, err := s.repository.ListCeramicProducts(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	result := make([]CeramicsProduct, 0, len(products))
	for _, product := range products {
		result = append(result, ceramicsProductDTO(product))
	}
	return result, total, nil
}

// validateCeramicsProductInput checks the three fields and reads the type, size and Chinese
// part from the name.
func validateCeramicsProductInput(input CeramicsProductInput) (repository.CeramicProduct, error) {
	mark := strings.TrimSpace(toHalfWidth(input.Mark))
	markKey := NormalizeCeramicMark(mark)
	if markKey == "" || len(markKey) > 128 {
		return repository.CeramicProduct{}, &service.ValidationError{Field: "mark", Message: "Укажите марку (до 128 символов)"}
	}
	name := strings.Join(strings.Fields(input.Name), " ")
	if name == "" || utf8.RuneCountInString(name) > maxCeramicsTextRunes {
		return repository.CeramicProduct{}, &service.ValidationError{Field: "name", Message: fmt.Sprintf("Укажите наименование (до %d символов)", maxCeramicsTextRunes)}
	}
	price, err := decimal.NewFromString(strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(input.Price, " ", ""), ",", ".")))
	if err != nil || !price.IsPositive() || price.Exponent() < -4 || price.GreaterThanOrEqual(decimal.NewFromInt(10000000000)) {
		return repository.CeramicProduct{}, &service.ValidationError{Field: "price", Message: "Цена — положительное число, не больше четырёх знаков после точки"}
	}
	parsed := parseCeramicName(name)
	product := repository.CeramicProduct{
		Mark: mark, MarkKey: markKey, Name: name, Price: price,
		ProductType: parsed.ProductType, ProductTypeKey: normalizeCeramicTypeKey(parsed.ProductType), ChineseName: parsed.Chinese,
	}
	if parsed.Size != nil {
		value := decimal.RequireFromString(parsed.Size.Value)
		unit := parsed.Size.Unit
		product.SizeValue, product.SizeUnit = &value, &unit
	}
	return product, nil
}

func (s *CeramicsService) Create(ctx context.Context, userID int64, input CeramicsProductInput) (CeramicsProduct, error) {
	product, err := validateCeramicsProductInput(input)
	if err != nil {
		return CeramicsProduct{}, err
	}
	product.CreatedBy, product.UpdatedBy = &userID, &userID
	saved, err := s.repository.SaveCeramicProduct(ctx, product)
	if errors.Is(err, repository.ErrCeramicMarkExists) {
		return CeramicsProduct{}, &service.ValidationError{Field: "mark", Message: fmt.Sprintf("Марка %s уже есть в базе", product.Mark)}
	}
	if err != nil {
		return CeramicsProduct{}, err
	}
	return ceramicsProductDTO(saved), nil
}

func (s *CeramicsService) Update(ctx context.Context, id, userID int64, input CeramicsProductInput) (CeramicsProduct, error) {
	product, err := validateCeramicsProductInput(input)
	if err != nil {
		return CeramicsProduct{}, err
	}
	product.ID = id
	product.UpdatedBy = &userID
	saved, err := s.repository.SaveCeramicProduct(ctx, product)
	if errors.Is(err, repository.ErrCeramicMarkExists) {
		return CeramicsProduct{}, &service.ValidationError{Field: "mark", Message: fmt.Sprintf("Марка %s уже есть в базе у другой записи", product.Mark)}
	}
	if err != nil {
		return CeramicsProduct{}, err
	}
	return ceramicsProductDTO(saved), nil
}

func (s *CeramicsService) Delete(ctx context.Context, id int64) error {
	return s.repository.DeleteCeramicProduct(ctx, id)
}

// Import adds many records at once (the company's existing price list).
func (s *CeramicsService) Import(ctx context.Context, userID int64, inputs []CeramicsProductInput, overwrite bool) (CeramicsImportResult, error) {
	if len(inputs) == 0 {
		return CeramicsImportResult{}, &service.ValidationError{Field: "items", Message: "Нет строк для загрузки"}
	}
	if len(inputs) > maxCeramicsImportItems {
		return CeramicsImportResult{}, &service.ValidationError{Field: "items", Message: fmt.Sprintf("Не больше %d строк за раз", maxCeramicsImportItems)}
	}
	products := make([]repository.CeramicProduct, 0, len(inputs))
	seen := map[string]int{}
	for index, input := range inputs {
		product, err := validateCeramicsProductInput(input)
		if err != nil {
			var validation *service.ValidationError
			if errors.As(err, &validation) {
				return CeramicsImportResult{}, &service.ValidationError{Field: validation.Field, Message: fmt.Sprintf("Строка %d: %s", index+1, validation.Message)}
			}
			return CeramicsImportResult{}, err
		}
		if first, ok := seen[product.MarkKey]; ok {
			return CeramicsImportResult{}, &service.ValidationError{Field: "mark", Message: fmt.Sprintf("Марка %s повторяется в строках %d и %d", product.Mark, first, index+1)}
		}
		seen[product.MarkKey] = index + 1
		product.CreatedBy, product.UpdatedBy = &userID, &userID
		products = append(products, product)
	}
	created, updated, skipped, err := s.repository.ImportCeramicProducts(ctx, products, overwrite)
	return CeramicsImportResult{Created: created, Updated: updated, Skipped: skipped}, err
}

// ── Loaded base ──

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

// Price answers the operator's manual choice of type and size.
func (s *CeramicsService) Price(ctx context.Context, productType, sizeValue, sizeUnit string) (CeramicsPriceResult, error) {
	cc, err := s.loadContext(ctx)
	if err != nil {
		return CeramicsPriceResult{}, err
	}
	item, ok := cc.typeByKey[normalizeCeramicTypeKey(productType)]
	if !ok {
		return CeramicsPriceResult{}, &service.ValidationError{Field: "productType", Message: "Такого типа товара нет в базе"}
	}
	var size *CeramicSize
	if value := strings.TrimSpace(strings.ReplaceAll(sizeValue, ",", ".")); value != "" {
		parsed, err := decimal.NewFromString(value)
		if err != nil || !parsed.IsPositive() || !validSizeUnit(sizeUnit) {
			return CeramicsPriceResult{}, &service.ValidationError{Field: "sizeValue", Message: "Проверьте размер и единицу"}
		}
		built := newCeramicSize(parsed, sizeUnit)
		size = &built
	}
	return cc.priceFor(item, size), nil
}

// ── Lookup and analysis ──

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
			if !containsString(seriesTypes, typeKey) {
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
		if typeItem := cc.typeByKey[typeKey]; typeItem != nil && !containsString(localNames, typeItem.Name) {
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

func containsString(values []string, value string) bool {
	for _, existing := range values {
		if existing == value {
			return true
		}
	}
	return false
}

func containsCandidate(candidates []CeramicsAICandidate, name string) bool {
	for _, candidate := range candidates {
		if candidate.ProductType == name {
			return true
		}
	}
	return false
}
