package service

// Imported products of an estimate: listing, manual editing and manually uploaded images.

import (
	"bytes"
	"context"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime/multipart"
	"os"
	"regexp"
	"strings"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
)

func (s *ImportService) ListProducts(ctx context.Context, estimateID, ownerID int64) ([]repository.ImportedProduct, error) {
	if _, err := s.repository.EstimateByIDForOwner(ctx, estimateID, ownerID); err != nil {
		return nil, err
	}
	products, err := s.repository.ListProductsForEstimate(ctx, estimateID)
	if err != nil {
		return nil, err
	}
	return s.withImageAssetIDs(ctx, products)
}

func (s *ImportService) withImageAssetIDs(ctx context.Context, products []repository.ImportedProduct) ([]repository.ImportedProduct, error) {
	ids := make([]int64, 0, len(products))
	for _, product := range products {
		ids = append(ids, product.ID)
	}
	assets, err := s.repository.ImageAssetIDsForProducts(ctx, ids)
	if err != nil {
		return nil, err
	}
	for index := range products {
		products[index].ImageAssetIDs = assets[products[index].ID]
	}
	return products, nil
}

func (s *ImportService) UpdateProduct(ctx context.Context, productID, ownerID int64, input ProductInput) (repository.ImportedProduct, error) {
	product, err := s.repository.ProductByIDForOwner(ctx, productID, ownerID)
	if err != nil {
		return repository.ImportedProduct{}, err
	}
	updated, err := applyProductInput(product, input)
	if err != nil {
		return repository.ImportedProduct{}, err
	}
	updated.UpdatedBy = ownerID
	productDataChanged := product.Name != updated.Name || product.Translation != updated.Translation || product.SKU != updated.SKU || product.Material != updated.Material || product.Characteristics != updated.Characteristics || product.PhotoURL != updated.PhotoURL || string(product.AdditionalFields) != string(updated.AdditionalFields)
	if productDataChanged {
		if err := s.repository.MarkProductAIAnalysesRecheck(ctx, product.ID); err != nil {
			return repository.ImportedProduct{}, err
		}
		updated.AIDataRevision = product.AIDataRevision + 1
	}
	saved, err := s.repository.UpdateProduct(ctx, updated)
	if err != nil {
		return repository.ImportedProduct{}, err
	}
	products, err := s.withImageAssetIDs(ctx, []repository.ImportedProduct{saved})
	if err != nil {
		return repository.ImportedProduct{}, err
	}
	return products[0], nil
}

type ProductInput struct {
	Name             string            `json:"name"`
	Translation      string            `json:"translation"`
	SKU              string            `json:"sku"`
	Quantity         string            `json:"quantity"`
	QuantityUnit     string            `json:"quantityUnit"`
	Pairs            string            `json:"pairs"`
	Places           string            `json:"places"`
	PairsPerPlace    string            `json:"pairsPerPlace"`
	UnitPrice        string            `json:"unitPrice"`
	LineTotal        string            `json:"lineTotal"`
	Currency         string            `json:"currency"`
	Weight           string            `json:"weight"`
	GrossWeight      string            `json:"grossWeight"`
	GrossWeightUnit  string            `json:"grossWeightUnit"`
	GrossWeightBasis string            `json:"grossWeightBasis"`
	NetWeight        string            `json:"netWeight"`
	NetWeightUnit    string            `json:"netWeightUnit"`
	NetWeightBasis   string            `json:"netWeightBasis"`
	Material         string            `json:"material"`
	Characteristics  string            `json:"characteristics"`
	PhotoURL         string            `json:"photoUrl"`
	AdditionalFields map[string]string `json:"additionalFields"`
	PriceBasis       string            `json:"priceBasis"`
	PriceUnitLabel   string            `json:"priceUnitLabel"`
	WeightBasis      string            `json:"weightBasis"`
	Excluded         bool              `json:"excluded"`
	ExclusionReason  string            `json:"exclusionReason"`
	TNVEDCode        string            `json:"tnVedCode"`
	TNVEDCodeSource  string            `json:"tnVedCodeSource"`
}

