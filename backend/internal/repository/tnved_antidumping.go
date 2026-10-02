package repository

// Действующие антидемпинговые меры ЕАЭС (реестр ЕЭК) и журнал их загрузок.

import (
	"context"
	"errors"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type TNVEDAntiDumpingMeasure struct {
	ID          int64            `gorm:"primaryKey"`
	Number      string           `gorm:"not null"`
	MeasureType string           `gorm:"column:measure_type;not null"`
	ShortName   string           `gorm:"not null"`
	Description string           `gorm:"not null"`
	Codes       JSON             `gorm:"type:jsonb;not null"`
	Countries   JSON             `gorm:"type:jsonb;not null"`
	Rates       JSON             `gorm:"type:jsonb;not null"`
	RateText    string           `gorm:"not null"`
	MinPercent  *decimal.Decimal `gorm:"type:numeric(7,3)"`
	MaxPercent  *decimal.Decimal `gorm:"type:numeric(7,3)"`
	StartDate   *time.Time       `gorm:"type:date"`
	EndDate     *time.Time       `gorm:"type:date"`
	Suspended   bool             `gorm:"not null"`
	NeedsCheck  string           `gorm:"not null"`
	Foundation  string           `gorm:"not null"`
	DocumentURL string           `gorm:"column:document_url;not null"`
	Note        string           `gorm:"not null"`
	UpdatedAt   time.Time        `gorm:"not null"`
}

func (TNVEDAntiDumpingMeasure) TableName() string { return "tnved_antidumping_measures" }

type TNVEDAntiDumpingImport struct {
	ID           int64     `gorm:"primaryKey"`
	Source       string    `gorm:"not null"`
	MeasureCount int       `gorm:"not null"`
	ImportedBy   *int64    `gorm:"column:imported_by"`
	ImportedAt   time.Time `gorm:"not null"`
	// Имя загрузившего (JOIN с users), пусто для загрузки из файла программы.
	ImportedByName string `gorm:"->;column:imported_by_name"`
}

func (TNVEDAntiDumpingImport) TableName() string { return "tnved_antidumping_imports" }

// AntiDumpingMeasures — все меры по номеру.
func (r *Repository) AntiDumpingMeasures(ctx context.Context) ([]TNVEDAntiDumpingMeasure, error) {
	var rows []TNVEDAntiDumpingMeasure
	err := r.db.WithContext(ctx).Order("id").Find(&rows).Error
	return rows, err
}

// ReplaceAntiDumpingMeasures заменяет все меры новым набором реестра и записывает загрузку.
func (r *Repository) ReplaceAntiDumpingMeasures(ctx context.Context, rows []TNVEDAntiDumpingMeasure, record TNVEDAntiDumpingImport) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DELETE FROM tnved_antidumping_measures").Error; err != nil {
			return err
		}
		if len(rows) > 0 {
			if err := tx.Create(&rows).Error; err != nil {
				return err
			}
		}
		return tx.Omit("ImportedByName").Create(&record).Error
	})
}

// LatestAntiDumpingImport — последняя загрузка реестра; ErrNotFound, если её не было.
func (r *Repository) LatestAntiDumpingImport(ctx context.Context) (TNVEDAntiDumpingImport, error) {
	var record TNVEDAntiDumpingImport
	err := r.db.WithContext(ctx).Table("tnved_antidumping_imports AS i").
		Select("i.*, COALESCE(u.name, '') AS imported_by_name").
		Joins("LEFT JOIN users u ON u.id = i.imported_by").
		Order("i.imported_at DESC, i.id DESC").Limit(1).Take(&record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return TNVEDAntiDumpingImport{}, ErrNotFound
	}
	return record, err
}
