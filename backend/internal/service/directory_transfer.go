package service

// Export and import of the TN VED and product directories as XLSX, so that a filled
// directory can be moved between servers or edited in Excel instead of card by card.
//
// Export writes the active (not archived) entries. Import matches rows with active entries
// by the directory's unique key — TN VED: code + category + description, product: category +
// name — updates matches and creates the rest. The whole file is checked first: one invalid
// row and nothing is saved. A column missing from the file keeps the stored value.

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

var rateTypeLabels = map[string]string{
	"unknown": "не указана", "percentage": "процент", "specific": "за единицу",
	"combined_minimum": "комбинированная", "exempt": "освобождение",
}

func rateTypeFromCell(value string) (string, bool) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "", "не указана", "не указан", "неизвестно", "нет", "unknown":
		return "unknown", true
	case "процент", "процентная", "%", "percentage", "percent":
		return "percentage", true
	case "за единицу", "специфическая", "specific":
		return "specific", true
	case "комбинированная", "combined", "combined_minimum":
		return "combined_minimum", true
	case "освобождение", "освобождён", "освобожден", "exempt":
		return "exempt", true
	}
	return "", false
}

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
		return directorySheet{}, &ValidationError{Field: "file", Message: fmt.Sprintf("Файл больше %d МБ", maxDirectoryImportBytes>>20)}
	}
	workbook, err := excelize.OpenReader(file)
	if err != nil {
		return directorySheet{}, &ValidationError{Field: "file", Message: "Файл повреждён или не является XLSX"}
	}
	defer workbook.Close()
	sheets := workbook.GetSheetList()
	if len(sheets) == 0 {
		return directorySheet{}, &ValidationError{Field: "file", Message: "В файле нет листов"}
	}
	rows, err := workbook.GetRows(sheets[0])
	if err != nil {
		return directorySheet{}, &ValidationError{Field: "file", Message: "Не удалось прочитать первый лист"}
	}
	raw, err := workbook.GetRows(sheets[0], excelize.Options{RawCellValue: true})
	if err != nil || len(raw) != len(rows) {
		return directorySheet{}, &ValidationError{Field: "file", Message: "Не удалось прочитать первый лист"}
	}
	if len(rows) == 0 {
		return directorySheet{}, &ValidationError{Field: "file", Message: "Первый лист пустой"}
	}
	if len(rows)-1 > maxDirectoryImportRows {
		return directorySheet{}, &ValidationError{Field: "file", Message: fmt.Sprintf("Не больше %d строк за раз", maxDirectoryImportRows)}
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
	if validation, ok := err.(*ValidationError); ok {
		return validation.Message
	}
	return err.Error()
}

// ── TN VED ──

var tnvedColumns = []directoryColumn{
	{"Код ТН ВЭД", 14}, {"Описание", 40}, {"Описание (кит.)", 24}, {"Категория", 14},
	{"Пошлина: тип", 16}, {"Пошлина, %", 11}, {"Пошлина: сумма", 13}, {"Пошлина: валюта", 12}, {"Пошлина: единица", 14},
	{"Пошлина: действует с", 14}, {"Пошлина: действует по", 14}, {"Пошлина: проверено", 14}, {"Пошлина: источник", 24},
	{"НДС: тип", 14}, {"НДС, %", 9}, {"НДС: основание освобождения", 24},
	{"НДС: действует с", 14}, {"НДС: действует по", 14}, {"НДС: проверено", 14}, {"НДС: источник", 24},
	{"Антидемпинг, %", 13}, {"ИТС компании", 13},
	{"Синонимы (рус.)", 24}, {"Синонимы (кит.)", 20}, {"Синонимы (англ.)", 20}, {"Статус", 20},
}

func tnvedKey(code, category, name string) string {
	return strings.TrimSpace(code) + "|" + strings.ToLower(strings.TrimSpace(category)) + "|" + strings.ToLower(strings.TrimSpace(name))
}

