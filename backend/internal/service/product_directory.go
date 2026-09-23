package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
)

var ErrProductDirectoryConflict = errors.New("product directory entry already exists")

type ProductDirectoryInput struct {
	Category              string   `json:"category"`
	Name                  string   `json:"name"`
	NameZH                string   `json:"nameZh"`
	Unit                  string   `json:"unit"`
	CompanyCoefficient    string   `json:"companyCoefficient"`
	SynonymsRU            []string `json:"synonymsRu"`
	SynonymsZH            []string `json:"synonymsZh"`
	SynonymsEN            []string `json:"synonymsEn"`
	TNVEDDirectoryEntryID *int64   `json:"tnvedDirectoryEntryId"`
	VerificationStatus    string   `json:"verificationStatus"`
}

type ProductDirectoryFilter struct {
	Query           string
	Category        string
	Status          string
	IncludeArchived bool
}

type ProductDirectoryService struct{ repository *repository.Repository }

func NewProductDirectoryService(r *repository.Repository) *ProductDirectoryService {
	return &ProductDirectoryService{repository: r}
}

func validateProductDirectoryInput(input ProductDirectoryInput) (repository.ProductDirectoryEntry, error) {
	entry := repository.ProductDirectoryEntry{
		Category: strings.TrimSpace(input.Category), Name: strings.TrimSpace(input.Name), NameZH: strings.TrimSpace(input.NameZH), Unit: strings.TrimSpace(input.Unit),
		TNVEDDirectoryEntryID: input.TNVEDDirectoryEntryID, VerificationStatus: strings.TrimSpace(input.VerificationStatus),
	}
	companyCoefficient, err := parseOptionalNonNegative(input.CompanyCoefficient, "companyCoefficient")
	if err != nil {
		return entry, err
	}
	entry.CompanyCoefficient = companyCoefficient
	if entry.Category == "" {
		entry.Category = "Товары"
	}
	if len([]rune(entry.Category)) > 64 {
		return entry, &ValidationError{Field: "category", Message: "Категория слишком длинная"}
	}
	if entry.Name == "" {
		return entry, &ValidationError{Field: "name", Message: "Укажите наименование товара"}
	}
	if len([]rune(entry.Name)) > 300 {
		return entry, &ValidationError{Field: "name", Message: "Наименование слишком длинное"}
	}
	if len([]rune(entry.NameZH)) > 300 {
		return entry, &ValidationError{Field: "nameZh", Message: "Китайское наименование слишком длинное"}
	}
	if entry.Unit == "" {
		entry.Unit = "шт."
	}
	if len([]rune(entry.Unit)) > 64 {
		return entry, &ValidationError{Field: "unit", Message: "Единица измерения слишком длинная"}
	}
	if entry.TNVEDDirectoryEntryID == nil {
		return entry, &ValidationError{Field: "tnvedDirectoryEntryId", Message: "Привяжите к товару код ТН ВЭД"}
	}
	if entry.VerificationStatus == "" {
		entry.VerificationStatus = "needs_review"
	}
	if entry.VerificationStatus != "needs_review" && entry.VerificationStatus != "operator_verified" {
		return entry, &ValidationError{Field: "verificationStatus", Message: "Укажите статус проверки"}
	}
	entry.SynonymsRU = repository.JSON(mustJSON(normalizeTNVEDSynonyms(input.SynonymsRU)))
	entry.SynonymsZH = repository.JSON(mustJSON(normalizeTNVEDSynonyms(input.SynonymsZH)))
	entry.SynonymsEN = repository.JSON(mustJSON(normalizeTNVEDSynonyms(input.SynonymsEN)))
	return entry, nil
}

func (s *ProductDirectoryService) validateLink(ctx context.Context, entry repository.ProductDirectoryEntry) error {
	if entry.TNVEDDirectoryEntryID == nil {
		return nil
	}
	linked, err := s.repository.TNVEDDirectoryEntryByID(ctx, *entry.TNVEDDirectoryEntryID)
	if err != nil {
		return err
	}
	if linked.ArchivedAt != nil {
		return &ValidationError{Field: "tnvedDirectoryEntryId", Message: "Нельзя привязать архивную карточку ТН ВЭД"}
	}
	return nil
}

