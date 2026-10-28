package utilfee

import (
	"errors"
	"strings"
	"testing"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
)

func rowsCalc(t *testing.T, input RowsInput) RowsResult {
	t.Helper()
	result, err := CalculateRows(input)
	if err != nil {
		t.Fatalf("CalculateRows: %v", err)
	}
	if len(result.Rows) != len(input.Rows) {
		t.Fatalf("строк %d, ожидалось %d", len(result.Rows), len(input.Rows))
	}
	return result
}

func hasWarning(row RowResult, part string) bool {
	for _, warning := range row.Warnings {
		if strings.Contains(warning, part) {
			return true
		}
	}
	return false
}

// Таблица по Excel поставщика: нетто пустое — брутто − 20 %, упаковка = брутто − нетто, один материал на таблицу.
func TestCalculateRows(t *testing.T) {
	result := rowsCalc(t, RowsInput{Mrp: "4325", Material: "paper", Rows: []RowInput{
		// разд. 18 (за упаковку): нетто 686 × 0,8 = 548,8; упаковка 137,2 кг → 0,1372 × 4325 × 1,76 = 1044,3664
		{Code: "4202929800", GrossKg: "686"},
		// своё нетто: упаковка 552 − 450 = 102 кг → 0,102 × 4325 × 1,76 = 776,424
		{Code: "6307 90 980 0", GrossKg: "552", NetKg: "450"},
		// разд. 7 (за товар, k 1,40) и разд. 18: 0,08 × 4325 × 1,40 = 484,40; 0,02 × 4325 × 1,76 = 152,24
		{Code: "3924100000", GrossKg: "100"},
		// не в перечне
		{Code: "0101210000", GrossKg: "1 800,5"},
	}})

	first := result.Rows[0]
	if first.NetKg != "548.800" || !first.NetCalculated || first.PackagingKg != "137.200" || first.Product != "" || first.Packaging != "1044.37" || first.Total != "1044.37" {
		t.Errorf("строка 1: %+v", first)
	}
	second := result.Rows[1]
	if second.NetKg != "450.000" || second.NetCalculated || second.PackagingKg != "102.000" || second.Packaging != "776.42" || second.Total != "776.42" {
		t.Errorf("строка 2: %+v", second)
	}
	third := result.Rows[2]
	if third.Product != "484.40" || third.Packaging != "152.24" || third.Total != "636.64" {
		t.Errorf("строка 3: %+v", third)
	}
	fourth := result.Rows[3]
	if len(fourth.Matches) != 0 || fourth.Total != "0.00" || fourth.Error != "" {
		t.Errorf("строка 4: %+v", fourth)
	}
	if result.Total != "2457.43" {
		t.Errorf("итог %s, ожидалось 2457.43", result.Total)
	}
}

// Свой % упаковки: брутто 100, 10 % → нетто 90, упаковка 10 кг → 0,01 × 4325 × 1,40 (полимер) = 60,55.
func TestCalculateRowsPercentAndMaterial(t *testing.T) {
	result := rowsCalc(t, RowsInput{Mrp: "4325", PackagingPercent: "10", Material: "polymer", Rows: []RowInput{{Code: "4202929800", GrossKg: "100"}}})
	row := result.Rows[0]
	if row.NetKg != "90.000" || row.PackagingKg != "10.000" || row.Packaging != "60.55" {
		t.Errorf("%+v", row)
	}
}

// Сбор за товар от брутто: 0,1 × 4325 × 1,40 = 605,50; упаковка — всё равно от брутто − нетто: 152,24.
func TestCalculateRowsProductByGross(t *testing.T) {
	result := rowsCalc(t, RowsInput{Mrp: "4325", Material: "paper", ProductWeight: "gross", Rows: []RowInput{{Code: "3924100000", GrossKg: "100"}}})
	row := result.Rows[0]
	if row.Product != "605.50" || row.Packaging != "152.24" || row.Total != "757.74" || row.NetKg != "80.000" {
		t.Errorf("%+v", row)
	}
}

