package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

func (r *Repository) ListTNVEDDirectory(ctx context.Context, category, status string, includeArchived bool) ([]TNVEDDirectoryEntry, error) {
	query := r.db.WithContext(ctx).Model(&TNVEDDirectoryEntry{}).Order("id ASC")
	if strings.TrimSpace(category) != "" {
		query = query.Where("category = ?", strings.TrimSpace(category))
	}
	if strings.TrimSpace(status) != "" && status != "all" {
		query = query.Where("verification_status = ?", strings.TrimSpace(status))
	}
	if !includeArchived {
		query = query.Where("archived_at IS NULL")
	}
	var entries []TNVEDDirectoryEntry
	return entries, query.Find(&entries).Error
}

func (r *Repository) TNVEDDirectoryEntryByID(ctx context.Context, id int64) (TNVEDDirectoryEntry, error) {
	var entry TNVEDDirectoryEntry
	err := r.db.WithContext(ctx).First(&entry, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return TNVEDDirectoryEntry{}, ErrNotFound
	}
	return entry, err
}

func (r *Repository) CreateTNVEDDirectoryEntry(ctx context.Context, entry TNVEDDirectoryEntry) (TNVEDDirectoryEntry, error) {
	if err := r.db.WithContext(ctx).Create(&entry).Error; err != nil {
		return TNVEDDirectoryEntry{}, err
	}
	return entry, nil
}

func (r *Repository) UpdateTNVEDDirectoryEntry(ctx context.Context, entry TNVEDDirectoryEntry) (TNVEDDirectoryEntry, error) {
	updates := map[string]any{
		"category": entry.Category, "name": entry.Name, "name_zh": entry.NameZH, "code": entry.Code, "company_coefficient": entry.CompanyCoefficient, "anti_dumping_percent": entry.AntiDumpingPercent,
		"synonyms_ru": entry.SynonymsRU, "synonyms_zh": entry.SynonymsZH, "synonyms_en": entry.SynonymsEN,
		"verification_status": entry.VerificationStatus,
		"duty_type":           entry.DutyType, "duty_percent": entry.DutyPercent, "duty_amount": entry.DutyAmount, "duty_currency": entry.DutyCurrency, "duty_unit": entry.DutyUnit, "duty_valid_from": entry.DutyValidFrom, "duty_valid_to": entry.DutyValidTo, "duty_checked_at": entry.DutyCheckedAt, "duty_source": entry.DutySource,
		"vat_type": entry.VATType, "vat_percent": entry.VATPercent, "vat_basis": entry.VATBasis, "vat_valid_from": entry.VATValidFrom, "vat_valid_to": entry.VATValidTo, "vat_checked_at": entry.VATCheckedAt, "vat_source": entry.VATSource,
		"updated_by": entry.UpdatedBy, "updated_at": entry.UpdatedAt,
	}
	result := r.db.WithContext(ctx).Model(&TNVEDDirectoryEntry{}).Where("id = ?", entry.ID).Updates(updates)
	if result.Error != nil {
		return TNVEDDirectoryEntry{}, result.Error
	}
	if result.RowsAffected == 0 {
		return TNVEDDirectoryEntry{}, ErrNotFound
	}
	return r.TNVEDDirectoryEntryByID(ctx, entry.ID)
}

func (r *Repository) ArchiveTNVEDDirectoryEntry(ctx context.Context, id, userID int64) error {
	result := r.db.WithContext(ctx).Model(&TNVEDDirectoryEntry{}).Where("id = ? AND archived_at IS NULL", id).Updates(map[string]any{"archived_at": gorm.Expr("now()"), "archived_by": userID, "updated_by": userID, "updated_at": gorm.Expr("now()")})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		var entry TNVEDDirectoryEntry
		if err := r.db.WithContext(ctx).First(&entry, id).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
	}
	return nil
}

// RestoreTNVEDDirectoryEntry makes an archived entry available for new
// selections again. The partial unique index deliberately lets PostgreSQL
// reject a restore when an active entry with the same code/name exists.
func (r *Repository) RestoreTNVEDDirectoryEntry(ctx context.Context, id, userID int64) error {
	result := r.db.WithContext(ctx).Model(&TNVEDDirectoryEntry{}).
		Where("id = ? AND archived_at IS NOT NULL", id).
		Updates(map[string]any{"archived_at": nil, "archived_by": nil, "updated_by": userID, "updated_at": time.Now().UTC()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		var entry TNVEDDirectoryEntry
		if err := r.db.WithContext(ctx).First(&entry, id).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		} else if err != nil {
			return err
		}
	}
	return nil
}
