package exportcheck

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// testdata: настоящие инвойсы (357-49 в .xlsx, LDO-10 в .xls) и декларации к ним, разобранные
// браузером из PDF (JSON из frontend/src/features/exportCheck/declarationPdf.test.ts).

func loadInvoice(t *testing.T, name string) Invoice {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	invoice, err := ParseInvoice(bytes.NewReader(data), int64(len(data)), name)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return invoice
}

func loadDeclarations(t *testing.T, name string) []Declaration {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	var declarations []Declaration
	if err := json.Unmarshal(data, &declarations); err != nil {
		t.Fatal(err)
	}
	return declarations
}

func failing(report Report) []string {
	var out []string
	for _, check := range append(append([]Check{}, report.Totals...), report.Details...) {
		if check.Status != statusOK && check.Status != statusSkip {
			out = append(out, check.Label+": "+check.Invoice+" / "+check.Declaration+" — "+check.Note)
		}
	}
	for _, row := range report.Rows {
		for _, check := range row.Checks {
			if check.Status != statusOK {
				out = append(out, "строка "+itoa(row.No)+" "+check.Label+": "+check.Invoice+" / "+check.Declaration+" — "+check.Note)
			}
		}
	}
	out = append(out, report.Problems...)
	return out
}

func itoa(value int) string { return strconv.Itoa(value) }

func TestParseInvoiceXlsx(t *testing.T) {
	invoice := loadInvoice(t, "357-49.xlsx")
	if invoice.Number != "357-49" || invoice.Contract != "JJ-1 от 15.09.22" || invoice.Receiver != "ТОО «357»" || !strings.HasPrefix(invoice.Sender, "Huairen") || invoice.Wagon != "" {
		t.Fatalf("details: %+v", invoice)
	}
	if len(invoice.Rows) != 46 {
		t.Fatalf("rows: %d", len(invoice.Rows))
	}
	first := invoice.Rows[0]
	want := InvoiceRow{No: 1, SourceRow: 19, Code: "8205598099", Name: "Ручной инструмент: Пистолеты для герметика / 泡沫 枪 黑", Places: "15", Qty: "600", GrossKg: "180.00", NetKg: "172.50", Price: "0.1700", Amount: "102.00"}
	if first != want {
		t.Fatalf("first row:\n got %+v\nwant %+v", first, want)
	}
	if invoice.Totals != (InvoiceTotals{Places: "2737", GrossKg: "63926.40", NetKg: "62557.90", Amount: "38233.00"}) {
		t.Fatalf("totals: %+v", invoice.Totals)
	}
}

func TestParseInvoiceXls(t *testing.T) {
	invoice := loadInvoice(t, "LDO-10.xls")
	if invoice.Number != "LDO-10" || invoice.Contract != "№LT-24/03 от «03» декабря 2024" || invoice.Receiver != "ТОО «Leader Trans Logistics»" || invoice.Wagon != "765BCS02-66AIV02" {
		t.Fatalf("details: %+v", invoice)
	}
	if !strings.HasPrefix(invoice.Sender, "Linyi Jikai") {
		t.Fatalf("sender: %q", invoice.Sender)
	}
	if len(invoice.Rows) != 107 || invoice.Rows[106].No != 107 || invoice.Rows[106].Qty != "512" || invoice.Rows[106].Amount != "61.44" {
		t.Fatalf("rows: %d, last %+v", len(invoice.Rows), invoice.Rows[len(invoice.Rows)-1])
	}
	if invoice.Totals != (InvoiceTotals{Places: "1596", GrossKg: "18202.23", NetKg: "17723.43", Amount: "102486.23"}) {
		t.Fatalf("totals: %+v", invoice.Totals)
	}
}

