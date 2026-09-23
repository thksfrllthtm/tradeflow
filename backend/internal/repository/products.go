package repository

// Products imported into an estimate.

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

func (r *Repository) ProductCount(ctx context.Context, estimateID int64) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&ImportedProduct{}).Where("estimate_id = ?", estimateID).Count(&count).Error
	return count, err
}

// ReplaceProducts atomically removes the currently confirmed product set and
// inserts the confirmed rows from one import. The caller checks replacement
// consent before this method is called.
func (r *Repository) ReplaceProducts(ctx context.Context, estimateID int64, importID, userID int64, products []ImportedProduct, assets []ImportAsset, links map[int][]int) ([]ImportedProduct, []ImportAsset, error) {
	var savedProducts []ImportedProduct
	var savedAssets []ImportAsset
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("estimate_id = ?", estimateID).Delete(&ImportedProduct{}).Error; err != nil {
			return err
		}
		for index := range products {
			products[index].EstimateID = estimateID
			products[index].SourceImportID = importID
			products[index].CreatedBy = userID
			products[index].UpdatedBy = userID
			if err := tx.Create(&products[index]).Error; err != nil {
				return err
			}
			savedProducts = append(savedProducts, products[index])
		}
		for index := range assets {
			assets[index].ImportID = importID
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
	return savedProducts, savedAssets, err
}

func (r *Repository) ListProductsForEstimate(ctx context.Context, estimateID int64) ([]ImportedProduct, error) {
	var products []ImportedProduct
	err := r.db.WithContext(ctx).Where("estimate_id = ?", estimateID).Order("id ASC").Find(&products).Error
	return products, err
}

func (r *Repository) ProductByIDForOwner(ctx context.Context, id, ownerID int64) (ImportedProduct, error) {
	var product ImportedProduct
	err := r.db.WithContext(ctx).
		Joins("JOIN estimates ON estimates.id = imported_products.estimate_id").
		Where("imported_products.id = ? AND estimates.owner_id = ?", id, ownerID).
		First(&product).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ImportedProduct{}, ErrNotFound
	}
	return product, err
}

func (r *Repository) ImageAssetIDsForProducts(ctx context.Context, productIDs []int64) (map[int64][]int64, error) {
	result := make(map[int64][]int64)
	if len(productIDs) == 0 {
		return result, nil
	}
	var rows []struct {
		ProductID int64
		AssetID   int64
	}
	if err := r.db.WithContext(ctx).Table("imported_product_images").
		Select("product_id, asset_id").Where("product_id IN ?", productIDs).Order("product_id, asset_id").Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.ProductID] = append(result[row.ProductID], row.AssetID)
	}
	return result, nil
}

func (r *Repository) UpdateProduct(ctx context.Context, product ImportedProduct) (ImportedProduct, error) {
	updates := map[string]any{
		"name": product.Name, "translation": product.Translation, "sku": product.SKU,
		"quantity": product.Quantity, "quantity_unit": product.QuantityUnit, "pairs": product.Pairs, "places": product.Places, "pairs_per_place": product.PairsPerPlace, "unit_price": product.UnitPrice, "line_total": product.LineTotal,
		"currency": product.Currency, "weight": product.Weight, "characteristics": product.Characteristics,
		"gross_weight": product.GrossWeight, "gross_weight_unit": product.GrossWeightUnit, "gross_weight_basis": product.GrossWeightBasis,
		"net_weight": product.NetWeight, "net_weight_unit": product.NetWeightUnit, "net_weight_basis": product.NetWeightBasis,
		"material": product.Material, "photo_url": product.PhotoURL, "additional_fields": product.AdditionalFields,
		"tn_ved_code": product.TNVEDCode, "tn_ved_code_source": product.TNVEDCodeSource,
		"tnved_directory_entry_id": product.TNVEDDirectoryEntryID, "tnved_directory_snapshot": product.TNVEDDirectorySnapshot,
		"ai_data_revision": product.AIDataRevision,
		"price_basis":      product.PriceBasis, "price_unit_label": product.PriceUnitLabel,
		"weight_basis": product.WeightBasis, "excluded": product.Excluded, "exclusion_reason": product.ExclusionReason,
		"updated_by": product.UpdatedBy, "updated_at": time.Now().UTC(),
	}
	result := r.db.WithContext(ctx).Model(&ImportedProduct{}).Where("id = ? AND estimate_id = ?", product.ID, product.EstimateID).Updates(updates)
	if result.Error != nil {
		return ImportedProduct{}, result.Error
	}
	if result.RowsAffected == 0 {
		return ImportedProduct{}, ErrNotFound
	}
	return r.ProductByIDForOwner(ctx, product.ID, product.UpdatedBy)
}

func (r *Repository) UpdateProductTNVEDDirectorySelection(ctx context.Context, productID, ownerID int64, entryID int64, snapshot JSON, code string) (ImportedProduct, error) {
	product, err := r.ProductByIDForOwner(ctx, productID, ownerID)
	if err != nil {
		return ImportedProduct{}, err
	}
	product.TNVEDCode = code
	product.TNVEDCodeSource = "manual"
	product.TNVEDDirectoryEntryID = &entryID
	product.TNVEDDirectorySnapshot = snapshot
	product.UpdatedBy = ownerID
	return r.UpdateProduct(ctx, product)
}
