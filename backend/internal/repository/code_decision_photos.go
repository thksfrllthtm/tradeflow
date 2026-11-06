package repository

// Фото записей базы решений: миниатюры, описание фото от ИИ и его вектор для поиска похожих по смыслу.

import (
	"context"
	"strconv"
	"strings"
	"time"
)

// DecisionPhoto — первое фото записи базы решений.
type DecisionPhoto struct {
	ID     int64
	Photos JSON
}

// DecisionsNeedingPhotoFeatures — записи с фото, для которых ИИ ещё не составил описание.
func (r *Repository) DecisionsNeedingPhotoFeatures(ctx context.Context, limit int) ([]DecisionPhoto, error) {
	var rows []DecisionPhoto
	err := r.db.WithContext(ctx).Table("code_decisions").Select("id, photos").
		Where("photo_features_at IS NULL AND photos <> '[]'::jsonb AND status <> 'rejected'").
		Order("id DESC").Limit(limit).Scan(&rows).Error
	return rows, err
}

// DecisionPhotos — фото записей (для визуальной сверки похожих товаров).
func (r *Repository) DecisionPhotos(ctx context.Context, ids []int64) ([]DecisionPhoto, error) {
	var rows []DecisionPhoto
	if len(ids) == 0 {
		return rows, nil
	}
	err := r.db.WithContext(ctx).Table("code_decisions").Select("id, photos").Where("id IN ?", ids).Scan(&rows).Error
	return rows, err
}

// DecisionsMissingThumb — записи с фото без миниатюры, по возрастанию id после after.
func (r *Repository) DecisionsMissingThumb(ctx context.Context, after int64, limit int) ([]DecisionPhoto, error) {
	var rows []DecisionPhoto
	err := r.db.WithContext(ctx).Table("code_decisions").Select("id, photos").
		Where("id > ? AND thumb = '' AND photos <> '[]'::jsonb", after).
		Order("id").Limit(limit).Scan(&rows).Error
	return rows, err
}

// DecisionThumbs — миниатюры записей по id (без миниатюры — нет в карте).
func (r *Repository) DecisionThumbs(ctx context.Context, ids []int64) (map[int64]string, error) {
	var rows []struct {
		ID    int64
		Thumb string
	}
	err := r.db.WithContext(ctx).Table("code_decisions").Select("id, thumb").Where("id IN ? AND thumb <> ''", ids).Scan(&rows).Error
	thumbs := make(map[int64]string, len(rows))
	for _, row := range rows {
		thumbs[row.ID] = row.Thumb
	}
	return thumbs, err
}

// SaveDecisionThumb сохраняет миниатюру записи.
func (r *Repository) SaveDecisionThumb(ctx context.Context, id int64, thumb string) error {
	return r.db.WithContext(ctx).Model(&CodeDecision{}).Where("id = ?", id).Update("thumb", thumb).Error
}

// SaveDecisionPhotoFeatures сохраняет описание фото (null — описать не удалось, больше не пробуем).
// Вектор описания сбрасывается: фоновый обработчик посчитает его заново.
func (r *Repository) SaveDecisionPhotoFeatures(ctx context.Context, id int64, features JSON) error {
	return r.db.WithContext(ctx).Model(&CodeDecision{}).Where("id = ?", id).
		Updates(map[string]any{"photo_features": features, "photo_features_at": time.Now().UTC(), "photo_embedding": nil}).Error
}

// vectorText — вектор в записи pgvector: «[0.1,0.2,…]».
func vectorText(vector []float32) string {
	parts := make([]string, len(vector))
	for index, value := range vector {
		parts[index] = strconv.FormatFloat(float64(value), 'g', -1, 32)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

type DecisionFeatures struct {
	ID            int64
	PhotoFeatures JSON
}

// DecisionsNeedingEmbedding — записи с описанием фото, но без его вектора.
func (r *Repository) DecisionsNeedingEmbedding(ctx context.Context, limit int) ([]DecisionFeatures, error) {
	var rows []DecisionFeatures
	err := r.db.WithContext(ctx).Table("code_decisions").Select("id, photo_features").
		Where("photo_embedding IS NULL AND jsonb_typeof(photo_features) = 'object' AND status <> 'rejected'").
		Order("id DESC").Limit(limit).Scan(&rows).Error
	return rows, err
}

// SaveDecisionEmbedding сохраняет вектор описания фото.
func (r *Repository) SaveDecisionEmbedding(ctx context.Context, id int64, vector []float32) error {
	return r.db.WithContext(ctx).Exec("UPDATE code_decisions SET photo_embedding = ?::vector WHERE id = ?", vectorText(vector), id).Error
}

type DecisionNeighbor struct {
	ID int64
	// Similarity — косинусная близость векторов (1 — одинаковые).
	Similarity float64
}

// NearestDecisions — записи, ближайшие по вектору описания (не отклонённые), лучшие первыми.
// Без индекса: записей сотни, перебор быстрее. ponytail: HNSW-индекс, когда записей станут десятки тысяч.
func (r *Repository) NearestDecisions(ctx context.Context, vector []float32, limit int) ([]DecisionNeighbor, error) {
	var rows []DecisionNeighbor
	text := vectorText(vector)
	err := r.db.WithContext(ctx).Raw(`SELECT id, 1 - (photo_embedding <=> ?::vector) AS similarity FROM code_decisions
		WHERE photo_embedding IS NOT NULL AND status <> 'rejected' ORDER BY photo_embedding <=> ?::vector LIMIT ?`, text, text, limit).Scan(&rows).Error
	return rows, err
}
