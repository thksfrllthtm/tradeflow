package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/shopspring/decimal"
)

type CalculationService struct{ repository *repository.Repository }

const (
	calculationFormulaVersion = "2026-09-kz-customs-v1"
	calculationRoundingPolicy = "kz-round-half-up-2"
)

func NewCalculationService(repo *repository.Repository) *CalculationService {
	return &CalculationService{repository: repo}
}

func (s *CalculationService) Get(ctx context.Context, estimateID, ownerID int64) (CalculationResult, error) {
	calc, err := s.repository.CalculationByEstimateForOwner(ctx, estimateID, ownerID)
	if errors.Is(err, repository.ErrNotFound) {
		return s.recalculate(ctx, estimateID, ownerID, CalculationInput{}, true)
	}
	if err != nil {
		return CalculationResult{}, err
	}
	return resultFromModel(calc)
}

func (s *CalculationService) Save(ctx context.Context, estimateID, ownerID int64, input CalculationInput) (CalculationResult, error) {
	// A newly created calculation has no delivery policy yet. Once the
	// operator explicitly chooses a delivery mode, include the allocated
	// delivery in the customs base by default for this scenario. An explicitly
	// saved calculation with no rows yet also applies that policy when the
	// confirmed import is loaded for the first time. Existing rows in a
	// calculation that already has a mode keep their per-row choices and are
	// never switched silently.
	existing, lookupErr := s.repository.CalculationByEstimateForOwner(ctx, estimateID, ownerID)
	if lookupErr != nil && !errors.Is(lookupErr, repository.ErrNotFound) {
		return CalculationResult{}, lookupErr
	}
	if strings.TrimSpace(input.DeliveryMode) != "" && (errors.Is(lookupErr, repository.ErrNotFound) || strings.TrimSpace(existing.DeliveryMode) == "" || len(input.Items) == 0) {
		if input.DeliveryIncludedDefault != nil {
			input.defaultDeliveryIncluded = *input.DeliveryIncludedDefault
		} else {
			input.defaultDeliveryIncluded = defaultDeliveryInCustoms(input.Items)
		}
		if input.defaultDeliveryIncluded {
			for index := range input.Items {
				input.Items[index].DeliveryIncluded = true
			}
		}
	}
	return s.recalculateWithInput(ctx, estimateID, ownerID, input, false)
}

func defaultDeliveryInCustoms(items []CalculationItemInput) bool {
	for _, item := range items {
		if !item.Excluded && item.DeliveryIncluded {
			return false
		}
	}
	return true
}

func (s *CalculationService) recalculate(ctx context.Context, estimateID, ownerID int64, input CalculationInput, initialize bool) (CalculationResult, error) {
	return s.recalculateWithInput(ctx, estimateID, ownerID, input, initialize)
}