func tnvedInputFromEntry(entry repository.TNVEDDirectoryEntry) TNVEDDirectoryInput {
	return TNVEDDirectoryInput{
		Category: entry.Category, Name: entry.Name, NameZH: entry.NameZH, Code: entry.Code,
		CompanyCoefficient: decimalString(entry.CompanyCoefficient), AntiDumpingPercent: decimalString(entry.AntiDumpingPercent),
		SynonymsRU: decodeJSONStrings(entry.SynonymsRU), SynonymsZH: decodeJSONStrings(entry.SynonymsZH), SynonymsEN: decodeJSONStrings(entry.SynonymsEN),
		VerificationStatus: entry.VerificationStatus,
		Duty: RateInput{Type: entry.DutyType, Percent: decimalString(entry.DutyPercent), Amount: decimalString(entry.DutyAmount), Currency: entry.DutyCurrency, Unit: entry.DutyUnit,
			ValidFrom: dateString(entry.DutyValidFrom), ValidTo: dateString(entry.DutyValidTo), CheckedAt: dateString(entry.DutyCheckedAt), Source: entry.DutySource},
		VAT: RateInput{Type: entry.VATType, Percent: decimalString(entry.VATPercent), Basis: entry.VATBasis,
			ValidFrom: dateString(entry.VATValidFrom), ValidTo: dateString(entry.VATValidTo), CheckedAt: dateString(entry.VATCheckedAt), Source: entry.VATSource},
	}
}

func (s *TNVEDDirectoryService) Export(ctx context.Context) ([]byte, error) {
	entries, err := s.repository.ListTNVEDDirectory(ctx, "", "", false)
	if err != nil {
		return nil, err
	}
	rows := make([][]string, 0, len(entries))
	for _, entry := range entries {
		rows = append(rows, []string{
			entry.Code, entry.Name, entry.NameZH, entry.Category,
			rateTypeLabels[entry.DutyType], decimalString(entry.DutyPercent), decimalString(entry.DutyAmount), entry.DutyCurrency, entry.DutyUnit,
			dateString(entry.DutyValidFrom), dateString(entry.DutyValidTo), dateString(entry.DutyCheckedAt), entry.DutySource,
			rateTypeLabels[entry.VATType], decimalString(entry.VATPercent), entry.VATBasis,
			dateString(entry.VATValidFrom), dateString(entry.VATValidTo), dateString(entry.VATCheckedAt), entry.VATSource,
			decimalString(entry.AntiDumpingPercent), decimalString(entry.CompanyCoefficient),
			joinSynonyms(entry.SynonymsRU), joinSynonyms(entry.SynonymsZH), joinSynonyms(entry.SynonymsEN), statusLabel(entry.VerificationStatus),
		})
	}
	return writeDirectoryXLSX("ТН ВЭД", tnvedColumns, rows)
}

// applyRateCells overlays the rate columns present in the file on a rate.
func applyRateCells(sheet directorySheet, i int, prefix string, rate *RateInput, withAmount bool) error {
	if value, ok := sheet.cell(i, prefix+": тип"); ok {
		rateType, valid := rateTypeFromCell(value)
		if !valid {
			return fmt.Errorf("%s: тип «%s» не распознан — процент, за единицу, комбинированная, освобождение или пусто", prefix, value)
		}
		rate.Type = rateType
	}
	if value, ok := sheet.number(i, prefix+", %"); ok {
		rate.Percent, rate.Value = value, ""
		if rate.Percent != "" && (rate.Type == "" || rate.Type == "unknown") {
			rate.Type = "percentage"
		}
	}
	if withAmount {
		if value, ok := sheet.number(i, prefix+": сумма"); ok {
			rate.Amount = value
		}
		if value, ok := sheet.cell(i, prefix+": валюта"); ok {
			rate.Currency = strings.ToUpper(value)
		}
		if value, ok := sheet.cell(i, prefix+": единица"); ok {
			rate.Unit = value
		}
	} else if value, ok := sheet.cell(i, prefix+": основание освобождения"); ok {
		rate.Basis = value
	}
	for header, target := range map[string]*string{": действует с": &rate.ValidFrom, ": действует по": &rate.ValidTo, ": проверено": &rate.CheckedAt} {
		date, ok, err := sheet.date(i, prefix+header)
		if err != nil {
			return fmt.Errorf("%s%s: %s", prefix, header, err.Error())
		}
		if ok {
			*target = date
		}
	}
	if value, ok := sheet.cell(i, prefix+": источник"); ok {
		rate.Source = value
	}
	// Fields that do not belong to the rate type are cleared, otherwise validation rejects them.
	switch rate.Type {
	case "unknown", "exempt":
		rate.Percent, rate.Value, rate.Amount, rate.Currency, rate.Unit = "", "", "", "", ""
	case "percentage":
		rate.Amount, rate.Currency, rate.Unit = "", "", ""
	case "specific":
		rate.Percent, rate.Value = "", ""
	}
	if rate.Type != "exempt" {
		rate.Basis = ""
	}
	return nil
}

