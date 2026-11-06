package goods

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestGoodsParseXlsxReturnsHeadersAndSourceRows(t *testing.T) {
	book := excelize.NewFile()
	if err := book.SetCellValue("Sheet1", "A1", "Наименование"); err != nil {
		t.Fatal(err)
	}
	if err := book.SetCellValue("Sheet1", "B1", "Места"); err != nil {
		t.Fatal(err)
	}
	if err := book.SetCellValue("Sheet1", "A2", "Перчатки"); err != nil {
		t.Fatal(err)
	}
	if err := book.SetCellValue("Sheet1", "B2", 3); err != nil {
		t.Fatal(err)
	}
	buffer, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := NewGoodsService().ParseXlsx(context.Background(), bytes.NewReader(buffer.Bytes()), int64(buffer.Len()), "test.xlsx", "Sheet1", 1, 2)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(parsed.Headers) != 2 || parsed.Headers[0] != "Наименование" || len(parsed.Rows) != 1 || parsed.Rows[0].SourceRow != 2 {
		t.Fatalf("unexpected parse result: %+v", parsed)
	}
}

func TestGoodsParseXlsxExcludesTotalRows(t *testing.T) {
	book := excelize.NewFile()
	if err := book.SetCellValue("Sheet1", "A1", "Наименование"); err != nil {
		t.Fatal(err)
	}
	if err := book.SetCellValue("Sheet1", "A2", "Перчатки"); err != nil {
		t.Fatal(err)
	}
	if err := book.SetCellValue("Sheet1", "A3", "Итого"); err != nil {
		t.Fatal(err)
	}
	buffer, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := NewGoodsService().ParseXlsx(context.Background(), bytes.NewReader(buffer.Bytes()), int64(buffer.Len()), "test.xlsx", "Sheet1", 1, 2)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(parsed.Rows) != 2 || !parsed.Rows[0].Included || parsed.Rows[1].Included {
		t.Fatalf("expected product and excluded total row, got %+v", parsed.Rows)
	}
	if len(parsed.Rows[1].Warnings) == 0 || len(parsed.Warnings) == 0 {
		t.Fatalf("expected explicit total-row warnings, got row=%v result=%v", parsed.Rows[1].Warnings, parsed.Warnings)
	}
}

func TestGoodsParseXlsxKeepsBlankAndDuplicateHeaders(t *testing.T) {
	book := excelize.NewFile()
	for cell, value := range map[string]string{"A1": "Наименование", "B1": "", "C1": "Наименование", "A2": "Перчатки", "B2": "10", "C2": "другое"} {
		if err := book.SetCellValue("Sheet1", cell, value); err != nil {
			t.Fatal(err)
		}
	}
	buffer, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := NewGoodsService().ParseXlsx(context.Background(), bytes.NewReader(buffer.Bytes()), int64(buffer.Len()), "test.xlsx", "Sheet1", 1, 2)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(parsed.Headers) != 3 || parsed.Headers[1] != "Колонка 2" || parsed.Headers[2] != "Наименование (2)" {
		t.Fatalf("unexpected canonical headers: %+v", parsed.Headers)
	}
	if parsed.Rows[0].Values["Колонка 2"] != "10" || parsed.Rows[0].Values["Наименование (2)"] != "другое" {
		t.Fatalf("values from blank/duplicate columns were lost: %+v", parsed.Rows[0].Values)
	}
}

func TestGoodsParseXlsxRejectsCorruptWorkbook(t *testing.T) {
	_, err := NewGoodsService().ParseXlsx(context.Background(), bytes.NewReader([]byte("not an xlsx")), int64(len("not an xlsx")), "bad.xlsx", "", 1, 2)
	if err == nil || !strings.Contains(err.Error(), "Файл повреждён") {
		t.Fatalf("expected clear corrupt-file error, got %v", err)
	}
}

