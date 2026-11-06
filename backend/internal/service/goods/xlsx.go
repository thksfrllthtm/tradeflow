package goods

// Разбор Excel поставщика для смет товаров: строки, заголовки и фото, привязанные к строкам.

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/xuri/excelize/v2"
)

// Ограничения на Excel и строки сметы. Разбор Excel не сохраняет файл, поэтому допускает
// большие файлы: фото из них браузер сжимает до сохранения сметы. 200 МБ — под «База решений.xlsx»
// (1 074 фото, 107 МБ: разбор держит около 190 МБ памяти; на Render 512 МБ с GOMEMLIMIT=400MiB).
const (
	MaxRowsPerSheet         = 5000
	MaxWorkbookRows         = 10000
	MaxColumns              = 100
	MaxImageBytes           = 5 << 20
	MaxGoodsXLSXBytes       = 200 << 20
	MaxGoodsParseImageBytes = 25 << 20
)

// WriteJSON writes the result as JSON with an extra "imageData" array holding
// ImageFiles in base64, encoding one picture at a time.
func (r GoodsParseResult) WriteJSON(w io.Writer) error {
	head, err := json.Marshal(r)
	if err != nil {
		return err
	}
	out := bufio.NewWriterSize(w, 64<<10)
	out.Write(head[:len(head)-1]) // without the closing brace
	out.WriteString(`,"imageData":[`)
	for index, file := range r.ImageFiles {
		if index > 0 {
			out.WriteByte(',')
		}
		out.WriteByte('"')
		encoder := base64.NewEncoder(base64.StdEncoding, out)
		encoder.Write(file)
		encoder.Close()
		out.WriteByte('"')
	}
	out.WriteString("]}")
	// bufio.Writer keeps the first write error and reports it here.
	return out.Flush()
}

func (s *GoodsService) ParseXlsx(ctx context.Context, file io.ReaderAt, size int64, filename string, sheet string, headerRow, dataStartRow int) (GoodsParseResult, error) {
	return s.parseXlsx(ctx, file, size, filename, sheet, headerRow, dataStartRow, false)
}

// ParseXlsxWithSharedImages is used by goods estimates. It keeps
// an image attached to every product row covered by the same Excel drawing
// anchor, which is how supplier sheets commonly show one photo for a group
// of products.
func (s *GoodsService) ParseXlsxWithSharedImages(ctx context.Context, file io.ReaderAt, size int64, filename string, sheet string, headerRow, dataStartRow int) (GoodsParseResult, error) {
	return s.parseXlsx(ctx, file, size, filename, sheet, headerRow, dataStartRow, true)
}

