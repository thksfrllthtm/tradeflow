package goods

// XLSX export of a calculation: sheet «Расчёт» with every row and sheet «Итоги»
// with the parameters and totals.

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	_ "image/jpeg" // photo decoders for AddPictureFromBytes
	_ "image/png"
	"strings"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/calculation"
	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"
)

// Columns of sheet «Расчёт» (1-based).
const (
	exportPhotoColumn    = 2
	exportNameColumn     = 3
	exportCodeColumn     = 5
	exportFirstNumber    = 8 // «Антидемпинг, %» … «Доставка, KZT» are numeric
	exportLastNumber     = 28
	exportPriceColumn    = 15
	exportGrossColumn    = 12
	exportNetColumn      = 13
	exportCurrencyColumn = 17
	exportErrorsColumn   = 30
	exportWarningsColumn = 31
)

// Export writes the calculation shown on the result screen. The browser sends
// the rows with the prices currently on screen, so manual edits and a
// distributed budget are exported as they are.
func (s *GoodsService) Export(ctx context.Context, input GoodsCalculationInput) (*bytes.Buffer, error) {
	result, err := s.Calculate(ctx, input)
	if err != nil {
		return nil, err
	}
	book := excelize.NewFile()
	styles, err := newExportStyles(book)
	if err != nil {
		return nil, err
	}
	if err := writeExportRows(book, styles, input, result); err != nil {
		return nil, err
	}
	if err := writeExportSummary(book, styles, input, result); err != nil {
		return nil, err
	}
	book.SetActiveSheet(0)
	return book.WriteToBuffer()
}

