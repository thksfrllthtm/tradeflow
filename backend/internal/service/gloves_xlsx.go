package service

// Reading supplier XLSX files for the gloves calculator: rows, headers and pictures anchored to rows.

import (
	"archive/zip"
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"strings"
	"unicode"

	"github.com/xuri/excelize/v2"
)

type glovesXLSXRelationship struct {
	ID     string `xml:"Id,attr"`
	Target string `xml:"Target,attr"`
}

type glovesXLSXRelationships struct {
	Relationships []glovesXLSXRelationship `xml:"Relationship"`
}

type glovesXLSXWorkbook struct {
	Sheets []struct {
		Name  string `xml:"name,attr"`
		RelID string `xml:"id,attr"`
	} `xml:"sheets>sheet"`
}

type glovesXLSXSheetDrawing struct {
	RelID string `xml:"id,attr"`
}

type glovesXLSXSheet struct {
	Drawing *glovesXLSXSheetDrawing `xml:"drawing"`
}

type glovesXLSXMarker struct {
	Col    int   `xml:"col"`
	Row    int   `xml:"row"`
	RowOff int64 `xml:"rowOff"`
}

type glovesXLSXPicture struct{}

type glovesXLSXTwoCellAnchor struct {
	From glovesXLSXMarker   `xml:"from"`
	To   glovesXLSXMarker   `xml:"to"`
	Pic  *glovesXLSXPicture `xml:"pic"`
}

type glovesXLSXOneCellAnchor struct {
	From glovesXLSXMarker   `xml:"from"`
	Pic  *glovesXLSXPicture `xml:"pic"`
}

type glovesXLSXDrawing struct {
	TwoCellAnchors []glovesXLSXTwoCellAnchor `xml:"twoCellAnchor"`
	OneCellAnchors []glovesXLSXOneCellAnchor `xml:"oneCellAnchor"`
}

func readGlovesXLSXPart(book *zip.Reader, name string) ([]byte, error) {
	for _, file := range book.File {
		if file.Name != name {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			return nil, err
		}
		defer reader.Close()
		return io.ReadAll(reader)
	}
	return nil, fmt.Errorf("XLSX part %q not found", name)
}

func glovesXLSXPartTarget(base, target string) string {
	target = strings.TrimPrefix(strings.TrimSpace(target), "/")
	if strings.HasPrefix(target, "xl/") {
		return path.Clean(target)
	}
	return path.Clean(path.Join(base, target))
}

func glovesXLSXRelationshipTarget(data []byte, relationID string) (string, error) {
	var relationships glovesXLSXRelationships
	if err := xml.Unmarshal(data, &relationships); err != nil {
		return "", err
	}
	for _, relationship := range relationships.Relationships {
		if relationship.ID == relationID {
			return relationship.Target, nil
		}
	}
	return "", fmt.Errorf("XLSX relationship %q not found", relationID)
}

func glovesXLSXPictureMIME(extension string) string {
	switch strings.ToLower(extension) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	default:
		return ""
	}
}