// parseXlsx reads the workbook from file (an uploaded file on disk for large
// uploads) instead of copying it into memory: excelize already keeps one copy
// of every part, which is the biggest cost for workbooks full of photos.
func (s *GoodsService) parseXlsx(ctx context.Context, file io.ReaderAt, size int64, filename string, sheet string, headerRow, dataStartRow int, shareSpanningImages bool) (GoodsParseResult, error) {
	_ = ctx
	name := strings.ToLower(strings.TrimSpace(filename))
	isXls := strings.HasSuffix(name, ".xls")
	if !isXls && !strings.HasSuffix(name, ".xlsx") {
		return GoodsParseResult{}, &service.ValidationError{Field: "file", Message: "Поддерживаются только файлы Excel .xlsx и .xls"}
	}
	if file == nil || size <= 0 {
		return GoodsParseResult{}, &service.ValidationError{Field: "file", Message: "Выберите файл Excel"}
	}
	if size > MaxGoodsXLSXBytes {
		return GoodsParseResult{}, &service.ValidationError{Field: "file", Message: fmt.Sprintf("Файл Excel больше %d МБ не поддерживается", MaxGoodsXLSXBytes/(1024*1024))}
	}
	if isXls {
		converted, err := XLSToXLSX(file, size)
		if err != nil {
			return GoodsParseResult{}, &service.ValidationError{Field: "file", Message: "Файл повреждён или не является книгой Excel .xls"}
		}
		file, size = bytes.NewReader(converted), int64(len(converted))
	}
	workbook, err := excelize.OpenReader(io.NewSectionReader(file, 0, size))
	if err != nil {
		return GoodsParseResult{}, &service.ValidationError{Field: "file", Message: "Файл повреждён или не является обычным XLSX"}
	}
	defer workbook.Close()

	var result GoodsParseResult
	sheets := workbook.GetSheetList()
	if len(sheets) == 0 {
		return result, fmt.Errorf("в XLSX нет доступных листов")
	}

	totalRows := 0
	for _, sheetName := range sheets {
		rows, rowsErr := workbook.GetRows(sheetName)
		if rowsErr != nil {
			return result, &service.ValidationError{Field: "file", Message: "Не удалось прочитать один из листов XLSX"}
		}
		if len(rows) > MaxRowsPerSheet {
			return result, &service.ValidationError{Field: "file", Message: fmt.Sprintf("Лист %q содержит больше %d строк", sheetName, MaxRowsPerSheet)}
		}
		for _, row := range rows {
			if len(row) > MaxColumns {
				return result, &service.ValidationError{Field: "file", Message: fmt.Sprintf("Лист %q содержит больше %d колонок", sheetName, MaxColumns)}
			}
		}
		totalRows += len(rows)
		if totalRows > MaxWorkbookRows {
			return result, &service.ValidationError{Field: "file", Message: fmt.Sprintf("Книга содержит больше %d строк", MaxWorkbookRows)}
		}
		result.Sheets = append(result.Sheets, GoodsSheetInfo{Name: sheetName, Rows: len(rows)})
	}

	targetSheet := sheet
	if targetSheet == "" {
		targetSheet = sheets[0]
	} else {
		knownSheet := false
		for _, sheetName := range sheets {
			if sheetName == targetSheet {
				knownSheet = true
				break
			}
		}
		if !knownSheet {
			result.Warnings = append(result.Warnings, fmt.Sprintf("Лист %q не найден; выбран первый доступный лист %q", targetSheet, sheets[0]))
			targetSheet = sheets[0]
		}
	}

	if headerRow < 1 {
		headerRow = 1
	}
	if dataStartRow < 2 {
		dataStartRow = 2
	}

	rows, err := workbook.GetRows(targetSheet)
	if err != nil {
		return result, &service.ValidationError{Field: "sheet", Message: "Выбранный лист недоступен"}
	}

	if headerRow > len(rows) {
		return result, &service.ValidationError{Field: "headerRow", Message: "Строка заголовков находится вне листа"}
	}
	if dataStartRow <= headerRow {
		return result, &service.ValidationError{Field: "dataStartRow", Message: "Первая строка товаров должна быть ниже строки заголовков"}
	}
	headerValues := rows[headerRow-1]
	headerWidth := len(headerValues)
	for _, row := range rows {
		if len(row) > headerWidth {
			headerWidth = len(row)
		}
	}
	// Keep blank and duplicate header columns available for manual mapping.
	// Excel files with merged cells often have exactly this shape.
	canonicalHeaders := make([]string, headerWidth)
	headerCounts := make(map[string]int, headerWidth)
	for index := 0; index < headerWidth; index++ {
		value := ""
		if index < len(headerValues) {
			value = strings.TrimSpace(headerValues[index])
		}
		if value == "" {
			value = fmt.Sprintf("Колонка %d", index+1)
		}
		headerCounts[value]++
		if headerCounts[value] > 1 {
			value = fmt.Sprintf("%s (%d)", value, headerCounts[value])
		}
		canonicalHeaders[index] = value
		result.Headers = append(result.Headers, value)
	}

	result.Rows = []GoodsParseRow{}
	for i := dataStartRow - 1; i < len(rows); i++ {
		rowVals := rows[i]
		rowMap := make(map[string]string)
		hasData := false

		for colIdx, val := range rowVals {
			if colIdx < len(canonicalHeaders) {
				colName := canonicalHeaders[colIdx]
				rowMap[colName] = strings.TrimSpace(val)
				if strings.TrimSpace(val) != "" {
					hasData = true
				}
			}
		}

		if hasData {
			included := true
			warnings := []string{}
			if gloveTotalRow(rowMap) {
				included = false
				warnings = append(warnings, "Строка похожа на итог и не будет добавлена как товар")
				result.Warnings = append(result.Warnings, fmt.Sprintf("Строка %d похожа на итог и исключена из товаров", i+1))
			}
			result.Rows = append(result.Rows, GoodsParseRow{
				SourceRow: i + 1,
				Values:    rowMap,
				Included:  included,
				Warnings:  warnings,
			})
		}
	}

	markMergedRows(workbook, targetSheet, canonicalHeaders, result.Rows)

	result.Images = []GoodsParseImage{}
	result.ImageFiles = [][]byte{}
	if shareSpanningImages {
		sharedImages, sharedFiles, sharedErr := parseGoodsSpanningImages(file, size, workbook, targetSheet)
		if sharedErr == nil && len(sharedImages) > 0 {
			result.Images = sharedImages
			result.ImageFiles = sharedFiles
		}
	}
	if len(result.Images) == 0 {
		cells, err := workbook.GetPictureCells(targetSheet)
		if err == nil {
			for _, cell := range cells {
				_, row, coordinateErr := excelize.CellNameToCoordinates(cell)
				if coordinateErr != nil {
					continue
				}
				pictures, pictureErr := workbook.GetPictures(targetSheet, cell)
				if pictureErr != nil || len(pictures) == 0 {
					continue
				}

				for _, picture := range pictures {
					ext := strings.ToLower(picture.Extension)
					mimeType := ""
					switch ext {
					case ".jpg", ".jpeg":
						mimeType = "image/jpeg"
					case ".png":
						mimeType = "image/png"
					default:
						result.Warnings = append(result.Warnings, fmt.Sprintf("Изображение в строке %d пропущено: формат %s не поддерживается; используйте JPG или PNG", row, picture.Extension))
						continue
					}
					if len(picture.File) > MaxGoodsParseImageBytes {
						return result, &service.ValidationError{Field: "file", Message: fmt.Sprintf("Изображение в строке %d больше %d МБ", row, MaxGoodsParseImageBytes/(1024*1024))}
					}
					result.ImageFiles = append(result.ImageFiles, picture.File)
					result.Images = append(result.Images, GoodsParseImage{
						SourceRow: row,
						DataIndex: len(result.ImageFiles) - 1,
						MimeType:  mimeType,
					})
				}
			}
		}
	}

	return result, nil
}

