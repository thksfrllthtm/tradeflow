package goods

// Расчёт сметы товаров: снимок справочника по каждой строке и общий расчёт растаможки.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/antidumping"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/calculation"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/riskindicators"
	"github.com/shopspring/decimal"
)

type GoodsService struct{ repository *repository.Repository }

// NewGoodsService accepts the repository optionally to keep the service easy
// to construct in isolated tests. In the running server it is always supplied
// so directory selections are re-read and verified on the server.
func NewGoodsService(repositories ...*repository.Repository) *GoodsService {
	var repo *repository.Repository
	if len(repositories) > 0 {
		repo = repositories[0]
	}
	return &GoodsService{repository: repo}
}

func (s *GoodsService) Calculate(ctx context.Context, input GoodsCalculationInput) (GoodsCalculationResult, error) {
	_ = ctx
	if len(input.Items) > MaxRowsPerSheet {
		return GoodsCalculationResult{}, &service.ValidationError{Field: "items", Message: fmt.Sprintf("В расчёте не может быть больше %d строк", MaxRowsPerSheet)}
	}
	if strings.TrimSpace(input.AssessmentDate) == "" {
		return GoodsCalculationResult{}, &service.ValidationError{Field: "assessmentDate", Message: "Укажите дату расчёта"}
	}
	assessmentDate, err := time.Parse("2006-01-02", strings.TrimSpace(input.AssessmentDate))
	if err != nil {
		return GoodsCalculationResult{}, &service.ValidationError{Field: "assessmentDate", Message: "Укажите дату в формате ГГГГ-ММ-ДД"}
	}

	var usd decimal.Decimal
	if strings.TrimSpace(input.UsdKzt) != "" {
		usd, err = service.ParsePositive(input.UsdKzt, "usdKzt", false)
		if err != nil {
			return GoodsCalculationResult{}, err
		}
	} else {
		return GoodsCalculationResult{}, &service.ValidationError{Field: "usdKzt", Message: "Курс USD обязателен"}
	}

	euro, err := service.ParseOptionalPositive(input.EurKzt, "eurKzt")
	if err != nil {
		return GoodsCalculationResult{}, err
	}

	deliveryMode := strings.ToLower(strings.TrimSpace(input.DeliveryMode))
	// "none" — смета без доставки; пустой режим (старые черновики) считается так же.
	if deliveryMode != "" && deliveryMode != "none" && deliveryMode != "wagons" && deliveryMode != "total" {
		return GoodsCalculationResult{}, &service.ValidationError{Field: "deliveryMode", Message: "Выберите режим доставки"}
	}

	var wagons *decimal.Decimal
	var deliveryTotalUSD *decimal.Decimal
	tariff := decimal.NewFromInt(420)

	switch deliveryMode {
	case "wagons":
		if strings.TrimSpace(input.Wagons) == "" {
			return GoodsCalculationResult{}, &service.ValidationError{Field: "wagons", Message: "Укажите количество вагонов"}
		}
		wagons, err = service.CalcOptionalNonNegative(input.Wagons, "wagons")
		if err != nil {
			return GoodsCalculationResult{}, err
		}
		tariff, err = service.ParseNonNegativeDefault(input.WagonTariffUsd, tariff, "wagonTariffUsd")
		if err != nil {
			return GoodsCalculationResult{}, err
		}
		total := wagons.Mul(tariff).Round(2)
		deliveryTotalUSD = &total
	case "total":
		if strings.TrimSpace(input.DeliveryTotalUsd) == "" {
			return GoodsCalculationResult{}, &service.ValidationError{Field: "deliveryTotalUsd", Message: "Укажите общую доставку"}
		}
		total, parseErr := service.CalcOptionalNonNegative(input.DeliveryTotalUsd, "deliveryTotalUsd")
		if parseErr != nil {
			return GoodsCalculationResult{}, parseErr
		}
		deliveryTotalUSD = total
	}

	pkg, err := service.ParseNonNegativeDefault(input.PackageWeightKg, decimal.NewFromFloat(0.3), "packageWeightKg")
	if err != nil {
		return GoodsCalculationResult{}, err
	}

	fee, err := service.ResolveCustomsFee(input.CustomsFeeKzt, input.Mrp, input.FeeMultiplier)
	if err != nil {
		return GoodsCalculationResult{}, err
	}

	cash, err := service.CalcOptionalNonNegative(input.CashUsd, "cashUsd")
	if err != nil {
		return GoodsCalculationResult{}, err
	}

	sir, err := parseSIRSettings(input)
	if err != nil {
		return GoodsCalculationResult{}, err
	}

	// A browser may keep a stale snapshot in local storage.  In the running
	// server the directory entry id is the only trusted reference; names and
	// ИТС are reloaded from PostgreSQL for every calculation, and duty/VAT come
	// from the nomenclature by the row's code.  This keeps a user from changing
	// a coefficient or duty by editing the request body.
	snapshots := make([]map[string]any, len(input.Items))
	codes := []string{}
	for index, inItem := range input.Items {
		snapshot, snapshotErr := s.resolveDirectorySnapshot(ctx, inItem, input.GenericItems)
		if snapshotErr != nil {
			return GoodsCalculationResult{}, &service.ValidationError{Field: "items", Message: fmt.Sprintf("Строка %d: %s", index+1, snapshotErr.Error())}
		}
		snapshots[index] = snapshot
		if code := calculation.RowTNVEDCode(snapshot, inItem.Code); code != "" {
			codes = append(codes, code)
		}
	}
	if s.repository != nil {
		rates, err := s.loadNomenclatureRates(ctx, codes)
		if err != nil {
			return GoodsCalculationResult{}, err
		}
		if rates.AntiDumping, err = antidumping.NewService(s.repository).Measures(ctx); err != nil {
			return GoodsCalculationResult{}, err
		}
		rates.Origin = input.Origin
		for index, inItem := range input.Items {
			snapshots[index] = calculation.ApplyNomenclatureRates(snapshots[index], inItem.Code, rates, inItem.AntiDumpingChoice)
		}
	}
	euro = resolveEUR(ctx, euro, assessmentDate.Format("2006-01-02"), input.Items, snapshots)
	indicators, indicatorsLoaded := map[string]riskindicators.Indicator{}, false
	if sir.enabled && s.repository != nil {
		if indicators, indicatorsLoaded, err = riskindicators.NewService(s.repository).Lookup(ctx, riskindicators.SortedCodes(codes), sir.country); err != nil {
			return GoodsCalculationResult{}, err
		}
	}

	items := make([]calculation.CalculationItem, 0, len(input.Items))
	for index, inItem := range input.Items {
		snapshot := snapshots[index]
		coefficient := inItem.Coefficient
		if input.DirectoryCoefficientOnly {
			// The copied calculator has no manual ИТС field. Only a trusted
			// directory snapshot may provide the coefficient for that screen.
			coefficient = ""
		}
		itemIn := calculation.CalculationItemInput{
			SourceRow:             inItem.SourceRow,
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
		if sir.enabled {
			itemIn.SIR = sir.basis(calculation.RowTNVEDCode(snapshot, inItem.Code), indicators, indicatorsLoaded)
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
		items = append(items, calculation.CalculateItem(itemIn, usd, service.DecimalValueOrZero(euro), pkg))
	}

	calculation.AllocateDelivery(items, deliveryTotalUSD, usd)

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
			if strings.TrimSpace(item.GrossKG) == "" || !service.MustDecimal(item.GrossKG).IsPositive() {
				missingGross = true
			}
			if item.DeliveryUSD != "" {
				actualDelivery = actualDelivery.Add(service.MustDecimal(item.DeliveryUSD))
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
		items[i].Warnings = calculation.FilterRateWarnings(allIssues)
		calculation.CalculateTariff(&items[i], calculation.ParseItemStringDecimal(items[i].NetKG), calculation.ParseItemStringDecimal(items[i].Pairs), calculation.ParseItemStringDecimal(items[i].Places), usd, service.DecimalValueOrZero(euro), &items[i].Warnings)
		items[i].Errors, items[i].Warnings = calculation.SplitCalculationIssues(items[i].Warnings)
		if len(items[i].Errors) == 0 && len(items[i].Warnings) == 0 && (items[i].GoodsCost != "" || items[i].CustomsBasis == "sir") && items[i].CustomsValueKZT != "" && items[i].DutyKZT != "" && items[i].VATKZT != "" {
			items[i].Status = "calculated"
		} else {
			items[i].Status = "needs_review"
		}
	}

	calculation.AllocateFee(items, fee, true)

	for i := range items {
		calculation.UpdateVATWithFee(&items[i])
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
			totalCost = totalCost.Add(service.MustDecimal(item.CostUSD))
		}
		if item.DeliveryUSD != "" {
			totalDelivery = totalDelivery.Add(service.MustDecimal(item.DeliveryUSD))
		}
		if item.DeliveryKZT != "" {
			totalDeliveryKZT = totalDeliveryKZT.Add(service.MustDecimal(item.DeliveryKZT))
		}
		if item.CustomsValueKZT != "" {
			customsValue = customsValue.Add(service.MustDecimal(item.CustomsValueKZT))
		}
		if item.DutyKZT != "" {
			duty = duty.Add(service.MustDecimal(item.DutyKZT))
		}
		if item.AntiDumpingKZT != "" {
			antiDumping = antiDumping.Add(service.MustDecimal(item.AntiDumpingKZT))
		}
		if item.VATKZT != "" {
			vat = vat.Add(service.MustDecimal(item.VATKZT))
		}
		if item.FeeKZT != "" {
			feeTotal = feeTotal.Add(service.MustDecimal(item.FeeKZT))
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
			if calculation.UnsupportedSchemeIssue(issue) {
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

	result := GoodsCalculationResult{
		AssessmentDate:     assessmentDate.Format("2006-01-02"),
		Origin:             strings.TrimSpace(input.Origin),
		UsdKzt:             usd.String(),
		EurKzt:             service.DecimalPtrString(euro),
		Wagons:             service.DecimalPtrString(wagons),
		WagonTariffUsd:     tariff.String(),
		DeliveryMode:       deliveryMode,
		DeliveryTotalUsd:   service.DecimalPtrString(deliveryTotalUSD),
		PackageWeightKg:    pkg.String(),
		CustomsFeeKzt:      service.DecimalPtrString(fee),
		Mrp:                strings.TrimSpace(input.Mrp),
		FeeMultiplier:      strings.TrimSpace(input.FeeMultiplier),
		CashUsd:            service.DecimalPtrString(cash),
		CustomsValueMode:   sir.mode(),
		SirWeight:          sir.weight,
		SirDiscountPercent: sir.discount.String(),
		SirCountry:         sir.country,
		Items:              items,
	}

	result.Totals = calculation.CalculationTotals{
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

func (s *GoodsService) resolveDirectorySnapshot(ctx context.Context, item GoodsItemInput, genericItems bool) (map[string]any, error) {
	if s.repository == nil {
		// Unit tests can exercise the arithmetic without a database. Production
		// servers always provide a repository and take the trusted path below.
		return item.DirectorySnapshot, nil
	}
	if item.ProductDirectoryID == nil || *item.ProductDirectoryID < 1 {
		// Строка без карточки товара: код — из ячейки, ставки — из номенклатуры по коду.
		return nil, nil
	}
	product, err := s.repository.ProductDirectoryEntryByID(ctx, *item.ProductDirectoryID)
	if err != nil {
		if service.IsNotFound(err) {
			return nil, fmt.Errorf("карточка товара не найдена")
		}
		return nil, fmt.Errorf("не удалось загрузить карточку товара")
	}
	if product.ArchivedAt != nil {
		return nil, fmt.Errorf("выбранная карточка товара архивирована")
	}
	if strings.TrimSpace(product.Code) == "" {
		return nil, fmt.Errorf("у выбранного товара не указан код ТН ВЭД")
	}
	if !genericItems && strings.TrimSpace(product.Category) != "Перчатки" {
		return nil, fmt.Errorf("выбранная карточка относится не к категории «Перчатки»")
	}
	// Название, код и ИТС — из карточки товара; пошлина, НДС и антидемпинг — из номенклатуры
	// и реестра ЕЭК по коду (ApplyNomenclatureRates).
	snapshot := map[string]any{
		"category": product.Category, "name": strings.TrimSpace(product.Name), "code": product.Code,
		"verificationStatus": product.VerificationStatus, "selectedAt": time.Now().UTC().Format(time.RFC3339),
		// Отмечает карточку товара: только тогда строка берёт название товара вместо названия из Excel.
		"productDirectoryId": product.ID,
	}
	if strings.TrimSpace(product.NameZH) != "" {
		snapshot["nameZh"] = strings.TrimSpace(product.NameZH)
	}
	if product.CompanyCoefficient != nil {
		snapshot["companyCoefficient"] = service.DecimalText(product.CompanyCoefficient)
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

// loadNomenclatureRates читает номенклатуру для всех кодов сметы одним запросом.
func (s *GoodsService) loadNomenclatureRates(ctx context.Context, codes []string) (calculation.NomenclatureRates, error) {
	rates := calculation.NomenclatureRates{ByCode: map[string]repository.TNVEDNomenclatureCode{}}
	latest, err := s.repository.LatestTNVEDNomenclatureImport(ctx)
	if service.IsNotFound(err) {
		return rates, nil
	}
	if err != nil {
		return rates, err
	}
	rates.Loaded = true
	if latest.ActualDate != nil {
		rates.ActualDate = latest.ActualDate.Format("02.01.2006")
	}
	rows, err := s.repository.TNVEDNomenclatureByCodes(ctx, codes)
	if err != nil {
		return rates, err
	}
	for _, row := range rows {
		rates.ByCode[row.Code] = row
	}
	return rates, nil
}
