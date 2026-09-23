package repository

// Stored image assets of imports and products.

import (
	"context"
	"errors"

	"gorm.io/gorm"
)

func (r *Repository) CreateAssetAndLink(ctx context.Context, asset ImportAsset, productID int64) (ImportAsset, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&asset).Error; err != nil {
			return err
		}
		return tx.Create(&ImportedProductImage{ProductID: productID, AssetID: asset.ID}).Error
	})
	return asset, err
}

func (r *Repository) CreateAsset(ctx context.Context, asset ImportAsset) (ImportAsset, error) {
	if err := r.db.WithContext(ctx).Create(&asset).Error; err != nil {
		return ImportAsset{}, err
	}
	return asset, nil
}

func (r *Repository) AssetsForImport(ctx context.Context, importID int64) ([]ImportAsset, error) {
	var assets []ImportAsset
	if err := r.db.WithContext(ctx).Where("import_id = ?", importID).Order("id ASC").Find(&assets).Error; err != nil {
		return nil, err
	}
	return assets, nil
}

func (r *Repository) DeleteManualAssetsForImportRowExcept(ctx context.Context, importID int64, rowID string, keepAssetID int64) error {
	return r.db.WithContext(ctx).Where("import_id = ? AND source_cell = ? AND kind = 'manual' AND id <> ?", importID, rowID, keepAssetID).Delete(&ImportAsset{}).Error
}

func (r *Repository) AssetByIDForProductOwner(ctx context.Context, assetID, productID, ownerID int64) (ImportAsset, error) {
	var asset ImportAsset
	err := r.db.WithContext(ctx).
		Joins("JOIN imported_product_images ON imported_product_images.asset_id = import_assets.id").
		Joins("JOIN imported_products ON imported_products.id = imported_product_images.product_id").
		Joins("JOIN estimates ON estimates.id = imported_products.estimate_id").
		Where("import_assets.id = ? AND imported_products.id = ? AND estimates.owner_id = ?", assetID, productID, ownerID).
		First(&asset).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ImportAsset{}, ErrNotFound
	}
	return asset, err
}

func (r *Repository) AssetByIDForImportOwner(ctx context.Context, assetID, importID, ownerID int64) (ImportAsset, error) {
	var asset ImportAsset
	err := r.db.WithContext(ctx).
		Joins("JOIN estimate_imports ON estimate_imports.id = import_assets.import_id").
		Joins("JOIN estimates ON estimates.id = estimate_imports.estimate_id").
		Where("import_assets.id = ? AND import_assets.import_id = ? AND estimates.owner_id = ?", assetID, importID, ownerID).
		First(&asset).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ImportAsset{}, ErrNotFound
	}
	return asset, err
}
