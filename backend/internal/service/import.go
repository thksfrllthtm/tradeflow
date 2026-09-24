package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime/multipart"
	"path/filepath"
	"sort"
	"strings"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/xuri/excelize/v2"
)

const (
	MaxXLSXBytes         = 50 << 20
	MaxRowsPerSheet      = 5000
	MaxWorkbookRows      = 10000
	MaxColumns           = 100
	MaxPreviewRows       = 250
	MaxImageBytes        = 5 << 20
	MaxXLSXExpandedBytes = 100 << 20
	lineTotalTolerance   = "0.01"
)

var ErrDuplicateImport = errors.New("the same file has already been uploaded for this estimate")

var ErrReplacementRequired = errors.New("replacement confirmation is required")

type ImportService struct {
	repository *repository.Repository
	files      repository.FileStore
}

func NewImportService(repository *repository.Repository, files repository.FileStore) *ImportService {
	return &ImportService{repository: repository, files: files}
}

type SheetInfo struct {
	Name string `json:"name"`
	Rows int    `json:"rows"`
}

type ImportUploadResult struct {
	Import repository.EstimateImport `json:"import"`
	Sheets []SheetInfo               `json:"sheets"`
}

func (s *ImportService) LatestImport(ctx context.Context, estimateID, ownerID int64) (repository.EstimateImport, error) {
	if _, err := s.repository.EstimateByIDForOwner(ctx, estimateID, ownerID); err != nil {
		return repository.EstimateImport{}, err
	}
	return s.repository.LatestImportForEstimateOwner(ctx, estimateID, ownerID)
}

type Preview struct {
	ImportID         int64          `json:"importId"`
	Sheets           []SheetInfo    `json:"sheets"`
	Sheet            string         `json:"sheet"`
	HeaderRow        int            `json:"headerRow"`
	Headers          []Header       `json:"headers"`
	SuggestedMapping map[string]int `json:"suggestedMapping"`
	Rows             []PreviewRow   `json:"rows"`
	Pictures         []PictureHint  `json:"pictures"`
	Warnings         []string       `json:"warnings"`
}

type Header struct {
	Index int    `json:"index"`
	Value string `json:"value"`
}

type PreviewRow struct {
	SourceRow       int            `json:"sourceRow"`
	Values          map[int]string `json:"values"`
	DefaultIncluded bool           `json:"defaultIncluded"`
	Warnings        []string       `json:"warnings"`
}

type PictureHint struct {
	Sheet     string `json:"sheet"`
	Cell      string `json:"cell"`
	SourceRow int    `json:"sourceRow"`
	Supported bool   `json:"supported"`
	Message   string `json:"message"`
}

type ConfirmInput struct {
	Sheet           string         `json:"sheet"`
	HeaderRow       int            `json:"headerRow"`
	Mapping         map[string]int `json:"mapping"`
	SelectedRows    []int          `json:"selectedRows"`
	PriceBasis      string         `json:"priceBasis"`
	PriceUnitLabel  string         `json:"priceUnitLabel"`
	WeightBasis     string         `json:"weightBasis"`
	ReplaceExisting bool           `json:"replaceExisting"`
}

func (s *ImportService) Upload(ctx context.Context, estimateID, ownerID int64, header *multipart.FileHeader) (ImportUploadResult, error) {
	if _, err := s.repository.EstimateByIDForOwner(ctx, estimateID, ownerID); err != nil {
		return ImportUploadResult{}, err
	}
	if header == nil || header.Size <= 0 {
		return ImportUploadResult{}, &ValidationError{Field: "file", Message: "Выберите непустой XLSX-файл"}
	}
	if header.Size > MaxXLSXBytes {
		return ImportUploadResult{}, &ValidationError{Field: "file", Message: "XLSX-файл больше 50 МБ не поддерживается"}
	}
	if strings.ToLower(filepath.Ext(header.Filename)) != ".xlsx" {
		return ImportUploadResult{}, &ValidationError{Field: "file", Message: "Поддерживаются только обычные файлы .xlsx"}
	}
	file, err := header.Open()
	if err != nil {
		return ImportUploadResult{}, fmt.Errorf("open uploaded file: %w", err)
	}
	defer file.Close()

	hash := sha256.New()
	storedPath, size, err := s.files.Save(io.TeeReader(io.LimitReader(file, MaxXLSXBytes+1), hash), "imports", ".xlsx")
	if err != nil {
		return ImportUploadResult{}, fmt.Errorf("save uploaded file: %w", err)
	}
	if size > MaxXLSXBytes {
		_ = s.files.Remove(storedPath)
		return ImportUploadResult{}, &ValidationError{Field: "file", Message: "XLSX-файл больше 50 МБ не поддерживается"}
	}

	sheets, err := s.inspectWorkbook(storedPath)
	if err != nil {
		_ = s.files.Remove(storedPath)
		return ImportUploadResult{}, err
	}
	hashValue := hex.EncodeToString(hash.Sum(nil))
	if previous, err := s.repository.LatestImportWithHash(ctx, estimateID, hashValue); err == nil {
		_ = s.files.Remove(storedPath)
		return ImportUploadResult{Import: previous, Sheets: sheets}, ErrDuplicateImport
	} else if !errors.Is(err, repository.ErrNotFound) {
		_ = s.files.Remove(storedPath)
		return ImportUploadResult{}, err
	}

	item, err := s.repository.CreateImport(ctx, repository.EstimateImport{
		EstimateID: estimateID, OriginalFilename: safeFilename(header.Filename), StoredPath: storedPath,
		SHA256: hashValue, FileSize: size, Status: "uploaded", CreatedBy: ownerID,
	})
	if err != nil {
		_ = s.files.Remove(storedPath)
		return ImportUploadResult{}, err
	}
	return ImportUploadResult{Import: item, Sheets: sheets}, nil
}

