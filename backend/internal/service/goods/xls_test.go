package goods

import (
	"bytes"
	"context"
	"os"
	"testing"
)

// testdata/set.xls сохранён в Excel (формат 97-2003): та же книга, что в TestParseXlsxMarksMergedRows,
// плюс колонка «Цена» с форматом 0.00 (и у заголовка), текстовый «Артикул» в ячейке с форматом даты
// и лист «Итог» с объединённой шапкой A1:C1.
func TestParseXlsReadsValuesAndMergedRows(t *testing.T) {
	data, err := os.ReadFile("testdata/set.xls")
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := NewGoodsService().ParseXlsx(context.Background(), bytes.NewReader(data), int64(len(data)), "Набор.XLS", "Лист1", 1, 2)
	if err != nil || len(parsed.Rows) != 4 {
		t.Fatalf("rows %+v, err %v", parsed.Rows, err)
	}
	if len(parsed.Sheets) != 2 || parsed.Sheets[0].Name != "Лист1" || parsed.Sheets[1].Name != "Итог" {
		t.Fatalf("sheets %+v", parsed.Sheets)
	}
	first := parsed.Rows[0].Values
	if first["Наименование"] != "三件套小刀叉" || first["Количество"] != "1200" || first["Брутто"] != "16.5" || first["Цена"] != "2.40" || first["Артикул"] != "мх08199" {
		t.Fatalf("first row %+v", first)
	}
	if parsed.Rows[0].Merged != nil || parsed.Rows[3].Merged != nil {
		t.Fatalf("первая строка набора и обычная строка не отмечаются: %+v %+v", parsed.Rows[0].Merged, parsed.Rows[3].Merged)
	}
	for _, row := range parsed.Rows[1:3] {
		if row.Merged["Количество"] != 2 || row.Merged["Брутто"] != 2 {
			t.Fatalf("строка %d: %+v", row.SourceRow, row.Merged)
		}
	}
}

func TestParseXlsRejectsCorruptFile(t *testing.T) {
	data := []byte("not an xls")
	if _, err := NewGoodsService().ParseXlsx(context.Background(), bytes.NewReader(data), int64(len(data)), "bad.xls", "", 1, 2); err == nil {
		t.Fatal("expected an error")
	}
}