func (s *CalculationService) recalculateWithInput(ctx context.Context, estimateID, ownerID int64, input CalculationInput, initialize bool) (CalculationResult, error) {
	estimate, err := s.repository.EstimateByIDForOwner(ctx, estimateID, ownerID)
	if err != nil {
		return CalculationResult{}, err
	}
	products, err := s.repository.ListProductsForEstimate(ctx, estimateID)
	if err != nil {
		return CalculationResult{}, err
	}
	productIDs := make([]int64, 0, len(products))
	for _, product := range products {
		productIDs = append(productIDs, product.ID)
	}
	imageAssetIDs, err := s.repository.ImageAssetIDsForProducts(ctx, productIDs)
	if err != nil {
		return CalculationResult{}, err
	}
	if input.AssessmentDate == "" {
		input.AssessmentDate = estimate.AssessmentDate.Format("2006-01-02")
	}
	date, err := time.Parse("2006-01-02", input.AssessmentDate)
	if err != nil {
		return CalculationResult{}, &ValidationError{Field: "assessmentDate", Message: "Укажите дату расчёта в формате ГГГГ-ММ-ДД"}
	}
	if input.feeIncludedSet && !input.FeeIncluded {
		return CalculationResult{}, &ValidationError{Field: "feeIncluded", Message: "Таможенный сбор включён в базу НДС по фиксированной предварительной формуле"}
	}
	var usd decimal.Decimal
	if strings.TrimSpace(input.USDKZT) == "" {
		usd = estimate.ExchangeRate
	} else {
		usd, err = parsePositive(input.USDKZT, "usdKzt", false)
		if err != nil {
			return CalculationResult{}, err
		}
	}
	euro, err := parseOptionalPositive(input.EURKZT, "eurKzt")
	if err != nil {
		return CalculationResult{}, err
	}
	deliveryMode := strings.ToLower(strings.TrimSpace(input.DeliveryMode))
	if deliveryMode != "" && deliveryMode != "wagons" && deliveryMode != "total" {
		return CalculationResult{}, &ValidationError{Field: "deliveryMode", Message: "Выберите режим доставки: по вагонам или общая сумма"}
	}
	var wagons *decimal.Decimal
	var deliveryTotalUSD *decimal.Decimal
	tariff := decimal.NewFromInt(420)
	switch deliveryMode {
	case "wagons":
		if strings.TrimSpace(input.Wagons) == "" {
			return CalculationResult{}, &ValidationError{Field: "wagons", Message: "Укажите количество вагонов (для нулевой доставки введите 0)"}
		}
		wagons, err = calcOptionalNonNegative(input.Wagons, "wagons")
		if err != nil {
			return CalculationResult{}, err
		}
		tariff, err = parseNonNegativeDefault(input.WagonTariffUSD, tariff, "wagonTariffUsd")
		if err != nil {
			return CalculationResult{}, err
		}
		total := wagons.Mul(tariff).Round(2)
		deliveryTotalUSD = &total
	case "total":
		if strings.TrimSpace(input.DeliveryTotalUSD) == "" {
			return CalculationResult{}, &ValidationError{Field: "deliveryTotalUsd", Message: "Укажите общую доставку в USD (для нулевой доставки введите 0)"}
		}
		total, parseErr := calcOptionalNonNegative(input.DeliveryTotalUSD, "deliveryTotalUsd")
		if parseErr != nil {
			return CalculationResult{}, parseErr
		}
		deliveryTotalUSD = total
	default:
		// An old calculation may contain allocated delivery but no mode. Do
		// not infer a mode from those values: require an explicit choice on the
		// next save and keep the old snapshot intact until then.
		if !initialize && hasDeliveryInput(input) {
			return CalculationResult{}, &ValidationError{Field: "deliveryMode", Message: "Сохранённая доставка не имеет режима. Выберите «По вагонам» или «Общая сумма» и сохраните заново"}
		}
	}
	pkg, err := parseNonNegativeDefault(input.PackageWeightKG, decimal.NewFromFloat(0.3), "packageWeightKg")
	if err != nil {
		return CalculationResult{}, err
	}
	fee, err := resolveCustomsFee(input.CustomsFeeKZT, input.MRP, input.FeeMultiplier)
	if err != nil {
		return CalculationResult{}, err
	}
	cash, err := calcOptionalNonNegative(input.CashUSD, "cashUsd")
	if err != nil {
		return CalculationResult{}, err
	}

	validProductIDs := make(map[int64]struct{}, len(products))
	byID := make(map[int64]CalculationItemInput, len(input.Items))
	for _, product := range products {
		validProductIDs[product.ID] = struct{}{}
	}
	for _, item := range input.Items {
		if item.ProductID == 0 {
			continue
		}
		if _, ok := validProductIDs[item.ProductID]; !ok {
			return CalculationResult{}, &ValidationError{Field: "items", Message: "Расчёт содержит позицию, которой нет в этой смете"}
		}
		byID[item.ProductID] = item
	}
	items := make([]CalculationItem, 0, len(products))
	for _, p := range products {
		item, ok := byID[p.ID]
		if !ok || initialize {
			item = itemFromProduct(p, imageAssetIDs[p.ID])
			if input.defaultDeliveryIncluded && deliveryTotalUSD != nil {
				item.DeliveryIncluded = true
			}
		}
		if item.ProductID == 0 {
			item.ProductID = p.ID
		}
		if item.SupplierName == "" {
			item.SupplierName = p.Name
		}
		if item.StandardName == "" {
			item.StandardName = p.Name
		}
		if p.UnitPrice != nil && p.UnitPrice.IsPositive() && supportedCostCurrency(p.Currency) {
			// The supplier price is the live source for pairs x price-per-pair.
			// Refresh it on every recalculation so a corrected price or quantity
			// propagates instead of reusing a previously stored amount. A manual
			// final cost stays in CostInput and is only flagged, never replaced.
			item.PricePerPair = p.UnitPrice.String()
		}
		if strings.TrimSpace(item.CostCurrency) == "" {
			if currency := strings.ToUpper(strings.TrimSpace(p.Currency)); currency == "USD" || currency == "EUR" || currency == "KZT" {
				item.CostCurrency = currency
			}
		}
		// Rates and directory metadata are server-owned. Ignore a snapshot sent
		// by the browser and use the snapshot attached to the product instead.
		if len(p.TNVEDDirectorySnapshot) > 0 {
			item.DirectorySnapshot = nil
			_ = json.Unmarshal(p.TNVEDDirectorySnapshot, &item.DirectorySnapshot)
		} else {
			item.DirectorySnapshot = nil
		}
		if p.TNVEDDirectoryEntryID != nil {
			entryID := *p.TNVEDDirectoryEntryID
			item.DirectoryEntryID = &entryID
		} else {
			item.DirectoryEntryID = nil
		}
		// Image IDs are derived from the ownership-checked relation, never from
		// the JSON submitted by the browser.
		item.ImageAssetIDs = append([]int64(nil), imageAssetIDs[p.ID]...)
		items = append(items, calculateItem(item, usd, decimalValueOrZero(euro), pkg))
	}
	allocateDelivery(items, deliveryTotalUSD, usd)
	globalCalculationIssues := make([]string, 0)
	if deliveryTotalUSD != nil {
		expectedDelivery := deliveryTotalUSD.Round(2)
		var actualDelivery decimal.Decimal
		manualDelivery := false
		missingGross := false
		includedRows := 0
		for _, item := range items {
			if item.Excluded {
				continue
			}
			includedRows++
			if item.DeliveryManual {
				manualDelivery = true
			}
			if strings.TrimSpace(item.GrossKG) == "" || !mustDecimal(item.GrossKG).IsPositive() {
				missingGross = true
			}
			if item.DeliveryUSD != "" {
				actualDelivery = actualDelivery.Add(mustDecimal(item.DeliveryUSD))
			}
		}
		if !manualDelivery && missingGross && expectedDelivery.IsPositive() {
			for i := range items {
				if items[i].Excluded {
					continue
				}
				items[i].Warnings = append(items[i].Warnings, "Распределение доставки: не для всех включённых строк указан положительный брутто; укажите его или распределите доставку вручную")
				items[i].Status = "needs_review"
			}
		}
		if manualDelivery && !actualDelivery.Equal(expectedDelivery) {
			for i := range items {
				if items[i].Excluded {
					continue
				}
				items[i].Warnings = append(items[i].Warnings, "Ручное распределение доставки не равно общей доставке")
				items[i].Status = "needs_review"
			}
		}
		if !manualDelivery && actualDelivery.IsZero() && expectedDelivery.IsPositive() {
			const noDistribution = "Распределение доставки: невозможно распределить положительную доставку без включённых строк с положительным брутто"
			if includedRows == 0 {
				globalCalculationIssues = append(globalCalculationIssues, noDistribution)
			}
			for i := range items {
				if items[i].Excluded {
					continue
				}
				items[i].Warnings = append(items[i].Warnings, noDistribution)
				items[i].Status = "needs_review"
			}
		}
	}
	for i := range items {
		// Re-run the tariff part once all rows are known. The customs value no
		// longer depends on allocated delivery: delivery is a separate party
		// expense, while customs-includable costs are per-item extra expenses.
		items[i].CustomsValueKZT, items[i].DutyKZT, items[i].AntiDumpingKZT, items[i].VATKZT = "", "", "", ""
		allIssues := append(append([]string{}, items[i].Errors...), items[i].Warnings...)
		items[i].Errors = nil
		items[i].Warnings = filterRateWarnings(allIssues)
		calculateTariff(&items[i], parseItemStringDecimal(items[i].NetKG), parseItemStringDecimal(items[i].Pairs), parseItemStringDecimal(items[i].Places), usd, decimalValueOrZero(euro), &items[i].Warnings)
		items[i].Errors, items[i].Warnings = splitCalculationIssues(items[i].Warnings)
		if len(items[i].Errors) == 0 && len(items[i].Warnings) == 0 && items[i].GoodsCost != "" && items[i].CustomsValueKZT != "" && items[i].DutyKZT != "" && items[i].VATKZT != "" {
			items[i].Status = "calculated"
		} else {
			items[i].Status = "needs_review"
		}
	}
	// The customs fee is paid once per declaration. It is allocated for
	// display/expense totals even when the operator excludes it from the
	// customs-value base.
	allocateFee(items, fee, true)
	// The customs fee is paid once per declaration and its allocated share is
	// part of the VAT base. Keep the legacy FeeIncluded field for stored API
	// compatibility, but do not let a client silently omit this required part
	// of the preliminary formula.
	for i := range items {
		updateVATWithFee(&items[i])
	}
	var totalCost, totalDelivery, totalDeliveryKZT, customsValue, duty, antiDumping, vat, feeTotal decimal.Decimal
	calculated, needsReview, errorCount, warningCount, totalCount := 0, len(globalCalculationIssues) > 0, len(globalCalculationIssues), 0, 0
	blockingErrors := append([]string(nil), globalCalculationIssues...)
	for i := range items {
		item := &items[i]
		if item.Excluded {
			continue
		}
		totalCount++
		if item.CostUSD != "" {
			totalCost = totalCost.Add(mustDecimal(item.CostUSD))
		}
		if item.DeliveryUSD != "" {
			totalDelivery = totalDelivery.Add(mustDecimal(item.DeliveryUSD))
		}
		if item.DeliveryKZT != "" {
			totalDeliveryKZT = totalDeliveryKZT.Add(mustDecimal(item.DeliveryKZT))
		}
		if item.CustomsValueKZT != "" {
			customsValue = customsValue.Add(mustDecimal(item.CustomsValueKZT))
		}
		if item.DutyKZT != "" {
			duty = duty.Add(mustDecimal(item.DutyKZT))
		}
		if item.AntiDumpingKZT != "" {
			antiDumping = antiDumping.Add(mustDecimal(item.AntiDumpingKZT))
		}
		if item.VATKZT != "" {
			vat = vat.Add(mustDecimal(item.VATKZT))
		}
		if item.FeeKZT != "" {
			feeTotal = feeTotal.Add(mustDecimal(item.FeeKZT))
		}
		errorCount += len(item.Errors)
		warningCount += len(item.Warnings)
		for _, issue := range item.Errors {
			if len(blockingErrors) < 20 {
				blockingErrors = append(blockingErrors, issue)
			}
		}
		if item.Status == "calculated" {
			calculated++
		} else {
			needsReview = true
		}
	}
	unsupportedCount := 0
	for i := range items {
		if items[i].Excluded {
			continue
		}
		for _, issue := range append(append([]string{}, items[i].Errors...), items[i].Warnings...) {
			if unsupportedSchemeIssue(issue) {
				unsupportedCount++
				break
			}
		}
	}
	// The configured party amount is the source of truth for delivery totals.
	// Row shares are a derived allocation and can be unavailable while a
	// calculation is incomplete (for example, when every gross weight is
	// missing). Keep the entered party cost visible and count it once in cash
	// expenses even in that incomplete state; the blocking allocation issue
	// remains visible on the rows and in the summary.
	if deliveryTotalUSD != nil {
		totalDelivery = deliveryTotalUSD.Round(2)
		totalDeliveryKZT = totalDelivery.Mul(usd).Round(2)
	}
	customsPaymentsKZT := duty.Add(antiDumping).Add(vat).Add(feeTotal).Round(2)
	customsPaymentsText := customsPaymentsKZT.StringFixed(2)
	if unsupportedCount > 0 {
		customsPaymentsText = ""
		blockingErrors = append(blockingErrors, fmt.Sprintf("Итог растаможки не выводится: расчёт не поддерживается для %d позиций", unsupportedCount))
	}
	expensesKZT := customsPaymentsKZT.Add(totalDeliveryKZT).Round(2)
	expensesUSD := expensesKZT.Div(usd).Round(2)
	balance := decimal.Zero
	if cash != nil {
		balance = cash.Sub(expensesUSD).Round(2)
	}
	now := time.Now().UTC()
	result := CalculationResult{EstimateID: estimateID, AssessmentDate: date.Format("2006-01-02"), Origin: strings.TrimSpace(input.Origin), USDKZT: usd.String(), EURKZT: decimalPtrString(euro), Wagons: decimalPtrString(wagons), WagonTariffUSD: tariff.String(), DeliveryMode: deliveryMode, DeliveryTotalUSD: decimalPtrString(deliveryTotalUSD), PackageWeightKG: pkg.String(), CustomsFeeKZT: decimalPtrString(fee), MRP: strings.TrimSpace(input.MRP), FeeMultiplier: strings.TrimSpace(input.FeeMultiplier), FeeIncluded: true, CashUSD: decimalPtrString(cash), Items: items, UpdatedAt: now.Format(time.RFC3339), FormulaVersion: calculationFormulaVersion, RoundingPolicy: calculationRoundingPolicy, CalculatedAt: now.Format(time.RFC3339)}
	result.Totals = CalculationTotals{TotalCostUSD: totalCost.Round(2).StringFixed(2), TotalDeliveryUSD: totalDelivery.Round(2).StringFixed(2), DeliveryKZT: totalDeliveryKZT.Round(2).StringFixed(2), DeliveryInCustomsKZT: "0.00", DeliveryInExpensesKZT: totalDeliveryKZT.Round(2).StringFixed(2), CustomsValueKZT: customsValue.Round(2).StringFixed(2), DutyKZT: duty.Round(2).StringFixed(2), AntiDumpingKZT: antiDumping.Round(2).StringFixed(2), VATKZT: vat.Round(2).StringFixed(2), FeeKZT: feeTotal.Round(2).StringFixed(2), CustomsPaymentsKZT: customsPaymentsText, ExpensesKZT: expensesKZT.StringFixed(2), ExpensesUSD: expensesUSD.StringFixed(2), CashBalanceUSD: balance.StringFixed(2), CalculatedCount: calculated, TotalCount: totalCount, NeedsReview: needsReview, ErrorCount: errorCount, WarningCount: warningCount, BlockingErrors: blockingErrors}
	storedItems, _ := json.Marshal(items)
	storedTotals, _ := json.Marshal(result.Totals)
	storedInput, _ := json.Marshal(map[string]any{"assessmentDate": result.AssessmentDate, "origin": result.Origin, "usdKzt": result.USDKZT, "eurKzt": result.EURKZT, "deliveryMode": result.DeliveryMode, "deliveryTotalUsd": result.DeliveryTotalUSD, "wagons": result.Wagons, "wagonTariffUsd": result.WagonTariffUSD, "packageWeightKg": result.PackageWeightKG, "customsFeeKzt": result.CustomsFeeKZT, "mrp": result.MRP, "feeMultiplier": result.FeeMultiplier, "cashUsd": result.CashUSD, "items": items})
	model := repository.CustomsCalculation{EstimateID: estimateID, AssessmentDate: date, Origin: result.Origin, USDKZT: usd, EURKZT: euro, Wagons: wagons, WagonTariffUSD: tariff, DeliveryMode: deliveryMode, DeliveryTotalUSD: deliveryTotalUSD, PackageWeightKG: pkg, CustomsFeeKZT: fee, FeeIncluded: true, CashUSD: cash, Items: repository.JSON(storedItems), Totals: repository.JSON(storedTotals), FormulaVersion: calculationFormulaVersion, RoundingPolicy: calculationRoundingPolicy, InputSnapshot: repository.JSON(storedInput), CalculatedAt: &now, UpdatedBy: ownerID, CreatedBy: ownerID, UpdatedAt: now, CreatedAt: now}
	saved, err := s.repository.SaveCalculation(ctx, model)
	if err != nil {
		return CalculationResult{}, err
	}
	result.ID = saved.ID
	result.UpdatedAt = saved.UpdatedAt.UTC().Format(time.RFC3339)
	return result, nil
}