func writeExportRows(book *excelize.File, styles exportStyles, input GoodsCalculationInput, result GoodsCalculationResult) error {
	sheet := "Расчёт"
	if err := book.SetSheetName(book.GetSheetName(0), sheet); err != nil {
		return err
	}
	nameHeader, quantityHeader, perPlaceHeader, unitPriceHeader := "Название поставщика", "Пары", "Пар в месте", "Цена пары"
	if input.GenericItems {
		nameHeader, quantityHeader, perPlaceHeader, unitPriceHeader = "Наименование", "Количество", "Количество в месте", "Цена за единицу"
	}
	headers := []string{"№", "Фото", nameHeader, "Название по карточке", "Код ТН ВЭД", "Пошлина (ставка)", "НДС (ставка)", "Антидемпинг, %",
		"Места", perPlaceHeader, quantityHeader, "Брутто, кг", "Нетто, кг", "ИТС, USD/кг", unitPriceHeader, "Стоимость", "Валюта", "Доп. расходы",
		"ТС напрямую, KZT", "Таможенная стоимость, KZT", "Пошлина, KZT", "Антидемпинг, KZT", "База НДС, KZT", "НДС, KZT", "Сбор, KZT", "Растаможка, KZT",
		"Доставка, USD", "Доставка, KZT", "Статус", "Ошибки", "Предупреждения"}
	for col, header := range headers {
		_ = book.SetCellValue(sheet, exportCell(col+1, 1), header)
	}
	_ = book.SetCellStyle(sheet, "A1", exportCell(len(headers), 1), styles.header)
	_ = book.SetRowHeight(sheet, 1, 32)
	widths := map[int]float64{1: 5, exportPhotoColumn: 12, exportNameColumn: 34, 4: 30, exportCodeColumn: 14, 6: 18, 7: 12, exportErrorsColumn: 40, exportWarningsColumn: 40}
	for col := 1; col <= len(headers); col++ {
		width, ok := widths[col]
		if !ok {
			width = 14
		}
		_ = book.SetColWidth(sheet, exportColumn(col), exportColumn(col), width)
	}
	_ = book.SetPanes(sheet, &excelize.Panes{Freeze: true, XSplit: exportNameColumn, YSplit: 1, TopLeftCell: "D2", ActivePane: "bottomRight"})

	var places, quantity, gross, net decimalSum
	for index, item := range result.Items {
		row := index + 2
		for col, value := range exportRowValues(input, index, item) {
			cell := exportCell(col+1, row)
			_ = book.SetCellValue(sheet, cell, value)
			_ = book.SetCellStyle(sheet, cell, cell, styles.forColumn(col+1))
		}
		if photo, extension, ok := exportPhoto(input, index); ok {
			_ = book.SetRowHeight(sheet, row, 60)
			_ = book.AddPictureFromBytes(sheet, exportCell(exportPhotoColumn, row), &excelize.Picture{
				Extension: extension, File: photo,
				Format: &excelize.GraphicOptions{AutoFit: true, LockAspectRatio: true, Positioning: "oneCell", OffsetX: 2, OffsetY: 2},
			})
		}
		if !item.Excluded {
			places.add(item.Places)
			quantity.add(item.Pairs)
			gross.add(item.GrossKG)
			net.add(item.NetKG)
		}
	}
	if len(result.Items) > 0 {
		// TN VED codes are identifiers: keep them numeric-looking without
		// Excel's green "number stored as text" marker.
		_ = book.AddIgnoredErrors(sheet, fmt.Sprintf("E2:E%d", len(result.Items)+1), excelize.IgnoredErrorsNumberStoredAsText)
	}

	totalRow := len(result.Items) + 2
	totals := result.Totals
	_ = book.SetCellValue(sheet, exportCell(exportNameColumn, totalRow), "Итого")
	for col, value := range map[int]string{
		9: places.String(), 11: quantity.String(), exportGrossColumn: gross.weight(), exportNetColumn: net.weight(), 16: totals.TotalCostUSD,
		20: totals.CustomsValueKZT, 21: totals.DutyKZT, 22: totals.AntiDumpingKZT, 24: totals.VATKZT, 25: totals.FeeKZT, 26: totals.CustomsPaymentsKZT,
		27: totals.TotalDeliveryUSD, 28: totals.DeliveryKZT,
	} {
		_ = book.SetCellValue(sheet, exportCell(col, totalRow), exportNumericCell(value))
	}
	if totals.TotalCostUSD != "" {
		_ = book.SetCellValue(sheet, exportCell(exportCurrencyColumn, totalRow), "USD")
	}
	_ = book.SetCellStyle(sheet, exportCell(1, totalRow), exportCell(len(headers), totalRow), styles.total)
	_ = book.SetCellStyle(sheet, exportCell(exportFirstNumber, totalRow), exportCell(exportLastNumber, totalRow), styles.totalNumber)
	_ = book.SetCellStyle(sheet, exportCell(exportGrossColumn, totalRow), exportCell(exportNetColumn, totalRow), styles.totalWeight)
	_ = book.AutoFilter(sheet, "A1:"+exportCell(len(headers), len(result.Items)+1), nil)
	return nil
}

func exportRowValues(input GoodsCalculationInput, index int, item calculation.CalculationItem) []any {
	displayName := calculationItemDisplayName(item)
	// Goods keep the supplier name from Excel unless a product card was chosen;
	// a TN ВЭД code description never replaces the product name.
	name := item.SupplierName
	if input.GenericItems && displayName != "" && (item.DirectorySnapshot["productDirectoryId"] != nil || strings.TrimSpace(name) == "") {
		name = displayName
	}
	coefficient := item.Coefficient
	if value, ok := item.DirectorySnapshot["companyCoefficient"].(string); ok && strings.TrimSpace(value) != "" {
		coefficient = value
	}
	antiDumping, _ := item.DirectorySnapshot["antiDumpingPercent"].(string)
	unitPrice := item.PairPrice
	if unitPrice == "" {
		unitPrice = item.PricePerPair
	}
	cost, currency := item.GoodsCost, item.GoodsCostCurrency
	if cost == "" {
		cost, currency = item.CostInput, item.CostCurrency
	}
	lineCustoms := sumDecimalStrings(item.DutyKZT, item.AntiDumpingKZT, item.VATKZT, item.FeeKZT)
	return []any{index + 1, "", name, displayName, exportNumericCell(item.Code),
		exportRateText(item.DirectorySnapshot, "duty"), exportRateText(item.DirectorySnapshot, "vat"), exportNumericCell(antiDumping),
		exportNumericCell(item.Places), exportNumericCell(item.PairsPerPlace), exportNumericCell(item.Pairs), exportNumericCell(item.GrossKG), exportNumericCell(item.NetKG),
		exportNumericCell(coefficient), exportNumericCell(unitPrice), exportNumericCell(cost), currency, exportNumericCell(item.ExtraExpenses),
		exportNumericCell(item.CustomsValueDirectKZT), exportNumericCell(item.CustomsValueKZT), exportNumericCell(item.DutyKZT), exportNumericCell(item.AntiDumpingKZT),
		exportNumericCell(item.VATBaseKZT), exportNumericCell(item.VATKZT), exportNumericCell(item.FeeKZT), exportNumericCell(lineCustoms),
		exportNumericCell(item.DeliveryUSD), exportNumericCell(item.DeliveryKZT), exportStatus(item), strings.Join(item.Errors, "; "), strings.Join(item.Warnings, "; ")}
}

