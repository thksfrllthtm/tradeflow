package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

func (r *Repository) ListGoodsEstimates(ctx context.Context, userID int64) ([]GoodsEstimate, error) {
	var estimates []GoodsEstimate
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("updated_at DESC").Find(&estimates).Error
	return estimates, err
}

func (r *Repository) GoodsEstimateByIDForUser(ctx context.Context, id, userID int64) (GoodsEstimate, error) {
	var estimate GoodsEstimate
	err := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&estimate).Error
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
	result := r.db.WithContext(ctx).Model(&GoodsEstimate{}).
		Where("id = ? AND user_id = ?", estimate.ID, estimate.UserID).
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

func (r *Repository) SaveGoodsEstimateDraft(ctx context.Context, id, userID int64, inputSnapshot, resultSnapshot JSON, assessmentDate time.Time, updatedAt time.Time) (GoodsEstimate, error) {
	result := r.db.WithContext(ctx).Model(&GoodsEstimate{}).
		Where("id = ? AND user_id = ?", id, userID).
		Updates(map[string]any{
			"assessment_date": assessmentDate,
			"input_snapshot":  inputSnapshot,
			"result_snapshot": resultSnapshot,
			"updated_at":      updatedAt,
		})
	if result.Error != nil {
		return GoodsEstimate{}, result.Error
	}
	if result.RowsAffected == 0 {
		return GoodsEstimate{}, ErrNotFound
	}
	return r.GoodsEstimateByIDForUser(ctx, id, userID)
}

func (r *Repository) DeleteGoodsEstimate(ctx context.Context, id, userID int64) error {
	result := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).Delete(&GoodsEstimate{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
