package goods

// Фото из Excel поставщика: какие строки покрывает каждый рисунок (якоря drawing XML, объединённые ячейки).

import (
	"archive/zip"
	"encoding/xml"
	"fmt"
	"io"
	"path"
	"strings"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/xuri/excelize/v2"
)

type goodsXLSXRelationship struct {
	ID     string `xml:"Id,attr"`
	Target string `xml:"Target,attr"`
}

type goodsXLSXRelationships struct {
	Relationships []goodsXLSXRelationship `xml:"Relationship"`
}

type goodsXLSXWorkbook struct {
	Sheets []struct {
		Name  string `xml:"name,attr"`
		RelID string `xml:"id,attr"`
	} `xml:"sheets>sheet"`
}

type goodsXLSXSheetDrawing struct {
	RelID string `xml:"id,attr"`
}

type goodsXLSXSheet struct {
	Drawing    *goodsXLSXSheetDrawing `xml:"drawing"`
	MergeCells []struct {
		Ref string `xml:"ref,attr"`
	} `xml:"mergeCells>mergeCell"`
}

type goodsXLSXMarker struct {
	Col    int   `xml:"col"`
	Row    int   `xml:"row"`
	RowOff int64 `xml:"rowOff"`
}

type goodsXLSXPicture struct{}

type goodsXLSXTwoCellAnchor struct {
	From goodsXLSXMarker   `xml:"from"`
	To   goodsXLSXMarker   `xml:"to"`
	Pic  *goodsXLSXPicture `xml:"pic"`
}

type goodsXLSXOneCellAnchor struct {
	From goodsXLSXMarker   `xml:"from"`
	Pic  *goodsXLSXPicture `xml:"pic"`
}

type goodsXLSXDrawing struct {
	TwoCellAnchors []goodsXLSXTwoCellAnchor `xml:"twoCellAnchor"`
	OneCellAnchors []goodsXLSXOneCellAnchor `xml:"oneCellAnchor"`
}