// writeExportSummary fills sheet «Итоги»: calculation parameters and totals.
func writeExportSummary(book *excelize.File, styles exportStyles, input GoodsCalculationInput, result GoodsCalculationResult) error {
	sheet := "Итоги"
	if _, err := book.NewSheet(sheet); err != nil {
		return err
	}
	totals := result.Totals
	rate := exportDecimal(result.UsdKzt)
	usd := func(kzt string) any {
		value := exportDecimal(kzt)
		if value == nil || rate == nil || !rate.IsPositive() {
			return ""
		}
		return exportNumericCell(value.Div(*rate).Round(2).StringFixed(2))
	}
	kzt := func(usdValue string) any {
		value := exportDecimal(usdValue)
		if value == nil || rate == nil {
			return ""
		}
		return exportNumericCell(value.Mul(*rate).Round(2).StringFixed(2))
	}
	delivery := "Без доставки"
	switch result.DeliveryMode {
	case "wagons":
		delivery = fmt.Sprintf("По вагонам: %s × %s USD", formatExportNumber(result.Wagons), formatExportNumber(result.WagonTariffUsd))
	case "total":
		delivery = "Общая сумма за партию"
	}
	fee := "Фиксированный"
	if strings.TrimSpace(input.CustomsFeeKzt) == "" && result.Mrp != "" && result.FeeMultiplier != "" {
		fee = fmt.Sprintf("МРП %s × %s", formatExportNumber(result.Mrp), formatExportNumber(result.FeeMultiplier))
	}
	kind := "Смета перчаток"
	if input.GenericItems {
		kind = "Смета товаров"
	}
	rows := [][]any{
		{"Предварительный расчёт — не таможенная декларация"},
		{"Вид расчёта", kind},
		{"Дата расчёта", result.AssessmentDate},
		{"Страна происхождения", result.Origin},
		{"Курс USD → KZT", exportNumericCell(result.UsdKzt)},
		{"Курс EUR → KZT", exportNumericCell(result.EurKzt)},
		{"Доставка", delivery},
		{"Вес упаковки одного места, кг", exportNumericCell(result.PackageWeightKg)},
		{"Сбор", fee},
		{},
		{"Итоги", "KZT", "USD"},
		{"Стоимость товаров", kzt(totals.TotalCostUSD), exportNumericCell(totals.TotalCostUSD)},
		{"Таможенная стоимость", exportNumericCell(totals.CustomsValueKZT), usd(totals.CustomsValueKZT)},
		{"Пошлина", exportNumericCell(totals.DutyKZT), usd(totals.DutyKZT)},
		{"Антидемпинг", exportNumericCell(totals.AntiDumpingKZT), usd(totals.AntiDumpingKZT)},
		{"НДС", exportNumericCell(totals.VATKZT), usd(totals.VATKZT)},
		{"Сбор", exportNumericCell(totals.FeeKZT), usd(totals.FeeKZT)},
		{"Растаможка к оплате", exportNumericCell(totals.CustomsPaymentsKZT), usd(totals.CustomsPaymentsKZT)},
		{"Доставка", exportNumericCell(totals.DeliveryKZT), exportNumericCell(totals.TotalDeliveryUSD)},
		{"Всего расходов (растаможка + доставка)", exportNumericCell(totals.ExpensesKZT), exportNumericCell(totals.ExpensesUSD)},
	}
	const paymentRow = 18 // «Растаможка к оплате»
	if strings.TrimSpace(result.CashUsd) != "" {
		rows = append(rows, []any{"Касса", "", exportNumericCell(result.CashUsd)}, []any{"Остаток кассы", "", exportNumericCell(totals.CashBalanceUSD)})
	}
	rows = append(rows, []any{}, []any{"Рассчитано строк", fmt.Sprintf("%d из %d", totals.CalculatedCount, totals.TotalCount)})
	if len(totals.BlockingErrors) > 0 {
		rows = append(rows, []any{"Ошибки", strings.Join(totals.BlockingErrors, "; ")})
	}
	for index, values := range rows {
		for col, value := range values {
			cell := exportCell(col+1, index+1)
			_ = book.SetCellValue(sheet, cell, value)
			if _, numeric := value.(float64); numeric {
				// Параметры (курсы, вес упаковки) — как есть, суммы — с двумя знаками.
				style := styles.count
				if index >= 11 {
					style = styles.number
				}
				_ = book.SetCellStyle(sheet, cell, cell, style)
			}
		}
	}
	_ = book.SetCellStyle(sheet, "A1", "C1", styles.header)
	_ = book.SetCellStyle(sheet, "A11", "C11", styles.header)
	_ = book.SetCellStyle(sheet, exportCell(1, paymentRow), exportCell(3, paymentRow), styles.totalNumber)
	_ = book.SetColWidth(sheet, "A", "A", 44)
	_ = book.SetColWidth(sheet, "B", "C", 20)
	return nil
}

