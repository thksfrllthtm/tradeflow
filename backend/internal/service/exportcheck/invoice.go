// Package exportcheck сверяет наш инвойс с китайской экспортной декларацией: инвойс читается из
// Excel здесь, декларацию читает браузер из PDF (frontend/src/features/exportCheck/declarationPdf.ts)
// и присылает уже разобранной. Сравнение — Compare.
package exportcheck

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/goods"
	"github.com/xuri/excelize/v2"
)

// MaxInvoiceBytes — инвойс без фото, больше не бывает.
const MaxInvoiceBytes = 32 << 20

type InvoiceRow struct {
	No        int    `json:"no"`
	SourceRow int    `json:"sourceRow"`
	Code      string `json:"code"`
	Name      string `json:"name"`
	Places    string `json:"places"`
	Qty       string `json:"qty"`
	GrossKg   string `json:"grossKg"`
	NetKg     string `json:"netKg"`
	Price     string `json:"price"`
	Amount    string `json:"amount"`
}

// InvoiceTotals — строка «итого» инвойса; пусто, если её нет.
type InvoiceTotals struct {
	Places  string `json:"places"`
	GrossKg string `json:"grossKg"`
	NetKg   string `json:"netKg"`
	Amount  string `json:"amount"`
}

type Invoice struct {
	Number   string        `json:"number"`
	Contract string        `json:"contract"`
	Receiver string        `json:"receiver"`
	Sender   string        `json:"sender"`
	Wagon    string        `json:"wagon"`
	Rows     []InvoiceRow  `json:"rows"`
	Totals   InvoiceTotals `json:"totals"`
	Warnings []string      `json:"warnings,omitempty"`
}

type column int

const (
	colNo column = iota
	colCode
	colName
	colPlaces
	colQty
	colGross
	colNet
	colPrice
	colAmount
)

func containsAny(text string, parts ...string) bool {
	for _, part := range parts {
		if strings.Contains(text, part) {
			return true
		}
	}
	return false
}

// classifyHeader — какая колонка инвойса за заголовком. Порядок важен: «Цена за штуку» — цена, а не
// штуки; «Количество мест» — места, а не количество; «Общая цена» — сумма, а не цена.
func classifyHeader(header string) (column, bool) {
	h := strings.ToLower(strings.Join(strings.Fields(header), " "))
	switch {
	case h == "":
		return 0, false
	case containsAny(h, "брутто", "毛重"):
		return colGross, true
	case containsAny(h, "нетто", "净重"):
		return colNet, true
	case containsAny(h, "мест", "件数", "箱数"):
		return colPlaces, true
	case containsAny(h, "общая цена", "стоимость", "сумма", "总价", "货值", "total"):
		return colAmount, true
	case containsAny(h, "цена", "单价", "每对价格", "price"):
		return colPrice, true
	case containsAny(h, "штук", "пар", "кол-во", "количество", "数量", "qty", "quantity"):
		return colQty, true
	case containsAny(h, "hc code", "hs code", "тн вэд", "тнвэд", "商品代码", "海关编码", "税号", "код"):
		return colCode, true
	case containsAny(h, "наименование", "品名", "товар", "name"):
		return colName, true
	case h == "№" || h == "n" || h == "no" || h == "№ п/п":
		return colNo, true
	}
	return 0, false
}

// ParseInvoice читает инвойс из Excel (.xlsx или .xls): таблицу товаров, строку «итого» и реквизиты
// над таблицей (номер инвойса, контракт, получатель, отправитель, вагон).
func ParseInvoice(file io.ReaderAt, size int64, filename string) (Invoice, error) {
	var book *excelize.File
	var err error
	if strings.HasSuffix(strings.ToLower(filename), ".xls") {
		converted, convErr := goods.XLSToXLSX(file, size)
		if convErr != nil {
			return Invoice{}, &service.ValidationError{Field: "invoice", Message: "Не удалось прочитать файл .xls: " + convErr.Error()}
		}
		book, err = excelize.OpenReader(bytes.NewReader(converted))
	} else {
		book, err = excelize.OpenReader(io.NewSectionReader(file, 0, size))
	}
	if err != nil {
		return Invoice{}, &service.ValidationError{Field: "invoice", Message: "Не удалось открыть файл Excel: " + err.Error()}
	}
	defer book.Close()

	for _, sheet := range book.GetSheetList() {
		rows, err := book.GetRows(sheet)
		if err != nil {
			return Invoice{}, fmt.Errorf("read sheet %q: %w", sheet, err)
		}
		if invoice, ok := parseInvoiceRows(rows); ok {
			return invoice, nil
		}
	}
	return Invoice{}, &service.ValidationError{Field: "invoice", Message: "В файле не найдена таблица инвойса: нужны колонки «Количество штук», «Вес нетто», «Цена» и «Общая цена»"}
}

func cellAt(row []string, index int) string {
	if index < 0 || index >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[index])
}

