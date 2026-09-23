package repository

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

func (r *Repository) ListGlovesEstimates(ctx context.Context, userID int64) ([]GlovesEstimate, error) {
	var estimates []GlovesEstimate
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).Order("updated_at DESC").Find(&estimates).Error
	return estimates, err
}

func (r *Repository) GlovesEstimateByIDForUser(ctx context.Context, id, userID int64) (GlovesEstimate, error) {
	var estimate GlovesEstimate
	err := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).First(&estimate).Error
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
	result := r.db.WithContext(ctx).Model(&GlovesEstimate{}).
		Where("id = ? AND user_id = ?", estimate.ID, estimate.UserID).
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
	result := r.db.WithContext(ctx).Model(&GlovesEstimate{}).
		Where("id = ? AND user_id = ?", id, userID).
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
	result := r.db.WithContext(ctx).Where("id = ? AND user_id = ?", id, userID).Delete(&GlovesEstimate{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) GlovesDraftByUser(ctx context.Context, userID int64) (GlovesCalculationDraft, error) {
	var draft GlovesCalculationDraft
	err := r.db.WithContext(ctx).Where("user_id = ?", userID).First(&draft).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return GlovesCalculationDraft{}, ErrNotFound
	}
	return draft, err
}

func (r *Repository) SaveGlovesDraft(ctx context.Context, draft GlovesCalculationDraft) (GlovesCalculationDraft, error) {
	var existing GlovesCalculationDraft
	err := r.db.WithContext(ctx).Where("user_id = ?", draft.UserID).First(&existing).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return GlovesCalculationDraft{}, err
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if createErr := r.db.WithContext(ctx).Create(&draft).Error; createErr != nil {
			return GlovesCalculationDraft{}, createErr
		}
		return draft, nil
	}
	draft.ID = existing.ID
	draft.CreatedAt = existing.CreatedAt
	if saveErr := r.db.WithContext(ctx).Model(&existing).Updates(map[string]any{
		"input_snapshot":  draft.InputSnapshot,
		"result_snapshot": draft.ResultSnapshot,
		"updated_at":      draft.UpdatedAt,
	}).Error; saveErr != nil {
		return GlovesCalculationDraft{}, saveErr
	}
	return r.GlovesDraftByUser(ctx, draft.UserID)
}
