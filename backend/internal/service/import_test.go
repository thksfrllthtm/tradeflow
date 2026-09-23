package service

import (
	"crypto/sha256"
	"encoding/hex"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/xuri/excelize/v2"
)

func TestSyntheticWorkbookPreviewAndPictures(t *testing.T) {
	temporary := t.TempDir()
	imagePath := filepath.Join(temporary, "product.png")
	imageFile, err := os.Create(imagePath)
	if err != nil {
		t.Fatal(err)
	}
	imageValue := image.NewRGBA(image.Rect(0, 0, 2, 2))
	imageValue.Set(0, 0, color.RGBA{R: 20, G: 120, B: 70, A: 255})
	if err := png.Encode(imageFile, imageValue); err != nil {
		t.Fatal(err)
	}
	if err := imageFile.Close(); err != nil {
		t.Fatal(err)
	}

	workbook := excelize.NewFile()
	if err := workbook.SetSheetName("Sheet1", "Товары"); err != nil {
		t.Fatal(err)
	}
	if err := workbook.SetCellValue("Товары", "A1", "Синтетическая книга"); err != nil {
		t.Fatal(err)
	}
	headers := []string{"Product Name", "Qty", "Unit Price", "Total", "Currency", "Weight", "Image URL"}
	for index, value := range headers {
		cell, _ := excelize.CoordinatesToCellName(index+1, 2)
		if err := workbook.SetCellValue("Товары", cell, value); err != nil {
			t.Fatal(err)
		}
	}
	values := []string{"Чайник", "2", "10,50", "21,00", "USD", "1,2", "https://images.example/product.png"}
	for index, value := range values {
		cell, _ := excelize.CoordinatesToCellName(index+1, 3)
		if err := workbook.SetCellValue("Товары", cell, value); err != nil {
			t.Fatal(err)
		}
	}
	for index, value := range []string{"Кружка", "0", "", "10", "USD"} {
		cell, _ := excelize.CoordinatesToCellName(index+1, 4)
		if err := workbook.SetCellValue("Товары", cell, value); err != nil {
			t.Fatal(err)
		}
	}
	for index, value := range []string{"Ложка", "2", "5", "13", "USD"} {
		cell, _ := excelize.CoordinatesToCellName(index+1, 5)
		if err := workbook.SetCellValue("Товары", cell, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := workbook.SetCellValue("Товары", "A6", "Итого"); err != nil {
		t.Fatal(err)
	}
	if err := workbook.SetCellFormula("Товары", "D6", "SUM(D3:D5)"); err != nil {
		t.Fatal(err)
	}
	if err := workbook.AddPicture("Товары", "G3", imagePath, nil); err != nil {
		t.Fatal(err)
	}
	extraSheet, err := workbook.NewSheet("Справка")
	if err != nil || extraSheet < 0 {
		t.Fatalf("create second sheet: %v", err)
	}
	if err := workbook.SetCellValue("Справка", "A1", "Не импортировать автоматически"); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(temporary, "synthetic.xlsx")
	if err := workbook.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	if err := workbook.Close(); err != nil {
		t.Fatal(err)
	}

	opened, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer opened.Close()
	if sheets := opened.GetSheetList(); len(sheets) != 2 {
		t.Fatalf("expected 2 sheets, got %d", len(sheets))
	}
	rows, err := opened.GetRows("Товары")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 6 {
		t.Fatalf("expected 6 source rows, got %d", len(rows))
	}
	mapping := suggestMapping(rows[1])
	if mapping["name"] != 1 || mapping["quantity"] != 2 || mapping["unitPrice"] != 3 || mapping["lineTotal"] != 4 {
		t.Fatalf("unexpected heuristic mapping: %#v", mapping)
	}
	input := ConfirmInput{Sheet: "Товары", HeaderRow: 2, Mapping: map[string]int{"name": 1, "quantity": 2, "unitPrice": 3, "lineTotal": 4, "currency": 5, "weight": 6, "photoURL": 7}, PriceBasis: "piece", WeightBasis: "line"}
	product, err := productFromRow(opened, "Товары", 3, rows[2], input)
	if err != nil {
		t.Fatal(err)
	}
	if product.Name != "Чайник" || product.Quantity == nil || product.Quantity.String() != "2" || product.UnitPrice == nil || product.UnitPrice.String() != "10.5" || product.LineTotal == nil || product.LineTotal.String() != "21" {
		t.Fatalf("unexpected normalised row: %#v", product)
	}
	if product.ImportStatus != "needs_correction" {
		t.Fatalf("expected URL warning, got %s", product.ImportStatus)
	}
	mismatch, err := productFromRow(opened, "Товары", 5, rows[4], input)
	if err != nil {
		t.Fatal(err)
	}
	if mismatch.ImportStatus != "needs_correction" {
		t.Fatalf("mismatched total must need correction: %#v", mismatch)
	}
	if included, _ := likelyProduct(rows[5]); included {
		t.Fatal("total row must not be included by default")
	}
	if included, _ := likelyProduct([]string{}); included {
		t.Fatal("empty row must not be included by default")
	}
	pictures, _ := pictureHints(opened, "Товары")
	if len(pictures) != 1 || !pictures[0].Supported || pictures[0].SourceRow != 3 {
		t.Fatalf("unexpected picture hints: %#v", pictures)
	}
	store, err := repository.NewFileStore(filepath.Join(temporary, "private"))
	if err != nil {
		t.Fatal(err)
	}
	assets, links, warnings := (&ImportService{files: store}).extractPictures(opened, "Товары", []repository.ImportedProduct{{SourceRow: 3}})
	if len(assets) != 1 || len(links[0]) != 1 || len(warnings[0]) != 0 {
		t.Fatalf("expected an attached image, assets=%d links=%#v warnings=%#v", len(assets), links, warnings)
	}
	firstHash := workbookSHA256(t, path)
	secondHash := workbookSHA256(t, path)
	if firstHash != secondHash {
		t.Fatalf("same synthetic XLSX must have a stable duplicate fingerprint: %s != %s", firstHash, secondHash)
	}
}

func TestStrictDecimalParsingRejectsAmbiguousSeparators(t *testing.T) {
	if _, err := parseDecimalStrict("1,000"); err == nil {
		t.Fatal("1,000 must require manual correction")
	}
	value, err := parseDecimalStrict("1 234,50")
	if err != nil || value.String() != "1234.5" {
		t.Fatalf("expected 1234.5, got %s, %v", value.String(), err)
	}
	for _, input := range []string{"0.041", "0,041"} {
		value, err := parseDecimalStrict(input)
		if err != nil || value.String() != "0.041" {
			t.Fatalf("expected %s to parse as 0.041, got %s, %v", input, value.String(), err)
		}
	}
}

func workbookSHA256(t *testing.T, path string) string {
	t.Helper()
	contents, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(contents)
	return hex.EncodeToString(sum[:])
}
