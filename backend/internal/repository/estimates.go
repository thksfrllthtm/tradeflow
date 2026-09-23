package repository

// Estimates of the import calculator.

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

func (r *Repository) ListEstimates(ctx context.Context, ownerID int64) ([]Estimate, error) {
	var estimates []Estimate
	err := r.db.WithContext(ctx).Where("owner_id = ?", ownerID).Order("updated_at DESC").Find(&estimates).Error
	return estimates, err
}

func (r *Repository) EstimateByIDForOwner(ctx context.Context, id, ownerID int64) (Estimate, error) {
	var estimate Estimate
	err := r.db.WithContext(ctx).Where("id = ? AND owner_id = ?", id, ownerID).First(&estimate).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Estimate{}, ErrNotFound
	}
	return estimate, err
}

func (r *Repository) CreateEstimate(ctx context.Context, estimate Estimate) (Estimate, error) {
	if err := r.db.WithContext(ctx).Create(&estimate).Error; err != nil {
		return Estimate{}, err
	}
	return estimate, nil
}

func (r *Repository) UpdateEstimate(ctx context.Context, estimate Estimate) (Estimate, error) {
	updates := map[string]any{
		"name":            estimate.Name,
		"client":          estimate.Client,
		"assessment_date": estimate.AssessmentDate,
		"currency":        estimate.Currency,
		"exchange_rate":   estimate.ExchangeRate,
		"origin":          estimate.Origin,
		"delivery_terms":  estimate.DeliveryTerms,
		"note":            estimate.Note,
		"updated_by":      estimate.UpdatedBy,
		"updated_at":      time.Now().UTC(),
	}
	result := r.db.WithContext(ctx).Model(&Estimate{}).Where("id = ? AND owner_id = ?", estimate.ID, estimate.OwnerID).Updates(updates)
	if result.Error != nil {
		return Estimate{}, result.Error
	}
	if result.RowsAffected == 0 {
		return Estimate{}, ErrNotFound
	}
	return r.EstimateByIDForOwner(ctx, estimate.ID, estimate.OwnerID)
}

// DeleteEstimateForOwner removes an estimate and all of its dependent import
// data after verifying ownership. It returns private file paths for cleanup by
// the service layer.
func (r *Repository) DeleteEstimateForOwner(ctx context.Context, id, ownerID int64) ([]string, error) {
	var estimate Estimate
	if err := r.db.WithContext(ctx).Where("id = ? AND owner_id = ?", id, ownerID).First(&estimate).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var imports []EstimateImport
	if err := r.db.WithContext(ctx).Where("estimate_id = ?", id).Find(&imports).Error; err != nil {
		return nil, err
	}
	paths := make([]string, 0, len(imports))
	importIDs := make([]int64, 0, len(imports))
	for _, item := range imports {
		paths = append(paths, item.StoredPath)
		importIDs = append(importIDs, item.ID)
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Delete in dependency order because imported_products.source_import_id
		// intentionally uses ON DELETE RESTRICT.
		if err := tx.Where("estimate_id = ?", id).Delete(&ImportedProduct{}).Error; err != nil {
			return err
		}
		if len(importIDs) > 0 {
			if err := tx.Where("import_id IN ?", importIDs).Delete(&ImportWorkspace{}).Error; err != nil {
				return err
			}
			if err := tx.Where("import_id IN ?", importIDs).Delete(&ImportAsset{}).Error; err != nil {
				return err
			}
			if err := tx.Where("id IN ?", importIDs).Delete(&EstimateImport{}).Error; err != nil {
				return err
			}
		}
		return tx.Delete(&Estimate{}, id).Error
	})
	return paths, err
}
