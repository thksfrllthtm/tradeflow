package exportcheck

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/shopspring/decimal"
)

// DeclarationItem — товар экспортной декларации, как его прочитал браузер из PDF.
type DeclarationItem struct {
	No       int    `json:"no"`
	Code     string `json:"code"`
	Name     string `json:"name"`
	Qty      string `json:"qty"`
	QtyUnit  string `json:"qtyUnit"`
	NetKg    string `json:"netKg"`
	Price    string `json:"price"`
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

// Declaration — одна экспортная декларация. Партию больше 50 товаров китайцы делят на несколько
// деклараций, в каждой нумерация товаров с 1, диапазон написан в поле 提运单号 («51-100»).
type Declaration struct {
	FileName    string            `json:"fileName"`
	BillNo      string            `json:"billNo"`
	Sender      string            `json:"sender"`
	Receiver    string            `json:"receiver"`
	Contract    string            `json:"contract"`
	Places      string            `json:"places"`
	GrossKg     string            `json:"grossKg"`
	NetKg       string            `json:"netKg"`
	TotalAmount string            `json:"totalAmount"` // 总价 из строки tips дополнительных страниц
	TotalQty    string            `json:"totalQty"`    // 成交数量合计 оттуда же: сумма количеств сделки
	Items       []DeclarationItem `json:"items"`
	Warnings    []string          `json:"warnings"`
}

// Check — одно сравнение: значение инвойса, значение декларации и вердикт.
// Status: ok, error, warn (расхождение, которое может быть законным) или skip (только показать).
type Check struct {
	Label       string `json:"label"`
	Invoice     string `json:"invoice"`
	Declaration string `json:"declaration"`
	Status      string `json:"status"`
	Note        string `json:"note,omitempty"`
}

type RowCheck struct {
	No              int     `json:"no"`
	Name            string  `json:"name"`
	CodeInvoice     string  `json:"codeInvoice"`
	CodeDeclaration string  `json:"codeDeclaration"`
	DeclarationNo   int     `json:"declarationNo"`   // 项号 в файле экспортной (в разбитой партии отличается от № инвойса)
	DeclarationName string  `json:"declarationName"` // китайское наименование
	ExcelRow        int     `json:"excelRow"`        // строка листа инвойса, чтобы открыть и проверить ячейку
	File            string  `json:"file"`
	Status          string  `json:"status"`
	Checks          []Check `json:"checks"`
}

type Report struct {
	InvoiceNumber    string     `json:"invoiceNumber"`
	InvoiceRows      int        `json:"invoiceRows"`
	DeclarationItems int        `json:"declarationItems"`
	Totals           []Check    `json:"totals"`
	Details          []Check    `json:"details"`
	Rows             []RowCheck `json:"rows"`
	Problems         []string   `json:"problems"` // ошибки вне строк: лишние товары, нестыковки внутри инвойса
	Notes            []string   `json:"notes"`    // предупреждения
	Errors           int        `json:"errors"`
	Warnings         int        `json:"warnings"`
}

const (
	statusOK    = "ok"
	statusError = "error"
	statusWarn  = "warn"
	statusSkip  = "skip"
)

var billRangePattern = regexp.MustCompile(`^(\d+)\s*-\s*(\d+)$`)

type placedItem struct {
	item DeclarationItem
	file string
}

// Compare сверяет инвойс с декларациями: итоги (места, брутто, нетто, сумма), реквизиты
// (получатель, контракт) и каждую строку (штуки, нетто, цена, сумма).
func Compare(invoice Invoice, declarations []Declaration) Report {
	report := Report{InvoiceNumber: invoice.Number, InvoiceRows: len(invoice.Rows), Totals: []Check{}, Details: []Check{}, Rows: []RowCheck{}, Problems: []string{}, Notes: []string{}}
	report.Notes = append(report.Notes, invoice.Warnings...)

	byNo := map[int]placedItem{}
	offset := 0
	for _, declaration := range declarations {
		start := offset
		if match := billRangePattern.FindStringSubmatch(strings.TrimSpace(declaration.BillNo)); match != nil {
			from, to := atoi(match[1]), atoi(match[2])
			start = from - 1
			if to-from+1 != len(declaration.Items) {
				report.Notes = append(report.Notes, fmt.Sprintf("%s: в поле 提运单号 диапазон %s, а товаров в файле %d", declaration.FileName, declaration.BillNo, len(declaration.Items)))
			}
		}
		if len(declaration.Items) > 50 {
			report.Notes = append(report.Notes, fmt.Sprintf("%s: %d товаров, в одной экспортной обычно не больше 50", declaration.FileName, len(declaration.Items)))
		}
		for _, warning := range declaration.Warnings {
			report.Notes = append(report.Notes, declaration.FileName+": "+warning)
		}
		for _, item := range declaration.Items {
			no := start + item.No
			if previous, duplicate := byNo[no]; duplicate {
				report.Problems = append(report.Problems, fmt.Sprintf("Товар № %d есть и в %s, и в %s", no, previous.file, declaration.FileName))
				continue
			}
			byNo[no] = placedItem{item: item, file: declaration.FileName}
		}
		offset = start + len(declaration.Items)
	}
	report.DeclarationItems = len(byNo)

	report.Totals = compareTotals(invoice, declarations, &report)
	report.Details = compareDetails(invoice, declarations)

	used := map[int]bool{}
	for _, row := range invoice.Rows {
		placed, ok := byNo[row.No]
		used[row.No] = true
		if !ok {
			report.Rows = append(report.Rows, RowCheck{No: row.No, Name: row.Name, CodeInvoice: row.Code, ExcelRow: row.SourceRow, Status: statusError, Checks: []Check{{Label: "Строка", Invoice: fmt.Sprintf("№ %d", row.No), Status: statusError, Note: "нет в экспортных"}}})
			continue
		}
		report.Rows = append(report.Rows, compareRow(row, placed))
		if problem := invoiceRowArithmetic(row); problem != "" {
			report.Problems = append(report.Problems, problem)
		}
	}
	extra := []int{}
	for no := range byNo {
		if !used[no] {
			extra = append(extra, no)
		}
	}
	sort.Ints(extra)
	for _, no := range extra {
		placed := byNo[no]
		report.Problems = append(report.Problems, fmt.Sprintf("В экспортной %s есть товар № %d (%s, %s %s, %s USD), которого нет в инвойсе", placed.file, no, placed.item.Name, placed.item.Qty, placed.item.QtyUnit, placed.item.Amount))
	}
	if len(byNo) != len(invoice.Rows) {
		report.Problems = append(report.Problems, fmt.Sprintf("В инвойсе %d строк, в экспортных %d товаров", len(invoice.Rows), len(byNo)))
	}

	for _, check := range append(append([]Check{}, report.Totals...), report.Details...) {
		report.count(check.Status)
	}
	for _, row := range report.Rows {
		for _, check := range row.Checks {
			report.count(check.Status)
		}
	}
	report.Errors += len(report.Problems)
	report.Warnings += len(report.Notes)
	return report
}

func (r *Report) count(status string) {
	switch status {
	case statusError:
		r.Errors++
	case statusWarn:
		r.Warnings++
	}
}

func atoi(text string) int {
	value := 0
	for _, digit := range text {
		value = value*10 + int(digit-'0')
	}
	return value
}

func parseDecimal(text string) (decimal.Decimal, bool) {
	value, err := decimal.NewFromString(normalizeNumber(text))
	return value, err == nil && text != ""
}

// sameDigits — одинаковый набор цифр: 1057.8 и 1075.8 (переставлены) или 10578 (потеряна запятая).
func sameDigits(a, b decimal.Decimal) bool {
	digits := func(value decimal.Decimal) string {
		runes := []rune(strings.TrimLeft(strings.ReplaceAll(value.String(), ".", ""), "0"))
		sort.Slice(runes, func(i, j int) bool { return runes[i] < runes[j] })
		return string(runes)
	}
	return digits(a) == digits(b)
}

// compareNumbers — сравнение двух чисел после округления по правилам графы декларации.
func compareNumbers(label, invoice, declaration string, round func(decimal.Decimal) decimal.Decimal) Check {
	check := Check{Label: label, Invoice: strings.TrimSpace(invoice), Declaration: strings.TrimSpace(declaration)}
	a, okA := parseDecimal(invoice)
	b, okB := parseDecimal(declaration)
	switch {
	case !okA && !okB:
		check.Status, check.Note = statusError, "нет значения ни в инвойсе, ни в экспортной"
	case !okA:
		check.Status, check.Note = statusError, "в инвойсе не число"
	case !okB:
		check.Status, check.Note = statusError, "в экспортной не число"
	case round(a).Equal(round(b)):
		check.Status = statusOK
	default:
		check.Status = statusError
		if sameDigits(a, b) {
			check.Note = "похоже, переставлены цифры или потеряна запятая"
		}
	}
	return check
}

func sumDecimals(values []string) (decimal.Decimal, []string) {
	total := decimal.Zero
	var bad []string
	for _, value := range values {
		parsed, ok := parseDecimal(value)
		if !ok {
			bad = append(bad, value)
			continue
		}
		total = total.Add(parsed)
	}
	return total, bad
}

func compareTotals(invoice Invoice, declarations []Declaration, report *Report) []Check {
	type total struct {
		label    string
		stated   string
		rows     func(InvoiceRow) string
		declared func(Declaration) string
		round    func(decimal.Decimal) decimal.Decimal
	}
	exact := func(value decimal.Decimal) decimal.Decimal { return value }
	itemsAmount := func(declaration Declaration) string {
		sum, _ := sumDecimals(mapItems(declaration.Items, func(item DeclarationItem) string { return item.Amount }))
		return sum.String()
	}
	totals := []total{
		{"Количество мест", invoice.Totals.Places, func(r InvoiceRow) string { return r.Places }, func(d Declaration) string { return d.Places }, exact},
		{"Вес брутто, кг", invoice.Totals.GrossKg, func(r InvoiceRow) string { return r.GrossKg }, func(d Declaration) string { return d.GrossKg }, service.RoundWeightKG},
		{"Вес нетто, кг", invoice.Totals.NetKg, func(r InvoiceRow) string { return r.NetKg }, func(d Declaration) string { return d.NetKg }, service.RoundWeightKG},
		{"Общая стоимость, USD", invoice.Totals.Amount, func(r InvoiceRow) string { return r.Amount }, itemsAmount, service.RoundMoney},
	}
	checks := make([]Check, 0, len(totals))
	for _, t := range totals {
		rowValues := make([]string, 0, len(invoice.Rows))
		for _, row := range invoice.Rows {
			rowValues = append(rowValues, t.rows(row))
		}
		rowSum, _ := sumDecimals(rowValues)
		invoiceValue := rowSum.String()
		if stated, ok := parseDecimal(t.stated); ok {
			invoiceValue = t.stated
			if !t.round(stated).Equal(t.round(rowSum)) {
				report.Problems = append(report.Problems, fmt.Sprintf("Инвойс, «%s»: в строке «итого» %s, а сумма по строкам %s — либо ошибка в инвойсе, либо ячейка прочитана неверно: откройте в Excel строки с расхождениями", t.label, t.stated, rowSum.String()))
			}
		}
		declValues := make([]string, 0, len(declarations))
		for _, declaration := range declarations {
			declValues = append(declValues, t.declared(declaration))
		}
		declSum, bad := sumDecimals(declValues)
		check := compareNumbers(t.label, invoiceValue, declSum.String(), t.round)
		if len(bad) > 0 {
			check.Status, check.Note = statusError, "в экспортной не найдено значение"
		}
		checks = append(checks, check)
	}
	// Самопроверка декларации: итоги 总价 и 成交数量合计 с дополнительных страниц должны сходиться
	// с суммой её строк, а в каждой строке количество × цена — с суммой. Расхождение здесь значит,
	// что PDF прочитан не полностью или ошиблась китайская сторона, и строкам верить нельзя.
	for _, declaration := range declarations {
		if stated, ok := parseDecimal(declaration.TotalAmount); ok {
			sum, _ := sumDecimals(mapItems(declaration.Items, func(item DeclarationItem) string { return item.Amount }))
			if !service.RoundMoney(stated).Equal(service.RoundMoney(sum)) {
				report.Problems = append(report.Problems, fmt.Sprintf("%s: общая стоимость в экспортной %s, а сумма по прочитанным строкам %s — экспортная прочитана не полностью или в ней ошибка", declaration.FileName, declaration.TotalAmount, sum.String()))
			}
		}
		if stated, ok := parseDecimal(declaration.TotalQty); ok {
			sum, _ := sumDecimals(mapItems(declaration.Items, func(item DeclarationItem) string { return item.Qty }))
			if !stated.Equal(sum) {
				report.Problems = append(report.Problems, fmt.Sprintf("%s: сумма количеств в экспортной %s, а по прочитанным строкам %s — экспортная прочитана не полностью или в ней ошибка", declaration.FileName, declaration.TotalQty, sum.String()))
			}
		}
		for _, item := range declaration.Items {
			qty, okQ := parseDecimal(item.Qty)
			price, okP := parseDecimal(item.Price)
			amount, okA := parseDecimal(item.Amount)
			if okQ && okP && okA && !service.RoundMoney(qty.Mul(price)).Equal(service.RoundMoney(amount)) {
				report.Notes = append(report.Notes, fmt.Sprintf("%s, товар %d: %s × %s = %s, а сумма в экспортной %s", declaration.FileName, item.No, qty.String(), price.String(), service.RoundMoney(qty.Mul(price)).StringFixed(2), item.Amount))
			}
		}
	}
	return checks
}

func mapItems(items []DeclarationItem, get func(DeclarationItem) string) []string {
	values := make([]string, 0, len(items))
	for _, item := range items {
		values = append(values, get(item))
	}
	return values
}

// normalizeText — для сравнения реквизитов: только буквы и цифры в нижнем регистре, без кавычек,
// «№», пробелов и знаков.
func normalizeText(text string) string {
	var builder strings.Builder
	for _, r := range strings.ToLower(text) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func distinctValues(declarations []Declaration, get func(Declaration) string) []string {
	var values []string
	for _, declaration := range declarations {
		value := strings.TrimSpace(get(declaration))
		if value == "" {
			continue
		}
		known := false
		for _, seen := range values {
			if normalizeText(seen) == normalizeText(value) {
				known = true
			}
		}
		if !known {
			values = append(values, value)
		}
	}
	return values
}

func compareDetails(invoice Invoice, declarations []Declaration) []Check {
	detail := func(label, invoiceValue string, get func(Declaration) string, equal func(a, b string) bool) Check {
		values := distinctValues(declarations, get)
		check := Check{Label: label, Invoice: strings.TrimSpace(invoiceValue), Declaration: strings.Join(values, "; ")}
		switch {
		case len(values) > 1:
			check.Status, check.Note = statusError, "в экспортных разные значения"
		case len(values) == 0:
			check.Status, check.Note = statusError, "в экспортной не найдено"
		case check.Invoice == "":
			check.Status, check.Note = statusWarn, "в инвойсе не найдено, сверьте вручную"
		case equal(check.Invoice, values[0]):
			check.Status = statusOK
		default:
			check.Status = statusError
		}
		return check
	}
	same := func(a, b string) bool { return normalizeText(a) == normalizeText(b) }
	// Поле контракта в декларации короткое: «№LT-24/03 от «03»» против «№LT-24/03 от «03» декабря 2024».
	prefix := func(a, b string) bool {
		x, y := normalizeText(a), normalizeText(b)
		if len(x) < 4 || len(y) < 4 {
			return x == y
		}
		return strings.HasPrefix(x, y) || strings.HasPrefix(y, x)
	}
	sender := Check{Label: "Отправитель", Invoice: strings.TrimSpace(invoice.Sender), Declaration: strings.Join(distinctValues(declarations, func(d Declaration) string { return d.Sender }), "; "), Status: statusSkip, Note: "в экспортной китайский отправитель-агент"}
	return []Check{
		detail("Получатель", invoice.Receiver, func(d Declaration) string { return d.Receiver }, same),
		detail("Номер контракта", invoice.Contract, func(d Declaration) string { return d.Contract }, prefix),
		sender,
	}
}

func isKgUnit(unit string) bool {
	unit = strings.ToLower(strings.TrimSpace(unit))
	return unit == "千克" || unit == "公斤" || unit == "kg" || unit == "кг"
}

func compareRow(row InvoiceRow, placed placedItem) RowCheck {
	item := placed.item
	exact := func(value decimal.Decimal) decimal.Decimal { return value }
	qty := compareNumbers("Количество, шт", row.Qty, item.Qty, exact)
	if qty.Status == statusError && isKgUnit(item.QtyUnit) {
		qty.Status, qty.Note = statusWarn, "в экспортной количество в килограммах: "+item.Qty+" кг"
	}
	net := compareNumbers("Нетто, кг", row.NetKg, item.NetKg, service.RoundWeightKG)
	if strings.TrimSpace(item.NetKg) == "" {
		net.Status, net.Note = statusWarn, "в экспортной нет веса по строке"
	}
	price := compareNumbers("Цена за штуку, USD", row.Price, item.Price, service.RoundUnitPrice)
	amount := compareNumbers("Сумма, USD", row.Amount, item.Amount, service.RoundMoney)
	if item.Currency != "" && item.Currency != "USD" && amount.Status == statusOK {
		amount.Status, amount.Note = statusWarn, "валюта в экспортной: "+item.Currency
	}
	// Суммы разошлись: штуки × цена подскажут, чья сумма верна.
	if amount.Status == statusError && amount.Note == "" {
		if hint := amountHint(row.Qty, row.Price, row.Amount, item.Amount); hint != "" {
			amount.Note = hint
		}
	}
	checks := []Check{qty, net, price, amount}
	status := statusOK
	for _, check := range checks {
		if check.Status == statusError || (check.Status == statusWarn && status == statusOK) {
			status = check.Status
		}
	}
	return RowCheck{No: row.No, Name: row.Name, CodeInvoice: row.Code, CodeDeclaration: item.Code, DeclarationNo: item.No, DeclarationName: item.Name, ExcelRow: row.SourceRow, File: placed.file, Status: status, Checks: checks}
}

// amountHint — чья сумма верна, если штуки × цена совпали с одной из сторон.
func amountHint(qtyText, priceText, invoiceAmount, declarationAmount string) string {
	qty, okQ := parseDecimal(qtyText)
	price, okP := parseDecimal(priceText)
	if !okQ || !okP {
		return ""
	}
	expected := service.RoundMoney(qty.Mul(price))
	if value, ok := parseDecimal(declarationAmount); ok && service.RoundMoney(value).Equal(expected) {
		return "штуки × цена = " + expected.StringFixed(2) + ", как в экспортной: ошибка в инвойсе"
	}
	if value, ok := parseDecimal(invoiceAmount); ok && service.RoundMoney(value).Equal(expected) {
		return "штуки × цена = " + expected.StringFixed(2) + ", как в инвойсе: ошибка в экспортной"
	}
	return ""
}

// invoiceRowArithmetic — ошибка в самом инвойсе: штуки × цена ≠ сумма.
func invoiceRowArithmetic(row InvoiceRow) string {
	qty, okQ := parseDecimal(row.Qty)
	price, okP := parseDecimal(row.Price)
	amount, okA := parseDecimal(row.Amount)
	if !okQ || !okP || !okA {
		return ""
	}
	expected := service.RoundMoney(qty.Mul(price))
	if expected.Equal(service.RoundMoney(amount)) {
		return ""
	}
	return fmt.Sprintf("Инвойс, строка %d: %s × %s = %s, а указано %s", row.No, qty.String(), price.String(), expected.StringFixed(2), row.Amount)
}