// gloveTotalRow deliberately uses only common total markers. It is a safety
// default for supplier sheets; the original source row and an explicit warning
// remain in the source snapshot for audit, while an operator can add a needed
// exceptional row manually.
func gloveTotalRow(values map[string]string) bool {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			parts = append(parts, trimmed)
		}
	}
	joined := strings.ToLower(strings.Join(parts, " "))
	joined = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsSpace(r) {
			return r
		}
		return ' '
	}, joined)
	joined = strings.Join(strings.Fields(joined), " ")
	for _, marker := range []string{"итого", "всего", "total", "合计", "总计"} {
		if strings.Contains(joined, marker) {
			return true
		}
	}
	return false
}

// markMergedRows отмечает строки, чьи ячейки входят в объединённую по вертикали ячейку («E4:E6»):
// значение Excel хранит только в первой строке, остальные строки ссылаются на неё.
func markMergedRows(workbook *excelize.File, sheet string, headers []string, rows []GoodsParseRow) {
	merges, err := workbook.GetMergeCells(sheet)
	if err != nil || len(merges) == 0 {
		return
	}
	bySource := make(map[int]*GoodsParseRow, len(rows))
	for index := range rows {
		bySource[rows[index].SourceRow] = &rows[index]
	}
	for _, merge := range merges {
		startCol, startRow, err1 := excelize.CellNameToCoordinates(merge.GetStartAxis())
		endCol, endRow, err2 := excelize.CellNameToCoordinates(merge.GetEndAxis())
		if err1 != nil || err2 != nil || endRow <= startRow {
			continue
		}
		for col := startCol; col <= endCol && col <= len(headers); col++ {
			for line := startRow + 1; line <= endRow; line++ {
				if row := bySource[line]; row != nil {
					if row.Merged == nil {
						row.Merged = map[string]int{}
					}
					row.Merged[headers[col-1]] = startRow
				}
			}
		}
	}
}