func TestGoodsParseXlsxSendsSharedPictureOnce(t *testing.T) {
	book := excelize.NewFile()
	for cell, value := range map[string]string{"A1": "Наименование", "A2": "Перчатки S", "A3": "Перчатки M"} {
		if err := book.SetCellValue("Sheet1", cell, value); err != nil {
			t.Fatal(err)
		}
	}
	// A tall picture anchored at B2 covers rows 2 and 3 (default row height is 20 px).
	var picture bytes.Buffer
	if err := png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 10, 35))); err != nil {
		t.Fatal(err)
	}
	if err := book.AddPictureFromBytes("Sheet1", "B2", &excelize.Picture{Extension: ".png", File: picture.Bytes()}); err != nil {
		t.Fatal(err)
	}
	buffer, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := NewGoodsService().ParseXlsxWithSharedImages(context.Background(), bytes.NewReader(buffer.Bytes()), int64(buffer.Len()), "test.xlsx", "Sheet1", 1, 2)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var written bytes.Buffer
	if err := parsed.WriteJSON(&written); err != nil {
		t.Fatal(err)
	}
	var response struct {
		Rows      []GoodsParseRow   `json:"rows"`
		Images    []GoodsParseImage `json:"images"`
		ImageData []string          `json:"imageData"`
	}
	if err := json.Unmarshal(written.Bytes(), &response); err != nil {
		t.Fatalf("response must be valid JSON: %v", err)
	}
	if len(response.Rows) != 2 || len(response.ImageData) != 1 || len(response.Images) != 2 {
		t.Fatalf("one picture over two rows must be sent once and referenced twice, got %d data, %d links", len(response.ImageData), len(response.Images))
	}
	for index, link := range response.Images {
		if link.SourceRow != index+2 || link.DataIndex != 0 || link.MimeType != "image/png" {
			t.Fatalf("unexpected image link %+v", link)
		}
	}
	if decoded, _ := base64.StdEncoding.DecodeString(response.ImageData[0]); !bytes.Equal(decoded, picture.Bytes()) {
		t.Fatal("image data must be the original picture")
	}
}

func TestGoodsParseResultWriteJSONWithoutImages(t *testing.T) {
	var written bytes.Buffer
	if err := (GoodsParseResult{Headers: []string{"A"}}).WriteJSON(&written); err != nil {
		t.Fatal(err)
	}
	var response map[string]any
	if err := json.Unmarshal(written.Bytes(), &response); err != nil {
		t.Fatalf("response must be valid JSON: %v (%s)", err, written.String())
	}
	if data, ok := response["imageData"].([]any); !ok || len(data) != 0 {
		t.Fatalf("imageData must be an empty array, got %s", written.String())
	}
}

// Картинка в объединённой ячейке (B2:B5) короче ячейки — фото получают все её строки.
func TestGoodsParseXlsxPictureInMergedCellCoversAllRows(t *testing.T) {
	book := excelize.NewFile()
	for cell, value := range map[string]string{"A1": "Наименование", "A2": "Цепь 3*2,3", "A3": "Цепь 4*2,3", "A4": "Цепь 5*2,3", "A5": "Цепь 10*2,3", "A6": "Брусок"} {
		if err := book.SetCellValue("Sheet1", cell, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := book.MergeCell("Sheet1", "B2", "B5"); err != nil {
		t.Fatal(err)
	}
	var picture bytes.Buffer
	if err := png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 10, 15))); err != nil {
		t.Fatal(err)
	}
	if err := book.AddPictureFromBytes("Sheet1", "B2", &excelize.Picture{Extension: ".png", File: picture.Bytes()}); err != nil {
		t.Fatal(err)
	}
	buffer, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := NewGoodsService().ParseXlsxWithSharedImages(context.Background(), bytes.NewReader(buffer.Bytes()), int64(buffer.Len()), "test.xlsx", "Sheet1", 1, 2)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	rows := map[int]bool{}
	for _, link := range parsed.Images {
		rows[link.SourceRow] = true
	}
	if !rows[2] || !rows[3] || !rows[4] || !rows[5] || rows[6] {
		t.Fatalf("фото должны получить строки 2–5 объединённой ячейки, получили %v", rows)
	}
}

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
