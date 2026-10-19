package goods

// Старый формат Excel (.xls): книгу читает grate и перекладывает в XLSX, дальше работает
// обычный разбор. Значения и объединённые ячейки сохраняются, фото из .xls — нет.

import (
	"fmt"
	"io"
	"os"

	"github.com/east-trans-logistic/import-calculator/backend/internal/thirdparty/grate"
	"github.com/east-trans-logistic/import-calculator/backend/internal/thirdparty/grate/xls"
	"github.com/xuri/excelize/v2"
)

func isMergeMark(value string) bool {
	return value == grate.ContinueColumnMerged || value == grate.EndColumnMerged ||
		value == grate.ContinueRowMerged || value == grate.EndRowMerged
}

// xlsToXlsx возвращает ту же книгу в формате XLSX.
func xlsToXlsx(file io.ReaderAt, size int64) (result []byte, err error) {
	// grate написан под доверенные файлы: на битом .xls он может упасть с паникой.
	defer func() {
		if recovered := recover(); recovered != nil {
			result, err = nil, fmt.Errorf("xls: %v", recovered)
		}
	}()

	// grate открывает книгу только по имени файла.
	temp, err := os.CreateTemp("", "goods-*.xls")
	if err != nil {
		return nil, err
	}
	defer os.Remove(temp.Name())
	_, err = io.Copy(temp, io.NewSectionReader(file, 0, size))
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return nil, err
	}

	source, err := xls.Open(temp.Name())
	if err != nil {
		return nil, err
	}
	defer source.Close()
	names, err := source.List()
	if err != nil {
		return nil, err
	}

	book := excelize.NewFile()
	defer book.Close()
	for index, name := range names {
		sheet, err := source.Get(name)
		if err != nil {
			return nil, err
		}
		var rows [][]string
		for sheet.Next() {
			rows = append(rows, sheet.Strings())
		}
		if index == 0 {
			if err := book.SetSheetName(book.GetSheetName(0), name); err != nil {
				return nil, err
			}
		} else if _, err := book.NewSheet(name); err != nil {
			return nil, err
		}
		if err := writeXlsRows(book, name, rows); err != nil {
			return nil, err
		}
	}
	buffer, err := book.WriteToBuffer()
	if err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

// writeXlsRows пишет значения листа и восстанавливает объединения по меткам grate:
// значение лежит в левой верхней ячейке, вправо идут → … ⇥, вниз ↓ … ⤓.
func writeXlsRows(book *excelize.File, sheet string, rows [][]string) error {
	at := func(r, c int) string {
		if r < len(rows) && c < len(rows[r]) {
			return rows[r][c]
		}
		return ""
	}
	for r, row := range rows {
		values := make([]any, len(row))
		for c, value := range row {
			if !isMergeMark(value) {
				values[c] = value
			}
		}
		cell, _ := excelize.CoordinatesToCellName(1, r+1)
		if err := book.SetSheetRow(sheet, cell, &values); err != nil {
			return err
		}
	}
	// Объединяем только после записи всех значений: запись в ячейку внутри объединения
	// excelize переносит в левую верхнюю и затёр бы её.
	for r, row := range rows {
		for c, value := range row {
			if isMergeMark(value) {
				continue
			}
			right, down := at(r, c+1), at(r+1, c)
			if right != grate.ContinueColumnMerged && right != grate.EndColumnMerged &&
				down != grate.ContinueRowMerged && down != grate.EndRowMerged {
				continue
			}
			lastCol := c
			for lastCol+1 < len(row) && (at(r, lastCol+1) == grate.ContinueColumnMerged || at(r, lastCol+1) == grate.EndColumnMerged) {
				lastCol++
				if at(r, lastCol) == grate.EndColumnMerged {
					break
				}
			}
			lastRow := r
			for at(lastRow+1, c) == grate.ContinueRowMerged || at(lastRow+1, c) == grate.EndRowMerged {
				lastRow++
				if at(lastRow, c) == grate.EndRowMerged {
					break
				}
			}
			from, _ := excelize.CoordinatesToCellName(c+1, r+1)
			to, _ := excelize.CoordinatesToCellName(lastCol+1, lastRow+1)
			if err := book.MergeCell(sheet, from, to); err != nil {
				return err
			}
		}
	}
	return nil
}
