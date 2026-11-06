package ceramics

// Ceramics base records: list, create, update, delete and import.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/shopspring/decimal"
)

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