// parseGlovesSpanningImages reads the drawing anchor ranges without exposing
// any XLSX internals from excelize. GetPictures still supplies the decoded
// image bytes, while the drawing XML tells us which rows the image covers.
func parseGlovesSpanningImages(file io.ReaderAt, size int64, workbook *excelize.File, targetSheet string) ([]GlovesParseImage, [][]byte, error) {
	archive, err := zip.NewReader(file, size)
	if err != nil {
		return nil, nil, err
	}

	workbookXML, err := readGlovesXLSXPart(archive, "xl/workbook.xml")
	if err != nil {
		return nil, nil, err
	}
	var workbookData glovesXLSXWorkbook
	if err := xml.Unmarshal(workbookXML, &workbookData); err != nil {
		return nil, nil, err
	}
	workbookRelationsXML, err := readGlovesXLSXPart(archive, "xl/_rels/workbook.xml.rels")
	if err != nil {
		return nil, nil, err
	}
	var workbookRelations glovesXLSXRelationships
	if err := xml.Unmarshal(workbookRelationsXML, &workbookRelations); err != nil {
		return nil, nil, err
	}

	sheetPath := ""
	for _, sheet := range workbookData.Sheets {
		if sheet.Name != targetSheet {
			continue
		}
		for _, relation := range workbookRelations.Relationships {
			if relation.ID == sheet.RelID {
				sheetPath = glovesXLSXPartTarget("xl", relation.Target)
				break
			}
		}
		break
	}
	if sheetPath == "" {
		return nil, nil, fmt.Errorf("XLSX sheet %q path not found", targetSheet)
	}

	sheetXML, err := readGlovesXLSXPart(archive, sheetPath)
	if err != nil {
		return nil, nil, err
	}
	var sheetData glovesXLSXSheet
	if err := xml.Unmarshal(sheetXML, &sheetData); err != nil {
		return nil, nil, err
	}
	if sheetData.Drawing == nil {
		return nil, nil, nil
	}

	sheetRelationsPath := path.Join(path.Dir(sheetPath), "_rels", path.Base(sheetPath)+".rels")
	sheetRelationsXML, err := readGlovesXLSXPart(archive, sheetRelationsPath)
	if err != nil {
		return nil, nil, err
	}
	drawingTarget, err := glovesXLSXRelationshipTarget(sheetRelationsXML, sheetData.Drawing.RelID)
	if err != nil {
		return nil, nil, err
	}
	drawingPath := glovesXLSXPartTarget(path.Dir(sheetPath), drawingTarget)
	drawingXML, err := readGlovesXLSXPart(archive, drawingPath)
	if err != nil {
		return nil, nil, err
	}
	var drawing glovesXLSXDrawing
	if err := xml.Unmarshal(drawingXML, &drawing); err != nil {
		return nil, nil, err
	}

	type imageRange struct{ startRow, endRow, col int }
	ranges := make([]imageRange, 0, len(drawing.TwoCellAnchors)+len(drawing.OneCellAnchors))
	for _, anchor := range drawing.TwoCellAnchors {
		if anchor.Pic == nil {
			continue
		}
		endRow := anchor.To.Row
		if anchor.To.RowOff > 0 {
			endRow++
		}
		if endRow <= anchor.From.Row {
			endRow = anchor.From.Row + 1
		}
		ranges = append(ranges, imageRange{startRow: anchor.From.Row, endRow: endRow, col: anchor.From.Col})
	}
	for _, anchor := range drawing.OneCellAnchors {
		if anchor.Pic == nil {
			continue
		}
		ranges = append(ranges, imageRange{startRow: anchor.From.Row, endRow: anchor.From.Row + 1, col: anchor.From.Col})
	}

	images := make([]GlovesParseImage, 0)
	imageFiles := make([][]byte, 0)
	for _, imageRange := range ranges {
		cell, cellErr := excelize.CoordinatesToCellName(imageRange.col+1, imageRange.startRow+1)
		if cellErr != nil {
			continue
		}
		pictures, pictureErr := workbook.GetPictures(targetSheet, cell)
		if pictureErr != nil {
			continue
		}
		for _, picture := range pictures {
			mimeType := glovesXLSXPictureMIME(picture.Extension)
			if mimeType == "" {
				continue
			}
			if len(picture.File) > MaxGlovesParseImageBytes {
				return nil, nil, &ValidationError{Field: "file", Message: fmt.Sprintf("Изображение в строке %d больше %d МБ", imageRange.startRow+1, MaxGlovesParseImageBytes/(1024*1024))}
			}
			imageFiles = append(imageFiles, picture.File)
			for row := imageRange.startRow; row < imageRange.endRow; row++ {
				images = append(images, GlovesParseImage{SourceRow: row + 1, DataIndex: len(imageFiles) - 1, MimeType: mimeType})
			}
		}
	}
	return images, imageFiles, nil
}

