package service

import (
	"path/filepath"
	"testing"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/xuri/excelize/v2"
)

func TestInspectXLSXContainerUsesActualFileSize(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "small.xlsx")
	workbook := excelize.NewFile()
	defer workbook.Close()
	if err := workbook.SetCellValue("Sheet1", "A1", "Тест"); err != nil {
		t.Fatal(err)
	}
	if err := workbook.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	store, err := repository.NewFileStore(dir)
	if err != nil {
		t.Fatal(err)
	}
	file, err := store.Open("small.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := inspectXLSXContainer(file); err != nil {
		t.Fatalf("a valid XLSX smaller than the limit must pass inspection: %v", err)
	}
}
