package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

func (r *Repository) ListGlovesEstimates(ctx context.Context, userID int64) ([]GlovesEstimate, error) {
	var estimates []GlovesEstimate
	err := ownedBy(r.db.WithContext(ctx), userID).Order("updated_at DESC").Find(&estimates).Error
	return estimates, err
}

func (r *Repository) GlovesEstimateByIDForUser(ctx context.Context, id, userID int64) (GlovesEstimate, error) {
	var estimate GlovesEstimate
	err := ownedBy(r.db.WithContext(ctx).Where("id = ?", id), userID).First(&estimate).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return GlovesEstimate{}, ErrNotFound
	}
	return estimate, err
}

func (r *Repository) CreateGlovesEstimate(ctx context.Context, estimate GlovesEstimate) (GlovesEstimate, error) {
	if err := r.db.WithContext(ctx).Create(&estimate).Error; err != nil {
		return GlovesEstimate{}, err
	}
	return estimate, nil
}

func (r *Repository) UpdateGlovesEstimate(ctx context.Context, estimate GlovesEstimate) (GlovesEstimate, error) {
	result := ownedBy(r.db.WithContext(ctx).Model(&GlovesEstimate{}), estimate.UserID).
		Where("id = ?", estimate.ID).
		Updates(map[string]any{
			"name":            estimate.Name,
			"client":          estimate.Client,
			"assessment_date": estimate.AssessmentDate,
			"updated_at":      estimate.UpdatedAt,
		})
	if result.Error != nil {
		return GlovesEstimate{}, result.Error
	}
	if result.RowsAffected == 0 {
		return GlovesEstimate{}, ErrNotFound
	}
	return r.GlovesEstimateByIDForUser(ctx, estimate.ID, estimate.UserID)
}

func (r *Repository) SaveGlovesEstimateDraft(ctx context.Context, id, userID int64, inputSnapshot, resultSnapshot JSON, assessmentDate time.Time, updatedAt time.Time) (GlovesEstimate, error) {
	result := ownedBy(r.db.WithContext(ctx).Model(&GlovesEstimate{}), userID).
		Where("id = ?", id).
		Updates(map[string]any{
			"assessment_date": assessmentDate,
			"input_snapshot":  inputSnapshot,
			"result_snapshot": resultSnapshot,
			"updated_at":      updatedAt,
		})
	if result.Error != nil {
		return GlovesEstimate{}, result.Error
	}
	if result.RowsAffected == 0 {
		return GlovesEstimate{}, ErrNotFound
	}
	return r.GlovesEstimateByIDForUser(ctx, id, userID)
}

func (r *Repository) DeleteGlovesEstimate(ctx context.Context, id, userID int64) error {
	result := ownedBy(r.db.WithContext(ctx).Where("id = ?", id), userID).Delete(&GlovesEstimate{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
