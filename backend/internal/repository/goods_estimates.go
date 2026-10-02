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

func (r *Repository) CreateGoodsEstimate(ctx context.Context, estimate GoodsEstimate) (GoodsEstimate, error) {
	if err := r.db.WithContext(ctx).Create(&estimate).Error; err != nil {
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
