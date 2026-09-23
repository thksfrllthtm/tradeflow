package repository

// XLSX imports of an estimate.

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

func (r *Repository) CreateImport(ctx context.Context, item EstimateImport) (EstimateImport, error) {
	if err := r.db.WithContext(ctx).Create(&item).Error; err != nil {
		return EstimateImport{}, err
	}
	return item, nil
}

func (r *Repository) ImportByIDForOwner(ctx context.Context, id, ownerID int64) (EstimateImport, error) {
	var item EstimateImport
	err := r.db.WithContext(ctx).
		Joins("JOIN estimates ON estimates.id = estimate_imports.estimate_id").
		Where("estimate_imports.id = ? AND estimates.owner_id = ?", id, ownerID).
		First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return EstimateImport{}, ErrNotFound
	}
	return item, err
}

func (r *Repository) LatestImportForEstimateOwner(ctx context.Context, estimateID, ownerID int64) (EstimateImport, error) {
	var item EstimateImport
	err := r.db.WithContext(ctx).
		Joins("JOIN estimates ON estimates.id = estimate_imports.estimate_id").
		Where("estimate_imports.estimate_id = ? AND estimates.owner_id = ?", estimateID, ownerID).
		Order("estimate_imports.created_at DESC, estimate_imports.id DESC").
		First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return EstimateImport{}, ErrNotFound
	}
	return item, err
}

func (r *Repository) LatestImportWithHash(ctx context.Context, estimateID int64, sha256 string) (EstimateImport, error) {
	var item EstimateImport
	err := r.db.WithContext(ctx).Where("estimate_id = ? AND sha256 = ?", estimateID, sha256).Order("created_at DESC").First(&item).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return EstimateImport{}, ErrNotFound
	}
	return item, err
}
