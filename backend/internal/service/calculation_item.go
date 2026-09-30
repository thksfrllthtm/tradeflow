package service

// Per-item customs math: customs value, duty, anti-dumping and VAT from the directory rate snapshot.

import (
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

func calculateItem(in CalculationItemInput, usd, euro, defaultPkg decimal.Decimal) CalculationItem {
	out := CalculationItem{CalculationItemInput: in, Status: "needs_review"}
	if in.Excluded {
		out.Status = "excluded"
		out.Warnings = []string{}
		return out
	}
	var warnings []string
	quantityLabel := "Количество пар"
	quantityPerPlaceLabel := "Пар в месте"
	priceLabel := "Цена за пару"
	if in.GenericItems {
		quantityLabel = "Количество товара"
		quantityPerPlaceLabel = "Количество товара в одном месте"
		priceLabel = "Цена за единицу"
	}
	pairs := parseItemDecimal(in.Pairs, quantityLabel, &warnings)
	places := parseItemDecimal(in.Places, "Количество мест", &warnings)
	ppp := parseItemDecimal(in.PairsPerPlace, quantityPerPlaceLabel, &warnings)
	if pairs != nil && !pairs.IsPositive() {
		warnings = append(warnings, quantityLabel+" должно быть положительным")
	}
	if places != nil && !places.IsPositive() {
		warnings = append(warnings, "Количество мест должно быть положительным")
	}
	if ppp != nil && !ppp.IsPositive() {
		warnings = append(warnings, quantityPerPlaceLabel+" должно быть положительным")
	}
	if pairs == nil && places != nil && ppp != nil && ppp.IsPositive() {
		pairs = decPtr(places.Mul(*ppp))
	}
	if places == nil && pairs != nil && ppp != nil && ppp.IsPositive() {
		places = decPtr(pairs.Div(*ppp))
	}
	if ppp == nil && pairs != nil && places != nil && places.IsPositive() {
		ppp = decPtr(pairs.Div(*places))
	}
	// Quantities represent physical counts. In particular, do not silently
	// round an inferred fractional number of places when pairs and pairs per
	// place do not divide evenly.
	if pairs != nil && !pairs.Truncate(0).Equal(*pairs) {
		warnings = append(warnings, quantityLabel+" должно быть целым числом")
	}
	if places != nil && !places.Truncate(0).Equal(*places) {
		warnings = append(warnings, "Количество мест должно быть целым числом; исправьте пары или пар в месте")
	}
	if ppp != nil && !ppp.Truncate(0).Equal(*ppp) {
		warnings = append(warnings, quantityPerPlaceLabel+" должно быть целым числом")
	}
	if pairs != nil && places != nil && ppp != nil && !pairs.Equal(places.Mul(*ppp)) {
		if in.GenericItems {
			warnings = append(warnings, "Количество товара не совпадает с местами × количества товара в одном месте")
		} else {
			warnings = append(warnings, "Количество пар не совпадает с местами × пар в месте")
		}
	}
	out.Pairs, out.Places, out.PairsPerPlace = decimalPtrString(pairs), decimalPtrString(places), decimalPtrString(ppp)
	gross := parseItemDecimal(in.GrossKG, "Общий вес", &warnings)
	net := parseItemDecimal(in.NetKG, "Вес нетто", &warnings)
	netCalculated := strings.EqualFold(strings.TrimSpace(in.NetSource), "calculated")
	netProvided := strings.TrimSpace(in.NetKG) != "" && !netCalculated
	netUnitInvalid := false
	if gross != nil && !validWeightUnit(in.GrossWeightUnit) {
		warnings = append(warnings, "Для брутто укажите единицу кг или г на этапе импорта")
		gross = nil
	} else if gross != nil && (strings.EqualFold(strings.TrimSpace(in.GrossWeightUnit), "g") || strings.EqualFold(strings.TrimSpace(in.GrossWeightUnit), "г")) {
		gross = decPtr(gross.Div(decimal.NewFromInt(1000)))
	}
	if net != nil && !netCalculated && !validWeightUnit(in.NetWeightUnit) {
		warnings = append(warnings, "Для нетто укажите единицу кг или г на этапе импорта")
		net = nil
		netUnitInvalid = true
	} else if net != nil && !netCalculated && (strings.EqualFold(strings.TrimSpace(in.NetWeightUnit), "g") || strings.EqualFold(strings.TrimSpace(in.NetWeightUnit), "г")) {
		net = decPtr(net.Div(decimal.NewFromInt(1000)))
	}
	pkg := parseItemDecimal(in.PackageWeightKG, "Вес упаковки", &warnings)
	// A calculated net weight is deliberately recomputed when its source
	// remains "calculated". This lets changes to gross weight, places or the
	// package-weight setting update only automatic values; Excel/manual net
	// values remain untouched.
	if netCalculated {
		net = nil
		netUnitInvalid = false
	}
	if gross != nil && gross.IsNegative() {
		warnings = append(warnings, "Общий вес не может быть отрицательным")
	}
	if pkg != nil && pkg.IsNegative() {
		warnings = append(warnings, "Вес упаковки не может быть отрицательным")
	}
	if pkg == nil {
		pkg = &defaultPkg
	}
	if net == nil && !netUnitInvalid && (!netProvided || netCalculated) && gross != nil && places != nil && places.IsPositive() {
		computed := gross.Sub(places.Mul(*pkg))
		if computed.IsNegative() {
			warnings = append(warnings, "Расчётный вес нетто отрицательный")
		} else {
			net = &computed
			out.NetSource = "calculated"
		}
	}
	if net != nil && net.IsNegative() {
		warnings = append(warnings, "Вес нетто не может быть отрицательным")
	}
	if net != nil && gross != nil && net.GreaterThan(*gross) {
		warnings = append(warnings, "Вес нетто не может быть больше брутто")
	}
	out.GrossKG, out.NetKG, out.PackageWeightKG = decimalPtrString(gross), decimalPtrString(net), decimalPtrString(pkg)
	if gross != nil {
		out.GrossWeightUnit = "kg"
	}
	if net != nil {
		out.NetWeightUnit = "kg"
	}
	delivery := parseItemDecimal(in.DeliveryUSD, "Доставка", &warnings)
	if delivery != nil && delivery.IsNegative() {
		warnings = append(warnings, "Доставка не может быть отрицательной")
	}
	if delivery != nil {
		roundedDelivery := delivery.Round(2)
		out.DeliveryUSD = roundedDelivery.StringFixed(2)
		out.DeliveryKZT = roundedDelivery.Mul(usd).Round(2).StringFixed(2)
	}
	costCurrency := strings.ToUpper(strings.TrimSpace(in.CostCurrency))
	if costCurrency == "" {
		costCurrency = "USD"
	}
	if costCurrency != "USD" && costCurrency != "EUR" && costCurrency != "KZT" {
		warnings = append(warnings, "Валюта стоимости не поддерживается: выберите USD, EUR или KZT")
		costCurrency = "USD"
	}
	var cost *decimal.Decimal
	var pricePerPair *decimal.Decimal
	explicitCost := parseItemDecimal(in.CostInput, "Стоимость товара", &warnings)
	if explicitCost != nil && explicitCost.IsNegative() {
		warnings = append(warnings, "Стоимость товара не может быть отрицательной")
		explicitCost = nil
	}
	if strings.TrimSpace(in.PricePerPair) != "" {
		pricePerPair = parseItemDecimal(in.PricePerPair, priceLabel, &warnings)
		if pricePerPair != nil && pricePerPair.IsNegative() {
			warnings = append(warnings, priceLabel+" не может быть отрицательной")
			pricePerPair = nil
		}
	}
	extra := parseItemDecimal(in.ExtraExpenses, "Дополнительные расходы", &warnings)
	if extra != nil && extra.IsNegative() {
		warnings = append(warnings, "Дополнительные расходы не могут быть отрицательными")
		extra = nil
	}
	out.extraDecimal = extra
	if pricePerPair != nil && pairs != nil && pairs.IsPositive() {
		calculated := pricePerPair.Round(4).Mul(*pairs)
		cost = &calculated
		if explicitCost != nil && !calculated.Round(2).Equal(explicitCost.Round(2)) {
			if in.GenericItems {
				warnings = append(warnings, "Стоимость пересчитана по количеству товара и цене за единицу")
			} else {
				warnings = append(warnings, "Стоимость пересчитана по количеству пар и цене за пару")
			}
		}
	} else if explicitCost != nil {
		cost = explicitCost
	} else {
		coeff := parseItemDecimal(in.Coefficient, "ИТС", &warnings)
		if coeff != nil && coeff.IsNegative() {
			warnings = append(warnings, "ИТС не может быть отрицательным")
		}
		if coeff != nil && net != nil && net.IsPositive() {
			// Legacy fallback for drafts created before explicit cost and
			// price-per-pair inputs existed.
			calculated := coeff.Mul(*net)
			cost = &calculated
		} else {
			if in.GenericItems {
				warnings = append(warnings, "Укажите цену за единицу товара")
			} else {
				warnings = append(warnings, "Для стоимости укажите стоимость товара либо количество пар и цену за пару")
			}
		}
	}
	if cost != nil {
		out.costDecimal = cost
		out.costCurrencyValue = costCurrency
		out.GoodsCost = cost.Round(2).StringFixed(2)
		out.GoodsCostCurrency = costCurrency
		if costCurrency == "USD" {
			out.CostUSD = out.GoodsCost
		}
		if pairs != nil && pairs.IsPositive() {
			// Four decimal places are the displayed/source precision for pair
			// price. Normalize the line cost from that same rounded price so the
			// two values always agree when multiplied back.
			normalizedPairPrice := cost.Div(*pairs).Round(4)
			normalizedCost := normalizedPairPrice.Mul(*pairs)
			cost = &normalizedCost
			out.costDecimal = cost
			out.GoodsCost = cost.Round(2).StringFixed(2)
			out.GoodsCostCurrency = costCurrency
			if costCurrency == "USD" {
				out.CostUSD = out.GoodsCost
			}
			out.PairPrice = normalizedPairPrice.StringFixed(4)
			if costCurrency == "USD" {
				out.PairPriceUSD = out.PairPrice
			}
		}
	}
	// Код есть, но его нет в номенклатуре: об этом скажет rateError, второе сообщение не нужно.
	if strings.TrimSpace(out.StandardName) == "" && stringValue(out.DirectorySnapshot, "rateError") == "" {
		if in.GenericItems {
			warnings = append(warnings, "Не выбрана карточка ТН ВЭД: укажите код ТН ВЭД в строке")
		} else {
			warnings = append(warnings, "Не выбрана карточка перчаток из справочника")
		}
	}
	calculateTariff(&out, net, pairs, places, usd, euro, &warnings)
	if warnings == nil {
		warnings = []string{}
	}
	out.Warnings = warnings
	out.Errors, out.Warnings = splitCalculationIssues(warnings)
	if len(out.Errors) == 0 && len(out.Warnings) == 0 && out.GoodsCost != "" && out.CustomsValueKZT != "" && out.DutyKZT != "" && out.VATKZT != "" {
		out.Status = "calculated"
	}
	return out
}

func calculateTariff(out *CalculationItem, net, pairs, places *decimal.Decimal, usd, euro decimal.Decimal, warnings *[]string) {
	_ = net
	_ = pairs
	_ = places
	snap := out.DirectorySnapshot
	if snap == nil {
		*warnings = append(*warnings, "Не выбраны ставки: укажите код ТН ВЭД")
		return
	}
	customs, customsKnown := customsValueForItem(out, usd, euro, warnings)
	if !customsKnown {
		out.CustomsValueKZT = ""
		return
	}
	out.CustomsValueKZT = customs.Round(2).StringFixed(2)
	// Ставки берутся из номенклатуры; кода в ней нет — пошлину и НДС не посчитать.
	if message := stringValue(snap, "rateError"); message != "" {
		*warnings = append(*warnings, message)
		out.DutyKZT, out.AntiDumpingKZT, out.VATKZT = "", "", ""
		return
	}
	duty, dutyKnown := percentageDuty(snap, customs.Round(2), warnings)
	if !dutyKnown {
		out.DutyKZT = ""
		out.AntiDumpingKZT = ""
		out.VATKZT = ""
		vat, _ := snap["vat"].(map[string]any)
		if stringValue(vat, "type") == "exempt" {
			out.VATKZT = "0.00"
			out.VATBaseKZT = customs.Round(2).StringFixed(2)
		} else {
			*warnings = append(*warnings, "НДС нельзя рассчитать до уточнения пошлины")
		}
		return
	}
	out.DutyKZT = duty.Round(2).StringFixed(2)
	antiDumping, antiDumpingKnown := antiDumpingDuty(snap, customs.Round(2), warnings)
	if !antiDumpingKnown {
		out.AntiDumpingKZT = ""
		out.VATKZT = ""
		return
	}
	out.AntiDumpingKZT = antiDumping.Round(2).StringFixed(2)
	vatBase := duty.Round(2).Add(antiDumping.Round(2))
	vat, vatKnown := percentageVAT(snap, customs.Round(2), vatBase, warnings)
	if !vatKnown {
		out.VATKZT = ""
		return
	}
	out.VATKZT = vat.Round(2).StringFixed(2)
	out.VATBaseKZT = customs.Round(2).Add(vatBase).StringFixed(2)
}

func antiDumpingDuty(snapshot map[string]any, customs decimal.Decimal, warnings *[]string) (decimal.Decimal, bool) {
	value, exists := snapshot["antiDumpingPercent"]
	if !exists || strings.TrimSpace(fmt.Sprint(value)) == "" {
		return decimal.Zero, true
	}
	percent, ok := decimalValueFromAny(value)
	if !ok || percent.IsNegative() {
		*warnings = append(*warnings, "Антидемпинговая ставка требует проверки")
		return decimal.Zero, false
	}
	return customs.Mul(percent).Div(decimal.NewFromInt(100)), true
}

func customsValueForItem(out *CalculationItem, usd, euro decimal.Decimal, warnings *[]string) (decimal.Decimal, bool) {
	if direct := strings.TrimSpace(out.CustomsValueDirectKZT); direct != "" {
		value := parseItemDecimal(direct, "Таможенная стоимость в тенге", warnings)
		if value == nil {
			return decimal.Zero, false
		}
		if value.IsNegative() {
			*warnings = append(*warnings, "Таможенная стоимость в тенге не может быть отрицательной")
			return decimal.Zero, false
		}
		return value.Round(2), true
	}
	cost := out.costDecimal
	currency := out.costCurrencyValue
	if cost == nil && strings.TrimSpace(out.CostUSD) != "" {
		parsed := mustDecimal(out.CostUSD)
		cost = &parsed
		currency = "USD"
	}
	if cost == nil && strings.TrimSpace(out.GoodsCost) != "" {
		parsed := mustDecimal(out.GoodsCost)
		cost = &parsed
		currency = out.GoodsCostCurrency
	}
	if cost == nil {
		if out.GenericItems {
			// The universal calculator already reports the missing manual price
			// above. Do not add a second, duplicate customs-value error for the
			// same row while the operator is still filling in the price.
			return decimal.Zero, false
		}
		*warnings = append(*warnings, "Для таможенной стоимости нужна стоимость товара")
		return decimal.Zero, false
	}
	if currency == "" {
		currency = "USD"
	}
	base := *cost
	if out.extraDecimal != nil {
		base = base.Add(*out.extraDecimal)
	}
	rate := decimal.NewFromInt(1)
	switch strings.ToUpper(currency) {
	case "USD":
		rate = usd
	case "EUR":
		if euro.IsZero() {
			*warnings = append(*warnings, "Для стоимости в EUR укажите курс EUR")
			return decimal.Zero, false
		}
		rate = euro
	case "KZT":
		rate = decimal.NewFromInt(1)
	default:
		*warnings = append(*warnings, "Валюта стоимости не поддерживается")
		return decimal.Zero, false
	}
	customs := base.Mul(rate)
	if out.deliveryShareExact != nil && !out.DeliveryAlreadyInCost {
		// The delivery share is a USD amount with full intermediate precision.
		// It enters the customs base once and only when the goods cost does not
		// already include the transport; a directly entered KZT customs value
		// bypasses this branch entirely.
		customs = customs.Add(out.deliveryShareExact.Mul(usd))
	}
	return customs.Round(2), true
}

func percentageDuty(snapshot map[string]any, customs decimal.Decimal, warnings *[]string) (decimal.Decimal, bool) {
	duty, _ := snapshot["duty"].(map[string]any)
	if stringValue(duty, "type") != "percentage" {
		*warnings = append(*warnings, "Пошлина поддерживается только процентная; выбранная схема не поддерживается")
		return decimal.Zero, false
	}
	percent, ok := decimalValue(duty, "percent")
	if !ok || percent.IsNegative() {
		*warnings = append(*warnings, "Ставка пошлины требует проверки")
		return decimal.Zero, false
	}
	return customs.Mul(percent).Div(decimal.NewFromInt(100)), true
}

func percentageVAT(snapshot map[string]any, customs, duty decimal.Decimal, warnings *[]string) (decimal.Decimal, bool) {
	vat, _ := snapshot["vat"].(map[string]any)
	switch stringValue(vat, "type") {
	case "exempt":
		return decimal.Zero, true
	case "percentage":
		percent, ok := decimalValue(vat, "percent")
		if !ok || percent.IsNegative() {
			*warnings = append(*warnings, "Ставка НДС требует проверки")
			return decimal.Zero, false
		}
		return customs.Add(duty).Mul(percent).Div(decimal.NewFromInt(100)), true
	default:
		*warnings = append(*warnings, "НДС поддерживается только процентный или освобождение; выбранная схема не поддерживается")
		return decimal.Zero, false
	}
}