func resultFromModel(model repository.CustomsCalculation) (CalculationResult, error) {
	var items []CalculationItem
	if err := json.Unmarshal(model.Items, &items); err != nil {
		return CalculationResult{}, err
	}
	for i := range items {
		// Do not recalculate or round a stored snapshot while reading it. New
		// revisions already contain values at the selected display precision;
		// legacy snapshots remain legacy/unknown until explicitly recalculated.
		if items[i].Errors == nil {
			items[i].Errors = []string{}
		}
		if items[i].Warnings == nil {
			items[i].Warnings = []string{}
		}
		if items[i].ImageAssetIDs == nil {
			items[i].ImageAssetIDs = []int64{}
		}
	}
	calculatedAt := ""
	if model.CalculatedAt != nil {
		calculatedAt = model.CalculatedAt.UTC().Format(time.RFC3339)
	}
	formulaVersion, roundingPolicy := model.FormulaVersion, model.RoundingPolicy
	if formulaVersion == "" {
		formulaVersion = "legacy/unknown"
	}
	if roundingPolicy == "" {
		roundingPolicy = "legacy/unknown"
	}
	mrp, feeMultiplier := "", ""
	if len(model.InputSnapshot) > 2 && string(model.InputSnapshot) != "null" {
		var stored struct {
			MRP           string `json:"mrp"`
			FeeMultiplier string `json:"feeMultiplier"`
		}
		if json.Unmarshal(model.InputSnapshot, &stored) == nil {
			mrp, feeMultiplier = stored.MRP, stored.FeeMultiplier
		}
	}
	result := CalculationResult{ID: model.ID, EstimateID: model.EstimateID, AssessmentDate: model.AssessmentDate.Format("2006-01-02"), Origin: model.Origin, USDKZT: model.USDKZT.String(), EURKZT: decimalPtrString(model.EURKZT), Wagons: decimalPtrString(model.Wagons), WagonTariffUSD: model.WagonTariffUSD.String(), DeliveryMode: model.DeliveryMode, DeliveryTotalUSD: decimalPtrString(model.DeliveryTotalUSD), PackageWeightKG: model.PackageWeightKG.String(), CustomsFeeKZT: decimalPtrString(model.CustomsFeeKZT), MRP: mrp, FeeMultiplier: feeMultiplier, FeeIncluded: model.FeeIncluded, CashUSD: decimalPtrString(model.CashUSD), Items: items, UpdatedAt: model.UpdatedAt.UTC().Format(time.RFC3339), FormulaVersion: formulaVersion, RoundingPolicy: roundingPolicy, CalculatedAt: calculatedAt}
	if len(model.Totals) > 2 && string(model.Totals) != "{}" && string(model.Totals) != "null" {
		if err := json.Unmarshal(model.Totals, &result.Totals); err != nil {
			return CalculationResult{}, err
		}
		if result.Totals.BlockingErrors == nil {
			result.Totals.BlockingErrors = []string{}
		}
		if result.Totals.CustomsPaymentsKZT == "" && !totalsBlockUnsupported(result.Totals.BlockingErrors) {
			result.Totals.CustomsPaymentsKZT = addString(addString(addString(result.Totals.DutyKZT, result.Totals.AntiDumpingKZT), result.Totals.VATKZT), result.Totals.FeeKZT)
		}
		if result.Totals.DeliveryInExpensesKZT == "" {
			result.Totals.DeliveryInExpensesKZT = result.Totals.DeliveryKZT
		}
		if result.Totals.ExpensesKZT == "" {
			result.Totals.ExpensesKZT = addString(result.Totals.CustomsPaymentsKZT, result.Totals.DeliveryKZT)
		}
		return result, nil
	}
	// Legacy rows have no persisted totals. Reconstruct their historical
	// snapshot without applying the current rounding policy.
	var totalDeliveryKZT, includedDeliveryKZT decimal.Decimal
	for _, item := range items {
		if item.Excluded {
			continue
		}
		result.Totals.TotalCount++
		if item.Status == "calculated" {
			result.Totals.CalculatedCount++
		} else {
			result.Totals.NeedsReview = true
		}
		result.Totals.ErrorCount += len(item.Errors)
		result.Totals.WarningCount += len(item.Warnings)
		result.Totals.TotalCostUSD = addString(result.Totals.TotalCostUSD, item.CostUSD)
		result.Totals.TotalDeliveryUSD = addString(result.Totals.TotalDeliveryUSD, item.DeliveryUSD)
		result.Totals.CustomsValueKZT = addString(result.Totals.CustomsValueKZT, item.CustomsValueKZT)
		result.Totals.DutyKZT = addString(result.Totals.DutyKZT, item.DutyKZT)
		result.Totals.AntiDumpingKZT = addString(result.Totals.AntiDumpingKZT, item.AntiDumpingKZT)
		result.Totals.VATKZT = addString(result.Totals.VATKZT, item.VATKZT)
		result.Totals.FeeKZT = addString(result.Totals.FeeKZT, item.FeeKZT)
		if item.DeliveryKZT != "" {
			delivery := mustDecimal(item.DeliveryKZT)
			totalDeliveryKZT = totalDeliveryKZT.Add(delivery)
			if item.DeliveryIncluded {
				includedDeliveryKZT = includedDeliveryKZT.Add(delivery)
			}
		}
	}
	result.Totals.DeliveryKZT = totalDeliveryKZT.String()
	result.Totals.DeliveryInCustomsKZT = includedDeliveryKZT.String()
	result.Totals.DeliveryInExpensesKZT = totalDeliveryKZT.String()
	result.Totals.CustomsPaymentsKZT = addString(addString(addString(result.Totals.DutyKZT, result.Totals.AntiDumpingKZT), result.Totals.VATKZT), result.Totals.FeeKZT)
	result.Totals.ExpensesKZT = addString(result.Totals.CustomsPaymentsKZT, result.Totals.DeliveryKZT)
	result.Totals.ExpensesUSD = mustDivString(result.Totals.ExpensesKZT, model.USDKZT.String())
	if model.CashUSD != nil {
		result.Totals.CashBalanceUSD = mustSubString(model.CashUSD.String(), result.Totals.ExpensesUSD)
	}
	result.Totals.BlockingErrors = []string{}
	return result, nil
}

