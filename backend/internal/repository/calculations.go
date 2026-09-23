package repository

// Customs calculations of an estimate with revision history.

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

func (r *Repository) CalculationByEstimateForOwner(ctx context.Context, estimateID, ownerID int64) (CustomsCalculation, error) {
	var calculation CustomsCalculation
	err := r.db.WithContext(ctx).
		Joins("JOIN estimates ON estimates.id = customs_calculations.estimate_id").
		Where("customs_calculations.estimate_id = ? AND estimates.owner_id = ?", estimateID, ownerID).
		First(&calculation).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return CustomsCalculation{}, ErrNotFound
	}
	return calculation, err
}

func (r *Repository) SaveCalculation(ctx context.Context, calculation CustomsCalculation) (CustomsCalculation, error) {
	db := r.db.WithContext(ctx)
	var saved CustomsCalculation
	err := db.Transaction(func(tx *gorm.DB) error {
		var existing CustomsCalculation
		err := tx.Where("estimate_id = ?", calculation.EstimateID).First(&existing).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := tx.Create(&calculation).Error; err != nil {
				return err
			}
			if err := createCalculationRevision(tx, calculation, 1); err != nil {
				return err
			}
			saved = calculation
			return nil
		}
		if err != nil {
			return err
		}
		var maxRevision int64
		if err := tx.Model(&CustomsCalculationRevision{}).Where("calculation_id = ?", existing.ID).Select("COALESCE(MAX(revision_no), 0)").Scan(&maxRevision).Error; err != nil {
			return err
		}
		if maxRevision == 0 {
			legacy := existing
			if legacy.FormulaVersion == "" {
				legacy.FormulaVersion = "legacy/unknown"
			}
			if legacy.RoundingPolicy == "" {
				legacy.RoundingPolicy = "legacy/unknown"
			}
			if err := createCalculationRevision(tx, legacy, 1); err != nil {
				return err
			}
			maxRevision = 1
		}
		calculation.ID = existing.ID
		calculation.CreatedBy = existing.CreatedBy
		calculation.CreatedAt = existing.CreatedAt
		calculation.UpdatedAt = time.Now().UTC()
		updates := map[string]any{
			"assessment_date": calculation.AssessmentDate, "origin": calculation.Origin,
			"usd_kzt": calculation.USDKZT, "eur_kzt": calculation.EURKZT, "wagons": calculation.Wagons,
			"wagon_tariff_usd": calculation.WagonTariffUSD, "delivery_mode": calculation.DeliveryMode,
			"delivery_total_usd": calculation.DeliveryTotalUSD, "package_weight_kg": calculation.PackageWeightKG,
			"customs_fee_kzt": calculation.CustomsFeeKZT, "fee_included": calculation.FeeIncluded,
			"cash_usd": calculation.CashUSD, "items": calculation.Items, "totals": calculation.Totals,
			"formula_version": calculation.FormulaVersion, "rounding_policy": calculation.RoundingPolicy,
			"input_snapshot": calculation.InputSnapshot, "calculated_at": calculation.CalculatedAt,
			"updated_by": calculation.UpdatedBy, "updated_at": calculation.UpdatedAt,
		}
		if err := tx.Model(&CustomsCalculation{}).Where("id = ?", existing.ID).Updates(updates).Error; err != nil {
			return err
		}
		if err := createCalculationRevision(tx, calculation, maxRevision+1); err != nil {
			return err
		}
		saved = calculation
		return nil
	})
	if err != nil {
		return CustomsCalculation{}, err
	}
	return saved, nil
}

func createCalculationRevision(tx *gorm.DB, calculation CustomsCalculation, revisionNo int64) error {
	resultSnapshot, err := json.Marshal(map[string]any{
		"assessmentDate":   calculation.AssessmentDate.Format("2006-01-02"),
		"origin":           calculation.Origin,
		"usdKzt":           calculation.USDKZT.String(),
		"eurKzt":           decimalStringPtr(calculation.EURKZT),
		"deliveryMode":     calculation.DeliveryMode,
		"deliveryTotalUsd": decimalStringPtr(calculation.DeliveryTotalUSD),
		"wagons":           decimalStringPtr(calculation.Wagons),
		"wagonTariffUsd":   calculation.WagonTariffUSD.String(),
		"packageWeightKg":  calculation.PackageWeightKG.String(),
		"customsFeeKzt":    decimalStringPtr(calculation.CustomsFeeKZT),
		"feeIncluded":      calculation.FeeIncluded,
		"cashUsd":          decimalStringPtr(calculation.CashUSD),
		"items":            calculation.Items,
		"totals":           calculation.Totals,
	})
	if err != nil {
		return err
	}
	return tx.Create(&CustomsCalculationRevision{
		CalculationID: calculation.ID, RevisionNo: revisionNo,
		FormulaVersion: calculation.FormulaVersion, RoundingPolicy: calculation.RoundingPolicy,
		InputSnapshot: calculation.InputSnapshot, ResultSnapshot: JSON(resultSnapshot),
		CreatedBy: calculation.UpdatedBy, CreatedAt: calculation.UpdatedAt,
	}).Error
}

func decimalStringPtr(value *decimal.Decimal) string {
	if value == nil {
		return ""
	}
	return value.String()
}
