package repository

// Раздел «Переводчик»: документы .docx, их сегменты, история правок, память переводов,
// глоссарий и кэш машинного перевода (миграция 000038_translator).

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"
)

type TranslatorDocument struct {
	ID           int64 `gorm:"primaryKey"`
	Name         string
	SourceLang   string
	TargetLang   string
	OriginalFile []byte
	Status       string
	Error        string
	Skipped      JSON `gorm:"type:jsonb"`
	CreatedBy    *int64
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (TranslatorDocument) TableName() string { return "translator_documents" }

type TranslatorSegment struct {
	ID         int64 `gorm:"primaryKey"`
	DocumentID int64
	Idx        int
	Kind       string
	SourceText string
	MTText     string `gorm:"column:mt_text"`
	TargetText string
	Tags       JSON `gorm:"type:jsonb"`
	Status     string
	Locked     bool
	QA         JSON `gorm:"column:qa;type:jsonb"`
	EditedBy   *int64
	UpdatedAt  time.Time
}

func (TranslatorSegment) TableName() string { return "translator_segments" }

// TranslatorDocumentSummary — документ без файла, с автором и прогрессом по сегментам.
type TranslatorDocumentSummary struct {
	ID             int64     `json:"id"`
	Name           string    `json:"name"`
	SourceLang     string    `json:"sourceLang"`
	TargetLang     string    `json:"targetLang"`
	Status         string    `json:"status"`
	Error          string    `json:"error"`
	Skipped        JSON      `json:"skipped"`
	CreatedBy      *int64    `json:"createdBy"`
	CreatedByName  string    `json:"createdByName"`
	SegmentCount   int64     `json:"segmentCount"`
	ConfirmedCount int64     `json:"confirmedCount"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// ponytail: без постраничной выдачи — последние 200 документов; добавить offset, когда их станет больше.
const maxTranslatorDocuments = 200

func translatorDocumentQuery(db *gorm.DB) *gorm.DB {
	return db.Table("translator_documents AS d").
		Joins("LEFT JOIN users u ON u.id = d.created_by").
		Joins("LEFT JOIN translator_segments s ON s.document_id = d.id").
		Select(`d.id, d.name, d.source_lang, d.target_lang, d.status, d.error, d.skipped, d.created_by, d.created_at, d.updated_at,
			COALESCE(NULLIF(u.name, ''), u.email, '') AS created_by_name,
			count(s.id) AS segment_count, count(s.id) FILTER (WHERE s.status = 'confirmed') AS confirmed_count`).
		Group("d.id, u.id")
}

// ListTranslatorDocuments — документы всех сотрудников, свежие сверху; query ищет по названию.
func (r *Repository) ListTranslatorDocuments(ctx context.Context, query string) ([]TranslatorDocumentSummary, error) {
	db := translatorDocumentQuery(r.db.WithContext(ctx))
	if text := strings.TrimSpace(query); text != "" {
		db = wordsWhere(db, text, "d.name")
	}
	rows := []TranslatorDocumentSummary{}
	err := db.Order("d.updated_at DESC, d.id DESC").Limit(maxTranslatorDocuments).Scan(&rows).Error
	return rows, err
}

func (r *Repository) TranslatorDocumentByID(ctx context.Context, id int64) (TranslatorDocumentSummary, error) {
	var rows []TranslatorDocumentSummary
	if err := translatorDocumentQuery(r.db.WithContext(ctx)).Where("d.id = ?", id).Scan(&rows).Error; err != nil {
		return TranslatorDocumentSummary{}, err
	}
	if len(rows) == 0 {
		return TranslatorDocumentSummary{}, ErrNotFound
	}
	return rows[0], nil
}

// CreateTranslatorDocument сохраняет документ вместе с сегментами (DocumentID и Idx проставляются здесь).
func (r *Repository) CreateTranslatorDocument(ctx context.Context, document *TranslatorDocument, segments []TranslatorSegment) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(document).Error; err != nil {
			return err
		}
		for i := range segments {
			segments[i].DocumentID = document.ID
			segments[i].Idx = i
		}
		return tx.CreateInBatches(segments, 500).Error
	})
}

func (r *Repository) TranslatorSegments(ctx context.Context, documentID int64) ([]TranslatorSegment, error) {
	segments := []TranslatorSegment{}
	err := r.db.WithContext(ctx).Where("document_id = ?", documentID).Order("idx").Find(&segments).Error
	return segments, err
}

func (r *Repository) DeleteTranslatorDocument(ctx context.Context, id int64) error {
	result := r.db.WithContext(ctx).Delete(&TranslatorDocument{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// TranslatorSegmentsSince — сегменты, изменённые после since: опрос страницы во время перевода.
func (r *Repository) TranslatorSegmentsSince(ctx context.Context, documentID int64, since time.Time) ([]TranslatorSegment, error) {
	segments := []TranslatorSegment{}
	err := r.db.WithContext(ctx).Where("document_id = ? AND updated_at > ?", documentID, since).Order("idx").Find(&segments).Error
	return segments, err
}

// SetTranslatorDocumentStatus меняет статус перевода документа (message — текст ошибки для failed).
func (r *Repository) SetTranslatorDocumentStatus(ctx context.Context, id int64, status, message string) error {
	return r.db.WithContext(ctx).Model(&TranslatorDocument{}).Where("id = ?", id).
		Updates(map[string]any{"status": status, "error": message, "updated_at": time.Now()}).Error
}

// TranslatingTranslatorDocuments — документы, перевод которых шёл при остановке сервера.
func (r *Repository) TranslatingTranslatorDocuments(ctx context.Context) ([]TranslatorDocumentSummary, error) {
	rows := []TranslatorDocumentSummary{}
	err := translatorDocumentQuery(r.db.WithContext(ctx)).Where("d.status = 'translating'").Scan(&rows).Error
	return rows, err
}

// SaveMachineTranslation записывает машинный перевод, только пока сегмент не правили: правленный
// или подтверждённый сегмент (locked) автоперевод не перезаписывает, даже если правка пришла во
// время перевода.
func (r *Repository) SaveMachineTranslation(ctx context.Context, segment TranslatorSegment) error {
	qa := segment.QA
	if len(qa) == 0 {
		qa = JSON("[]")
	}
	return r.db.WithContext(ctx).Model(&TranslatorSegment{}).
		Where("id = ? AND locked = false AND status = 'machine'", segment.ID).
		Updates(map[string]any{"mt_text": segment.MTText, "qa": qa, "updated_at": time.Now()}).Error
}

// TranslatorCache — машинный перевод того же текста той же моделью (ErrNotFound — нет).
func (r *Repository) TranslatorCache(ctx context.Context, hash string) (string, error) {
	var rows []string
	if err := r.db.WithContext(ctx).Table("translator_mt_cache").Where("hash = ?", hash).Limit(1).Pluck("translation", &rows).Error; err != nil {
		return "", err
	}
	if len(rows) == 0 {
		return "", ErrNotFound
	}
	return rows[0], nil
}

func (r *Repository) SaveTranslatorCache(ctx context.Context, hash, translation string) error {
	return r.db.WithContext(ctx).Exec(
		"INSERT INTO translator_mt_cache (hash, translation) VALUES (?, ?) ON CONFLICT (hash) DO NOTHING", hash, translation).Error
}

func (r *Repository) TranslatorSegmentByID(ctx context.Context, id int64) (TranslatorSegment, error) {
	var segments []TranslatorSegment
	if err := r.db.WithContext(ctx).Where("id = ?", id).Limit(1).Find(&segments).Error; err != nil {
		return TranslatorSegment{}, err
	}
	if len(segments) == 0 {
		return TranslatorSegment{}, ErrNotFound
	}
	return segments[0], nil
}

// PreviousTranslatorSource — текст оригинала предыдущего сегмента (контекст для перевода), "" — первый.
func (r *Repository) PreviousTranslatorSource(ctx context.Context, segment TranslatorSegment) (string, error) {
	var rows []string
	err := r.db.WithContext(ctx).Model(&TranslatorSegment{}).
		Where("document_id = ? AND idx = ?", segment.DocumentID, segment.Idx-1).Pluck("source_text", &rows).Error
	if err != nil || len(rows) == 0 {
		return "", err
	}
	return rows[0], nil
}

// TranslatorSegmentEdit — правка или подтверждение перевода сотрудником.
type TranslatorSegmentEdit struct {
	Segment     TranslatorSegment // с новыми TargetText, Status и QA
	UserID      int64
	TextChanged bool // текст изменился — версия пишется в историю
}

// SaveTranslatorSegmentEdit одной транзакцией: сегмент блокируется от автоперевода, изменённый
// текст пишется в историю.
func (r *Repository) SaveTranslatorSegmentEdit(ctx context.Context, edit TranslatorSegmentEdit) error {
	segment := edit.Segment
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now()
		err := tx.Model(&TranslatorSegment{}).Where("id = ?", segment.ID).Updates(map[string]any{
			"target_text": segment.TargetText, "status": segment.Status, "locked": true, "qa": segment.QA,
			"edited_by": edit.UserID, "updated_at": now,
		}).Error
		if err != nil {
			return err
		}
		if edit.TextChanged {
			err := tx.Exec("INSERT INTO translator_segment_history (segment_id, target_text, changed_by, changed_at) VALUES (?, ?, ?, ?)",
				segment.ID, segment.TargetText, edit.UserID, now).Error
			if err != nil {
				return err
			}
		}
		return tx.Model(&TranslatorDocument{}).Where("id = ?", segment.DocumentID).Update("updated_at", now).Error
	})
}

// ResetTranslatorSegment — «Перевести заново»: новый машинный перевод, правка снимается, сегмент
// снова открыт для автоперевода. Прежние версии остаются в истории.
func (r *Repository) ResetTranslatorSegment(ctx context.Context, segment TranslatorSegment) error {
	qa := segment.QA
	if len(qa) == 0 {
		qa = JSON("[]")
	}
	return r.db.WithContext(ctx).Model(&TranslatorSegment{}).Where("id = ?", segment.ID).Updates(map[string]any{
		"mt_text": segment.MTText, "target_text": "", "status": "machine", "locked": false, "qa": qa, "updated_at": time.Now(),
	}).Error
}

// ReplaceTranslatorCache — новый перевод того же текста заменяет прежний («Перевести заново»).
func (r *Repository) ReplaceTranslatorCache(ctx context.Context, hash, translation string) error {
	return r.db.WithContext(ctx).Exec(
		"INSERT INTO translator_mt_cache (hash, translation) VALUES (?, ?) ON CONFLICT (hash) DO UPDATE SET translation = EXCLUDED.translation, created_at = now()",
		hash, translation).Error
}

type TranslatorSegmentVersion struct {
	ID            int64     `json:"id"`
	TargetText    string    `json:"targetText"`
	ChangedByName string    `json:"changedByName"`
	ChangedAt     time.Time `json:"changedAt"`
}

// TranslatorSegmentHistory — версии перевода сегмента, новые сверху.
func (r *Repository) TranslatorSegmentHistory(ctx context.Context, segmentID int64) ([]TranslatorSegmentVersion, error) {
	rows := []TranslatorSegmentVersion{}
	err := r.db.WithContext(ctx).Table("translator_segment_history AS h").
		Joins("LEFT JOIN users u ON u.id = h.changed_by").
		Select("h.id, h.target_text, h.changed_at, COALESCE(NULLIF(u.name, ''), u.email, '') AS changed_by_name").
		Where("h.segment_id = ?", segmentID).Order("h.changed_at DESC, h.id DESC").Limit(100).Scan(&rows).Error
	return rows, err
}

// TranslatorDocumentFile — исходный .docx документа.
func (r *Repository) TranslatorDocumentFile(ctx context.Context, id int64) ([]byte, error) {
	var files [][]byte
	if err := r.db.WithContext(ctx).Model(&TranslatorDocument{}).Where("id = ?", id).Pluck("original_file", &files).Error; err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, ErrNotFound
	}
	return files[0], nil
}
