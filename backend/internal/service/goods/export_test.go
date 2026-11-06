package goods

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/png"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestGoodsExportKeepsExcelNameUnlessProductChosen(t *testing.T) {
	codeOnly := testGoodsSnapshot()
	codeOnly["name"] = "Цистерны, бочки, барабаны"
	product := testGoodsSnapshot()
	product["name"], product["productDirectoryId"] = "Перчатки нитриловые", float64(5)
	row := func(name string, snapshot map[string]any) GoodsItemInput {
		return GoodsItemInput{Name: name, SupplierName: name, StandardName: snapshot["name"].(string), DirectorySnapshot: snapshot, Pairs: "10", Places: "1", GrossKg: "5", Coefficient: "1"}
	}
	buffer, err := NewGoodsService().Export(context.Background(), GoodsCalculationInput{
		AssessmentDate: "2026-09-17", UsdKzt: "500", DeliveryMode: "none", GenericItems: true,
		Items: []GoodsItemInput{row("Товар из Excel", codeOnly), row("Перчатки из Excel", product)},
	})
	if err != nil {
		t.Fatal(err)
	}
	book, err := excelize.OpenReader(buffer)
	if err != nil {
		t.Fatal(err)
	}
	sheet := book.GetSheetName(0)
	codeRow, _ := book.GetCellValue(sheet, "C2")
	productRow, _ := book.GetCellValue(sheet, "C3")
	if codeRow != "Товар из Excel" {
		t.Fatalf("a TN ВЭД code must not replace the Excel name, got %q", codeRow)
	}
	if productRow != "Перчатки нитриловые" {
		t.Fatalf("a chosen product card must give its name, got %q", productRow)
	}
}

func TestExportContainsRowsPhotoTotalsAndSummary(t *testing.T) {
	var photo bytes.Buffer
	if err := png.Encode(&photo, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	snapshot := testGoodsSnapshot()
	snapshot["duty"] = map[string]any{"type": "percentage", "percent": "10"}
	snapshot["vat"] = map[string]any{"type": "percentage", "percent": "12"}
	row := GoodsItemInput{Name: "Бочка", SupplierName: "Бочка", StandardName: "Резиновые перчатки", DirectorySnapshot: snapshot, Places: "10", PairsPerPlace: "80", GrossKg: "1220",
		PricePerPair: "6.4792", PhotoURL: "data:image/png;base64," + base64.StdEncoding.EncodeToString(photo.Bytes())}
	buffer, err := NewGoodsService().Export(context.Background(), GoodsCalculationInput{
		AssessmentDate: "2026-09-17", UsdKzt: "500", DeliveryMode: "none", Mrp: "4325", FeeMultiplier: "6", GenericItems: true, Items: []GoodsItemInput{row},
	})
	if err != nil {
		t.Fatal(err)
	}
	book, err := excelize.OpenReader(buffer)
	if err != nil {
		t.Fatal(err)
	}
	if sheets := book.GetSheetList(); len(sheets) != 2 || sheets[0] != "Расчёт" || sheets[1] != "Итоги" {
		t.Fatalf("expected sheets Расчёт and Итоги, got %v", sheets)
	}
	cell := func(sheet, name string) string { value, _ := book.GetCellValue(sheet, name); return value }
	if cell("Расчёт", "C1") != "Наименование" || cell("Расчёт", "F2") != "10%" || cell("Расчёт", "G2") != "12%" || cell("Расчёт", "K2") != "800" {
		t.Fatalf("unexpected row: name header %q duty %q vat %q quantity %q", cell("Расчёт", "C1"), cell("Расчёт", "F2"), cell("Расчёт", "G2"), cell("Расчёт", "K2"))
	}
	if price, _ := book.GetCellValue("Расчёт", "O2", excelize.Options{RawCellValue: true}); price != "6.4792" {
		t.Fatalf("the price on screen must be exported, got %q", price)
	}
	if pictures, _ := book.GetPictures("Расчёт", "B2"); len(pictures) != 1 {
		t.Fatalf("the row photo must be embedded, got %d pictures", len(pictures))
	}
	if cell("Расчёт", "C3") != "Итого" || cell("Расчёт", "Z3") == "" {
		t.Fatalf("a total row with the customs payment is expected, got %q / %q", cell("Расчёт", "C3"), cell("Расчёт", "Z3"))
	}
	lineCustoms, _ := book.GetCellValue("Расчёт", "Z2", excelize.Options{RawCellValue: true})
	payment, _ := book.GetCellValue("Итоги", "B18", excelize.Options{RawCellValue: true})
	paymentUSD, _ := book.GetCellValue("Итоги", "C18", excelize.Options{RawCellValue: true})
	if cell("Итоги", "A18") != "Растаможка к оплате" || payment != lineCustoms || paymentUSD == "" {
		t.Fatalf("summary must show the customs payment in KZT and USD: %q %q %q (row %q)", cell("Итоги", "A18"), payment, paymentUSD, lineCustoms)
	}
	if cell("Итоги", "B9") != "МРП 4 325 × 6" {
		t.Fatalf("fee parameters expected, got %q", cell("Итоги", "B9"))
	}
}