// WriteJSON writes the result as JSON with an extra "imageData" array holding
// ImageFiles in base64, encoding one picture at a time.
func (r GlovesParseResult) WriteJSON(w io.Writer) error {
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

func (s *GlovesService) ParseXlsx(ctx context.Context, file io.ReaderAt, size int64, filename string, sheet string, headerRow, dataStartRow int) (GlovesParseResult, error) {
	return s.parseXlsx(ctx, file, size, filename, sheet, headerRow, dataStartRow, false)
}

// ParseXlsxWithSharedImages is used by the copied gloves estimate. It keeps
// an image attached to every product row covered by the same Excel drawing
// anchor, which is how supplier sheets commonly show one photo for a group
// of products.
func (s *GlovesService) ParseXlsxWithSharedImages(ctx context.Context, file io.ReaderAt, size int64, filename string, sheet string, headerRow, dataStartRow int) (GlovesParseResult, error) {
	return s.parseXlsx(ctx, file, size, filename, sheet, headerRow, dataStartRow, true)
}

// parseXlsx reads the workbook from file (an uploaded file on disk for large
// uploads) instead of copying it into memory: excelize already keeps one copy
// of every part, which is the biggest cost for workbooks full of photos.
func (s *GlovesService) parseXlsx(ctx context.Context, file io.ReaderAt, size int64, filename string, sheet string, headerRow, dataStartRow int, shareSpanningImages bool) (GlovesParseResult, error) {
	_ = ctx
	if !strings.HasSuffix(strings.ToLower(strings.TrimSpace(filename)), ".xlsx") {
		return GlovesParseResult{}, &ValidationError{Field: "file", Message: "Поддерживаются только обычные файлы .xlsx"}
	}
	if file == nil || size <= 0 {
		return GlovesParseResult{}, &ValidationError{Field: "file", Message: "Выберите XLSX-файл"}
	}
	if size > MaxGlovesXLSXBytes {
		return GlovesParseResult{}, &ValidationError{Field: "file", Message: fmt.Sprintf("XLSX-файл больше %d МБ не поддерживается", MaxGlovesXLSXBytes/(1024*1024))}
	}
	workbook, err := excelize.OpenReader(io.NewSectionReader(file, 0, size))
	if err != nil {
		return GlovesParseResult{}, &ValidationError{Field: "file", Message: "Файл повреждён или не является обычным XLSX"}
	}
	defer workbook.Close()

	var result GlovesParseResult
	sheets := workbook.GetSheetList()
	if len(sheets) == 0 {
		return result, fmt.Errorf("в XLSX нет доступных листов")
	}

	totalRows := 0
	for _, sheetName := range sheets {
		rows, rowsErr := workbook.GetRows(sheetName)
		if rowsErr != nil {
			return result, &ValidationError{Field: "file", Message: "Не удалось прочитать один из листов XLSX"}
		}
		if len(rows) > MaxRowsPerSheet {
			return result, &ValidationError{Field: "file", Message: fmt.Sprintf("Лист %q содержит больше %d строк", sheetName, MaxRowsPerSheet)}
		}
		for _, row := range rows {
			if len(row) > MaxColumns {
				return result, &ValidationError{Field: "file", Message: fmt.Sprintf("Лист %q содержит больше %d колонок", sheetName, MaxColumns)}
			}
		}
		totalRows += len(rows)
		if totalRows > MaxWorkbookRows {
			return result, &ValidationError{Field: "file", Message: fmt.Sprintf("Книга содержит больше %d строк", MaxWorkbookRows)}
		}
		result.Sheets = append(result.Sheets, GlovesSheetInfo{Name: sheetName, Rows: len(rows)})
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
		return result, &ValidationError{Field: "sheet", Message: "Выбранный лист недоступен"}
	}

	if headerRow > len(rows) {
		return result, &ValidationError{Field: "headerRow", Message: "Строка заголовков находится вне листа"}
	}
	if dataStartRow <= headerRow {
		return result, &ValidationError{Field: "dataStartRow", Message: "Первая строка товаров должна быть ниже строки заголовков"}
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

	result.Rows = []GlovesParseRow{}
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
			result.Rows = append(result.Rows, GlovesParseRow{
				SourceRow: i + 1,
				Values:    rowMap,
				Included:  included,
				Warnings:  warnings,
			})
		}
	}

	result.Images = []GlovesParseImage{}
	result.ImageFiles = [][]byte{}
	if shareSpanningImages {
		sharedImages, sharedFiles, sharedErr := parseGlovesSpanningImages(file, size, workbook, targetSheet)
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
					if len(picture.File) > MaxGlovesParseImageBytes {
						return result, &ValidationError{Field: "file", Message: fmt.Sprintf("Изображение в строке %d больше %d МБ", row, MaxGlovesParseImageBytes/(1024*1024))}
					}
					result.ImageFiles = append(result.ImageFiles, picture.File)
					result.Images = append(result.Images, GlovesParseImage{
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
