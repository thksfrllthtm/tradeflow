package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

func (r *Repository) ListProductDirectory(ctx context.Context, category, status string, includeArchived bool) ([]ProductDirectoryEntry, error) {
	query := r.db.WithContext(ctx).Model(&ProductDirectoryEntry{}).Preload("TNVEDDirectoryEntry").Order("id ASC")
	if strings.TrimSpace(category) != "" {
		query = query.Where("category = ?", strings.TrimSpace(category))
	}
	if strings.TrimSpace(status) != "" && status != "all" {
		query = query.Where("verification_status = ?", strings.TrimSpace(status))
	}
	if !includeArchived {
		query = query.Where("archived_at IS NULL")
	}
	var entries []ProductDirectoryEntry
	return entries, query.Find(&entries).Error
}

func (r *Repository) ProductDirectoryEntryByID(ctx context.Context, id int64) (ProductDirectoryEntry, error) {
	var entry ProductDirectoryEntry
	err := r.db.WithContext(ctx).Preload("TNVEDDirectoryEntry").First(&entry, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ProductDirectoryEntry{}, ErrNotFound
	}
	return entry, err
}

func (r *Repository) CreateProductDirectoryEntry(ctx context.Context, entry ProductDirectoryEntry) (ProductDirectoryEntry, error) {
	if err := r.db.WithContext(ctx).Create(&entry).Error; err != nil {
		return ProductDirectoryEntry{}, err
	}
	return r.ProductDirectoryEntryByID(ctx, entry.ID)
}

func (r *Repository) UpdateProductDirectoryEntry(ctx context.Context, entry ProductDirectoryEntry) (ProductDirectoryEntry, error) {
	updates := map[string]any{
		"category": entry.Category, "name": entry.Name, "name_zh": entry.NameZH, "unit": entry.Unit,
		"company_coefficient": entry.CompanyCoefficient,
		"synonyms_ru":         entry.SynonymsRU, "synonyms_zh": entry.SynonymsZH, "synonyms_en": entry.SynonymsEN,
		"tnved_directory_entry_id": entry.TNVEDDirectoryEntryID, "verification_status": entry.VerificationStatus,
		"updated_by": entry.UpdatedBy, "updated_at": entry.UpdatedAt,
	}
	result := r.db.WithContext(ctx).Model(&ProductDirectoryEntry{}).Where("id = ?", entry.ID).Updates(updates)
	if result.Error != nil {
		return ProductDirectoryEntry{}, result.Error
	}
	if result.RowsAffected == 0 {
		return ProductDirectoryEntry{}, ErrNotFound
	}
	return r.ProductDirectoryEntryByID(ctx, entry.ID)
}

func (r *Repository) ArchiveProductDirectoryEntry(ctx context.Context, id, userID int64) error {
	result := r.db.WithContext(ctx).Model(&ProductDirectoryEntry{}).
		Where("id = ? AND archived_at IS NULL", id).
		Updates(map[string]any{"archived_at": gorm.Expr("now()"), "archived_by": userID, "updated_by": userID, "updated_at": gorm.Expr("now()")})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		var entry ProductDirectoryEntry
		if err := r.db.WithContext(ctx).First(&entry, id).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
	}
	return nil
}

func (r *Repository) RestoreProductDirectoryEntry(ctx context.Context, id, userID int64) error {
	result := r.db.WithContext(ctx).Model(&ProductDirectoryEntry{}).
		Where("id = ? AND archived_at IS NOT NULL", id).
		Updates(map[string]any{"archived_at": nil, "archived_by": nil, "updated_by": userID, "updated_at": time.Now().UTC()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		var entry ProductDirectoryEntry
		if err := r.db.WithContext(ctx).First(&entry, id).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
	}
	return nil
}
