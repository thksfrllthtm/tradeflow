package service

// Reading an uploaded estimate XLSX: workbook inspection, preview, header mapping, row → product and pictures.

import (
	"archive/zip"
	"bytes"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"strings"
	"unicode"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"
)

func (s *ImportService) inspectWorkbook(storedPath string) ([]SheetInfo, error) {
	file, err := s.files.Open(storedPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	if err := inspectXLSXContainer(file); err != nil {
		return nil, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	workbook, err := excelize.OpenReader(file)
	if err != nil {
		return nil, &ValidationError{Field: "file", Message: "Файл повреждён или не является поддерживаемым XLSX"}
	}
	defer workbook.Close()
	info := make([]SheetInfo, 0)
	totalRows := 0
	for _, sheet := range workbook.GetSheetList() {
		rows, err := workbook.GetRows(sheet)
		if err != nil {
			return nil, &ValidationError{Field: "file", Message: "Не удалось прочитать один из листов XLSX"}
		}
		if len(rows) > MaxRowsPerSheet {
			return nil, &ValidationError{Field: "file", Message: "Лист содержит больше 5 000 строк"}
		}
		for _, row := range rows {
			if len(row) > MaxColumns {
				return nil, &ValidationError{Field: "file", Message: "Лист содержит больше 100 колонок"}
			}
		}
		totalRows += len(rows)
		if totalRows > MaxWorkbookRows {
			return nil, &ValidationError{Field: "file", Message: "Книга содержит больше 10 000 строк"}
		}
		info = append(info, SheetInfo{Name: sheet, Rows: len(rows)})
	}
	if len(info) == 0 {
		return nil, &ValidationError{Field: "file", Message: "В XLSX нет доступных листов"}
	}
	return info, nil
}

func inspectXLSXContainer(file repository.StoredFile) error {
	size := file.Size()
	if size <= 0 || size > MaxXLSXBytes {
		return &ValidationError{Field: "file", Message: "XLSX-файл больше 50 МБ не поддерживается"}
	}
	archive, err := zip.NewReader(file, size)
	if err != nil {
		return &ValidationError{Field: "file", Message: "Файл повреждён или не является обычным XLSX"}
	}
	var expandedSize uint64
	for _, item := range archive.File {
		expandedSize += item.UncompressedSize64
		if expandedSize > MaxXLSXExpandedBytes {
			return &ValidationError{Field: "file", Message: "XLSX содержит слишком большой распакованный объём"}
		}
		name := strings.ToLower(item.Name)
		if strings.Contains(name, "vbaproject.bin") {
			return &ValidationError{Field: "file", Message: "XLSX с макросами не поддерживается"}
		}
		if strings.HasPrefix(name, "xl/externallinks/") {
			return &ValidationError{Field: "file", Message: "XLSX с внешними связями не поддерживается"}
		}
		if strings.HasSuffix(name, ".rels") {
			reader, openErr := item.Open()
			if openErr != nil {
				return &ValidationError{Field: "file", Message: "Не удалось проверить связи XLSX"}
			}
			contents, readErr := io.ReadAll(io.LimitReader(reader, 1<<20))
			reader.Close()
			if readErr != nil {
				return &ValidationError{Field: "file", Message: "Не удалось проверить связи XLSX"}
			}
			if bytes.Contains(contents, []byte(`TargetMode="External"`)) {
				return &ValidationError{Field: "file", Message: "XLSX с внешними связями не поддерживается"}
			}
		}
	}
	return nil
}

func (s *ImportService) previewForImport(item repository.EstimateImport, sheet string, headerRow int) (Preview, error) {
	file, err := s.files.Open(item.StoredPath)
	if err != nil {
		return Preview{}, err
	}
	defer file.Close()
	workbook, err := excelize.OpenReader(file)
	if err != nil {
		return Preview{}, &ValidationError{Field: "file", Message: "Не удалось прочитать сохранённый XLSX-файл"}
	}
	defer workbook.Close()
	sheets, err := s.inspectWorkbook(item.StoredPath)
	if err != nil {
		return Preview{}, err
	}
	if sheet == "" {
		sheet = sheets[0].Name
	}
	if headerRow < 1 {
		headerRow = 1
	}
	rows, err := workbook.GetRows(sheet)
	if err != nil {
		return Preview{}, &ValidationError{Field: "sheet", Message: "Выбранный лист недоступен"}
	}
	if headerRow > len(rows) {
		return Preview{}, &ValidationError{Field: "headerRow", Message: "Строка заголовков находится вне листа"}
	}
	headerValues := rows[headerRow-1]
	headers := make([]Header, 0, len(headerValues))
	for index, value := range headerValues {
		headers = append(headers, Header{Index: index + 1, Value: strings.TrimSpace(value)})
	}
	preview := Preview{ImportID: item.ID, Sheets: sheets, Sheet: sheet, HeaderRow: headerRow, Headers: headers, SuggestedMapping: suggestMapping(headerValues)}
	limit := len(rows)
	if limit > headerRow+MaxPreviewRows {
		limit = headerRow + MaxPreviewRows
	}
	for sourceRow := headerRow + 1; sourceRow <= limit; sourceRow++ {
		values := map[int]string{}
		row := rows[sourceRow-1]
		for index, value := range row {
			values[index+1] = strings.TrimSpace(value)
		}
		warnings := rowFormulaWarnings(workbook, sheet, sourceRow, row)
		included, reason := likelyProduct(row)
		if reason != "" {
			warnings = append(warnings, reason)
		}
		preview.Rows = append(preview.Rows, PreviewRow{SourceRow: sourceRow, Values: values, DefaultIncluded: included, Warnings: warnings})
	}
	if len(rows) > limit {
		preview.Warnings = append(preview.Warnings, fmt.Sprintf("Для предварительного просмотра показаны первые %d строк; в листе всего %d", MaxPreviewRows, len(rows)-headerRow))
	}
	pictures, warnings := pictureHints(workbook, sheet)
	preview.Pictures, preview.Warnings = pictures, append(preview.Warnings, warnings...)
	preview.Warnings = append(preview.Warnings, "URL изображений сохраняются как данные и не скачиваются автоматически")
	return preview, nil
}

func validateConfirmInput(input ConfirmInput) error {
	if strings.TrimSpace(input.Sheet) == "" {
		return &ValidationError{Field: "sheet", Message: "Выберите лист"}
	}
	if input.HeaderRow < 1 {
		return &ValidationError{Field: "headerRow", Message: "Укажите строку заголовков"}
	}
	if input.Mapping["name"] < 1 {
		return &ValidationError{Field: "mapping", Message: "Подтвердите колонку с названием товара"}
	}
	if !oneOf(input.PriceBasis, "piece", "explicit_unit", "unknown") {
		return &ValidationError{Field: "priceBasis", Message: "Укажите единицу цены или оставьте её неизвестной"}
	}
	if input.PriceBasis == "explicit_unit" && strings.TrimSpace(input.PriceUnitLabel) == "" {
		return &ValidationError{Field: "priceUnitLabel", Message: "Укажите явную единицу цены"}
	}
	if !oneOf(input.WeightBasis, "line", "unit", "unknown") {
		return &ValidationError{Field: "weightBasis", Message: "Укажите, к чему относится вес"}
	}
	return nil
}

func productFromRow(workbook *excelize.File, sheet string, sourceRow int, row []string, input ConfirmInput) (repository.ImportedProduct, error) {
	raw := map[string]map[string]string{}
	values := map[string]string{}
	for field, column := range input.Mapping {
		if column < 1 || column > MaxColumns {
			continue
		}
		value := ""
		if column <= len(row) {
			value = strings.TrimSpace(row[column-1])
		}
		cell, _ := excelize.CoordinatesToCellName(column, sourceRow)
		formula, _ := workbook.GetCellFormula(sheet, cell)
		raw[field] = map[string]string{"value": value, "formula": formula}
		values[field] = value
	}
	warnings := make([]string, 0)
	for field, rawValue := range raw {
		if rawValue["formula"] != "" && rawValue["value"] == "" {
			warnings = append(warnings, fmt.Sprintf("%s: формула без доступного численного результата требует исправления", field))
		}
	}
	quantity, quantityWarning := parseOptionalDecimal(values["quantity"], "Количество", true)
	if quantityWarning != "" {
		warnings = append(warnings, quantityWarning)
	}
	price, priceWarning := parseOptionalDecimal(values["unitPrice"], "Цена за единицу", false)
	if priceWarning != "" {
		warnings = append(warnings, priceWarning)
	}
	lineTotal, totalWarning := parseOptionalDecimal(values["lineTotal"], "Сумма строки", false)
	if totalWarning != "" {
		warnings = append(warnings, totalWarning)
	}
	weight, weightWarning := parseOptionalDecimal(values["weight"], "Вес", false)
	if weightWarning != "" {
		warnings = append(warnings, weightWarning)
	}
	if price == nil && quantity != nil && lineTotal != nil && quantity.IsPositive() {
		calculated := lineTotal.Div(*quantity)
		price = &calculated
		warnings = append(warnings, "Цена рассчитана из суммы строки и количества; исходная сумма сохранена")
	}
	if quantity != nil && price != nil && lineTotal != nil {
		expected := quantity.Mul(*price)
		tolerance := decimal.RequireFromString(lineTotalTolerance)
		if expected.Sub(*lineTotal).Abs().GreaterThan(tolerance) {
			warnings = append(warnings, "Сумма строки не совпадает с количеством × ценой (допуск 0,01)")
		}
	}
	currency := strings.ToUpper(strings.TrimSpace(values["currency"]))
	if currency != "" && (len(currency) != 3 || !isLatinUppercase(currency)) {
		warnings = append(warnings, "Валюта не является трёхбуквенным кодом")
	}
	if strings.TrimSpace(values["name"]) == "" {
		warnings = append(warnings, "Нет названия товара")
	}
	if strings.TrimSpace(values["photoURL"]) != "" {
		warnings = append(warnings, "URL фото не загружается автоматически")
	}
	status := "imported"
	if len(warnings) > 0 {
		status = "needs_correction"
	}
	return repository.ImportedProduct{
		SourceSheet: sheet, SourceRow: sourceRow, SourceValues: encodeJSON(raw), Name: strings.TrimSpace(values["name"]), Translation: strings.TrimSpace(values["translation"]), SKU: strings.TrimSpace(values["sku"]),
		Quantity: quantity, UnitPrice: price, LineTotal: lineTotal, Currency: currency, Weight: weight, Characteristics: strings.TrimSpace(values["characteristics"]), PhotoURL: strings.TrimSpace(values["photoURL"]),
		PriceBasis: input.PriceBasis, PriceUnitLabel: strings.TrimSpace(input.PriceUnitLabel), WeightBasis: input.WeightBasis, ImportStatus: status, Warnings: encodeJSON(warnings),
	}, nil
}

func (s *ImportService) extractPictures(workbook *excelize.File, sheet string, products []repository.ImportedProduct) ([]repository.ImportAsset, map[int][]int, map[int][]string) {
	assets := []repository.ImportAsset{}
	links := map[int][]int{}
	warnings := map[int][]string{}
	byRow := map[int]int{}
	for index, product := range products {
		byRow[product.SourceRow] = index
	}
	cells, err := workbook.GetPictureCells(sheet)
	if err != nil {
		return assets, links, warnings
	}
	for _, cell := range cells {
		column, row, coordinateErr := excelize.CellNameToCoordinates(cell)
		if coordinateErr != nil || column < 1 || row < 1 {
			continue
		}
		pictures, pictureErr := workbook.GetPictures(sheet, cell)
		if pictureErr != nil {
			continue
		}
		productIndex, matched := byRow[row]
		if matched && len(pictures) > 1 {
			warnings[productIndex] = append(warnings[productIndex], fmt.Sprintf("В ячейке %s найдено несколько изображений; проверьте их привязку к строке", cell))
		}
		for _, picture := range pictures {
			mimeType, extension, supported := supportedImage(picture.File)
			if !supported {
				if matched {
					warnings[productIndex] = append(warnings[productIndex], fmt.Sprintf("Изображение в %s не поддерживается; доступны только JPG и PNG", cell))
				}
				continue
			}
			storedPath, size, saveErr := s.files.Save(bytes.NewReader(picture.File), "images", extension)
			if saveErr != nil {
				if matched {
					warnings[productIndex] = append(warnings[productIndex], "Не удалось сохранить встроенное изображение")
				}
				continue
			}
			assetIndex := len(assets)
			assets = append(assets, repository.ImportAsset{SourceSheet: sheet, SourceCell: cell, SourceRow: row, StoredPath: storedPath, MIMEType: mimeType, FileSize: size, Kind: "embedded"})
			if matched {
				links[productIndex] = append(links[productIndex], assetIndex)
			} else {
				_ = assetIndex
			}
		}
	}
	return assets, links, warnings
}

func pictureHints(workbook *excelize.File, sheet string) ([]PictureHint, []string) {
	cells, err := workbook.GetPictureCells(sheet)
	if err != nil {
		return nil, []string{"Не удалось проверить встроенные изображения"}
	}
	result := make([]PictureHint, 0, len(cells))
	for _, cell := range cells {
		_, row, coordinateErr := excelize.CellNameToCoordinates(cell)
		if coordinateErr != nil {
			continue
		}
		pictures, pictureErr := workbook.GetPictures(sheet, cell)
		if pictureErr != nil {
			result = append(result, PictureHint{Sheet: sheet, Cell: cell, SourceRow: row, Message: "Не удалось извлечь изображение"})
			continue
		}
		for _, picture := range pictures {
			_, _, supported := supportedImage(picture.File)
			message := "Будет привязано к товару из той же строки после подтверждения"
			if !supported {
				message = "Неподдерживаемый формат; доступны только JPG и PNG"
			}
			result = append(result, PictureHint{Sheet: sheet, Cell: cell, SourceRow: row, Supported: supported, Message: message})
		}
	}
	return result, nil
}

func supportedImage(contents []byte) (string, string, bool) {
	config, format, err := image.DecodeConfig(bytes.NewReader(contents))
	if err != nil || config.Width < 1 || config.Height < 1 {
		return "", "", false
	}
	switch format {
	case "jpeg":
		return "image/jpeg", ".jpg", true
	case "png":
		return "image/png", ".png", true
	default:
		return "", "", false
	}
}

func suggestMapping(headers []string) map[string]int {
	result := map[string]int{}
	// Check the glove-specific quantity fields before the generic quantity
	// heuristic. For example, "количество пар" also contains "количество";
	// the more specific field must win deterministically.
	fieldOrder := []string{"name", "translation", "sku", "pairsPerPlace", "places", "pairs", "quantity", "quantityUnit", "grossWeight", "netWeight", "material", "characteristics", "photoURL", "unitPrice", "lineTotal", "currency"}
	for index, header := range headers {
		normalized := normalizeHeader(header)
		for _, field := range fieldOrder {
			patterns := headerPatterns[field]
			if _, exists := result[field]; exists {
				continue
			}
			for _, pattern := range patterns {
				if strings.Contains(normalized, pattern) {
					result[field] = index + 1
					break
				}
			}
		}
	}
	return result
}

var headerPatterns = map[string][]string{
	"name":            {"наименование", "название", "товар", "品名", "product", "name"},
	"translation":     {"перевод", "translation"},
	"sku":             {"артикул", "sku", "код товара", "vendor code"},
	"quantity":        {"количество", "кол во", "件数", "数量", "qty", "quantity"},
	"pairs":           {"пары", "количество пар", "пар", "pairs", "总数量", "双数", "双"},
	"places":          {"места", "количество мест", "мест", "places", "packages", "cartons", "箱数"},
	"pairsPerPlace":   {"пар в месте", "пар в одном месте", "pairs per place", "单件个数", "每件数量"},
	"unitPrice":       {"цена за единиц", "цена", "unit price", "price"},
	"lineTotal":       {"сумма строки", "итого", "сумма", "total", "amount"},
	"currency":        {"валюта", "currency"},
	"weight":          {"вес", "weight", "брутто", "gross", "毛重"},
	"characteristics": {"материал", "характерист", "specification", "material"},
	"photoURL":        {"url фото", "фото url", "photo url", "image url", "ссылка на фото"},
}

func normalizeHeader(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.Map(func(char rune) rune {
		if unicode.IsLetter(char) || unicode.IsNumber(char) || char == ' ' {
			return char
		}
		return ' '
	}, value)
	return strings.Join(strings.Fields(value), " ")
}

func likelyProduct(row []string) (bool, string) {
	nonEmpty := []string{}
	for _, value := range row {
		if strings.TrimSpace(value) != "" {
			nonEmpty = append(nonEmpty, strings.TrimSpace(value))
		}
	}
	if len(nonEmpty) == 0 {
		return false, "Пустая строка не будет импортирована"
	}
	joined := normalizeHeader(strings.Join(nonEmpty, " "))
	if strings.Contains(joined, "итого") || strings.Contains(joined, "всего") || strings.Contains(joined, "total") {
		return false, "Строка похожа на итог и не будет импортирована автоматически"
	}
	return true, ""
}

func rowFormulaWarnings(workbook *excelize.File, sheet string, rowNumber int, row []string) []string {
	warnings := []string{}
	for index, value := range row {
		cell, _ := excelize.CoordinatesToCellName(index+1, rowNumber)
		formula, _ := workbook.GetCellFormula(sheet, cell)
		if formula != "" && strings.TrimSpace(value) == "" {
			warnings = append(warnings, fmt.Sprintf("%s: формула без доступного результата", cell))
		}
	}
	return warnings
}
