package directory

// Export and import of the product directory as XLSX, so that a filled directory can be moved
// between servers or edited in Excel instead of card by card.
//
// Export writes the active (not archived) entries. Import matches rows with active entries by
// category + name, updates matches and creates the rest. The whole file is checked first: one
// invalid row and nothing is saved. A column missing from the file keeps the stored value.

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/xuri/excelize/v2"
)

const (
	maxDirectoryImportRows  = 5000
	maxDirectoryImportBytes = 20 << 20
	maxDirectoryImportErrs  = 50
)

// DirectoryImportResult reports an import; with Errors nothing was saved.
type DirectoryImportResult struct {
	Created int                    `json:"created"`
	Updated int                    `json:"updated"`
	Skipped int                    `json:"skipped"`
	Errors  []DirectoryImportError `json:"errors"`
}

type DirectoryImportError struct {
	Row     int    `json:"row"`
	Message string `json:"message"`
}

// ── Values in cells ──

func statusLabel(status string) string {
	if status == "operator_verified" {
		return "Проверено оператором"
	}
	return "Требует проверки"
}

func statusFromCell(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch {
	case value == "" || strings.HasPrefix(value, "требует") || value == "needs_review" || value == "нет":
		return "needs_review", true
	case strings.HasPrefix(value, "проверен") || value == "operator_verified" || value == "да":
		return "operator_verified", true
	}
	return "", false
}

func joinSynonyms(raw repository.JSON) string { return strings.Join(decodeJSONStrings(raw), "; ") }

func splitSynonyms(value string) []string {
	return strings.FieldsFunc(value, func(r rune) bool { return r == ';' || r == '\n' || r == ',' })
}

// cellDate reads a date typed as text (2026-01-31, 31.01.2026) or stored by Excel as a serial
// number, and returns it as YYYY-MM-DD.
func cellDate(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	for _, layout := range []string{"2006-01-02", "02.01.2006", "2.1.2006", "02/01/2006", "2006/01/02"} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed.Format("2006-01-02"), nil
		}
	}
	if serial, err := strconv.ParseFloat(value, 64); err == nil && serial > 1 && serial < 100000 {
		base := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
		return base.AddDate(0, 0, int(math.Floor(serial))).Format("2006-01-02"), nil
	}
	return "", fmt.Errorf("дата «%s» не распознана — используйте ГГГГ-ММ-ДД или ДД.ММ.ГГГГ", value)
}

// cellNumber turns a number cell into text the directory parser accepts: 12, 12.5, «12,5 %».
func cellNumber(value string) string {
	value = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(value), "%"))
	return strings.ReplaceAll(strings.ReplaceAll(value, " ", ""), ",", ".")
}

// ── XLSX ──

type directoryColumn struct {
	header string
	width  float64
}

func writeDirectoryXLSX(sheet string, columns []directoryColumn, rows [][]string) ([]byte, error) {
	file := excelize.NewFile()
	defer file.Close()
	if err := file.SetSheetName("Sheet1", sheet); err != nil {
		return nil, err
	}
	headerStyle, _ := file.NewStyle(&excelize.Style{Font: &excelize.Font{Bold: true}, Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"E8F3EC"}}})
	textStyle, _ := file.NewStyle(&excelize.Style{NumFmt: 49, Alignment: &excelize.Alignment{Vertical: "top", WrapText: true}})
	for index, column := range columns {
		cell, _ := excelize.CoordinatesToCellName(index+1, 1)
		_ = file.SetCellStr(sheet, cell, column.header)
		name, _ := excelize.ColumnNumberToName(index + 1)
		_ = file.SetColWidth(sheet, name, name, column.width)
	}
	last, _ := excelize.CoordinatesToCellName(len(columns), 1)
	_ = file.SetCellStyle(sheet, "A1", last, headerStyle)
	for rowIndex, row := range rows {
		for colIndex, value := range row {
			cell, _ := excelize.CoordinatesToCellName(colIndex+1, rowIndex+2)
			// Text cells: codes keep leading zeros, dates and numbers are not reformatted by Excel.
			_ = file.SetCellStr(sheet, cell, value)
		}
	}
	if len(rows) > 0 {
		lastCell, _ := excelize.CoordinatesToCellName(len(columns), len(rows)+1)
		_ = file.SetCellStyle(sheet, "A2", lastCell, textStyle)
	}
	_ = file.SetPanes(sheet, &excelize.Panes{Freeze: true, YSplit: 1, TopLeftCell: "A2", ActivePane: "bottomLeft"})
	var buffer bytes.Buffer
	if err := file.Write(&buffer); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// directorySheet is an uploaded sheet: column index by normalized header and the data rows as
// shown in Excel (rows) and as stored (raw: dates as serial numbers, 12% as 0.12, unrounded numbers).
type directorySheet struct {
	columns map[string]int
	rows    [][]string
	raw     [][]string
	first   int // Excel row number of rows[0]
}

func normalizeHeader(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.ReplaceAll(value, "ё", "е")), " "))
}

