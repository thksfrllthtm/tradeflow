package repository

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

type CatalogSearchRow struct {
	Code    ClassifierCode
	Version ClassifierVersion
}

type CatalogDetails struct {
	Code      ClassifierCode
	Version   ClassifierVersion
	Documents []ClassifierDocument
	Rules     []CalculationRule
}

func (r *Repository) CatalogVersionByKey(ctx context.Context, key string) (ClassifierVersion, error) {
	var v ClassifierVersion
	err := r.db.WithContext(ctx).Where("version_key = ?", key).First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ClassifierVersion{}, ErrNotFound
	}
	return v, err
}

func (r *Repository) ImportCatalog(ctx context.Context, version ClassifierVersion, codes []ClassifierCode, docs []ClassifierDocument, rules []CalculationRule) (ClassifierVersion, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing ClassifierVersion
		if err := tx.Where("version_key = ?", version.VersionKey).First(&existing).Error; err == nil {
			version = existing
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if err := tx.Create(&version).Error; err != nil {
			return err
		}
		ids := make(map[string]int64, len(codes))
		for i := range codes {
			codes[i].VersionID = version.ID
			if err := tx.Create(&codes[i]).Error; err != nil {
				return err
			}
			ids[codes[i].Code] = codes[i].ID
		}
		for i := range docs {
			docs[i].VersionID = version.ID
			if docs[i].CodeID == nil && docs[i].ParentCode != "" {
				if id, ok := ids[docs[i].ParentCode]; ok {
					docs[i].CodeID = &id
				}
			}
			if err := tx.Create(&docs[i]).Error; err != nil {
				return err
			}
		}
		for i := range rules {
			rules[i].VersionID = version.ID
			if rules[i].CodeID == nil && rules[i].ParentCode != "" {
				if id, ok := ids[rules[i].ParentCode]; ok {
					rules[i].CodeID = &id
				}
			}
			if err := tx.Create(&rules[i]).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return ClassifierVersion{}, err
	}
	return r.CatalogVersionByKey(ctx, version.VersionKey)
}

func (r *Repository) SearchCatalog(ctx context.Context, q string, on time.Time, includeDemo bool) ([]CatalogSearchRow, error) {
	query := r.db.WithContext(ctx).Model(&ClassifierCode{}).Joins("JOIN classifier_versions v ON v.id = classifier_codes.version_id").Where("classifier_codes.valid_from <= ? AND (classifier_codes.valid_to IS NULL OR classifier_codes.valid_to >= ?) AND v.status = 'active'", on, on)
	if !includeDemo {
		query = query.Where("v.dataset_kind <> 'demo'")
	}
	q = strings.TrimSpace(q)
	if q != "" {
		query = query.Where("classifier_codes.code ILIKE ? OR classifier_codes.official_description ILIKE ?", q+"%", "%"+q+"%")
	}
	var codes []ClassifierCode
	if err := query.Order("classifier_codes.code ASC").Find(&codes).Error; err != nil {
		return nil, err
	}
	rows := make([]CatalogSearchRow, 0, len(codes))
	for _, code := range codes {
		var version ClassifierVersion
		if err := r.db.WithContext(ctx).First(&version, code.VersionID).Error; err != nil {
			return nil, err
		}
		rows = append(rows, CatalogSearchRow{Code: code, Version: version})
	}
	return rows, nil
}

func (r *Repository) CatalogDetails(ctx context.Context, code string, on time.Time, includeDemo bool) ([]CatalogDetails, error) {
	query := r.db.WithContext(ctx).Model(&ClassifierCode{}).Joins("JOIN classifier_versions v ON v.id = classifier_codes.version_id").Where("classifier_codes.code = ? AND classifier_codes.valid_from <= ? AND (classifier_codes.valid_to IS NULL OR classifier_codes.valid_to >= ?) AND v.status = 'active'", code, on, on)
	if !includeDemo {
		query = query.Where("v.dataset_kind <> 'demo'")
	}
	var codes []ClassifierCode
	if err := query.Find(&codes).Error; err != nil {
		return nil, err
	}
	rows := make([]CatalogSearchRow, 0, len(codes))
	for _, c := range codes {
		var v ClassifierVersion
		if err := r.db.WithContext(ctx).First(&v, c.VersionID).Error; err != nil {
			return nil, err
		}
		rows = append(rows, CatalogSearchRow{Code: c, Version: v})
	}
	result := make([]CatalogDetails, 0, len(rows))
	for _, row := range rows {
		var docs []ClassifierDocument
		var rules []CalculationRule
		if err := r.db.WithContext(ctx).Where("version_id = ? AND (code_id = ? OR parent_code = ?) AND valid_from <= ? AND (valid_to IS NULL OR valid_to >= ?)", row.Version.ID, row.Code.ID, code, on, on).Find(&docs).Error; err != nil {
			return nil, err
		}
		if err := r.db.WithContext(ctx).Where("version_id = ? AND (code_id = ? OR parent_code = ?) AND valid_from <= ? AND (valid_to IS NULL OR valid_to >= ?)", row.Version.ID, row.Code.ID, code, on, on).Find(&rules).Error; err != nil {
			return nil, err
		}
		result = append(result, CatalogDetails{Code: row.Code, Version: row.Version, Documents: docs, Rules: rules})
	}
	return result, nil
}

func (r *Repository) RulesForCode(ctx context.Context, code string, on time.Time, includeDemo bool) ([]CalculationRule, []ClassifierCode, []ClassifierVersion, error) {
	var codes []ClassifierCode
	q := r.db.WithContext(ctx).Table("classifier_codes c").Joins("JOIN classifier_versions v ON v.id = c.version_id").Where("c.code = ? AND c.valid_from <= ? AND (c.valid_to IS NULL OR c.valid_to >= ?) AND v.status = 'active'", code, on, on)
	if !includeDemo {
		q = q.Where("v.dataset_kind <> 'demo'")
	}
	if err := q.Find(&codes).Error; err != nil {
		return nil, nil, nil, err
	}
	if len(codes) == 0 {
		return nil, nil, nil, ErrNotFound
	}
	var rules []CalculationRule
	var versions []ClassifierVersion
	for _, c := range codes {
		var v ClassifierVersion
		if err := r.db.WithContext(ctx).First(&v, c.VersionID).Error; err != nil {
			return nil, nil, nil, err
		}
		versions = append(versions, v)
		var rs []CalculationRule
		if err := r.db.WithContext(ctx).Where("version_id = ? AND (code_id = ? OR parent_code = ?) AND valid_from <= ? AND (valid_to IS NULL OR valid_to >= ?)", c.VersionID, c.ID, code, on, on).Find(&rs).Error; err != nil {
			return nil, nil, nil, err
		}
		rules = append(rules, rs...)
	}
	return rules, codes, versions, nil
}