// normalizeNumber — число из ячейки Excel как строка для decimal: без пробелов и разделителей тысяч
// («1 234,56», «1,234.56», «11,568.42»), десятичная запятая → точка.
func normalizeNumber(text string) string {
	text = strings.NewReplacer(" ", "", " ", "", " ", "", "'", "").Replace(strings.TrimSpace(text))
	if strings.Contains(text, ",") && strings.Contains(text, ".") {
		// Оба знака: последний — десятичный, остальные — тысячи.
		if strings.LastIndex(text, ",") > strings.LastIndex(text, ".") {
			text = strings.ReplaceAll(text, ".", "")
		} else {
			text = strings.ReplaceAll(text, ",", "")
		}
	}
	return strings.ReplaceAll(text, ",", ".")
}

func parseInvoiceRows(rows [][]string) (Invoice, bool) {
	headerRow, columns := findInvoiceHeader(rows)
	if headerRow < 0 {
		return Invoice{}, false
	}
	invoice := Invoice{}
	get := func(row []string, col column) string {
		index, ok := columns[col]
		if !ok {
			return ""
		}
		return cellAt(row, index)
	}
	number := func(row []string, col column) string { return normalizeNumber(get(row, col)) }

	for index := headerRow + 1; index < len(rows); index++ {
		row := rows[index]
		lead := strings.ToLower(strings.Join([]string{cellAt(row, 0), cellAt(row, 1), cellAt(row, 2)}, " "))
		if containsAny(lead, "итого", "total", "合计") {
			invoice.Totals = InvoiceTotals{Places: number(row, colPlaces), GrossKg: number(row, colGross), NetKg: number(row, colNet), Amount: number(row, colAmount)}
			break
		}
		if get(row, colQty) == "" && get(row, colAmount) == "" {
			continue
		}
		no, err := strconv.Atoi(strings.TrimSuffix(get(row, colNo), "."))
		if err != nil {
			no = len(invoice.Rows) + 1
		}
		invoice.Rows = append(invoice.Rows, InvoiceRow{
			No: no, SourceRow: index + 1, Code: strings.ReplaceAll(get(row, colCode), " ", ""), Name: get(row, colName),
			Places: number(row, colPlaces), Qty: number(row, colQty), GrossKg: number(row, colGross), NetKg: number(row, colNet),
			Price: number(row, colPrice), Amount: number(row, colAmount),
		})
	}
	if len(invoice.Rows) == 0 {
		return Invoice{}, false
	}
	if invoice.Totals == (InvoiceTotals{}) {
		invoice.Warnings = append(invoice.Warnings, "В инвойсе нет строки «итого»: места, брутто, нетто и сумма посчитаны по строкам")
	}
	invoice.Number, invoice.Contract, invoice.Receiver, invoice.Sender, invoice.Wagon = invoiceDetails(rows[:headerRow])
	return invoice, true
}

// findInvoiceHeader — строка заголовков: первая, где нашлись количество, нетто, цена и сумма.
func findInvoiceHeader(rows [][]string) (int, map[column]int) {
	for index, row := range rows {
		columns := map[column]int{}
		for cellIndex, cell := range row {
			if col, ok := classifyHeader(cell); ok {
				if _, seen := columns[col]; !seen {
					columns[col] = cellIndex
				}
			}
		}
		_, qty := columns[colQty]
		_, net := columns[colNet]
		_, price := columns[colPrice]
		_, amount := columns[colAmount]
		if qty && net && price && amount {
			return index, columns
		}
	}
	return -1, nil
}

// invoiceDetails — реквизиты над таблицей. Подпись и значение либо в одной ячейке через двоеточие
// («Получатель: ТОО «357»»), либо значение в следующей непустой ячейке строки.
func invoiceDetails(rows [][]string) (number, contract, receiver, sender, wagon string) {
	var seller string
	value := func(row []string, index int) string {
		cell := row[index]
		if at := strings.IndexAny(cell, ":："); at >= 0 {
			if rest := strings.TrimSpace(cell[at+1:]); rest != "" && !strings.HasPrefix(rest, ":") {
				return rest
			}
		}
		for next := index + 1; next < len(row); next++ {
			if text := strings.TrimSpace(row[next]); text != "" {
				return text
			}
		}
		return ""
	}
	pick := func(target *string, row []string, index int) {
		if *target == "" {
			*target = value(row, index)
		}
	}
	for _, row := range rows {
		for index, cell := range row {
			label := strings.ToLower(cell)
			if at := strings.IndexAny(label, ":："); at >= 0 {
				label = label[:at]
			}
			if strings.TrimSpace(label) == "" || containsAny(label, "адрес", "地址", "e-mail", "почт") {
				continue
			}
			switch {
			case containsAny(label, "инвойс", "invoice", "发票"):
				pick(&number, row, index)
			case containsAny(label, "контракт", "contract", "合同"):
				pick(&contract, row, index)
			case containsAny(label, "получатель", "收货人", "consignee"):
				pick(&receiver, row, index)
			case containsAny(label, "отправитель", "发货人", "shipper"):
				pick(&sender, row, index)
			case containsAny(label, "продавец", "поставщик", "卖主", "供应商", "seller"):
				pick(&seller, row, index)
			case containsAny(label, "вагон", "车皮号", "атс", "车号", "контейнер"):
				pick(&wagon, row, index)
			}
		}
	}
	if sender == "" {
		sender = seller
	}
	return number, contract, receiver, sender, wagon
}