type exportStyles struct{ header, text, count, number, price, code, total, totalNumber, weight, totalWeight int }

func (s exportStyles) forColumn(col int) int {
	switch {
	case col == exportCodeColumn:
		return s.code
	case col == exportPriceColumn:
		return s.price
	case col == exportGrossColumn || col == exportNetColumn:
		return s.weight
	case col >= exportFirstNumber && col < exportPriceColumn:
		return s.count // проценты, места, количества, веса, ИТС — без лишних нулей
	case col >= exportFirstNumber && col <= exportLastNumber && col != exportCurrencyColumn:
		return s.number
	default:
		return s.text
	}
}

func newExportStyles(book *excelize.File) (exportStyles, error) {
	numberFormat, countFormat, priceFormat, codeFormat := "#,##0.00", "#,##0.###", "#,##0.0000", "0"
	// Вес — как в гр.35 и 38 декларации: три знака, меньше одного грамма — шесть.
	weightFormat := "[=0]0.000;[<0.001]0.000000;#,##0.000"
	border := []excelize.Border{{Type: "left", Color: "D9E2DC", Style: 1}, {Type: "right", Color: "D9E2DC", Style: 1}, {Type: "top", Color: "D9E2DC", Style: 1}, {Type: "bottom", Color: "D9E2DC", Style: 1}}
	top := &excelize.Alignment{Vertical: "top", WrapText: true}
	totalFill := excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"E8F3EC"}}
	definitions := []*excelize.Style{
		{Font: &excelize.Font{Bold: true, Color: "FFFFFF"}, Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"1F5C3A"}}, Alignment: &excelize.Alignment{Vertical: "center", WrapText: true}, Border: border},
		{Alignment: top, Border: border},
		{CustomNumFmt: &countFormat, Alignment: &excelize.Alignment{Vertical: "top"}, Border: border},
		{CustomNumFmt: &numberFormat, Alignment: &excelize.Alignment{Vertical: "top"}, Border: border},
		{CustomNumFmt: &priceFormat, Alignment: &excelize.Alignment{Vertical: "top"}, Border: border},
		{CustomNumFmt: &codeFormat, Alignment: &excelize.Alignment{Vertical: "top"}, Border: border},
		{Font: &excelize.Font{Bold: true}, Fill: totalFill, Border: border},
		{Font: &excelize.Font{Bold: true}, Fill: totalFill, CustomNumFmt: &numberFormat, Border: border},
		{CustomNumFmt: &weightFormat, Alignment: &excelize.Alignment{Vertical: "top"}, Border: border},
		{Font: &excelize.Font{Bold: true}, Fill: totalFill, CustomNumFmt: &weightFormat, Border: border},
	}
	ids := make([]int, len(definitions))
	for index, definition := range definitions {
		id, err := book.NewStyle(definition)
		if err != nil {
			return exportStyles{}, err
		}
		ids[index] = id
	}
	return exportStyles{header: ids[0], text: ids[1], count: ids[2], number: ids[3], price: ids[4], code: ids[5], total: ids[6], totalNumber: ids[7], weight: ids[8], totalWeight: ids[9]}, nil
}

