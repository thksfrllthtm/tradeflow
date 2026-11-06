package repository

// Full TN VED EAEU nomenclature with EAEU tariff import duty rates: estimates and product cards
// take codes and rates from it. Each import replaces it.

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type TNVEDNomenclatureCode struct {
	Code         string           `gorm:"primaryKey;size:10"`
	Name         string           `gorm:"not null"`
	TariffText   string           `gorm:"size:200;not null"`
	DutyType     string           `gorm:"size:24;not null"`
	DutyPercent  *decimal.Decimal `gorm:"type:numeric(7,3)"`
	DutyAmount   *decimal.Decimal `gorm:"type:numeric(14,4)"`
	DutyCurrency string           `gorm:"size:3;not null"`
	DutyUnit     string           `gorm:"size:40;not null"`
	VATPercent   decimal.Decimal  `gorm:"column:vat_percent;type:numeric(5,2);not null"`
	DetailsURL   string           `gorm:"size:500;not null"`
}

func (TNVEDNomenclatureCode) TableName() string { return "tnved_nomenclature" }

type TNVEDNomenclatureImport struct {
	ID           int64 `gorm:"primaryKey"`
	Source       string
	FileName     string
	ActualDate   *time.Time `gorm:"type:date"`
	CodeCount    int
	AddedCount   int
	RemovedCount int
	ChangedCount int
	ImportedBy   *int64
	ImportedAt   time.Time
	// Filled by LatestTNVEDNomenclatureImport: name of the user who imported.
	ImportedByName string `gorm:"->;-:migration"`
}

func (TNVEDNomenclatureImport) TableName() string { return "tnved_nomenclature_imports" }

type TNVEDNomenclatureFilter struct {
	Query  string
	Limit  int
	Offset int
}

// ListTNVEDNomenclature searches by the beginning of the code (digits, spaces allowed:
// «4015 19») or by words of the name — the stem of every word must occur.
func (r *Repository) ListTNVEDNomenclature(ctx context.Context, filter TNVEDNomenclatureFilter) ([]TNVEDNomenclatureCode, int64, error) {
	query := r.db.WithContext(ctx).Table("tnved_nomenclature AS n")
	text := strings.TrimSpace(filter.Query)
	if digitsQuery.MatchString(text) {
		query = query.Where("n.code LIKE ?", strings.NewReplacer(" ", "", ".", "").Replace(text)+"%")
	} else {
		for _, word := range strings.Fields(text) {
			query = query.Where("n.name ILIKE ?", "%"+escapeLike(SearchStem(word))+"%")
		}
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var codes []TNVEDNomenclatureCode
	err := query.Select("n.*").
		Order("n.code ASC").Limit(filter.Limit).Offset(filter.Offset).Scan(&codes).Error
	return codes, total, err
}

// TNVEDNomenclatureByCodes finds the given codes (a pasted column) with the same fields as
// the list; codes the nomenclature does not have are simply absent.
func (r *Repository) TNVEDNomenclatureByCodes(ctx context.Context, codes []string) ([]TNVEDNomenclatureCode, error) {
	var found []TNVEDNomenclatureCode
	if len(codes) == 0 {
		return found, nil
	}
	err := r.db.WithContext(ctx).Table("tnved_nomenclature AS n").Where("n.code IN ?", codes).
		Select("n.*").
		Order("n.code ASC").Scan(&found).Error
	return found, err
}

// AllTNVEDNomenclature reads the whole list: an import compares it with the new file.
func (r *Repository) AllTNVEDNomenclature(ctx context.Context) ([]TNVEDNomenclatureCode, error) {
	var codes []TNVEDNomenclatureCode
	return codes, r.db.WithContext(ctx).Order("code ASC").Find(&codes).Error
}

// ReplaceTNVEDNomenclature swaps the whole list in one transaction and records the import.
func (r *Repository) ReplaceTNVEDNomenclature(ctx context.Context, codes []TNVEDNomenclatureCode, record TNVEDNomenclatureImport) (TNVEDNomenclatureImport, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DELETE FROM tnved_nomenclature").Error; err != nil {
			return err
		}
		if len(codes) > 0 {
			if err := tx.CreateInBatches(codes, 1000).Error; err != nil {
				return err
			}
		}
		record.ImportedAt = time.Now().UTC()
		return tx.Omit("ImportedByName").Create(&record).Error
	})
	return record, err
}

// LatestTNVEDNomenclatureImport is the import the current list came from; ErrNotFound
// while nothing has been imported.
func (r *Repository) LatestTNVEDNomenclatureImport(ctx context.Context) (TNVEDNomenclatureImport, error) {
	var record TNVEDNomenclatureImport
	err := r.db.WithContext(ctx).Table("tnved_nomenclature_imports AS i").
		Select("i.*, COALESCE(u.name, '') AS imported_by_name").
		Joins("LEFT JOIN users u ON u.id = i.imported_by").
		Order("i.imported_at DESC, i.id DESC").Limit(1).Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return TNVEDNomenclatureImport{}, ErrNotFound
	}
	return record, err
}

// TNVEDNomenclatureCodeSet is every code of the nomenclature: an import reports list codes
// the nomenclature does not have.
func (r *Repository) TNVEDNomenclatureCodeSet(ctx context.Context) (map[string]string, error) {
	var rows []struct {
		Code       string
		TariffText string
	}
	if err := r.db.WithContext(ctx).Table("tnved_nomenclature").Select("code, tariff_text").Scan(&rows).Error; err != nil {
		return nil, err
	}
	codes := make(map[string]string, len(rows))
	for _, row := range rows {
		codes[row.Code] = row.TariffText
	}
	return codes, nil
}
