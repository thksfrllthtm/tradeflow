package repository

// «Документы для ввоза»: сохранённые ответы по кодам и перечни СС/ДС общей базы из справок tnved.info.

import (
	"context"
	"time"

	"gorm.io/gorm/clause"
)

// CodeDocuments — сохранённый ответ «Документы для ввоза» по коду: описание товара, резюме и меры
// (записи AgentDocument с видом документа). Показывается последний по коду.
type CodeDocuments struct {
	ID          int64     `gorm:"primaryKey"`
	Code        string    `gorm:"size:10;not null"`
	Product     string    `gorm:"not null"`
	Description string    `gorm:"not null"`
	Documents   JSON      `gorm:"type:jsonb;not null"`
	SessionID   *int64    `gorm:"column:session_id"`
	UserID      *int64    `gorm:"column:user_id"`
	CreatedAt   time.Time `gorm:"not null"`
}

func (CodeDocuments) TableName() string { return "tnved_code_documents" }

func (r *Repository) SaveCodeDocuments(ctx context.Context, row CodeDocuments) error {
	return r.db.WithContext(ctx).Create(&row).Error
}

// LatestCodeDocuments — последний сохранённый ответ по коду; ErrNotFound — ответа ещё не было.
func (r *Repository) LatestCodeDocuments(ctx context.Context, code string) (CodeDocuments, error) {
	var rows []CodeDocuments
	if err := r.db.WithContext(ctx).Where("code = ?", code).Order("created_at DESC, id DESC").Limit(1).Find(&rows).Error; err != nil {
		return CodeDocuments{}, err
	}
	if len(rows) == 0 {
		return CodeDocuments{}, ErrNotFound
	}
	return rows[0], nil
}

// GeneralAct — перечень СС/ДС общей базы «Документов для ввоза», добавленный из справки tnved.info.
type GeneralAct struct {
	URL     string    `gorm:"primaryKey;size:1000"`
	Title   string    `gorm:"not null"`
	AddedAt time.Time `gorm:"not null"`
}

func (GeneralAct) TableName() string { return "general_acts" }

// SaveGeneralActs — новые перечни; уже известные адреса не меняются.
func (r *Repository) SaveGeneralActs(ctx context.Context, acts []GeneralAct) error {
	if len(acts) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&acts).Error
}

// GeneralActs — все перечни, добавленные из справок, в порядке добавления.
func (r *Repository) GeneralActs(ctx context.Context) ([]GeneralAct, error) {
	var acts []GeneralAct
	return acts, r.db.WithContext(ctx).Order("added_at, url").Find(&acts).Error
}
