package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Список и сохранение смет товаров частями: содержимое сметы (строки, результат) и фото
// строк читаются и пишутся только там, где они нужны.

const goodsSummaryColumns = "id, user_id, name, client, assessment_date, created_at, updated_at, " +
	"result_snapshot NOT IN ('{}'::jsonb, 'null'::jsonb) AS has_result"

// ListGoodsEstimates returns the estimates list without rows, results and photos.
func (r *Repository) ListGoodsEstimates(ctx context.Context, userID int64) ([]GoodsEstimateSummary, error) {
	var rows []GoodsEstimateSummary
	err := ownedBy(r.db.WithContext(ctx).Model(&GoodsEstimate{}), userID).Select(goodsSummaryColumns).Order("updated_at DESC").Scan(&rows).Error
	return rows, err
}

func goodsEstimateSummary(db *gorm.DB, id int64) (GoodsEstimateSummary, error) {
	var rows []GoodsEstimateSummary
	if err := db.Model(&GoodsEstimate{}).Select(goodsSummaryColumns).Where("id = ?", id).Scan(&rows).Error; err != nil {
		return GoodsEstimateSummary{}, err
	}
	if len(rows) == 0 {
		return GoodsEstimateSummary{}, ErrNotFound
	}
	return rows[0], nil
}

// GoodsEstimatePhotos returns the photos of an estimate by row id.
func (r *Repository) GoodsEstimatePhotos(ctx context.Context, estimateID int64) (map[string][]string, error) {
	var rows []GoodsEstimatePhoto
	if err := r.db.WithContext(ctx).Where("estimate_id = ?", estimateID).Find(&rows).Error; err != nil {
		return nil, err
	}
	photos := make(map[string][]string, len(rows))
	for _, row := range rows {
		var list []string
		if err := json.Unmarshal(row.Photos, &list); err != nil {
			return nil, err
		}
		photos[row.ItemID] = list
	}
	return photos, nil
}

// GoodsDraftPatch is the changed part of an estimate draft; nil parts stay as they are.
type GoodsDraftPatch struct {
	// Changed top-level input fields (a JSON object merged into input_snapshot).
	Input JSON
	// New calculation result; "{}" removes it.
	Result         JSON
	AssessmentDate *time.Time
	// Row ids after the change, when the rows changed: photos of removed rows are deleted.
	ItemIDs []string
	// Photos of changed rows; an empty list deletes the row's photos.
	Photos    map[string][]string
	UpdatedAt time.Time
}

// PatchGoodsEstimateDraft overwrites only the changed parts of the draft in one transaction.
// Photos are kept only for rows that exist in the saved draft.
func (r *Repository) PatchGoodsEstimateDraft(ctx context.Context, id, userID int64, patch GoodsDraftPatch) (GoodsEstimateSummary, error) {
	var summary GoodsEstimateSummary
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked GoodsEstimate
		err := ownedBy(tx.Model(&GoodsEstimate{}), userID).Select("id").Where("id = ?", id).Clauses(clause.Locking{Strength: "UPDATE"}).Take(&locked).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		if err != nil {
			return err
		}
		updates := map[string]any{"updated_at": patch.UpdatedAt}
		if patch.Input != nil {
			updates["input_snapshot"] = gorm.Expr("input_snapshot || ?::jsonb", string(patch.Input))
		}
		if patch.Result != nil {
			updates["result_snapshot"] = patch.Result
		}
		if patch.AssessmentDate != nil {
			updates["assessment_date"] = *patch.AssessmentDate
		}
		if err := tx.Model(&GoodsEstimate{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return err
		}
		if patch.ItemIDs != nil {
			stale := tx.Where("estimate_id = ?", id)
			if len(patch.ItemIDs) > 0 {
				stale = stale.Where("item_id NOT IN ?", patch.ItemIDs)
			}
			if err := stale.Delete(&GoodsEstimatePhoto{}).Error; err != nil {
				return err
			}
		}
		if len(patch.Photos) > 0 {
			var ids []string
			if err := tx.Raw(`SELECT item->>'id' FROM goods_estimates e
				CROSS JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(e.input_snapshot->'items') = 'array' THEN e.input_snapshot->'items' ELSE '[]'::jsonb END) AS item
				WHERE e.id = ?`, id).Scan(&ids).Error; err != nil {
				return err
			}
			rows := make(map[string]bool, len(ids))
			for _, itemID := range ids {
				rows[itemID] = true
			}
			for itemID, photos := range patch.Photos {
				if !rows[itemID] {
					continue
				}
				if len(photos) == 0 {
					if err := tx.Where("estimate_id = ? AND item_id = ?", id, itemID).Delete(&GoodsEstimatePhoto{}).Error; err != nil {
						return err
					}
					continue
				}
				data, err := json.Marshal(photos)
				if err != nil {
					return err
				}
				photo := GoodsEstimatePhoto{EstimateID: id, ItemID: itemID, Photos: JSON(data)}
				if err := tx.Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "estimate_id"}, {Name: "item_id"}}, DoUpdates: clause.AssignmentColumns([]string{"photos"})}).Create(&photo).Error; err != nil {
					return err
				}
			}
		}
		summary, err = goodsEstimateSummary(tx, id)
		return err
	})
	return summary, err
}