func TestCompareSingleDeclaration(t *testing.T) {
	report := Compare(loadInvoice(t, "357-49.xlsx"), loadDeclarations(t, "357-49.declarations.json"))
	if report.InvoiceRows != 46 || report.DeclarationItems != 46 {
		t.Fatalf("counts: %+v", report)
	}
	for _, check := range report.Totals {
		if check.Status != statusOK {
			t.Errorf("total %s: %+v", check.Label, check)
		}
	}
	if report.Details[0].Status != statusOK || report.Details[1].Status != statusOK || report.Details[2].Status != statusSkip {
		t.Errorf("details: %+v", report.Details)
	}
	if report.Errors != 0 {
		t.Errorf("expected a clean report, got %d errors:\n%s", report.Errors, strings.Join(failing(report), "\n"))
	}
}

func TestCompareSplitDeclarations(t *testing.T) {
	report := Compare(loadInvoice(t, "LDO-10.xls"), loadDeclarations(t, "LDO-10.declarations.json"))
	if report.DeclarationItems != 107 {
		t.Fatalf("items: %d", report.DeclarationItems)
	}
	// Места, брутто и нетто складываются по трём декларациям; контракт в декларации обрезан.
	for _, check := range report.Totals[:3] {
		if check.Status != statusOK {
			t.Errorf("total %s: %+v", check.Label, check)
		}
	}
	if report.Details[0].Status != statusOK || report.Details[1].Status != statusOK {
		t.Errorf("details: %+v", report.Details)
	}
	// Сумма строки 31 (11568.42) в .xls хранится как RK «целое ×1/100» — раньше читалась нулём.
	if got := failing(report); len(got) > 0 || report.Errors != 0 {
		t.Errorf("errors %d, failing checks:\n%s", report.Errors, strings.Join(got, "\n"))
	}
	if row := report.Rows[30]; row.No != 31 || row.Checks[3].Invoice != "11568.42" || row.DeclarationNo != 31 || row.File != "1-50预录单.pdf" {
		t.Errorf("row 31: %+v", row)
	}
}

func TestCompareAmountHintTellsWhichSideIsWrong(t *testing.T) {
	invoice := Invoice{Rows: []InvoiceRow{{No: 1, Qty: "999", NetKg: "1750", Price: "11.58", Amount: "0.00"}, {No: 2, Qty: "10", NetKg: "1", Price: "2", Amount: "20"}}}
	declarations := []Declaration{{Items: []DeclarationItem{
		{No: 1, Qty: "999", QtyUnit: "双", NetKg: "1750", Price: "11.5800", Amount: "11568.42", Currency: "USD"},
		{No: 2, Qty: "10", QtyUnit: "双", NetKg: "1", Price: "2.0000", Amount: "200.00", Currency: "USD"},
	}}}
	report := Compare(invoice, declarations)
	if note := report.Rows[0].Checks[3].Note; note != "штуки × цена = 11568.42, как в декларации: ошибка в инвойсе" {
		t.Errorf("invoice wrong: %q", note)
	}
	if note := report.Rows[1].Checks[3].Note; note != "штуки × цена = 20.00, как в инвойсе: ошибка в декларации" {
		t.Errorf("declaration wrong: %q", note)
	}
}

