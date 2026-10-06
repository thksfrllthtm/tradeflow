package riskindicators

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/xuri/excelize/v2"
)

// Файл как «25_прил CN»: шапка с номером профиля, строка групп стран, «-» и 0 — индикатора нет.
func profileWorkbook(t *testing.T, rows [][]any) *bytes.Buffer {
	t.Helper()
	book := excelize.NewFile()
	for index, row := range rows {
		cell, _ := excelize.CoordinatesToCellName(1, index+1)
		if err := book.SetSheetRow("Sheet1", cell, &row); err != nil {
			t.Fatal(err)
		}
	}
	buffer, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	return buffer
}

func TestParseProfile(t *testing.T) {
	buffer := profileWorkbook(t, [][]any{
		{"", "", "", "", "Приложение 1\nк профилю риска\n № 50100/190318/00025"},
		{},
		{"Унифицированные стоимостные индикаторы риска (стоимость килограмма в долларах США)"},
		{"№ п/п", "Kод товара по THBЭД", "Страна происхождения, стоимостной индикатор риска"},
		{"", "", "AD, AG, AI, CN,", "AM, AZ, BY.", "CN", "00", " PL"},
		{"1", "0701100000", "0.60", "0.25", "0.60", "-", "0"},
		{"2", "6907 40 100 0", "1,50", "0", "1.10", "1.50", "1.50"},
		{"3", 701905000, 0.6, 0.35, 0.35, 0.6, "-"},
		{"", "", "Примечание: внизу таблицы"},
	})
	book, rowErrors, err := Parse(buffer, int64(buffer.Len()), "25_прил CN.xlsx")
	if err != nil || len(rowErrors) > 0 {
		t.Fatalf("Parse: %v %v", err, rowErrors)
	}
	if book.Number != "50100/190318/00025" || len(book.Groups) != 5 || len(book.Rows) != 3 {
		t.Fatalf("book = %+v", book)
	}
	if got := book.Rows[1]; got.Code != "6907401000" || got.Prices[0] != "1.5" || got.Prices[1] != "" {
		t.Fatalf("row 2 = %+v", got)
	}
	if got := book.Rows[2]; got.Code != "0701905000" || got.Prices[2] != "0.35" || got.Prices[4] != "" {
		t.Fatalf("row 3 (числа Excel) = %+v", got)
	}

	groups, _ := json.Marshal(book.Groups)
	rows := []repository.RiskIndicatorRow{}
	for _, row := range book.Rows {
		prices := make([]*string, len(row.Prices))
		for index := range row.Prices {
			if row.Prices[index] != "" {
				prices[index] = &row.Prices[index]
			}
		}
		encoded, _ := json.Marshal(prices)
		rows = append(rows, repository.RiskIndicatorRow{ProfileNumber: book.Number, Groups: groups, Code: row.Code, Prices: encoded})
	}
	// CN есть и в общей группе, и отдельной колонкой — берётся отдельная колонка.
	china := Pick(rows, "cn")
	if got := china["0701100000"]; got.PriceUSD.String() != "0.6" || got.Profile != "50100/190318/00025" {
		t.Fatalf("CN = %+v", got)
	}
	if got := china["0701905000"]; got.PriceUSD.String() != "0.35" {
		t.Fatalf("CN отдельной колонкой = %+v", got)
	}
	// «-» и 0 — индикатора нет; страны нет ни в одной группе — тоже.
	if _, ok := Pick(rows, "00")["0701100000"]; ok {
		t.Fatal("«-» дал индикатор")
	}
	if _, ok := Pick(rows, "PL")["0701100000"]; ok {
		t.Fatal("0 дал индикатор")
	}
	if got := Pick(rows, "RU"); len(got) != 0 {
		t.Fatalf("RU = %+v", got)
	}
}

func TestParseRejectsBadRows(t *testing.T) {
	buffer := profileWorkbook(t, [][]any{
		{"№ п/п", "Код товара по ТН ВЭД"},
		{"", "", "CN"},
		{"1", "6107290000", "22"},
		{"2", "61072900", "22"},
		{"3", "6117100000", "abc"},
		{"4", "6107290000", "23"},
	})
	book, rowErrors, err := Parse(buffer, int64(buffer.Len()), "профиль.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	if book.Number != "профиль" || len(rowErrors) != 3 {
		t.Fatalf("number %q, errors %+v", book.Number, rowErrors)
	}
	if rowErrors[0].Row != 4 || rowErrors[1].Row != 5 || rowErrors[2].Row != 6 {
		t.Fatalf("rows = %+v", rowErrors)
	}
}

func TestParseWithoutTable(t *testing.T) {
	buffer := profileWorkbook(t, [][]any{{"Наименование", "Цена"}, {"Перчатки", "1.5"}})
	if _, _, err := Parse(buffer, int64(buffer.Len()), "x.xlsx"); err == nil {
		t.Fatal("ожидалась ошибка")
	}
}