// Доллары по курсу: 1044,37 / 457,09 = 2,2848… → 2,28; 776,42 / 457,09 = 1,6986… → 1,70; итог 1820,79 / 457,09 = 3,9834… → 3,98.
// Без курса доллары пустые; строка без суммы — без долларов.
func TestCalculateRowsUSD(t *testing.T) {
	rows := []RowInput{{Code: "4202929800", GrossKg: "686"}, {Code: "6307909800", GrossKg: "552", NetKg: "450"}, {Code: "8516", GrossKg: "10"}}
	result := rowsCalc(t, RowsInput{Mrp: "4325", Material: "paper", UsdRate: "457.09", Rows: rows})
	if result.Rows[0].TotalUsd != "2.28" || result.Rows[1].TotalUsd != "1.70" || result.Rows[2].TotalUsd != "" || result.TotalUsd != "3.98" {
		t.Errorf("%+v / %s", result.Rows, result.TotalUsd)
	}
	noRate := rowsCalc(t, RowsInput{Mrp: "4325", Material: "paper", Rows: rows})
	if noRate.Rows[0].TotalUsd != "" || noRate.TotalUsd != "" {
		t.Errorf("без курса: %+v / %s", noRate.Rows[0], noRate.TotalUsd)
	}
}

// Ошибка в строке не мешает остальным; пустая строка пропускается без ошибки.
func TestCalculateRowsRowIssues(t *testing.T) {
	result := rowsCalc(t, RowsInput{Mrp: "4325", Material: "paper", Rows: []RowInput{
		{Code: "4202929800", GrossKg: "10", NetKg: "12"},
		{Code: "42", GrossKg: "10"},
		{Code: "4202929800"},
		{},
		{Code: "8703", GrossKg: "1500"},
		{Code: "8516", GrossKg: "10"},
		{Code: "4202929800", GrossKg: "10", NetKg: "10"},
		{Code: "4202929800", GrossKg: "abc"},
	}})
	rows := result.Rows
	if !strings.Contains(rows[0].Error, "Нетто больше брутто") || rows[0].Total != "" {
		t.Errorf("нетто > брутто: %+v", rows[0])
	}
	if !strings.Contains(rows[1].Error, "код ТН ВЭД") {
		t.Errorf("короткий код: %+v", rows[1])
	}
	if !strings.Contains(rows[2].Error, "брутто") {
		t.Errorf("без брутто: %+v", rows[2])
	}
	if rows[3].Error != "" || rows[3].Total != "" || len(rows[3].Warnings) != 0 {
		t.Errorf("пустая строка: %+v", rows[3])
	}
	if !hasWarning(rows[4], "Один код") || rows[4].Total != "" {
		t.Errorf("транспорт: %+v", rows[4])
	}
	if !hasWarning(rows[5], "полного кода") {
		t.Errorf("неполный код: %+v", rows[5])
	}
	if rows[6].PackagingKg != "0.000" || rows[6].Total != "0.00" || !hasWarning(rows[6], "упаковки 0") {
		t.Errorf("упаковка 0: %+v", rows[6])
	}
	if !strings.Contains(rows[7].Error, "Брутто") {
		t.Errorf("брутто не число: %+v", rows[7])
	}
	if result.Total != "0.00" {
		t.Errorf("итог %s", result.Total)
	}
}

// Ошибки всей таблицы — МРП, материал, %, число строк — отклоняют запрос целиком.
func TestCalculateRowsValidation(t *testing.T) {
	tooMany := make([]RowInput, MaxRows+1)
	for name, input := range map[string]RowsInput{
		"mrp":              {Mrp: "", Material: "paper"},
		"material":         {Mrp: "4325", Material: "wood"},
		"packagingPercent": {Mrp: "4325", Material: "paper", PackagingPercent: "100"},
		"rows":             {Mrp: "4325", Material: "paper", Rows: tooMany},
		"productWeight":    {Mrp: "4325", Material: "paper", ProductWeight: "tare"},
		"usdRate":          {Mrp: "4325", Material: "paper", UsdRate: "0"},
	} {
		_, err := CalculateRows(input)
		var validation *service.ValidationError
		if !errors.As(err, &validation) || validation.Field != name {
			t.Errorf("%s: %v", name, err)
		}
	}
}
