package goods

import (
	"bytes"
	"context"
	"testing"

	"github.com/xuri/excelize/v2"
)

// Набор из трёх строк: количество и вес объединены на три строки — строки 3 и 4 ссылаются на строку 2.
func TestParseXlsxMarksMergedRows(t *testing.T) {
	book := excelize.NewFile()
	for cell, value := range map[string]any{"A1": "Наименование", "B1": "Количество", "C1": "Брутто", "A2": "三件套小刀叉", "B2": 1200, "C2": 16, "A3": "动物模具", "A4": "两件套铲子", "A5": "Лопата", "B5": 10, "C5": 5} {
		if err := book.SetCellValue("Sheet1", cell, value); err != nil {
			t.Fatal(err)
		}
	}
	for _, area := range [][2]string{{"B2", "B4"}, {"C2", "C4"}} {
		if err := book.MergeCell("Sheet1", area[0], area[1]); err != nil {
			t.Fatal(err)
		}
	}
	buffer, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := NewGoodsService().ParseXlsx(context.Background(), bytes.NewReader(buffer.Bytes()), int64(buffer.Len()), "set.xlsx", "Sheet1", 1, 2)
	if err != nil || len(parsed.Rows) != 4 {
		t.Fatalf("rows %+v, err %v", parsed.Rows, err)
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