func (s *ProductDirectoryService) List(ctx context.Context, filter ProductDirectoryFilter) ([]repository.ProductDirectoryEntry, error) {
	entries, err := s.repository.ListProductDirectory(ctx, filter.Category, filter.Status, filter.IncludeArchived)
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(strings.TrimSpace(filter.Query))
	if q == "" {
		return entries, nil
	}
	result := entries[:0]
	for _, entry := range entries {
		parts := []string{entry.Name, entry.NameZH, entry.Category, entry.Unit}
		parts = append(parts, decodeJSONStrings(entry.SynonymsRU)...)
		parts = append(parts, decodeJSONStrings(entry.SynonymsZH)...)
		parts = append(parts, decodeJSONStrings(entry.SynonymsEN)...)
		if entry.TNVEDDirectoryEntry != nil {
			parts = append(parts, entry.TNVEDDirectoryEntry.Code, entry.TNVEDDirectoryEntry.Name, entry.TNVEDDirectoryEntry.NameZH)
		}
		for _, part := range parts {
			if strings.Contains(strings.ToLower(part), q) {
				result = append(result, entry)
				break
			}
		}
	}
	return result, nil
}

func (s *ProductDirectoryService) Create(ctx context.Context, userID int64, input ProductDirectoryInput) (repository.ProductDirectoryEntry, error) {
	entry, err := validateProductDirectoryInput(input)
	if err != nil {
		return repository.ProductDirectoryEntry{}, err
	}
	if err := s.validateLink(ctx, entry); err != nil {
		return repository.ProductDirectoryEntry{}, err
	}
	now := time.Now().UTC()
	entry.CreatedBy, entry.UpdatedBy, entry.CreatedAt, entry.UpdatedAt = userID, userID, now, now
	saved, err := s.repository.CreateProductDirectoryEntry(ctx, entry)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "product_directory_active_name_idx") {
		return repository.ProductDirectoryEntry{}, ErrProductDirectoryConflict
	}
	return saved, err
}

func (s *ProductDirectoryService) Update(ctx context.Context, id, userID int64, input ProductDirectoryInput) (repository.ProductDirectoryEntry, error) {
	entry, err := validateProductDirectoryInput(input)
	if err != nil {
		return repository.ProductDirectoryEntry{}, err
	}
	if err := s.validateLink(ctx, entry); err != nil {
		return repository.ProductDirectoryEntry{}, err
	}
	old, err := s.repository.ProductDirectoryEntryByID(ctx, id)
	if err != nil {
		return repository.ProductDirectoryEntry{}, err
	}
	entry.ID, entry.CreatedBy, entry.CreatedAt, entry.ArchivedAt, entry.ArchivedBy = id, old.CreatedBy, old.CreatedAt, old.ArchivedAt, old.ArchivedBy
	entry.UpdatedBy, entry.UpdatedAt = userID, time.Now().UTC()
	saved, err := s.repository.UpdateProductDirectoryEntry(ctx, entry)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "product_directory_active_name_idx") {
		return repository.ProductDirectoryEntry{}, ErrProductDirectoryConflict
	}
	return saved, err
}

func (s *ProductDirectoryService) Archive(ctx context.Context, id, userID int64) error {
	return s.repository.ArchiveProductDirectoryEntry(ctx, id, userID)
}

func (s *ProductDirectoryService) Restore(ctx context.Context, id, userID int64) error {
	if err := s.repository.RestoreProductDirectoryEntry(ctx, id, userID); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "product_directory_active_name_idx") {
			return ErrProductDirectoryConflict
		}
		return err
	}
	return nil
}

func (s *ProductDirectoryService) Copy(ctx context.Context, id, userID int64) (repository.ProductDirectoryEntry, error) {
	source, err := s.repository.ProductDirectoryEntryByID(ctx, id)
	if err != nil {
		return repository.ProductDirectoryEntry{}, err
	}
	input := ProductDirectoryInput{
		Category: source.Category, Name: source.Name + " (копия)", NameZH: source.NameZH, Unit: source.Unit,
		CompanyCoefficient: decimalString(source.CompanyCoefficient),
		SynonymsRU:         decodeJSONStrings(source.SynonymsRU), SynonymsZH: decodeJSONStrings(source.SynonymsZH), SynonymsEN: decodeJSONStrings(source.SynonymsEN),
		TNVEDDirectoryEntryID: source.TNVEDDirectoryEntryID, VerificationStatus: source.VerificationStatus,
	}
	return s.Create(ctx, userID, input)
}