func readGoodsXLSXPart(book *zip.Reader, name string) ([]byte, error) {
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

func goodsXLSXPartTarget(base, target string) string {
	target = strings.TrimPrefix(strings.TrimSpace(target), "/")
	if strings.HasPrefix(target, "xl/") {
		return path.Clean(target)
	}
	return path.Clean(path.Join(base, target))
}

func goodsXLSXRelationshipTarget(data []byte, relationID string) (string, error) {
	var relationships goodsXLSXRelationships
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

func goodsXLSXPictureMIME(extension string) string {
	switch strings.ToLower(extension) {
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	default:
		return ""
	}
}

// parseGoodsSpanningImages reads the drawing anchor ranges without exposing
// any XLSX internals from excelize. GetPictures still supplies the decoded
// image bytes, while the drawing XML tells us which rows the image covers.
func parseGoodsSpanningImages(file io.ReaderAt, size int64, workbook *excelize.File, targetSheet string) ([]GoodsParseImage, [][]byte, error) {
	archive, err := zip.NewReader(file, size)
	if err != nil {
		return nil, nil, err
	}

	workbookXML, err := readGoodsXLSXPart(archive, "xl/workbook.xml")
	if err != nil {
		return nil, nil, err
	}
	var workbookData goodsXLSXWorkbook
	if err := xml.Unmarshal(workbookXML, &workbookData); err != nil {
		return nil, nil, err
	}
	workbookRelationsXML, err := readGoodsXLSXPart(archive, "xl/_rels/workbook.xml.rels")
	if err != nil {
		return nil, nil, err
	}
	var workbookRelations goodsXLSXRelationships
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
				sheetPath = goodsXLSXPartTarget("xl", relation.Target)
				break
			}
		}
		break
	}
	if sheetPath == "" {
		return nil, nil, fmt.Errorf("XLSX sheet %q path not found", targetSheet)
	}

	sheetXML, err := readGoodsXLSXPart(archive, sheetPath)
	if err != nil {
		return nil, nil, err
	}
	var sheetData goodsXLSXSheet
	if err := xml.Unmarshal(sheetXML, &sheetData); err != nil {
		return nil, nil, err
	}
	if sheetData.Drawing == nil {
		return nil, nil, nil
	}

	sheetRelationsPath := path.Join(path.Dir(sheetPath), "_rels", path.Base(sheetPath)+".rels")
	sheetRelationsXML, err := readGoodsXLSXPart(archive, sheetRelationsPath)
	if err != nil {
		return nil, nil, err
	}
	drawingTarget, err := goodsXLSXRelationshipTarget(sheetRelationsXML, sheetData.Drawing.RelID)
	if err != nil {
		return nil, nil, err
	}
	drawingPath := goodsXLSXPartTarget(path.Dir(sheetPath), drawingTarget)
	drawingXML, err := readGoodsXLSXPart(archive, drawingPath)
	if err != nil {
		return nil, nil, err
	}
	var drawing goodsXLSXDrawing
	if err := xml.Unmarshal(drawingXML, &drawing); err != nil {
		return nil, nil, err
	}

	// anchorRow — строка, где стоит картинка (по ней её отдаёт excelize); startRow…endRow — строки,
	// которые получают фото.
	type imageRange struct{ anchorRow, startRow, endRow, col int }
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
		ranges = append(ranges, imageRange{anchorRow: anchor.From.Row, startRow: anchor.From.Row, endRow: endRow, col: anchor.From.Col})
	}
	for _, anchor := range drawing.OneCellAnchors {
		if anchor.Pic == nil {
			continue
		}
		ranges = append(ranges, imageRange{anchorRow: anchor.From.Row, startRow: anchor.From.Row, endRow: anchor.From.Row + 1, col: anchor.From.Col})
	}

	// Фото в объединённой ячейке («A22:A27») относится ко всем её строкам, даже если сама картинка
	// короче ячейки и заканчивается раньше её последней строки.
	for index, imageRange := range ranges {
		for _, merged := range sheetData.MergeCells {
			first, last, ok := goodsXLSXMergedRange(merged.Ref)
			if !ok || imageRange.col < first[0] || imageRange.col > last[0] || imageRange.startRow < first[1] || imageRange.startRow > last[1] {
				continue
			}
			ranges[index].startRow = min(imageRange.startRow, first[1])
			ranges[index].endRow = max(imageRange.endRow, last[1]+1)
		}
	}

	images := make([]GoodsParseImage, 0)
	imageFiles := make([][]byte, 0)
	for _, imageRange := range ranges {
		cell, cellErr := excelize.CoordinatesToCellName(imageRange.col+1, imageRange.anchorRow+1)
		if cellErr != nil {
			continue
		}
		pictures, pictureErr := workbook.GetPictures(targetSheet, cell)
		if pictureErr != nil {
			continue
		}
		for _, picture := range pictures {
			mimeType := goodsXLSXPictureMIME(picture.Extension)
			if mimeType == "" {
				continue
			}
			if len(picture.File) > MaxGoodsParseImageBytes {
				return nil, nil, &service.ValidationError{Field: "file", Message: fmt.Sprintf("Изображение в строке %d больше %d МБ", imageRange.startRow+1, MaxGoodsParseImageBytes/(1024*1024))}
			}
			imageFiles = append(imageFiles, picture.File)
			for row := imageRange.startRow; row < imageRange.endRow; row++ {
				images = append(images, GoodsParseImage{SourceRow: row + 1, DataIndex: len(imageFiles) - 1, MimeType: mimeType})
			}
		}
	}
	return images, imageFiles, nil
}

// goodsXLSXMergedRange — объединённая ячейка «A22:A27» как первая и последняя клетка {колонка, строка}
// с отсчётом от нуля, как в якорях рисунков.
func goodsXLSXMergedRange(ref string) ([2]int, [2]int, bool) {
	start, end, found := strings.Cut(ref, ":")
	if !found {
		return [2]int{}, [2]int{}, false
	}
	firstCol, firstRow, err1 := excelize.CellNameToCoordinates(start)
	lastCol, lastRow, err2 := excelize.CellNameToCoordinates(end)
	if err1 != nil || err2 != nil {
		return [2]int{}, [2]int{}, false
	}
	return [2]int{firstCol - 1, firstRow - 1}, [2]int{lastCol - 1, lastRow - 1}, true
}
