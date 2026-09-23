package repository

// Import workspaces (editable copy of the imported sheet).

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

func (r *Repository) WorkspaceByImportForOwner(ctx context.Context, importID, ownerID int64) (ImportWorkspace, error) {
	var workspace ImportWorkspace
	err := r.db.WithContext(ctx).
		Joins("JOIN estimate_imports ON estimate_imports.id = import_workspaces.import_id").
		Joins("JOIN estimates ON estimates.id = estimate_imports.estimate_id").
		Where("import_workspaces.import_id = ? AND estimates.owner_id = ?", importID, ownerID).
		First(&workspace).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ImportWorkspace{}, ErrNotFound
	}
	return workspace, err
}

func (r *Repository) SaveWorkspace(ctx context.Context, workspace ImportWorkspace) (ImportWorkspace, error) {
	var existing ImportWorkspace
	err := r.db.WithContext(ctx).Where("import_id = ?", workspace.ImportID).First(&existing).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if workspace.Revision < 1 {
			workspace.Revision = 1
		}
		if err := r.db.WithContext(ctx).Create(&workspace).Error; err != nil {
			return ImportWorkspace{}, err
		}
		return workspace, nil
	}
	if err != nil {
		return ImportWorkspace{}, err
	}
	workspace.Revision = existing.Revision + 1
	updates := map[string]any{
		"sheet": workspace.Sheet, "header_row": workspace.HeaderRow, "data_start_row": workspace.DataStartRow,
		"columns": workspace.Columns, "rows": workspace.Rows, "deleted_rows": workspace.DeletedRows,
		"deleted_columns": workspace.DeletedColumns, "batch": workspace.Batch, "price_basis": workspace.PriceBasis,
		"price_unit_label": workspace.PriceUnitLabel, "revision": workspace.Revision, "updated_by": workspace.UpdatedBy, "updated_at": time.Now().UTC(),
	}
	if err := r.db.WithContext(ctx).Model(&ImportWorkspace{}).Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
		return ImportWorkspace{}, err
	}
	workspace.ID = existing.ID
	workspace.UpdatedAt = time.Now().UTC()
	return workspace, nil
}

// SaveWorkspaceAndReplaceProducts commits the draft and its normalized products
// in one transaction. Existing products are untouched if any insert fails.
func (r *Repository) SaveWorkspaceAndReplaceProducts(ctx context.Context, workspace ImportWorkspace, estimateID, importID, userID int64, products []ImportedProduct, assets []ImportAsset, links map[int][]int) ([]ImportedProduct, error) {
	var savedProducts []ImportedProduct
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing ImportWorkspace
		findErr := tx.Where("import_id = ?", workspace.ImportID).First(&existing).Error
		if errors.Is(findErr, gorm.ErrRecordNotFound) {
			if workspace.Revision < 1 {
				workspace.Revision = 1
			}
			if err := tx.Create(&workspace).Error; err != nil {
				return err
			}
		} else if findErr != nil {
			return findErr
		} else {
			workspace.Revision = existing.Revision + 1
			if err := tx.Model(&ImportWorkspace{}).Where("id = ?", existing.ID).Updates(map[string]any{
				"sheet": workspace.Sheet, "header_row": workspace.HeaderRow, "data_start_row": workspace.DataStartRow,
				"columns": workspace.Columns, "rows": workspace.Rows, "deleted_rows": workspace.DeletedRows,
				"deleted_columns": workspace.DeletedColumns, "batch": workspace.Batch, "price_basis": workspace.PriceBasis,
				"price_unit_label": workspace.PriceUnitLabel, "revision": workspace.Revision, "updated_by": workspace.UpdatedBy, "updated_at": time.Now().UTC(),
			}).Error; err != nil {
				return err
			}
		}
		// Embedded pictures are regenerated from the immutable XLSX on every
		// apply. Remove the previous derived records so repeated applies do not
		// accumulate duplicate thumbnails. Manual uploads are retained.
		if err := tx.Where("import_id = ? AND kind = 'embedded'", importID).Delete(&ImportAsset{}).Error; err != nil {
			return err
		}
		if err := tx.Where("estimate_id = ?", estimateID).Delete(&ImportedProduct{}).Error; err != nil {
			return err
		}
		for index := range products {
			products[index].EstimateID, products[index].SourceImportID = estimateID, importID
			products[index].CreatedBy, products[index].UpdatedBy = userID, userID
			if err := tx.Create(&products[index]).Error; err != nil {
				return err
			}
			savedProducts = append(savedProducts, products[index])
		}
		savedAssets := make([]ImportAsset, 0, len(assets))
		for index := range assets {
			assets[index].ImportID = importID
			if assets[index].ID > 0 {
				// Existing manual assets are reused when a draft is applied again;
				// do not attempt to insert their primary key a second time.
				savedAssets = append(savedAssets, assets[index])
				continue
			}
			if err := tx.Create(&assets[index]).Error; err != nil {
				return err
			}
			savedAssets = append(savedAssets, assets[index])
		}
		for productIndex, assetIndexes := range links {
			if productIndex < 0 || productIndex >= len(savedProducts) {
				continue
			}
			for _, assetIndex := range assetIndexes {
				if assetIndex < 0 || assetIndex >= len(savedAssets) {
					continue
				}
				if err := tx.Create(&ImportedProductImage{ProductID: savedProducts[productIndex].ID, AssetID: savedAssets[assetIndex].ID}).Error; err != nil {
					return err
				}
			}
		}
		now := time.Now().UTC()
		return tx.Model(&EstimateImport{}).Where("id = ?", importID).Updates(map[string]any{"status": "confirmed", "confirmed_at": now, "confirmed_by": userID}).Error
	})
	return savedProducts, err
}
