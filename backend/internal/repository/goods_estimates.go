package repository

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

func (r *Repository) GoodsEstimateByIDForUser(ctx context.Context, id, userID int64) (GoodsEstimate, error) {
	var estimate GoodsEstimate
	err := ownedBy(r.db.WithContext(ctx).Where("id = ?", id), userID).First(&estimate).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return GoodsEstimate{}, ErrNotFound
	}
	return estimate, err
}

// blankGoodsEstimate — смета, в которую ничего не положили: стандартное название, без клиента,
// строк, файла, результата, фото и вопросов агенту. Такие копятся от каждого «Создать смету».
const blankGoodsEstimate = `user_id = ? AND client = '' AND name ~ '^Смета товаров( · \d{4}-\d{2}-\d{2})?$'
	AND CASE WHEN jsonb_typeof(input_snapshot->'items') = 'array' THEN jsonb_array_length(input_snapshot->'items') ELSE 0 END = 0
	AND COALESCE(input_snapshot->'source', 'null'::jsonb) = 'null'::jsonb
	AND result_snapshot IN ('{}'::jsonb, 'null'::jsonb)
	AND NOT EXISTS (SELECT 1 FROM goods_estimate_photos p WHERE p.estimate_id = goods_estimates.id)
	AND NOT EXISTS (SELECT 1 FROM tnved_agent_sessions s WHERE s.estimate_id = goods_estimates.id)`

// CreateGoodsEstimate создаёт смету. Если у сотрудника уже есть пустые сметы, новая не заводится:
// самая свежая пустая становится новой (название, клиент, дата, создана — из estimate), остальные
// пустые удаляются — в них нечего терять.
func (r *Repository) CreateGoodsEstimate(ctx context.Context, estimate GoodsEstimate) (GoodsEstimate, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var blank []int64
		if err := tx.Model(&GoodsEstimate{}).Where(blankGoodsEstimate, estimate.UserID).Order("id DESC").Pluck("id", &blank).Error; err != nil {
			return err
		}
		if len(blank) == 0 {
			return tx.Create(&estimate).Error
		}
		if len(blank) > 1 {
			if err := tx.Where("id IN ?", blank[1:]).Delete(&GoodsEstimate{}).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&GoodsEstimate{}).Where("id = ?", blank[0]).Updates(map[string]any{
			"name": estimate.Name, "client": estimate.Client, "assessment_date": estimate.AssessmentDate,
			"input_snapshot": estimate.InputSnapshot, "result_snapshot": estimate.ResultSnapshot,
			"created_at": estimate.CreatedAt, "updated_at": estimate.UpdatedAt,
		}).Error; err != nil {
			return err
		}
		return tx.First(&estimate, blank[0]).Error
	})
	if err != nil {
		return GoodsEstimate{}, err
	}
	return estimate, nil
}

func (r *Repository) UpdateGoodsEstimate(ctx context.Context, estimate GoodsEstimate) (GoodsEstimate, error) {
	result := ownedBy(r.db.WithContext(ctx).Model(&GoodsEstimate{}), estimate.UserID).
		Where("id = ?", estimate.ID).
		Updates(map[string]any{
			"name":            estimate.Name,
			"client":          estimate.Client,
			"assessment_date": estimate.AssessmentDate,
			"updated_at":      estimate.UpdatedAt,
		})
	if result.Error != nil {
		return GoodsEstimate{}, result.Error
	}
	if result.RowsAffected == 0 {
		return GoodsEstimate{}, ErrNotFound
	}
	return r.GoodsEstimateByIDForUser(ctx, estimate.ID, estimate.UserID)
}

func (r *Repository) DeleteGoodsEstimate(ctx context.Context, id, userID int64) error {
	result := ownedBy(r.db.WithContext(ctx).Where("id = ?", id), userID).Delete(&GoodsEstimate{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// AllUsers, passed instead of a user ID, lifts the owner filter on estimates:
// an administrator sees every employee's estimates. Real user IDs are
// positive, so a forgotten zero never widens access.
const AllUsers int64 = -1

func ownedBy(query *gorm.DB, userID int64) *gorm.DB {
	if userID == AllUsers {
		return query
	}
	return query.Where("user_id = ?", userID)
}

// UnitPriceHistory returns, for each requested TN ВЭД code, the unit price in
// USD from the most recently saved goods estimate result that contains it
// (excluded rows and the current estimate are skipped). The JSON is read
// inside PostgreSQL, so estimate snapshots with photos never reach the server.
func (r *Repository) UnitPriceHistory(ctx context.Context, userID, excludeEstimateID int64, codes []string) (map[string]float64, error) {
	query := `SELECT DISTINCT ON (item->>'code') item->>'code' AS code, (item->>'pairPriceUsd')::numeric AS price
		FROM goods_estimates e
		CROSS JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(e.result_snapshot->'items') = 'array' THEN e.result_snapshot->'items' ELSE '[]'::jsonb END) AS item
		WHERE item->>'code' IN ? AND e.id <> ?
		  AND COALESCE(item->>'excluded', 'false') <> 'true'
		  AND item->>'pairPriceUsd' ~ '^[0-9]+(\.[0-9]+)?$' AND (item->>'pairPriceUsd')::numeric > 0`
	args := []any{codes, excludeEstimateID}
	if userID != AllUsers {
		query += ` AND e.user_id = ?`
		args = append(args, userID)
	}
	query += ` ORDER BY item->>'code', e.updated_at DESC`
	var rows []struct {
		Code  string
		Price float64
	}
	if err := r.db.WithContext(ctx).Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	prices := make(map[string]float64, len(rows))
	for _, row := range rows {
		prices[row.Code] = row.Price
	}
	return prices, nil
}