func readDirectorySheet(file io.Reader, size int64) (directorySheet, error) {
	if size > maxDirectoryImportBytes {
		return directorySheet{}, &service.ValidationError{Field: "file", Message: fmt.Sprintf("Файл больше %d МБ", maxDirectoryImportBytes>>20)}
	}
	workbook, err := excelize.OpenReader(file)
	if err != nil {
		return directorySheet{}, &service.ValidationError{Field: "file", Message: "Файл повреждён или не является XLSX"}
	}
	defer workbook.Close()
	sheets := workbook.GetSheetList()
	if len(sheets) == 0 {
		return directorySheet{}, &service.ValidationError{Field: "file", Message: "В файле нет листов"}
	}
	rows, err := workbook.GetRows(sheets[0])
	if err != nil {
		return directorySheet{}, &service.ValidationError{Field: "file", Message: "Не удалось прочитать первый лист"}
	}
	raw, err := workbook.GetRows(sheets[0], excelize.Options{RawCellValue: true})
	if err != nil || len(raw) != len(rows) {
		return directorySheet{}, &service.ValidationError{Field: "file", Message: "Не удалось прочитать первый лист"}
	}
	if len(rows) == 0 {
		return directorySheet{}, &service.ValidationError{Field: "file", Message: "Первый лист пустой"}
	}
	if len(rows)-1 > maxDirectoryImportRows {
		return directorySheet{}, &service.ValidationError{Field: "file", Message: fmt.Sprintf("Не больше %d строк за раз", maxDirectoryImportRows)}
	}
	sheet := directorySheet{columns: map[string]int{}, rows: rows[1:], raw: raw[1:], first: 2}
	for index, header := range rows[0] {
		if key := normalizeHeader(header); key != "" {
			if _, exists := sheet.columns[key]; !exists {
				sheet.columns[key] = index
			}
		}
	}
	return sheet, nil
}

func (s directorySheet) has(header string) bool {
	_, ok := s.columns[normalizeHeader(header)]
	return ok
}

// cell returns the value of a column in data row i as Excel shows it; present is false when the
// file has no such column.
func (s directorySheet) cell(i int, header string) (value string, present bool) {
	return s.at(s.rows, i, header)
}

func (s directorySheet) at(rows [][]string, i int, header string) (string, bool) {
	index, ok := s.columns[normalizeHeader(header)]
	if !ok {
		return "", false
	}
	if i < len(rows) && index < len(rows[i]) {
		return strings.TrimSpace(rows[i][index]), true
	}
	return "", true
}

// number reads a number column: «12%» as shown becomes 12 (stored 0.12), other numbers are taken
// as stored so that the cell format does not round them.
func (s directorySheet) number(i int, header string) (string, bool) {
	shown, ok := s.cell(i, header)
	if !ok || shown == "" || strings.Contains(shown, "%") {
		return cellNumber(shown), ok
	}
	stored, _ := s.at(s.raw, i, header)
	if _, err := strconv.ParseFloat(stored, 64); err == nil {
		return cellNumber(stored), true
	}
	return cellNumber(shown), true
}

// date reads a date column: text as typed or Excel's stored serial number.
func (s directorySheet) date(i int, header string) (string, bool, error) {
	stored, ok := s.at(s.raw, i, header)
	if !ok {
		return "", false, nil
	}
	value, err := cellDate(stored)
	return value, true, err
}

// code reads a TN VED code without spaces and dots; a code typed as a number loses its leading
// zero in Excel (0101210000 → 101210000), so nine digits get it back.
func (s directorySheet) code(i int, header string) (string, bool) {
	value, ok := s.cell(i, header)
	value = strings.NewReplacer(" ", "", ".", "").Replace(value)
	if len(value) == 9 && strings.Trim(value, "0123456789") == "" {
		value = "0" + value
	}
	return value, ok
}

func emptyRow(row []string) bool {
	for _, value := range row {
		if strings.TrimSpace(value) != "" {
			return false
		}
	}
	return true
}

func (r *DirectoryImportResult) fail(row int, format string, args ...any) {
	if len(r.Errors) < maxDirectoryImportErrs {
		r.Errors = append(r.Errors, DirectoryImportError{Row: row, Message: fmt.Sprintf(format, args...)})
	}
}

func validationMessage(err error) string {
	if validation, ok := err.(*service.ValidationError); ok {
		return validation.Message
	}
	return err.Error()
}

// ── Products ──

var productColumns = []directoryColumn{
	{"Наименование", 40}, {"Наименование (кит.)", 24}, {"Категория", 14}, {"Ед. изм.", 10},
	{"Код ТН ВЭД", 14}, {"ИТС компании", 13},
	{"Синонимы (рус.)", 24}, {"Синонимы (кит.)", 20}, {"Синонимы (англ.)", 20}, {"Статус", 20},
}

func productKey(category, name string) string {
	return strings.ToLower(strings.TrimSpace(category)) + "|" + strings.ToLower(strings.TrimSpace(name))
}

