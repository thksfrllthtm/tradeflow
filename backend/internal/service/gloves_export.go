package service

// Export of a gloves calculation result to an XLSX workbook.

import (
	"bytes"
	"context"
	"fmt"
	"strconv"
	"strings"
	"unicode"

	"github.com/xuri/excelize/v2"
)

func formatExportNumber(value string) string {
	normalized := strings.TrimSpace(strings.ReplaceAll(value, ",", "."))
	if normalized == "" {
		return value
	}

	sign := ""
	if normalized[0] == '-' || normalized[0] == '+' {
		sign = normalized[:1]
		normalized = normalized[1:]
	}
	parts := strings.Split(normalized, ".")
	if len(parts) > 2 || parts[0] == "" {
		return value
	}
	for _, char := range parts[0] {
		if !unicode.IsDigit(char) {
			return value
		}
	}
	if len(parts) == 2 {
		for _, char := range parts[1] {
			if !unicode.IsDigit(char) {
				return value
			}
		}
	}

	integer := parts[0]
	for index := len(integer) - 3; index > 0; index -= 3 {
		integer = integer[:index] + " " + integer[index:]
	}
	if len(parts) == 2 {
		return sign + integer + "," + parts[1]
	}
	return sign + integer
}

// exportNumericCell returns a real numeric value for Excel instead of a
// pre-formatted string. Excel stores the numeric value independently from the
// display separator and applies the user's locale (comma in Russian Excel).
func exportNumericCell(value string) any {
	normalized := strings.NewReplacer(" ", "", "\u00a0", "", "\u202f", "").Replace(strings.TrimSpace(value))
	normalized = strings.ReplaceAll(normalized, ",", ".")
	if normalized == "" {
		return ""
	}
	number, err := strconv.ParseFloat(normalized, 64)
	if err != nil {
		return value
	}
	return number
}

func calculationItemDisplayName(item CalculationItem) string {
	if item.DirectorySnapshot != nil {
		if name := directoryDisplayName(item.DirectorySnapshot); strings.TrimSpace(name) != "" {
			return name
		}
	}
	return strings.TrimSpace(item.StandardName)
}