func (s *ImportService) UploadManualImage(ctx context.Context, productID, ownerID int64, header *multipart.FileHeader) (repository.ImportAsset, error) {
	product, err := s.repository.ProductByIDForOwner(ctx, productID, ownerID)
	if err != nil {
		return repository.ImportAsset{}, err
	}
	if header == nil || header.Size <= 0 || header.Size > MaxImageBytes {
		return repository.ImportAsset{}, &ValidationError{Field: "image", Message: "Выберите JPG или PNG размером до 5 МБ"}
	}
	file, err := header.Open()
	if err != nil {
		return repository.ImportAsset{}, err
	}
	defer file.Close()
	limited := io.LimitReader(file, MaxImageBytes+1)
	contents, err := io.ReadAll(limited)
	if err != nil {
		return repository.ImportAsset{}, err
	}
	if len(contents) > MaxImageBytes {
		return repository.ImportAsset{}, &ValidationError{Field: "image", Message: "Изображение больше 5 МБ"}
	}
	config, format, err := image.DecodeConfig(bytes.NewReader(contents))
	if err != nil || config.Width < 1 || config.Height < 1 || (format != "jpeg" && format != "png") {
		return repository.ImportAsset{}, &ValidationError{Field: "image", Message: "Поддерживаются корректные изображения JPG и PNG"}
	}
	extension, mimeType := ".jpg", "image/jpeg"
	if format == "png" {
		extension, mimeType = ".png", "image/png"
	}
	storedPath, size, err := s.files.Save(bytes.NewReader(contents), "images", extension)
	if err != nil {
		return repository.ImportAsset{}, err
	}
	asset, err := s.repository.CreateAssetAndLink(ctx, repository.ImportAsset{ImportID: product.SourceImportID, StoredPath: storedPath, MIMEType: mimeType, FileSize: size, Kind: "manual"}, product.ID)
	if err != nil {
		_ = s.files.Remove(storedPath)
		return repository.ImportAsset{}, err
	}
	if err := s.repository.MarkProductAIAnalysesRecheck(ctx, product.ID); err != nil {
		return repository.ImportAsset{}, err
	}
	if err := s.repository.BumpProductAIRevision(ctx, product.ID, ownerID); err != nil {
		return repository.ImportAsset{}, err
	}
	return asset, nil
}

func (s *ImportService) OpenProductAsset(ctx context.Context, productID, assetID, ownerID int64) (*os.File, repository.ImportAsset, error) {
	asset, err := s.repository.AssetByIDForProductOwner(ctx, assetID, productID, ownerID)
	if err != nil {
		return nil, repository.ImportAsset{}, err
	}
	file, err := s.files.Open(asset.StoredPath)
	return file, asset, err
}