func (s *ImportService) Preview(ctx context.Context, importID, ownerID int64, sheet string, headerRow int) (Preview, error) {
	item, err := s.repository.ImportByIDForOwner(ctx, importID, ownerID)
	if err != nil {
		return Preview{}, err
	}
	return s.previewForImport(item, sheet, headerRow)
}

func (s *ImportService) Confirm(ctx context.Context, importID, ownerID int64, input ConfirmInput) ([]repository.ImportedProduct, error) {
	item, err := s.repository.ImportByIDForOwner(ctx, importID, ownerID)
	if err != nil {
		return nil, err
	}
	if err := validateConfirmInput(input); err != nil {
		return nil, err
	}
	preview, err := s.previewForImport(item, input.Sheet, input.HeaderRow)
	if err != nil {
		return nil, err
	}
	knownRows := make(map[int]PreviewRow, len(preview.Rows))
	for _, row := range preview.Rows {
		knownRows[row.SourceRow] = row
	}
	if len(input.SelectedRows) == 0 {
		return nil, &ValidationError{Field: "selectedRows", Message: "Выберите хотя бы одну строку для импорта"}
	}
	selected := make(map[int]bool, len(input.SelectedRows))
	for _, row := range input.SelectedRows {
		if _, exists := knownRows[row]; !exists {
			return nil, &ValidationError{Field: "selectedRows", Message: "Выбрана строка вне предварительного просмотра"}
		}
		selected[row] = true
	}
	count, err := s.repository.ProductCount(ctx, item.EstimateID)
	if err != nil {
		return nil, err
	}
	if count > 0 && !input.ReplaceExisting {
		return nil, ErrReplacementRequired
	}

	file, err := s.files.Open(item.StoredPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	workbook, err := excelize.OpenReader(file)
	if err != nil {
		return nil, &ValidationError{Field: "file", Message: "Не удалось прочитать сохранённый XLSX-файл"}
	}
	defer workbook.Close()
	rows, err := workbook.GetRows(input.Sheet)
	if err != nil {
		return nil, &ValidationError{Field: "sheet", Message: "Выбранный лист недоступен"}
	}

	products := make([]repository.ImportedProduct, 0, len(selected))
	for sourceRow := range selected {
		if sourceRow <= input.HeaderRow || sourceRow > len(rows) {
			continue
		}
		product, err := productFromRow(workbook, input.Sheet, sourceRow, rows[sourceRow-1], input)
		if err != nil {
			return nil, err
		}
		products = append(products, product)
	}
	sort.Slice(products, func(i, j int) bool { return products[i].SourceRow < products[j].SourceRow })
	assets, links, pictureWarnings := s.extractPictures(workbook, input.Sheet, products)
	for productIndex, warnings := range pictureWarnings {
		current := decodeWarnings(products[productIndex].Warnings)
		current = append(current, warnings...)
		products[productIndex].Warnings = encodeJSON(current)
		if len(current) > 0 {
			products[productIndex].ImportStatus = "needs_correction"
		}
	}
	savedProducts, _, err := s.repository.ReplaceProducts(ctx, item.EstimateID, item.ID, ownerID, products, assets, links)
	if err != nil {
		return nil, err
	}
	return s.withImageAssetIDs(ctx, savedProducts)
}