func productInputFromEntry(entry repository.ProductDirectoryEntry) ProductDirectoryInput {
	return ProductDirectoryInput{
		Category: entry.Category, Name: entry.Name, NameZH: entry.NameZH, Unit: entry.Unit,
		CompanyCoefficient: service.DecimalText(entry.CompanyCoefficient),
		SynonymsRU:         decodeJSONStrings(entry.SynonymsRU), SynonymsZH: decodeJSONStrings(entry.SynonymsZH), SynonymsEN: decodeJSONStrings(entry.SynonymsEN),
		Code: entry.Code, VerificationStatus: entry.VerificationStatus,
	}
}

func (s *ProductDirectoryService) Export(ctx context.Context) ([]byte, error) {
	entries, err := s.repository.ListProductDirectory(ctx, "", "", false)
	if err != nil {
		return nil, err
	}
	rows := make([][]string, 0, len(entries))
	for _, entry := range entries {
		rows = append(rows, []string{
			entry.Name, entry.NameZH, entry.Category, entry.Unit, entry.Code, service.DecimalText(entry.CompanyCoefficient),
			joinSynonyms(entry.SynonymsRU), joinSynonyms(entry.SynonymsZH), joinSynonyms(entry.SynonymsEN), statusLabel(entry.VerificationStatus),
		})
	}
	return writeDirectoryXLSX("Товары", productColumns, rows)
}

// Import loads product cards. «Код ТН ВЭД» must be a code of the EAEU nomenclature.
func (s *ProductDirectoryService) Import(ctx context.Context, userID int64, file io.Reader, size int64, update bool) (DirectoryImportResult, error) {
	result := DirectoryImportResult{Errors: []DirectoryImportError{}}
	sheet, err := readDirectorySheet(file, size)
	if err != nil {
		return result, err
	}
	if !sheet.has("Наименование") {
		return result, &service.ValidationError{Field: "file", Message: "В первой строке нет колонки «Наименование». Выгрузите справочник и заполните файл по его образцу"}
	}
	existing, err := s.repository.ListProductDirectory(ctx, "", "", false)
	if err != nil {
		return result, err
	}
	byKey := map[string]repository.ProductDirectoryEntry{}
	for _, entry := range existing {
		byKey[productKey(entry.Category, entry.Name)] = entry
	}
	nomenclature, err := s.repository.TNVEDNomenclatureCodeSet(ctx)
	if err != nil {
		return result, err
	}

	type planned struct {
		id    int64
		input ProductDirectoryInput
	}
	plan := []planned{}
	seen := map[string]int{}
	for i, row := range sheet.rows {
		rowNumber := sheet.first + i
		if emptyRow(row) {
			continue
		}
		name, _ := sheet.cell(i, "Наименование")
		category, hasCategory := sheet.cell(i, "Категория")
		if !hasCategory || category == "" {
			category = "Товары"
		}
		if name == "" {
			result.fail(rowNumber, "нет наименования")
			continue
		}
		key := productKey(category, name)
		if first, ok := seen[key]; ok {
			result.fail(rowNumber, "товар «%s» уже есть в строке %d", name, first)
			continue
		}
		seen[key] = rowNumber
		current, found := byKey[key]
		if found && !update {
			result.Skipped++
			continue
		}
		input := ProductDirectoryInput{Category: category, VerificationStatus: "needs_review"}
		if found {
			input = productInputFromEntry(current)
		}
		input.Category, input.Name = category, name
		for header, target := range map[string]*string{"Наименование (кит.)": &input.NameZH, "Ед. изм.": &input.Unit} {
			if value, ok := sheet.cell(i, header); ok {
				*target = value
			}
		}
		if value, ok := sheet.number(i, "ИТС компании"); ok {
			input.CompanyCoefficient = value
		}
		for header, target := range map[string]*[]string{"Синонимы (рус.)": &input.SynonymsRU, "Синонимы (кит.)": &input.SynonymsZH, "Синонимы (англ.)": &input.SynonymsEN} {
			if value, ok := sheet.cell(i, header); ok {
				*target = splitSynonyms(value)
			}
		}
		if value, ok := sheet.cell(i, "Статус"); ok {
			status, valid := statusFromCell(value)
			if !valid {
				result.fail(rowNumber, "статус «%s» не распознан — «Проверено оператором» или «Требует проверки»", value)
				continue
			}
			input.VerificationStatus = status
		}
		if code, ok := sheet.code(i, "Код ТН ВЭД"); ok {
			input.Code = code
		}
		if _, err := validateProductDirectoryInput(input); err != nil {
			result.fail(rowNumber, "%s", validationMessage(err))
			continue
		}
		if err := checkNomenclatureCode(nomenclature, input.Code); err != nil {
			result.fail(rowNumber, "%s", validationMessage(err))
			continue
		}
		plan = append(plan, planned{id: current.ID, input: input})
	}
	if len(result.Errors) > 0 {
		return result, nil
	}
	for _, item := range plan {
		if item.id != 0 {
			if _, err := s.Update(ctx, item.id, userID, item.input); err != nil {
				return result, err
			}
			result.Updated++
			continue
		}
		if _, err := s.Create(ctx, userID, item.input); err != nil {
			return result, err
		}
		result.Created++
	}
	return result, nil
}