func applyProductInput(product repository.ImportedProduct, input ProductInput) (repository.ImportedProduct, error) {
	if strings.TrimSpace(input.Name) == "" {
		return repository.ImportedProduct{}, &ValidationError{Field: "name", Message: "Название товара обязательно"}
	}
	quantity, warning := parseOptionalDecimal(input.Quantity, "Количество", true)
	if warning != "" {
		return repository.ImportedProduct{}, &ValidationError{Field: "quantity", Message: warning}
	}
	pairs, warning := parseOptionalDecimal(input.Pairs, "pairs", false)
	if warning != "" {
		return repository.ImportedProduct{}, &ValidationError{Field: "pairs", Message: warning}
	}
	places, warning := parseOptionalDecimal(input.Places, "places", false)
	if warning != "" {
		return repository.ImportedProduct{}, &ValidationError{Field: "places", Message: warning}
	}
	pairsPerPlace, warning := parseOptionalDecimal(input.PairsPerPlace, "pairsPerPlace", false)
	if warning != "" {
		return repository.ImportedProduct{}, &ValidationError{Field: "pairsPerPlace", Message: warning}
	}
	price, warning := parseOptionalDecimal(input.UnitPrice, "Цена", false)
	if warning != "" {
		return repository.ImportedProduct{}, &ValidationError{Field: "unitPrice", Message: warning}
	}
	total, warning := parseOptionalDecimal(input.LineTotal, "Сумма строки", false)
	if warning != "" {
		return repository.ImportedProduct{}, &ValidationError{Field: "lineTotal", Message: warning}
	}
	weight, warning := parseOptionalDecimal(input.Weight, "Вес", false)
	if warning != "" {
		return repository.ImportedProduct{}, &ValidationError{Field: "weight", Message: warning}
	}
	if !oneOf(input.PriceBasis, "piece", "explicit_unit", "unknown") || !oneOf(input.WeightBasis, "line", "unit", "unknown") {
		return repository.ImportedProduct{}, &ValidationError{Field: "units", Message: "Укажите единицы цены и веса"}
	}
	if input.PriceBasis == "explicit_unit" && strings.TrimSpace(input.PriceUnitLabel) == "" {
		return repository.ImportedProduct{}, &ValidationError{Field: "priceUnitLabel", Message: "Укажите единицу цены"}
	}
	if input.Excluded && strings.TrimSpace(input.ExclusionReason) == "" {
		return repository.ImportedProduct{}, &ValidationError{Field: "exclusionReason", Message: "Укажите причину исключения позиции"}
	}
	tnved := strings.TrimSpace(input.TNVEDCode)
	if tnved != "" && !regexp.MustCompile(`^[0-9]{10}$`).MatchString(tnved) {
		return repository.ImportedProduct{}, &ValidationError{Field: "tnVedCode", Message: "Код ТН ВЭД должен содержать 10 цифр или быть пустым"}
	}
	currency := strings.ToUpper(strings.TrimSpace(input.Currency))
	if currency != "" && (len(currency) != 3 || !isLatinUppercase(currency)) {
		return repository.ImportedProduct{}, &ValidationError{Field: "currency", Message: "Валюта должна быть трёхбуквенным кодом"}
	}
	product.Name, product.Translation, product.SKU = strings.TrimSpace(input.Name), strings.TrimSpace(input.Translation), strings.TrimSpace(input.SKU)
	gross, warning := parseOptionalDecimal(input.GrossWeight, "Вес брутто", false)
	if warning != "" {
		return repository.ImportedProduct{}, &ValidationError{Field: "grossWeight", Message: warning}
	}
	net, warning := parseOptionalDecimal(input.NetWeight, "Вес нетто", false)
	if warning != "" {
		return repository.ImportedProduct{}, &ValidationError{Field: "netWeight", Message: warning}
	}
	if gross == nil {
		gross = weight
	}
	product.Quantity, product.QuantityUnit, product.Pairs, product.Places, product.PairsPerPlace, product.UnitPrice, product.LineTotal, product.Weight = quantity, strings.TrimSpace(input.QuantityUnit), pairs, places, pairsPerPlace, price, total, gross
	product.GrossWeight, product.GrossWeightUnit, product.GrossWeightBasis = gross, strings.TrimSpace(input.GrossWeightUnit), input.GrossWeightBasis
	product.NetWeight, product.NetWeightUnit, product.NetWeightBasis = net, strings.TrimSpace(input.NetWeightUnit), input.NetWeightBasis
	product.Currency, product.Material, product.Characteristics, product.PhotoURL = currency, strings.TrimSpace(input.Material), strings.TrimSpace(input.Characteristics), strings.TrimSpace(input.PhotoURL)
	product.AdditionalFields = encodeJSON(input.AdditionalFields)
	product.PriceBasis, product.PriceUnitLabel, product.WeightBasis = input.PriceBasis, strings.TrimSpace(input.PriceUnitLabel), input.WeightBasis
	product.Excluded, product.ExclusionReason = input.Excluded, strings.TrimSpace(input.ExclusionReason)
	product.TNVEDCode, product.TNVEDCodeSource = tnved, strings.TrimSpace(input.TNVEDCodeSource)
	return product, nil
}