func (s *GlovesService) Export(ctx context.Context, input GlovesCalculationInput) (*bytes.Buffer, error) {
	result, err := s.Calculate(ctx, input)
	if err != nil {
		return nil, err
	}

	book := excelize.NewFile()
	sheet := book.GetSheetName(0)
	supplierHeader := "Название поставщика"
	quantityHeader, quantityPerPlaceHeader, unitPriceHeader, kind := "Пары", "Пар в месте", "Цена пары", "GLOVES"
	if input.GenericItems {
		supplierHeader = "Наименование поставщика"
		quantityHeader, quantityPerPlaceHeader, unitPriceHeader, kind = "Количество товара", "Количество товара в месте", "Цена за единицу", "GOODS"
	}
	headers := []string{"Предварительный расчёт", supplierHeader, "Название по карточке", "Код ТН ВЭД", quantityHeader, "Места", quantityPerPlaceHeader, "Брутто, кг", "Нетто, кг", unitPriceHeader, "Стоимость", "Валюта", "Доп. расходы", "ТС напрямую, KZT", "Таможенная стоимость, KZT", "Пошлина, KZT", "Антидемпинг, KZT", "Сбор, KZT", "База НДС, KZT", "НДС, KZT", "Доставка, USD", "Доставка, KZT", "Статус", "Ошибки", "Предупреждения"}

	for col, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(col+1, 1)
		_ = book.SetCellValue(sheet, cell, header)
	}

	// Keep numeric columns numeric in the generated workbook. The format uses
	// Excel's locale-aware separators, so Russian Excel displays 0.082 as 0,082
	// while still allowing calculations and sorting.
	numericFormat := "#,##0.############"
	numericStyle, styleErr := book.NewStyle(&excelize.Style{CustomNumFmt: &numericFormat})
	if styleErr != nil {
		return nil, styleErr
	}
	codeFormat := "0"
	codeStyle, styleErr := book.NewStyle(&excelize.Style{CustomNumFmt: &codeFormat})
	if styleErr != nil {
		return nil, styleErr
	}
	numericColumns := map[int]struct{}{
		3: {}, 4: {}, 5: {}, 6: {}, 7: {}, 8: {}, 9: {}, 10: {}, 12: {},
		13: {}, 14: {}, 15: {}, 16: {}, 17: {}, 18: {}, 19: {}, 20: {}, 21: {},
	}

	for row, item := range result.Items {
		displayName := calculationItemDisplayName(item)
		exportSupplierName := item.SupplierName
		if input.GenericItems && displayName != "" {
			exportSupplierName = displayName
		}
		values := []any{kind, exportSupplierName, displayName, exportNumericCell(item.Code), exportNumericCell(item.Pairs), exportNumericCell(item.Places), exportNumericCell(item.PairsPerPlace), exportNumericCell(item.GrossKG), exportNumericCell(item.NetKG), exportNumericCell(item.PricePerPair), exportNumericCell(item.CostInput), item.CostCurrency, exportNumericCell(item.ExtraExpenses), exportNumericCell(item.CustomsValueDirectKZT), exportNumericCell(item.CustomsValueKZT), exportNumericCell(item.DutyKZT), exportNumericCell(item.AntiDumpingKZT), exportNumericCell(item.FeeKZT), exportNumericCell(item.VATBaseKZT), exportNumericCell(item.VATKZT), exportNumericCell(item.DeliveryUSD), exportNumericCell(item.DeliveryKZT), item.Status, strings.Join(item.Errors, "; "), strings.Join(item.Warnings, "; ")}
		for col, value := range values {
			cell, _ := excelize.CoordinatesToCellName(col+1, row+2)
			_ = book.SetCellValue(sheet, cell, value)
			if _, ok := numericColumns[col]; ok {
				if col == 3 {
					_ = book.SetCellStyle(sheet, cell, cell, codeStyle)
				} else {
					_ = book.SetCellStyle(sheet, cell, cell, numericStyle)
				}
			}
		}
	}
	if len(result.Items) > 0 {
		// TN VED codes are identifiers. Keep them numeric-looking without
		// allowing Excel's green "number stored as text" marker to appear.
		codeRange := fmt.Sprintf("D2:D%d", len(result.Items)+1)
		_ = book.AddIgnoredErrors(sheet, codeRange, excelize.IgnoredErrorsNumberStoredAsText)
	}

	metaRow := len(result.Items) + 3
	_ = book.SetCellValue(sheet, fmt.Sprintf("A%d", metaRow), "Итоги")
	_ = book.SetCellValue(sheet, fmt.Sprintf("B%d", metaRow), fmt.Sprintf("Дата расчёта: %s", result.AssessmentDate))
	_ = book.SetCellValue(sheet, fmt.Sprintf("C%d", metaRow), fmt.Sprintf("Происхождение: %s", result.Origin))
	_ = book.SetCellValue(sheet, fmt.Sprintf("D%d", metaRow), fmt.Sprintf("Растаможка (сбор + пошлина + НДС): %s KZT", formatExportNumber(result.Totals.CustomsPaymentsKZT)))
	_ = book.SetCellValue(sheet, fmt.Sprintf("E%d", metaRow), fmt.Sprintf("Доставка: %s USD / %s KZT", formatExportNumber(result.Totals.TotalDeliveryUSD), formatExportNumber(result.Totals.DeliveryKZT)))
	_ = book.SetCellValue(sheet, fmt.Sprintf("F%d", metaRow), fmt.Sprintf("Всего из кассы: %s KZT", formatExportNumber(result.Totals.ExpensesKZT)))
	_ = book.SetCellValue(sheet, fmt.Sprintf("G%d", metaRow), fmt.Sprintf("Таможенная стоимость: %s KZT", formatExportNumber(result.Totals.CustomsValueKZT)))
	_ = book.SetCellValue(sheet, fmt.Sprintf("H%d", metaRow), fmt.Sprintf("Статус: рассчитано %d из %d", result.Totals.CalculatedCount, result.Totals.TotalCount))
	_ = book.SetCellValue(sheet, fmt.Sprintf("I%d", metaRow), fmt.Sprintf("Пошлина: %s KZT; НДС: %s KZT; Сбор: %s KZT", formatExportNumber(result.Totals.DutyKZT), formatExportNumber(result.Totals.VATKZT), formatExportNumber(result.Totals.FeeKZT)))

	return book.WriteToBuffer()
}
