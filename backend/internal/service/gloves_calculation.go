package service

// Gloves calculation: resolves directory snapshots per row and runs the shared customs calculation.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

func (s *GlovesService) Calculate(ctx context.Context, input GlovesCalculationInput) (GlovesCalculationResult, error) {
	_ = ctx
	if len(input.Items) > MaxRowsPerSheet {
		return GlovesCalculationResult{}, &ValidationError{Field: "items", Message: fmt.Sprintf("В расчёте не может быть больше %d строк", MaxRowsPerSheet)}
	}
	if strings.TrimSpace(input.AssessmentDate) == "" {
		return GlovesCalculationResult{}, &ValidationError{Field: "assessmentDate", Message: "Укажите дату расчёта"}
	}
	assessmentDate, err := time.Parse("2006-01-02", strings.TrimSpace(input.AssessmentDate))
	if err != nil {
		return GlovesCalculationResult{}, &ValidationError{Field: "assessmentDate", Message: "Укажите дату в формате ГГГГ-ММ-ДД"}
	}

	var usd decimal.Decimal
	if strings.TrimSpace(input.UsdKzt) != "" {
		usd, err = parsePositive(input.UsdKzt, "usdKzt", false)
		if err != nil {
			return GlovesCalculationResult{}, err
		}
	} else {
		return GlovesCalculationResult{}, &ValidationError{Field: "usdKzt", Message: "Курс USD обязателен"}
	}

	euro, err := parseOptionalPositive(input.EurKzt, "eurKzt")
	if err != nil {
		return GlovesCalculationResult{}, err
	}

	deliveryMode := strings.ToLower(strings.TrimSpace(input.DeliveryMode))
	// "none" — смета без доставки; пустой режим (старые черновики) считается так же.
	if deliveryMode != "" && deliveryMode != "none" && deliveryMode != "wagons" && deliveryMode != "total" {
		return GlovesCalculationResult{}, &ValidationError{Field: "deliveryMode", Message: "Выберите режим доставки"}
	}

	var wagons *decimal.Decimal
	var deliveryTotalUSD *decimal.Decimal
	tariff := decimal.NewFromInt(420)

	switch deliveryMode {
	case "wagons":
		if strings.TrimSpace(input.Wagons) == "" {
			return GlovesCalculationResult{}, &ValidationError{Field: "wagons", Message: "Укажите количество вагонов"}
		}
		wagons, err = calcOptionalNonNegative(input.Wagons, "wagons")
		if err != nil {
			return GlovesCalculationResult{}, err
		}
		tariff, err = parseNonNegativeDefault(input.WagonTariffUsd, tariff, "wagonTariffUsd")
		if err != nil {
			return GlovesCalculationResult{}, err
		}
		total := wagons.Mul(tariff).Round(2)
		deliveryTotalUSD = &total
	case "total":
		if strings.TrimSpace(input.DeliveryTotalUsd) == "" {
			return GlovesCalculationResult{}, &ValidationError{Field: "deliveryTotalUsd", Message: "Укажите общую доставку"}
		}
		total, parseErr := calcOptionalNonNegative(input.DeliveryTotalUsd, "deliveryTotalUsd")
		if parseErr != nil {
			return GlovesCalculationResult{}, parseErr
		}
		deliveryTotalUSD = total
	}

	pkg, err := parseNonNegativeDefault(input.PackageWeightKg, decimal.NewFromFloat(0.3), "packageWeightKg")
	if err != nil {
		return GlovesCalculationResult{}, err
	}

	fee, err := resolveCustomsFee(input.CustomsFeeKzt, input.Mrp, input.FeeMultiplier)
	if err != nil {
		return GlovesCalculationResult{}, err
	}

	cash, err := calcOptionalNonNegative(input.CashUsd, "cashUsd")
	if err != nil {
		return GlovesCalculationResult{}, err
	}

	items := make([]CalculationItem, 0, len(input.Items))
	for index, inItem := range input.Items {
		// A browser may keep a stale snapshot in local storage.  In the running
		// server the directory entry id is the only trusted reference; rates and
		// names are reloaded from PostgreSQL for every calculation.  This keeps a
		// user from changing a coefficient or duty by editing the request body.
		snapshot, snapshotErr := s.resolveDirectorySnapshot(ctx, inItem, input.GenericItems)
		if snapshotErr != nil {
			return GlovesCalculationResult{}, &ValidationError{Field: "items", Message: fmt.Sprintf("Строка %d: %s", index+1, snapshotErr.Error())}
		}
		coefficient := inItem.Coefficient
		if input.DirectoryCoefficientOnly {
			// The copied calculator has no manual ИТС field. Only a trusted
			// directory snapshot may provide the coefficient for that screen.
			coefficient = ""
		}
		itemIn := CalculationItemInput{
			SourceRow:             inItem.SourceRow,
			DirectoryEntryID:      inItem.DirectoryEntryID,
			StandardName:          strings.TrimSpace(inItem.StandardName),
			SupplierName:          strings.TrimSpace(inItem.SupplierName),
			Code:                  inItem.Code,
			Pairs:                 inItem.Pairs,
			Places:                inItem.Places,
			PairsPerPlace:         inItem.PairsPerPlace,
			GrossKG:               inItem.GrossKg,
			NetKG:                 inItem.NetKg,
			NetSource:             inItem.NetSource,
			PackageWeightKG:       inItem.PackageWeightKg,
			Coefficient:           coefficient,
			PricePerPair:          inItem.PricePerPair,
			CostInput:             inItem.CostInput,
			CostCurrency:          inItem.CostCurrency,
			ExtraExpenses:         inItem.ExtraExpenses,
			CustomsValueDirectKZT: inItem.CustomsValueDirectKZT,
			DeliveryUSD:           inItem.DeliveryUsd,
			DeliveryManual:        inItem.DeliveryManual,
			DeliveryAlreadyInCost: inItem.DeliveryAlreadyInCost,
			DeliveryIncluded:      inItem.DeliveryIncluded,
			Excluded:              inItem.Excluded,
			DirectorySnapshot:     snapshot,
			GenericItems:          input.GenericItems,
		}
		if snapshot != nil {
			itemIn.StandardName = directoryDisplayName(snapshot)
			if value, ok := snapshot["code"].(string); ok {
				itemIn.Code = value
			}
			// The directory value is the default. An operator may intentionally
			// override the coefficient for this one shipment row; an empty value
			// still falls back to the trusted directory snapshot. Duty/VAT remain
			// immutable directory values.
			if strings.TrimSpace(itemIn.Coefficient) == "" {
				if value, ok := snapshot["companyCoefficient"].(string); ok {
					itemIn.Coefficient = value
				}
			}
		}
		if itemIn.SupplierName == "" {
			itemIn.SupplierName = strings.TrimSpace(inItem.Name)
		}
		if itemIn.StandardName == "" {
			// Keep the supplier name separate. A directory card is still required
			// by calculateItem before a row can become calculated.
			itemIn.StandardName = ""
		}
		itemIn.GrossWeightUnit = strings.TrimSpace(inItem.GrossWeightUnit)
		if itemIn.GrossWeightUnit == "" && inItem.GrossKg != "" {
			// The dedicated screen labels its weight inputs in kilograms. Older
			// browser drafts predate the explicit unit field, so retain that
			// documented screen contract when upgrading such a draft.
			itemIn.GrossWeightUnit = "kg"
		}
		itemIn.NetWeightUnit = strings.TrimSpace(inItem.NetWeightUnit)
		if itemIn.NetWeightUnit == "" && inItem.NetKg != "" {
			itemIn.NetWeightUnit = "kg"
		}
		items = append(items, calculateItem(itemIn, usd, decimalValueOrZero(euro), pkg))
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
				items[i].Warnings = append(items[i].Warnings, "Распределение доставки: не для всех включённых строк указан положительный брутто")
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
			const noDistribution = "Распределение доставки: невозможно распределить"
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

	allocateFee(items, fee, true)

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

	result := GlovesCalculationResult{
		AssessmentDate:   assessmentDate.Format("2006-01-02"),
		Origin:           strings.TrimSpace(input.Origin),
		UsdKzt:           usd.String(),
		EurKzt:           decimalPtrString(euro),
		Wagons:           decimalPtrString(wagons),
		WagonTariffUsd:   tariff.String(),
		DeliveryMode:     deliveryMode,
		DeliveryTotalUsd: decimalPtrString(deliveryTotalUSD),
		PackageWeightKg:  pkg.String(),
		CustomsFeeKzt:    decimalPtrString(fee),
		Mrp:              strings.TrimSpace(input.Mrp),
		FeeMultiplier:    strings.TrimSpace(input.FeeMultiplier),
		CashUsd:          decimalPtrString(cash),
		Items:            items,
	}

	result.Totals = CalculationTotals{
		TotalCostUSD:          totalCost.Round(2).StringFixed(2),
		TotalDeliveryUSD:      totalDelivery.Round(2).StringFixed(2),
		DeliveryKZT:           totalDeliveryKZT.Round(2).StringFixed(2),
		DeliveryInCustomsKZT:  "0.00",
		DeliveryInExpensesKZT: totalDeliveryKZT.Round(2).StringFixed(2),
		CustomsValueKZT:       customsValue.Round(2).StringFixed(2),
		DutyKZT:               duty.Round(2).StringFixed(2),
		AntiDumpingKZT:        antiDumping.Round(2).StringFixed(2),
		VATKZT:                vat.Round(2).StringFixed(2),
		FeeKZT:                feeTotal.Round(2).StringFixed(2),
		CustomsPaymentsKZT:    customsPaymentsText,
		ExpensesKZT:           expensesKZT.StringFixed(2),
		ExpensesUSD:           expensesUSD.StringFixed(2),
		CashBalanceUSD:        balance.StringFixed(2),
		CalculatedCount:       calculated,
		TotalCount:            totalCount,
		NeedsReview:           needsReview,
		ErrorCount:            errorCount,
		WarningCount:          warningCount,
		BlockingErrors:        blockingErrors,
	}

	return result, nil
}

func (s *GlovesService) resolveDirectorySnapshot(ctx context.Context, item GlovesItemInput, genericItems bool) (map[string]any, error) {
	if s.repository == nil {
		// Unit tests can exercise the arithmetic without a database. Production
		// servers always provide a repository and take the trusted path below.
		return item.DirectorySnapshot, nil
	}
	if item.ProductDirectoryID != nil && *item.ProductDirectoryID > 0 {
		product, err := s.repository.ProductDirectoryEntryByID(ctx, *item.ProductDirectoryID)
		if err != nil {
			if IsNotFound(err) {
				return nil, fmt.Errorf("карточка товара не найдена")
			}
			return nil, fmt.Errorf("не удалось загрузить карточку товара")
		}
		if product.ArchivedAt != nil {
			return nil, fmt.Errorf("выбранная карточка товара архивирована")
		}
		if product.TNVEDDirectoryEntry == nil {
			return nil, fmt.Errorf("у выбранного товара не привязан код ТН ВЭД")
		}
		entry := *product.TNVEDDirectoryEntry
		if entry.ArchivedAt != nil {
			return nil, fmt.Errorf("карточка ТН ВЭД выбранного товара архивирована")
		}
		if !genericItems && strings.TrimSpace(entry.Category) != "Перчатки" {
			return nil, fmt.Errorf("выбранная карточка относится не к категории «Перчатки»")
		}
		snapshot := DirectorySnapshot(entry)
		// In the universal goods calculator ИТП belongs to the selected
		// product, not to its TN ВЭД code. Do not inherit a legacy coefficient
		// that may still be stored on the tariff card.
		if genericItems {
			delete(snapshot, "companyCoefficient")
		}
		snapshot["name"] = strings.TrimSpace(product.Name)
		if strings.TrimSpace(product.NameZH) != "" {
			snapshot["nameZh"] = strings.TrimSpace(product.NameZH)
		}
		if product.CompanyCoefficient != nil {
			snapshot["companyCoefficient"] = decimalString(product.CompanyCoefficient)
		}
		// Marks a product card (not just a TN ВЭД code): only then the row takes
		// the product name instead of the supplier name from Excel.
		snapshot["productDirectoryId"] = product.ID
		return snapshot, nil
	}
	if item.DirectoryEntryID == nil || *item.DirectoryEntryID < 1 {
		return nil, nil
	}
	entry, err := s.repository.TNVEDDirectoryEntryByID(ctx, *item.DirectoryEntryID)
	if err != nil {
		if IsNotFound(err) {
			if genericItems {
				return nil, fmt.Errorf("карточка ТН ВЭД не найдена")
			}
			return nil, fmt.Errorf("карточка перчаток не найдена")
		}
		if genericItems {
			return nil, fmt.Errorf("не удалось загрузить карточку ТН ВЭД")
		}
		return nil, fmt.Errorf("не удалось загрузить карточку перчаток")
	}
	if entry.ArchivedAt != nil {
		if genericItems {
			return nil, fmt.Errorf("выбранная карточка ТН ВЭД архивирована")
		}
		return nil, fmt.Errorf("выбранная карточка перчаток архивирована")
	}
	if !genericItems && strings.TrimSpace(entry.Category) != "Перчатки" {
		return nil, fmt.Errorf("выбранная карточка относится не к категории «Перчатки»")
	}
	// A code selected directly is only a TN ВЭД card. ИТП and the product
	// description are trusted only in the product branch above, where the row
	// contains an explicit ProductDirectoryID. The same TN ВЭД code may belong
	// to several different goods, so never restore product data from a stale
	// browser snapshot here.
	snapshot := DirectorySnapshot(entry)
	if genericItems {
		delete(snapshot, "companyCoefficient")
	}
	return snapshot, nil
}

func directoryDisplayName(snapshot map[string]any) string {
	russian, _ := snapshot["name"].(string)
	chinese, _ := snapshot["nameZh"].(string)
	russian = strings.TrimSpace(russian)
	chinese = strings.TrimSpace(chinese)
	if russian != "" && chinese != "" {
		return russian + " / " + chinese
	}
	if russian != "" {
		return russian
	}
	return chinese
}