func tnvedInputFromRow(sheet directorySheet, i int, base TNVEDDirectoryInput) (TNVEDDirectoryInput, error) {
	input := base
	if value, ok := sheet.code(i, "Код ТН ВЭД"); ok {
		input.Code = value
	}
	if value, ok := sheet.cell(i, "Описание (кит.)"); ok {
		input.NameZH = value
	}
	if value, ok := sheet.number(i, "Антидемпинг, %"); ok {
		input.AntiDumpingPercent = value
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
			return input, fmt.Errorf("статус «%s» не распознан — «Проверено оператором» или «Требует проверки»", value)
		}
		input.VerificationStatus = status
	}
	if err := applyRateCells(sheet, i, "Пошлина", &input.Duty, true); err != nil {
		return input, err
	}
	if err := applyRateCells(sheet, i, "НДС", &input.VAT, false); err != nil {
		return input, err
	}
	return input, nil
}

// Import loads TN VED cards; update=false only adds new codes and skips existing ones.
func (s *TNVEDDirectoryService) Import(ctx context.Context, userID int64, file io.Reader, size int64, update bool) (DirectoryImportResult, error) {
	result := DirectoryImportResult{Errors: []DirectoryImportError{}}
	sheet, err := readDirectorySheet(file, size)
	if err != nil {
		return result, err
	}
	if !sheet.has("Код ТН ВЭД") {
		return result, &ValidationError{Field: "file", Message: "В первой строке нет колонки «Код ТН ВЭД». Выгрузите справочник и заполните файл по его образцу"}
	}
	existing, err := s.repository.ListTNVEDDirectory(ctx, "", "", false)
	if err != nil {
		return result, err
	}
	byKey := map[string]repository.TNVEDDirectoryEntry{}
	for _, entry := range existing {
		byKey[tnvedKey(entry.Code, entry.Category, entry.Name)] = entry
	}

	type planned struct {
		id    int64
		input TNVEDDirectoryInput
	}
	plan := []planned{}
	seen := map[string]int{}
	for i, row := range sheet.rows {
		rowNumber := sheet.first + i
		if emptyRow(row) {
			continue
		}
		// The key comes from the file: code + category + description (description defaults to the code).
		code, _ := sheet.code(i, "Код ТН ВЭД")
		category, hasCategory := sheet.cell(i, "Категория")
		if !hasCategory || category == "" {
			category = "Товары"
		}
		name, _ := sheet.cell(i, "Описание")
		if name == "" {
			name = code
		}
		key := tnvedKey(code, category, name)
		if first, ok := seen[key]; ok {
			result.fail(rowNumber, "код %s с тем же описанием и категорией уже есть в строке %d", code, first)
			continue
		}
		seen[key] = rowNumber
		current, found := byKey[key]
		if found && !update {
			result.Skipped++
			continue
		}
		base := TNVEDDirectoryInput{Category: category, VerificationStatus: "needs_review", Duty: RateInput{Type: "unknown"}, VAT: RateInput{Type: "unknown"}}
		if found {
			base = tnvedInputFromEntry(current)
		}
		input, err := tnvedInputFromRow(sheet, i, base)
		if err == nil {
			input.Category, input.Name = category, name
			_, err = validateDirectoryInput(input)
		}
		if err != nil {
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

// ── Products ──

var productColumns = []directoryColumn{
	{"Наименование", 40}, {"Наименование (кит.)", 24}, {"Категория", 14}, {"Ед. изм.", 10},
	{"Код ТН ВЭД", 14}, {"Описание ТН ВЭД", 30}, {"ИТС компании", 13},
	{"Синонимы (рус.)", 24}, {"Синонимы (кит.)", 20}, {"Синонимы (англ.)", 20}, {"Статус", 20},
}

func productKey(category, name string) string {
	return strings.ToLower(strings.TrimSpace(category)) + "|" + strings.ToLower(strings.TrimSpace(name))
}

func productInputFromEntry(entry repository.ProductDirectoryEntry) ProductDirectoryInput {
	return ProductDirectoryInput{
		Category: entry.Category, Name: entry.Name, NameZH: entry.NameZH, Unit: entry.Unit,
		CompanyCoefficient: decimalString(entry.CompanyCoefficient),
		SynonymsRU:         decodeJSONStrings(entry.SynonymsRU), SynonymsZH: decodeJSONStrings(entry.SynonymsZH), SynonymsEN: decodeJSONStrings(entry.SynonymsEN),
		TNVEDDirectoryEntryID: entry.TNVEDDirectoryEntryID, VerificationStatus: entry.VerificationStatus,
	}
}

func (s *ProductDirectoryService) Export(ctx context.Context) ([]byte, error) {
	entries, err := s.repository.ListProductDirectory(ctx, "", "", false)
	if err != nil {
		return nil, err
	}
	rows := make([][]string, 0, len(entries))
	for _, entry := range entries {
		code, tnvedName := "", ""
		if entry.TNVEDDirectoryEntry != nil {
			code, tnvedName = entry.TNVEDDirectoryEntry.Code, entry.TNVEDDirectoryEntry.Name
		}
		rows = append(rows, []string{
			entry.Name, entry.NameZH, entry.Category, entry.Unit, code, tnvedName, decimalString(entry.CompanyCoefficient),
			joinSynonyms(entry.SynonymsRU), joinSynonyms(entry.SynonymsZH), joinSynonyms(entry.SynonymsEN), statusLabel(entry.VerificationStatus),
		})
	}
	return writeDirectoryXLSX("Товары", productColumns, rows)
}

// Import loads product cards. The TN VED card is found by its code among active cards; when
// one code has several cards, «Описание ТН ВЭД» (or the product category) picks one.
func (s *ProductDirectoryService) Import(ctx context.Context, userID int64, file io.Reader, size int64, update bool) (DirectoryImportResult, error) {
	result := DirectoryImportResult{Errors: []DirectoryImportError{}}
	sheet, err := readDirectorySheet(file, size)
	if err != nil {
		return result, err
	}
	if !sheet.has("Наименование") {
		return result, &ValidationError{Field: "file", Message: "В первой строке нет колонки «Наименование». Выгрузите справочник и заполните файл по его образцу"}
	}
	existing, err := s.repository.ListProductDirectory(ctx, "", "", false)
	if err != nil {
		return result, err
	}
	byKey := map[string]repository.ProductDirectoryEntry{}
	for _, entry := range existing {
		byKey[productKey(entry.Category, entry.Name)] = entry
	}
	tnvedEntries, err := s.repository.ListTNVEDDirectory(ctx, "", "", false)
	if err != nil {
		return result, err
	}
	tnvedByCode := map[string][]repository.TNVEDDirectoryEntry{}
	for _, entry := range tnvedEntries {
		tnvedByCode[entry.Code] = append(tnvedByCode[entry.Code], entry)
	}
	findTNVED := func(code, tnvedName, category string) (*int64, error) {
		candidates := tnvedByCode[code]
		if len(candidates) == 0 {
			return nil, fmt.Errorf("кода ТН ВЭД %s нет в справочнике ТН ВЭД — сначала загрузите или добавьте его", code)
		}
		narrow := func(keep func(repository.TNVEDDirectoryEntry) bool) {
			filtered := []repository.TNVEDDirectoryEntry{}
			for _, entry := range candidates {
				if keep(entry) {
					filtered = append(filtered, entry)
				}
			}
			if len(filtered) > 0 {
				candidates = filtered
			}
		}
		if len(candidates) > 1 && tnvedName != "" {
			narrow(func(entry repository.TNVEDDirectoryEntry) bool {
				return strings.EqualFold(strings.TrimSpace(entry.Name), tnvedName)
			})
		}
		if len(candidates) > 1 {
			narrow(func(entry repository.TNVEDDirectoryEntry) bool { return strings.EqualFold(entry.Category, category) })
		}
		if len(candidates) > 1 {
			return nil, fmt.Errorf("в справочнике несколько карточек с кодом %s — укажите «Описание ТН ВЭД»", code)
		}
		id := candidates[0].ID
		return &id, nil
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
			if code == "" {
				input.TNVEDDirectoryEntryID = nil
			} else {
				tnvedName, _ := sheet.cell(i, "Описание ТН ВЭД")
				id, err := findTNVED(code, tnvedName, category)
				if err != nil {
					result.fail(rowNumber, "%s", err.Error())
					continue
				}
				input.TNVEDDirectoryEntryID = id
			}
		}
		if _, err := validateProductDirectoryInput(input); err != nil {
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
