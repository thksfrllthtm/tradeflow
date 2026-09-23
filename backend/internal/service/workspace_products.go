package service

// Turning workspace rows into imported products (units, weights, per-row validation messages).

import (
	"fmt"
	"sort"
	"strings"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/shopspring/decimal"
)

func productsFromWorkspace(workspace Workspace, item repository.EstimateImport, estimateCurrency string) ([]repository.ImportedProduct, error) {
	if strings.TrimSpace(workspace.Batch.GrossWeight) != "" {
		batchWeight, warning := parseOptionalDecimal(workspace.Batch.GrossWeight, "Общий вес партии", true)
		if warning != "" || batchWeight == nil {
			return nil, &ValidationError{Field: "batch.grossWeight", Message: warningOrDefault(warning, "Общий вес партии должен быть положительным")}
		}
		batchUnit := normalizeWeightUnit(workspace.Batch.GrossWeightUnit)
		if batchUnit != "kg" && batchUnit != "g" {
			return nil, &ValidationError{Field: "batch.grossWeightUnit", Message: "Для общего веса укажите кг или г"}
		}
	}
	columnsByField := map[string]WorkspaceColumn{}
	for _, column := range workspace.Columns {
		if column.Field != "unassigned" && column.Field != "unused" && column.Field != "additional" {
			columnsByField[column.Field] = column
		}
	}
	if _, ok := columnsByField["name"]; !ok {
		return nil, &ValidationError{Field: "mapping", Message: "Назначьте колонку «Название товара»"}
	}
	// Вес не является обязательным. Если весовая колонка не назначена,
	// сохраняем нейтральную базу unknown, чтобы не нарушать CHECK-ограничение
	// imported_products.weight_basis.
	grossColumn := columnsByField["grossWeight"]
	netColumn := columnsByField["netWeight"]
	grossUnit, grossBasis := grossColumn.WeightUnit, grossColumn.WeightBasis
	netUnit, netBasis := netColumn.WeightUnit, netColumn.WeightBasis
	if grossBasis == "" {
		grossBasis = "unknown"
	}
	if netBasis == "" {
		netBasis = "unknown"
	}
	products := make([]repository.ImportedProduct, 0)
	for _, row := range workspace.Rows {
		if !row.Included || (row.SourceRow > 0 && row.SourceRow < workspace.DataStartRow) {
			continue
		}
		value := func(field string) string {
			column, ok := columnsByField[field]
			if !ok {
				return ""
			}
			return strings.TrimSpace(row.Values[column.ID])
		}
		name := value("name")
		if name == "" {
			return nil, rowError(row, "Название товара обязательно")
		}
		quantity, quantityWarning := parseOptionalDecimal(value("quantity"), "Количество", true)
		if quantityWarning != "" {
			return nil, rowError(row, quantityWarning)
		}
		pairs, pairsWarning := parseOptionalDecimal(value("pairs"), "Количество пар", false)
		if pairsWarning != "" {
			return nil, rowError(row, pairsWarning)
		}
		places, placesWarning := parseOptionalDecimal(value("places"), "Количество мест", false)
		if placesWarning != "" {
			return nil, rowError(row, placesWarning)
		}
		pairsPerPlace, pairsPerPlaceWarning := parseOptionalDecimal(value("pairsPerPlace"), "Пар в месте", false)
		if pairsPerPlaceWarning != "" {
			return nil, rowError(row, pairsPerPlaceWarning)
		}
		unitPrice, priceWarning := parseOptionalDecimal(value("unitPrice"), "Цена за единицу", false)
		if priceWarning != "" {
			return nil, rowError(row, priceWarning)
		}
		lineTotal, totalWarning := parseOptionalDecimal(value("lineTotal"), "Сумма позиции", false)
		if totalWarning != "" {
			return nil, rowError(row, totalWarning)
		}
		if unitPrice == nil && lineTotal != nil && quantity != nil && quantity.IsPositive() {
			derived := lineTotal.Div(*quantity)
			unitPrice = &derived
		}
		if unitPrice != nil && lineTotal != nil && quantity != nil {
			expected := quantity.Mul(*unitPrice)
			if expected.Sub(*lineTotal).Abs().GreaterThan(decimal.RequireFromString(lineTotalTolerance)) {
				return nil, rowError(row, "Сумма позиции не совпадает с количеством и ценой (допуск 0,01)")
			}
		}
		currency := strings.ToUpper(value("currency"))
		if currency == "" {
			currency = strings.ToUpper(strings.TrimSpace(estimateCurrency))
		}
		if (unitPrice != nil || lineTotal != nil) && (len(currency) != 3 || !isLatinUppercase(currency)) {
			return nil, rowError(row, "Укажите корректную валюту строки или валюту сметы")
		}
		gross, grossWarning := parseOptionalDecimal(value("grossWeight"), "Вес брутто", false)
		if grossWarning != "" {
			return nil, rowError(row, grossWarning)
		}
		net, netWarning := parseOptionalDecimal(value("netWeight"), "Вес нетто", false)
		if netWarning != "" {
			return nil, rowError(row, netWarning)
		}
		for _, field := range []string{"grossWeight", "netWeight"} {
			if value(field) == "" {
				continue
			}
			column := columnsByField[field]
			if column.WeightUnit == "" || column.WeightBasis == "" {
				return nil, rowError(row, "Для указанного веса выберите кг/г и строка/единица")
			}
		}
		additional := map[string]string{}
		for _, column := range workspace.Columns {
			if column.Field == "additional" {
				additional[column.ExtraName] = strings.TrimSpace(row.Values[column.ID])
			}
		}
		raw := map[string]map[string]string{}
		for _, column := range workspace.Columns {
			raw[column.ID] = map[string]string{"sourceName": column.SourceName, "original": row.OriginalValues[column.ID], "value": row.Values[column.ID], "formula": row.Formulas[column.ID]}
		}
		product := repository.ImportedProduct{EstimateID: item.EstimateID, SourceImportID: item.ID, SourceSheet: workspace.Sheet, SourceRow: row.SourceRow, SourceValues: encodeJSON(raw), Name: name, Translation: value("translation"), SKU: value("sku"), Quantity: quantity, QuantityUnit: value("quantityUnit"), Pairs: pairs, Places: places, PairsPerPlace: pairsPerPlace, UnitPrice: unitPrice, LineTotal: lineTotal, Currency: currency, GrossWeight: gross, GrossWeightUnit: grossUnit, GrossWeightBasis: grossBasis, NetWeight: net, NetWeightUnit: netUnit, NetWeightBasis: netBasis, Weight: gross, Material: value("material"), Characteristics: value("characteristics"), PhotoURL: value("photo"), AdditionalFields: encodeJSON(additional), PriceBasis: workspace.PriceBasis, PriceUnitLabel: workspace.PriceUnitLabel, WeightBasis: grossBasis, ImportStatus: "imported", Warnings: encodeJSON(row.Warnings), Excluded: false, ExclusionReason: ""}
		if len(row.Warnings) > 0 {
			product.ImportStatus = "needs_correction"
		}
		products = append(products, product)
	}
	if len(products) == 0 {
		return nil, &ValidationError{Field: "rows", Message: "Выберите хотя бы одну строку товара"}
	}
	sort.SliceStable(products, func(i, j int) bool { return products[i].SourceRow < products[j].SourceRow })
	return products, nil
}

func normalizeWeightUnit(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "кг", "килограмм", "килограммы":
		return "kg"
	case "г", "грамм", "граммы":
		return "g"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func warningOrDefault(warning, fallback string) string {
	if warning != "" {
		return warning
	}
	return fallback
}

func rowError(row WorkspaceRow, message string) error {
	label := fmt.Sprintf("Строка %d", row.SourceRow)
	if row.SourceRow == 0 {
		label = "Новая строка"
	}
	return &ValidationError{Field: "row:" + row.ID, Message: label + ": " + message}
}
