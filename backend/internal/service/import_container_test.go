package service

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestInspectXLSXContainerUsesActualFileSize(t *testing.T) {
	path := filepath.Join(t.TempDir(), "small.xlsx")
	workbook := excelize.NewFile()
	defer workbook.Close()
	if err := workbook.SetCellValue("Sheet1", "A1", "Тест"); err != nil {
		t.Fatal(err)
	}
	if err := workbook.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if err := inspectXLSXContainer(file); err != nil {
		t.Fatalf("a valid XLSX smaller than the limit must pass inspection: %v", err)
	}
}
