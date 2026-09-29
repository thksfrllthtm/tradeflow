package repository

// Full TN VED EAEU nomenclature with EAEU tariff import duty rates: a reference list the
// working directory (tnved_directory_entries) takes codes from. Each import replaces it.

import (
	"context"
	"errors"
	"regexp"
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
	// Filled by the list query: the code has an active entry in the working directory.
	InDirectory bool `gorm:"->;-:migration"`
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

var digitsQuery = regexp.MustCompile(`^[\d\s.]+$`)

// SearchStem cuts the ending off a word so other forms match: «резиновые» → «резин»
// finds «резиной», «фарфоровая» → «фарфор» finds «фарфора». Short words stay whole.
func SearchStem(word string) string {
	runes := []rune(word)
	n := len(runes)
	switch {
	case n <= 4:
		return word
	case n <= 7:
		return string(runes[:max(4, n-2)])
	default:
		return string(runes[:n-4])
	}
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
	err := query.Select("n.*, EXISTS (SELECT 1 FROM tnved_directory_entries d WHERE d.code = n.code AND d.archived_at IS NULL) AS in_directory").
		Order("n.code ASC").Limit(filter.Limit).Offset(filter.Offset).Scan(&codes).Error
	return codes, total, err
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
			if err := tx.Omit("InDirectory").CreateInBatches(codes, 1000).Error; err != nil {
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