func TestCompareFindsSwappedDigitsMissingAndExtraRows(t *testing.T) {
	invoice := Invoice{Receiver: "ТОО «357»", Contract: "JJ-1 от 15.09.22", Rows: []InvoiceRow{
		{No: 1, Qty: "2150", NetKg: "1057.8", Price: "0.17", Amount: "365.5"},
		{No: 2, Qty: "100", NetKg: "10", Price: "1", Amount: "100"},
	}, Totals: InvoiceTotals{Places: "5", GrossKg: "1100", NetKg: "1067.8", Amount: "465.5"}}
	declarations := []Declaration{{FileName: "d.pdf", BillNo: "123", Receiver: "ТОО «357»", Contract: "JJ-1 от 15.09.22", Places: "5", GrossKg: "1100", NetKg: "1067.8", Items: []DeclarationItem{
		{No: 1, Qty: "2150", QtyUnit: "个", NetKg: "1075.8", Price: "0.1700", Amount: "365.50", Currency: "USD"},
		{No: 3, Qty: "7", QtyUnit: "个", NetKg: "1", Price: "1.0000", Amount: "7.00", Currency: "USD"},
	}}}
	report := Compare(invoice, declarations)
	net := report.Rows[0].Checks[1]
	if net.Status != statusError || net.Note != "похоже, переставлены цифры или потеряна запятая" {
		t.Errorf("swapped digits: %+v", net)
	}
	if report.Rows[1].Status != statusError || report.Rows[1].Checks[0].Note != "нет в декларациях" {
		t.Errorf("missing row: %+v", report.Rows[1])
	}
	joined := strings.Join(report.Problems, "\n")
	if !strings.Contains(joined, "товар № 3") || strings.Contains(joined, "В инвойсе 2 строк") {
		t.Errorf("problems: %s", joined)
	}
	if report.Totals[3].Status != statusError || report.Totals[3].Invoice != "465.5" || report.Totals[3].Declaration != "372.5" {
		t.Errorf("amount total: %+v", report.Totals[3])
	}
}

func TestNormalizeNumberHandlesSeparators(t *testing.T) {
	for text, want := range map[string]string{
		"11568.42": "11568.42", "11 568,42": "11568.42", "1,234.56": "1234.56", "1.234,56": "1234.56", "11,568.42": "11568.42",
		"0,1700": "0.1700", " 2737 ": "2737", "1 234": "1234", "": "",
	} {
		if got := normalizeNumber(text); got != want {
			t.Errorf("%q: got %q, want %q", text, got, want)
		}
	}
}

func TestCompareReportsDeclarationSelfCheckFailures(t *testing.T) {
	invoice := Invoice{Rows: []InvoiceRow{{No: 1, SourceRow: 19, Qty: "600", NetKg: "172.5", Price: "0.17", Amount: "102"}}}
	// Итоги tips больше суммы прочитанных строк: значит, строки потеряны при чтении PDF.
	declarations := []Declaration{{FileName: "d.pdf", TotalAmount: "200", TotalQty: "1200", Items: []DeclarationItem{
		{No: 1, Qty: "600", QtyUnit: "个", NetKg: "172.5", Price: "0.1700", Amount: "103.00", Currency: "USD"},
	}}}
	report := Compare(invoice, declarations)
	joined := strings.Join(report.Problems, "\n")
	if !strings.Contains(joined, "общая стоимость в декларации 200, а сумма по прочитанным строкам 103") || !strings.Contains(joined, "сумма количеств в декларации 1200, а по прочитанным строкам 600") {
		t.Errorf("problems: %s", joined)
	}
	if notes := strings.Join(report.Notes, "\n"); !strings.Contains(notes, "d.pdf, товар 1: 600 × 0.17 = 102.00, а сумма в декларации 103.00") {
		t.Errorf("notes: %s", notes)
	}
	if report.Rows[0].ExcelRow != 19 || report.Rows[0].Checks[3].Note != "штуки × цена = 102.00, как в инвойсе: ошибка в декларации" {
		t.Errorf("row: %+v", report.Rows[0])
	}
}

func TestCompareKilogramQuantityIsWarning(t *testing.T) {
	invoice := Invoice{Rows: []InvoiceRow{{No: 1, Qty: "5000", NetKg: "5322.5", Price: "0.28", Amount: "1400"}}}
	declarations := []Declaration{{Items: []DeclarationItem{{No: 1, Qty: "5322.5", QtyUnit: "千克", NetKg: "5322.5", Price: "0.2800", Amount: "1400.00", Currency: "USD"}}}}
	report := Compare(invoice, declarations)
	if qty := report.Rows[0].Checks[0]; qty.Status != statusWarn || !strings.Contains(qty.Note, "в килограммах") {
		t.Errorf("qty: %+v", qty)
	}
	if report.Rows[0].Status != statusWarn {
		t.Errorf("row status: %s", report.Rows[0].Status)
	}
}
