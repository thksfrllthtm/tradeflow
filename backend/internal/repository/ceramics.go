package repository

// Ceramics module: known marks with their name and price. Product type, size and Chinese
// name are derived from the name by the service. Independent of estimates and the customs
// calculation.

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

var ErrCeramicMarkExists = errors.New("ceramic mark already exists")

type CeramicProduct struct {
	ID             int64            `gorm:"primaryKey"`
	Mark           string           `gorm:"size:128;not null"`
	MarkKey        string           `gorm:"size:128;not null;unique"`
	Name           string           `gorm:"not null"`
	Price          decimal.Decimal  `gorm:"type:numeric(14,4);not null"`
	ProductType    string           `gorm:"size:500;not null"`
	ProductTypeKey string           `gorm:"size:500;not null"`
	SizeValue      *decimal.Decimal `gorm:"type:numeric(10,2)"`
	SizeUnit       *string          `gorm:"size:8"`
	ChineseName    string           `gorm:"not null"`
	CreatedBy      *int64
	UpdatedBy      *int64
	CreatedAt      time.Time `gorm:"not null"`
	UpdatedAt      time.Time `gorm:"not null"`
}

func (CeramicProduct) TableName() string { return "ceramic_products" }

// CeramicProductType is a product type with the number of marks of it.
type CeramicProductType struct {
	Name         string
	Key          string
	ProductCount int64
}

type CeramicProductFilter struct {
	Query   string
	TypeKey string
	Limit   int
	Offset  int
}

// CeramicProductTypes groups the base by product type.
func (r *Repository) CeramicProductTypes(ctx context.Context) ([]CeramicProductType, error) {
	var types []CeramicProductType
	err := r.db.WithContext(ctx).Model(&CeramicProduct{}).
		Select("min(product_type) AS name, product_type_key AS key, count(*) AS product_count").
		Group("product_type_key").Order("name ASC").Scan(&types).Error
	return types, err
}

// CeramicCatalog reads the whole base: mark patterns, Chinese name matches and prices.
func (r *Repository) CeramicCatalog(ctx context.Context) ([]CeramicProduct, error) {
	var rows []CeramicProduct
	return rows, r.db.WithContext(ctx).Order("id ASC").Find(&rows).Error
}

func (r *Repository) CeramicProductsByMarkKeys(ctx context.Context, keys []string) ([]CeramicProduct, error) {
	var products []CeramicProduct
	if len(keys) == 0 {
		return products, nil
	}
	return products, r.db.WithContext(ctx).Where("mark_key IN ?", keys).Find(&products).Error
}

func (r *Repository) CeramicProductByID(ctx context.Context, id int64) (CeramicProduct, error) {
	var product CeramicProduct
	err := r.db.WithContext(ctx).First(&product, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return CeramicProduct{}, ErrNotFound
	}
	return product, err
}

func (r *Repository) ListCeramicProducts(ctx context.Context, filter CeramicProductFilter) ([]CeramicProduct, int64, error) {
	query := r.db.WithContext(ctx).Model(&CeramicProduct{})
	if filter.TypeKey != "" {
		query = query.Where("product_type_key = ?", filter.TypeKey)
	}
	if text := strings.TrimSpace(filter.Query); text != "" {
		like := "%" + escapeLike(text) + "%"
		query = query.Where("mark ILIKE ? OR name ILIKE ?", like, like)
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var products []CeramicProduct
	err := query.Order("mark_key ASC").Limit(filter.Limit).Offset(filter.Offset).Find(&products).Error
	return products, total, err
}

func ceramicProductUpdates(product CeramicProduct) map[string]any {
	return map[string]any{
		"mark": product.Mark, "mark_key": product.MarkKey, "name": product.Name, "price": product.Price,
		"product_type": product.ProductType, "product_type_key": product.ProductTypeKey,
		"size_value": product.SizeValue, "size_unit": product.SizeUnit, "chinese_name": product.ChineseName,
		"updated_by": product.UpdatedBy, "updated_at": product.UpdatedAt,
	}
}

// SaveCeramicProduct creates (ID == 0) or updates a product. A mark taken by another product
// gives ErrCeramicMarkExists.
func (r *Repository) SaveCeramicProduct(ctx context.Context, product CeramicProduct) (CeramicProduct, error) {
	product.UpdatedAt = time.Now().UTC()
	var err error
	if product.ID == 0 {
		product.CreatedAt = product.UpdatedAt
		err = r.db.WithContext(ctx).Create(&product).Error
	} else {
		result := r.db.WithContext(ctx).Model(&CeramicProduct{}).Where("id = ?", product.ID).Updates(ceramicProductUpdates(product))
		err = result.Error
		if err == nil && result.RowsAffected == 0 {
			return CeramicProduct{}, ErrNotFound
		}
	}
	if err != nil {
		if isUniqueViolation(err) {
			return CeramicProduct{}, ErrCeramicMarkExists
		}
		return CeramicProduct{}, err
	}
	return r.CeramicProductByID(ctx, product.ID)
}

func (r *Repository) DeleteCeramicProduct(ctx context.Context, id int64) error {
	result := r.db.WithContext(ctx).Delete(&CeramicProduct{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// ImportCeramicProducts adds new marks and, with overwrite, updates existing ones in one
// transaction. Existing marks without overwrite are skipped.
func (r *Repository) ImportCeramicProducts(ctx context.Context, products []CeramicProduct, overwrite bool) (created, updated, skipped int, err error) {
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, product := range products {
			product.UpdatedAt = time.Now().UTC()
			var existing CeramicProduct
			findErr := tx.Select("id").Where("mark_key = ?", product.MarkKey).First(&existing).Error
			switch {
			case errors.Is(findErr, gorm.ErrRecordNotFound):
				product.CreatedAt = product.UpdatedAt
				if err := tx.Create(&product).Error; err != nil {
					return err
				}
				created++
			case findErr != nil:
				return findErr
			case !overwrite:
				skipped++
			default:
				if err := tx.Model(&CeramicProduct{}).Where("id = ?", existing.ID).Updates(ceramicProductUpdates(product)).Error; err != nil {
					return err
				}
				updated++
			}
		}
		return nil
	})
	if err != nil {
		return 0, 0, 0, err
	}
	return created, updated, skipped, nil
}

func isUniqueViolation(err error) bool {
	return err != nil && (strings.Contains(err.Error(), "SQLSTATE 23505") || strings.Contains(err.Error(), "duplicate key"))
}

func escapeLike(value string) string {
	return strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(value)
}
