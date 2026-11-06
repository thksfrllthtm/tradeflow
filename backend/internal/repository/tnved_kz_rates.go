package repository

// «Перечень РК»: codes Kazakhstan imports at duty rates below the EAEU tariff (WTO
// commitments). Each import replaces the list.

import (
	"context"
	"errors"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type TNVEDKZRate struct {
	Code         string `gorm:"primaryKey;size:10"`
	Name         string
	RateText     string           `gorm:"size:300;not null"`
	DutyType     string           `gorm:"size:24;not null"`
	DutyPercent  *decimal.Decimal `gorm:"type:numeric(7,3)"`
	DutyAmount   *decimal.Decimal `gorm:"type:numeric(14,4)"`
	DutyCurrency string           `gorm:"size:3;not null"`
	DutyUnit     string           `gorm:"size:60;not null"`
	ValidFrom    *time.Time       `gorm:"type:date"`
	ValidTo      *time.Time       `gorm:"type:date"`
	Notes        string           `gorm:"size:60;not null"`
	NotesText    string           `gorm:"not null"`
	Page         *int
}

func (TNVEDKZRate) TableName() string { return "tnved_kz_rates" }

type TNVEDKZRateImport struct {
	ID         int64 `gorm:"primaryKey"`
	Source     string
	FileName   string
	RowCount   int
	ImportedBy *int64
	ImportedAt time.Time
	// Filled by LatestTNVEDKZRateImport: name of the user who imported.
	ImportedByName string `gorm:"->;-:migration"`
}

func (TNVEDKZRateImport) TableName() string { return "tnved_kz_rate_imports" }

// TNVEDKZRatesByCodes reads the list entries for the given codes (a nomenclature page).
func (r *Repository) TNVEDKZRatesByCodes(ctx context.Context, codes []string) ([]TNVEDKZRate, error) {
	var rates []TNVEDKZRate
	if len(codes) == 0 {
		return rates, nil
	}
	return rates, r.db.WithContext(ctx).Where("code IN ?", codes).Find(&rates).Error
}

// ReplaceTNVEDKZRates swaps the whole list in one transaction and records the import.
func (r *Repository) ReplaceTNVEDKZRates(ctx context.Context, rates []TNVEDKZRate, record TNVEDKZRateImport) (TNVEDKZRateImport, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DELETE FROM tnved_kz_rates").Error; err != nil {
			return err
		}
		if len(rates) > 0 {
			if err := tx.CreateInBatches(rates, 1000).Error; err != nil {
				return err
			}
		}
		record.ImportedAt = time.Now().UTC()
		return tx.Omit("ImportedByName").Create(&record).Error
	})
	return record, err
}

// LatestTNVEDKZRateImport is the import the current list came from; ErrNotFound while
// nothing has been imported.
func (r *Repository) LatestTNVEDKZRateImport(ctx context.Context) (TNVEDKZRateImport, error) {
	var record TNVEDKZRateImport
	err := r.db.WithContext(ctx).Table("tnved_kz_rate_imports AS i").
		Select("i.*, COALESCE(u.name, '') AS imported_by_name").
		Joins("LEFT JOIN users u ON u.id = i.imported_by").
		Order("i.imported_at DESC, i.id DESC").Limit(1).Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return TNVEDKZRateImport{}, ErrNotFound
	}
	return record, err
}