func itemFromProduct(p repository.ImportedProduct, imageAssetIDs []int64) CalculationItemInput {
	places := p.Places
	if places == nil {
		places = p.Quantity
	}
	item := CalculationItemInput{ProductID: p.ID, SupplierName: p.Name, StandardName: p.Name, PhotoURL: p.PhotoURL, ImageAssetIDs: append([]int64(nil), imageAssetIDs...), GrossKG: weightKG(p.GrossWeight, p.GrossWeightUnit), GrossWeightUnit: normalizedWeightUnit(p.GrossWeightUnit), NetKG: weightKG(p.NetWeight, p.NetWeightUnit), NetWeightUnit: normalizedWeightUnit(p.NetWeightUnit), NetSource: "", Pairs: decimalPtrString(p.Pairs), Places: decimalPtrString(places), PairsPerPlace: decimalPtrString(p.PairsPerPlace), Excluded: p.Excluded, ExclusionReason: p.ExclusionReason}
	if item.NetKG != "" {
		item.NetSource = "excel"
	}
	// The supplier price belongs to the product and is the live source for the
	// derived goods cost: pairs x price-per-pair. Keep the price here so a
	// price or quantity change propagates into the recalculation instead of
	// silently reusing a previously stored amount.
	if p.UnitPrice != nil && p.UnitPrice.IsPositive() && supportedCostCurrency(p.Currency) {
		item.PricePerPair = p.UnitPrice.String()
	}
	if currency := strings.ToUpper(strings.TrimSpace(p.Currency)); supportedCostCurrency(currency) && currency != "" {
		item.CostCurrency = currency
	}
	if len(p.TNVEDDirectorySnapshot) > 0 {
		_ = json.Unmarshal(p.TNVEDDirectorySnapshot, &item.DirectorySnapshot)
	}
	if item.DirectorySnapshot != nil {
		if v, ok := item.DirectorySnapshot["entryId"].(float64); ok {
			n := int64(v)
			item.DirectoryEntryID = &n
		}
		if v, ok := item.DirectorySnapshot["name"].(string); ok {
			item.StandardName = v
		}
		if v, ok := item.DirectorySnapshot["code"].(string); ok {
			item.Code = v
		}
		if v, ok := item.DirectorySnapshot["companyCoefficient"].(string); ok {
			item.Coefficient = v
		}
	}
	return item
}