func exportColumn(col int) string {
	name, _ := excelize.ColumnNumberToName(col)
	return name
}

func exportCell(col, row int) string {
	cell, _ := excelize.CoordinatesToCellName(col, row)
	return cell
}

// exportRateText describes a tariff rate from the directory snapshot like the directory table does.
func exportRateText(snapshot map[string]any, key string) string {
	rate, _ := snapshot[key].(map[string]any)
	if rate == nil {
		return ""
	}
	percent, amount := service.StringValue(rate, "percent"), service.StringValue(rate, "amount")
	unit := service.StringValue(rate, "currency") + "/" + service.StringValue(rate, "unit")
	switch service.StringValue(rate, "type") {
	case "percentage":
		if percent != "" {
			return percent + "%"
		}
	case "specific":
		if amount != "" {
			return amount + " " + unit
		}
	case "combined_minimum":
		if percent != "" && amount != "" {
			return percent + "%, но не менее " + amount + " " + unit
		}
	case "exempt":
		return "Освобождение"
	}
	return ""
}

func exportStatus(item calculation.CalculationItem) string {
	switch {
	case item.Excluded:
		return "Исключена"
	case item.Status == "calculated":
		return "Рассчитано"
	case len(item.Errors) > 0:
		return "Ошибка"
	default:
		return "Проверить"
	}
}

// exportPhoto decodes the first photo of a row sent as a data URL (JPG or PNG).
func exportPhoto(input GoodsCalculationInput, index int) ([]byte, string, bool) {
	if index >= len(input.Items) {
		return nil, "", false
	}
	value := strings.TrimSpace(input.Items[index].PhotoURL)
	if value == "" && len(input.Items[index].PhotoURLs) > 0 {
		value = strings.TrimSpace(input.Items[index].PhotoURLs[0])
	}
	comma := strings.IndexByte(value, ',')
	if comma < 0 {
		return nil, "", false
	}
	header := strings.ToLower(value[:comma])
	extension := ""
	switch {
	case strings.HasPrefix(header, "data:image/jpeg;base64"), strings.HasPrefix(header, "data:image/jpg;base64"):
		extension = ".jpg"
	case strings.HasPrefix(header, "data:image/png;base64"):
		extension = ".png"
	default:
		return nil, "", false
	}
	photo, err := base64.StdEncoding.DecodeString(value[comma+1:])
	if err != nil || len(photo) == 0 || len(photo) > MaxImageBytes {
		return nil, "", false
	}
	return photo, extension, true
}

func exportDecimal(value string) *decimal.Decimal {
	normalized := strings.NewReplacer(" ", "", " ", "", ",", ".").Replace(strings.TrimSpace(value))
	parsed, err := decimal.NewFromString(normalized)
	if err != nil {
		return nil
	}
	return &parsed
}

func sumDecimalStrings(values ...string) string {
	var sum decimalSum
	for _, value := range values {
		sum.add(value)
	}
	return sum.String()
}

// decimalSum adds decimal strings, ignoring empty or invalid ones.
type decimalSum struct {
	value decimal.Decimal
	any   bool
}

func (s *decimalSum) add(value string) {
	if parsed := exportDecimal(value); parsed != nil {
		s.value = s.value.Add(*parsed)
		s.any = true
	}
}

// weight — итог веса по правилу гр.35 и 38: до трёх знаков, меньше грамма — до шести.
func (s decimalSum) weight() string {
	if !s.any {
		return ""
	}
	return service.WeightKGText(s.value)
}

func (s decimalSum) String() string {
	if !s.any {
		return ""
	}
	return s.value.String()
}
