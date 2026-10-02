package directory

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestParseNomenclatureTariffCoversTWSForms(t *testing.T) {
	for text, want := range map[string]NomenclatureDuty{
		"12%":   {Type: "percentage", Percent: "12"},
		"12.5%": {Type: "percentage", Percent: "12.5"},
		"0%":    {Type: "percentage", Percent: "0"},
		"50%, но не менее 1 EUR за 1 кг":        {Type: "combined_minimum", Percent: "50", Amount: "1", Currency: "EUR", Unit: "кг"},
		"20%, но не менее 0.8 EUR за 1 см3, мл": {Type: "combined_minimum", Percent: "20", Amount: "0.8", Currency: "EUR", Unit: "см3, мл"},
		"0.75 EUR за 1 пар":                     {Type: "specific", Amount: "0.75", Currency: "EUR", Unit: "пар"},
		"250 USD за 1 т":                        {Type: "specific", Amount: "250", Currency: "USD", Unit: "т"},
		"2 EUR за 1 1000 шт":                    {Type: "specific", Amount: "2", Currency: "EUR", Unit: "1000 шт"},
		"4.5% плюс 0.02 EUR за 1 кг":            {Type: "combined_plus", Percent: "4.5", Amount: "0.02", Currency: "EUR", Unit: "кг"},
		"":                                      {Type: "unknown"},
		"по особому решению":                    {Type: "unknown"},
	} {
		if got := ParseNomenclatureTariff(text); got != want {
			t.Errorf("%q: got %+v, want %+v", text, got, want)
		}
	}
}

// links: cell reference («D2») → hyperlink target, like «подробнее» in the TWS export.
func nomenclatureWorkbook(t *testing.T, rows [][]string, links map[string]string) *bytes.Reader {
	t.Helper()
	book := excelize.NewFile()
	book.SetSheetName("Sheet1", "Система TWS")
	_ = book.SetSheetRow("Система TWS", "A2", &[]any{"Актуальность данных", "29.09.2026"})
	if _, err := book.NewSheet("ТНВЭД"); err != nil {
		t.Fatal(err)
	}
	_ = book.SetSheetRow("ТНВЭД", "A1", &[]any{"Код", "Наименование", "Тариф", "Подробности"})
	for index, row := range rows {
		values := make([]any, len(row))
		for i, value := range row {
			values[i] = value
		}
		cellName, _ := excelize.CoordinatesToCellName(1, index+2)
		_ = book.SetSheetRow("ТНВЭД", cellName, &values)
	}
	for ref, target := range links {
		if err := book.SetCellHyperLink("ТНВЭД", ref, target, "External"); err != nil {
			t.Fatal(err)
		}
	}
	var buffer bytes.Buffer
	if err := book.Write(&buffer); err != nil {
		t.Fatal(err)
	}
	return bytes.NewReader(buffer.Bytes())
}

func TestReadNomenclatureFileReadsTWSExport(t *testing.T) {
	parsed, err := readNomenclatureFile(nomenclatureWorkbook(t, [][]string{
		{"4015190000", "перчатки, рукавицы и митенки: 🠺 прочие", "10%", "подробнее"},
		{"6401100000", "обувь: 🠺 с металлическим подноском", "0.75 EUR за 1 пар", "подробнее"},
		{"", "", "", ""},
	}, map[string]string{"D2": "https://www.tws.by/tws/tnved/code/3251", "D3": "javascript:alert(1)"}))
	if err != nil {
		t.Fatal(err)
	}
	if len(parsed.errors) != 0 || len(parsed.codes) != 2 {
		t.Fatalf("unexpected result: %+v", parsed)
	}
	if parsed.actualDate == nil || parsed.actualDate.Format("2006-01-02") != "2026-09-29" {
		t.Fatalf("actual date not read: %v", parsed.actualDate)
	}
	first := parsed.codes[0]
	if first.Code != "4015190000" || first.DutyType != "percentage" || first.DutyPercent.String() != "10" || first.TariffText != "10%" {
		t.Fatalf("first code parsed wrong: %+v", first)
	}
	if second := parsed.codes[1]; second.DutyType != "specific" || second.DutyAmount.String() != "0.75" || second.DutyUnit != "пар" {
		t.Fatalf("second code parsed wrong: %+v", second)
	}
	if first.DetailsURL != "https://www.tws.by/tws/tnved/code/3251" {
		t.Fatalf("«подробнее» link not read: %q", first.DetailsURL)
	}
	if parsed.codes[1].DetailsURL != "" {
		t.Fatalf("only http(s) links may be kept, got %q", parsed.codes[1].DetailsURL)
	}
	for _, code := range parsed.codes {
		if code.VATPercent.String() != "16" {
			t.Fatalf("every code gets 16%% VAT, %s has %s", code.Code, code.VATPercent)
		}
	}
}

func TestReadNomenclatureFileReportsRowsToFix(t *testing.T) {
	parsed, err := readNomenclatureFile(nomenclatureWorkbook(t, [][]string{
		{"4015190000", "перчатки", "10%"},
		{"401519000", "короткий код", "10%"},
		{"4015190000", "повтор", "10%"},
		{"6116102000", "", "12%"},
	}, nil))
	if err != nil {
		t.Fatal(err)
	}
	rows := []int{}
	for _, item := range parsed.errors {
		rows = append(rows, item.Row)
	}
	if !reflect.DeepEqual(rows, []int{3, 4, 5}) {
		t.Fatalf("expected errors in rows 3, 4, 5, got %+v", parsed.errors)
	}
}

func TestReadNomenclatureFileRejectsOtherWorkbooks(t *testing.T) {
	book := excelize.NewFile()
	_ = book.SetSheetRow("Sheet1", "A1", &[]any{"Марка", "Цена"})
	var buffer bytes.Buffer
	_ = book.Write(&buffer)
	if _, err := readNomenclatureFile(bytes.NewReader(buffer.Bytes())); err == nil {
		t.Fatal("a workbook without «Код» and «Наименование» must be rejected")
	}
}
