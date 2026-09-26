package service

import (
	"bytes"
	"testing"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"
)

func TestTNVEDExportRowImportsBack(t *testing.T) {
	percent := decimal.RequireFromString("12")
	vat := decimal.RequireFromString("16")
	coefficient := decimal.RequireFromString("0.0689")
	entry := repository.TNVEDDirectoryEntry{
		Code: "0101210000", Name: "Лошади", Category: "Товары", DutyType: "percentage", DutyPercent: &percent,
		VATType: "percentage", VATPercent: &vat, CompanyCoefficient: &coefficient, VerificationStatus: "operator_verified",
		SynonymsRU: repository.JSON(`["конь","лошадь"]`), SynonymsZH: repository.JSON(`[]`), SynonymsEN: repository.JSON(`[]`),
	}
	input := tnvedInputFromEntry(entry)
	row := []string{
		entry.Code, entry.Name, "", entry.Category, "процент", "12", "", "", "", "2026-01-01", "", "", "",
		"процент", "16", "", "", "", "", "", "", "0.0689", "конь; лошадь", "", "", "Проверено оператором",
	}
	data, err := writeDirectoryXLSX("ТН ВЭД", tnvedColumns, [][]string{row})
	if err != nil {
		t.Fatal(err)
	}
	sheet, err := readDirectorySheet(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := tnvedInputFromRow(sheet, 0, TNVEDDirectoryInput{Duty: RateInput{Type: "unknown"}, VAT: RateInput{Type: "unknown"}})
	if err != nil {
		t.Fatal(err)
	}
	parsed.Category, parsed.Name = "Товары", "Лошади"
	if _, err := validateDirectoryInput(parsed); err != nil {
		t.Fatalf("exported row must import back: %v", err)
	}
	if parsed.Code != input.Code || parsed.Duty.Percent != "12" || parsed.VAT.Percent != "16" || parsed.CompanyCoefficient != "0.0689" ||
		parsed.Duty.ValidFrom != "2026-01-01" || parsed.VerificationStatus != "operator_verified" || len(parsed.SynonymsRU) != 2 {
		t.Fatalf("parsed = %+v", parsed)
	}
}

// A file typed in Excel: code as a number (leading zero lost), 12% as a percent cell, a real date.
func TestTNVEDImportReadsExcelTypedValues(t *testing.T) {
	file := excelize.NewFile()
	sheet := "Sheet1"
	for index, header := range []string{"Код ТН ВЭД", "Пошлина, %", "Пошлина: действует с", "Статус"} {
		cell, _ := excelize.CoordinatesToCellName(index+1, 1)
		_ = file.SetCellStr(sheet, cell, header)
	}
	_ = file.SetCellInt(sheet, "A2", 101210000)
	_ = file.SetCellFloat(sheet, "B2", 0.12, -1, 64)
	percentStyle, _ := file.NewStyle(&excelize.Style{NumFmt: 9})
	_ = file.SetCellStyle(sheet, "B2", "B2", percentStyle)
	_ = file.SetCellInt(sheet, "C2", 46023) // 2026-01-01
	dateStyle, _ := file.NewStyle(&excelize.Style{NumFmt: 14})
	_ = file.SetCellStyle(sheet, "C2", "C2", dateStyle)
	_ = file.SetCellStr(sheet, "D2", "да")
	var buffer bytes.Buffer
	if err := file.Write(&buffer); err != nil {
		t.Fatal(err)
	}
	parsedSheet, err := readDirectorySheet(bytes.NewReader(buffer.Bytes()), int64(buffer.Len()))
	if err != nil {
		t.Fatal(err)
	}
	input, err := tnvedInputFromRow(parsedSheet, 0, TNVEDDirectoryInput{Duty: RateInput{Type: "unknown"}, VAT: RateInput{Type: "unknown"}})
	if err != nil {
		t.Fatal(err)
	}
	if input.Code != "0101210000" || input.Duty.Type != "percentage" || input.Duty.Percent != "12" || input.Duty.ValidFrom != "2026-01-01" || input.VerificationStatus != "operator_verified" {
		t.Fatalf("input = %+v", input)
	}
}

func TestDirectoryCellParsers(t *testing.T) {
	for value, want := range map[string]string{"": "unknown", "Процент": "percentage", "за единицу": "specific", "освобождение": "exempt"} {
		if got, ok := rateTypeFromCell(value); !ok || got != want {
			t.Errorf("rateTypeFromCell(%q) = %q, %v", value, got, ok)
		}
	}
	if _, ok := rateTypeFromCell("какой-то"); ok {
		t.Error("unknown rate type must be rejected")
	}
	for value, want := range map[string]string{"31.01.2026": "2026-01-31", "2026-01-31": "2026-01-31", "46053": "2026-01-31"} {
		if got, err := cellDate(value); err != nil || got != want {
			t.Errorf("cellDate(%q) = %q, %v", value, got, err)
		}
	}
	if _, err := cellDate("вчера"); err == nil {
		t.Error("text date must be rejected")
	}
}
