package trademarks

// Разбор таможенного реестра объектов интеллектуальной собственности КГД РК (ТРОИС): Excel .xls (как
// публикует КГД) или .xlsx. Колонки: регистрационный номер, наименование или изображение знака (логотип
// — картинкой в ячейке), товары с классами МКТУ и кодами ТН ВЭД, правообладатель, охранный документ,
// срок защиты, доверенные лица. Исключённые записи («Исключён приказом …») пропускаются.

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/decisions"
	"github.com/xuri/excelize/v2"
	_ "golang.org/x/image/bmp"  // логотипы из EMF и DIB
	_ "golang.org/x/image/tiff" // логотипы TIFF
)

const (
	maxFileBytes = 80 << 20
	maxRows      = 20000
	// Логотип хранится в исходном качестве; огромные (сканы) — JPEG до logoBigSide.
	logoMaxSide  = 2000
	logoMaxBytes = 1500 << 10
	logoBigSide  = 1600
	logoQuality  = 90
	// logoMaxPixels — больше картинка не распаковывается (около 160 МБ памяти).
	logoMaxPixels = 40_000_000
	// Для ИИ — копия до 1024 px, если логотип больше 300 КБ.
	aiLogoSide  = 1024
	aiLogoBytes = 300 << 10
)

// Entry — действующая запись реестра.
type Entry struct {
	RegNo           string
	Name            string
	Goods           string
	Codes           []string
	Holder          string
	Document        string
	ProtectedUntil  *time.Time
	Representatives string
	// Logo — data URL логотипа ("" — картинки нет или её не удалось прочитать).
	Logo string
}

// Parsed — итог разбора: действующие записи, сколько исключённых пропущено, номера записей с логотипом,
// который не прочитался (векторный рисунок), и пропущенные строки (повтор номера, пустая запись).
type Parsed struct {
	Entries     []Entry
	Excluded    int
	LogoEntries int
	BadLogos    []string
	Skipped     []string
}

var (
	regNoPattern = regexp.MustCompile(`^\d{3,6}\s*/`)
	codeDigits   = regexp.MustCompile(`\d{4,10}`)
	ruDate       = regexp.MustCompile(`(\d{1,2})\.(\d{1,2})\.(\d{4})`)
	// spaces — пробелы, в том числе неразрывный (\s в Go — только ASCII).
	spaces = regexp.MustCompile(`[\s\p{Zs}]+`)
)

type columns struct{ reg, name, goods, holder, document, until, reps int }

// Parse читает первый лист файла реестра.
func Parse(file io.Reader, size int64) (result Parsed, err error) {
	// Битый файл не должен ронять сервер: разбор двоичного формата и картинок — под recover.
	defer func() {
		if recovered := recover(); recovered != nil {
			result, err = Parsed{}, &service.ValidationError{Field: "file", Message: "Файл повреждён: не удалось прочитать таблицу или картинки"}
		}
	}()
	if size > maxFileBytes {
		return Parsed{}, &service.ValidationError{Field: "file", Message: fmt.Sprintf("Файл больше %d МБ", maxFileBytes>>20)}
	}
	data, err := io.ReadAll(io.LimitReader(file, maxFileBytes+1))
	if err != nil || len(data) > maxFileBytes {
		return Parsed{}, &service.ValidationError{Field: "file", Message: "Файл не прочитался"}
	}
	sheet, err := readXLS(data)
	if err == errNotXLS {
		sheet, err = readXLSX(data)
	}
	if err != nil {
		return Parsed{}, &service.ValidationError{Field: "file", Message: "Файл повреждён или не является Excel (.xls или .xlsx)"}
	}
	return parseSheet(sheet)
}

func readXLSX(data []byte) (sheetData, error) {
	book, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return sheetData{}, err
	}
	defer book.Close()
	name := book.GetSheetName(0)
	rows, err := book.GetRows(name, excelize.Options{RawCellValue: true})
	if err != nil {
		return sheetData{}, err
	}
	if len(rows) > maxRows {
		rows = rows[:maxRows]
	}
	sheet := sheetData{Rows: rows}
	cells, _ := book.GetPictureCells(name)
	for _, cell := range cells {
		col, row, err := excelize.CellNameToCoordinates(cell)
		if err != nil {
			continue
		}
		pictures, _ := book.GetPictures(name, cell)
		for _, picture := range pictures {
			sheet.Pictures = append(sheet.Pictures, sheetPicture{Row: row - 1, Col: col - 1, Ext: picture.Extension, Data: picture.File})
		}
	}
	return sheet, nil
}

func parseSheet(sheet sheetData) (Parsed, error) {
	header, cols := -1, columns{}
	for index, row := range sheet.Rows {
		if found, ok := headerColumns(row); ok {
			header, cols = index, found
			break
		}
		if index > 30 {
			break
		}
	}
	if header < 0 {
		return Parsed{}, &service.ValidationError{Field: "file", Message: "Не найдена таблица реестра: нужны колонки «Регистрационный номер», «Наименование (вид, описание, изображение)», «Наименование товаров» и «Сведения о правообладателе»"}
	}
	// Несколько картинок в ячейке — самая крупная в Excel: бывают сплющенные в линию копии и дубли
	// логотипа (строки 1217, 1222, 1314, 2424 реестра 2026-10).
	// Не прочиталась крупная (векторный EMF) — берётся следующая по размеру.
	logos := map[int][]sheetPicture{}
	for _, picture := range sheet.Pictures {
		if picture.Col == cols.name {
			logos[picture.Row] = append(logos[picture.Row], picture)
		}
	}
	for _, pictures := range logos {
		sort.SliceStable(pictures, func(i, j int) bool { return pictures[i].Area > pictures[j].Area })
	}
	result := Parsed{}
	seen := map[string]int{}
	for index := header + 1; index < len(sheet.Rows); index++ {
		row := sheet.Rows[index]
		cell := func(column int) string {
			if column >= 0 && column < len(row) {
				return strings.TrimSpace(row[column])
			}
			return ""
		}
		regNo, name := spaces.ReplaceAllString(cell(cols.reg), ""), cleanText(cell(cols.name))
		if strings.Contains(strings.ToLower(cell(cols.reg)+" "+name), "исключ") {
			result.Excluded++
			continue
		}
		if !regNoPattern.MatchString(regNo) {
			continue // нумерация колонок, пустые строки, примечания
		}
		if first, ok := seen[regNo]; ok {
			result.Skipped = append(result.Skipped, fmt.Sprintf("строка %d: номер %s уже был в строке %d", index+1, regNo, first))
			continue
		}
		seen[regNo] = index + 1
		entry := Entry{
			RegNo: service.TruncateRunes(regNo, 40), Name: service.TruncateRunes(name, 300),
			Goods: service.TruncateRunes(cleanText(cell(cols.goods)), 4000), Holder: service.TruncateRunes(cleanText(cell(cols.holder)), 1000),
			Document: service.TruncateRunes(cleanText(cell(cols.document)), 500), Representatives: service.TruncateRunes(cleanText(cell(cols.reps)), 1000),
			ProtectedUntil: parseUntil(cell(cols.until)),
		}
		entry.Codes = goodsCodes(entry.Goods)
		pictures := logos[index]
		hasLogo := len(pictures) > 0
		if hasLogo {
			result.LogoEntries++
			for _, picture := range pictures {
				if entry.Logo = logoThumb(picture); entry.Logo != "" {
					break
				}
			}
			if entry.Logo == "" {
				result.BadLogos = append(result.BadLogos, regNo)
			}
		}
		if entry.Name == "" && !hasLogo {
			result.Skipped = append(result.Skipped, fmt.Sprintf("строка %d: у записи %s нет ни наименования, ни изображения знака", index+1, regNo))
			continue
		}
		result.Entries = append(result.Entries, entry)
	}
	if len(result.Entries) == 0 {
		return Parsed{}, &service.ValidationError{Field: "file", Message: "В файле нет действующих записей реестра"}
	}
	return result, nil
}

func headerColumns(row []string) (columns, bool) {
	cols := columns{-1, -1, -1, -1, -1, -1, -1}
	for index, value := range row {
		// Без пробелов и переносов: в заголовках КГД слова разорваны переносом («Наименова⏎ние»).
		text := strings.ToLower(spaces.ReplaceAllString(value, ""))
		switch {
		case strings.Contains(text, "регистрационный"):
			cols.reg = index
		case strings.Contains(text, "наименованиетоваров") || strings.Contains(text, "мкту"):
			cols.goods = index
		case strings.HasPrefix(text, "наименование") && cols.name < 0:
			cols.name = index
		case strings.Contains(text, "доверенн"):
			cols.reps = index
		case strings.Contains(text, "правообладател"):
			cols.holder = index
		case strings.Contains(text, "охранногодокумента"):
			cols.document = index
		case strings.Contains(text, "срок"):
			cols.until = index
		}
	}
	return cols, cols.reg >= 0 && cols.name >= 0 && cols.goods >= 0 && cols.holder >= 0
}

func cleanText(value string) string { return strings.TrimSpace(spaces.ReplaceAllString(value, " ")) }

// goodsCodes — коды ТН ВЭД из колонки товаров: после последней «/» («… / 30 / 170410; 170490»).
func goodsCodes(goods string) []string {
	if index := strings.LastIndex(goods, "/"); index >= 0 {
		goods = goods[index+1:]
	}
	codes := []string{}
	seen := map[string]bool{}
	for _, code := range codeDigits.FindAllString(goods, -1) {
		if !seen[code] {
			seen[code] = true
			codes = append(codes, code)
		}
	}
	return codes
}

// parseUntil — срок защиты: число-дата Excel или «19.07.2028»; nil — нет даты.
func parseUntil(value string) *time.Time {
	if number, err := strconv.ParseFloat(value, 64); err == nil && number > 20000 && number < 80000 {
		if date, err := excelize.ExcelDateToTime(number, false); err == nil {
			date = time.Date(date.Year(), date.Month(), date.Day(), 0, 0, 0, 0, time.UTC)
			return &date
		}
	}
	// «с 01.01.2018 по 19.07.2028» — срок до последней даты; несуществующая дата (31.06) — не дата.
	if matches := ruDate.FindAllStringSubmatch(value, -1); len(matches) > 0 {
		match := matches[len(matches)-1]
		day, _ := strconv.Atoi(match[1])
		month, _ := strconv.Atoi(match[2])
		year, _ := strconv.Atoi(match[3])
		date := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
		if date.Day() == day && int(date.Month()) == month && date.Year() == year {
			return &date
		}
	}
	return nil
}

// logoThumb — логотип data URL в исходном качестве: PNG и JPEG как в файле, остальное (растр из EMF, BMP,
// TIFF) и обрезанные в Excel — PNG без потерь (JPEG — JPEG). Больше logoMaxSide или logoMaxBytes — JPEG
// logoBigSide. "" — не читается (векторный EMF, WMF, битая картинка).
func logoThumb(source sheetPicture) string {
	ext, raw := source.Ext, source.Data
	switch ext {
	case ".emf":
		ext, raw = emfBitmap(raw)
	case ".wmf":
		return ""
	}
	if ext == "" || len(raw) == 0 {
		return ""
	}
	// Размер из заголовка проверяется до распаковки: картинка «60000×60000» в 70 байт — 14 ГБ памяти.
	if config, _, err := image.DecodeConfig(bytes.NewReader(raw)); err != nil || config.Width*config.Height > logoMaxPixels {
		return ""
	}
	picture, format, err := image.Decode(bytes.NewReader(raw))
	if err != nil {
		return ""
	}
	// CMYK-JPEG браузеры и ИИ показывают с искажёнными цветами — перекодируется в обычный.
	_, changed := picture.(*image.CMYK)
	if rect := cropRect(picture.Bounds(), source.Crop); rect != picture.Bounds() {
		if sub, ok := picture.(interface {
			SubImage(image.Rectangle) image.Image
		}); ok {
			picture, changed = sub.SubImage(rect), true
		}
	}
	if changed || (format != "png" && format != "jpeg") {
		var buffer bytes.Buffer
		if format == "jpeg" {
			err = jpeg.Encode(&buffer, picture, &jpeg.Options{Quality: logoQuality})
		} else {
			err, format = png.Encode(&buffer, picture), "png"
		}
		if err != nil {
			return ""
		}
		raw = buffer.Bytes()
	}
	if bounds := picture.Bounds(); max(bounds.Dx(), bounds.Dy()) > logoMaxSide {
		return decisions.ImageThumb(raw, logoBigSide, logoQuality)
	}
	if len(raw) > logoMaxBytes {
		return decisions.ImageThumb(raw, logoBigSide, logoQuality)
	}
	return "data:image/" + format + ";base64," + base64.StdEncoding.EncodeToString(raw)
}

// cropRect — видимая в Excel часть картинки (crop — доли сверху, снизу, слева, справа).
func cropRect(bounds image.Rectangle, crop [4]float64) image.Rectangle {
	width, height := float64(bounds.Dx()), float64(bounds.Dy())
	rect := image.Rect(
		bounds.Min.X+int(crop[2]*width), bounds.Min.Y+int(crop[0]*height),
		bounds.Max.X-int(crop[3]*width), bounds.Max.Y-int(crop[1]*height),
	)
	if rect.Dx() < 2 || rect.Dy() < 2 {
		return bounds
	}
	return rect
}

// AILogo — копия логотипа для ИИ не больше aiLogoSide: пачка из 10 логотипов укладывается в запрос.
func AILogo(dataURL string) string {
	header, data, ok := strings.Cut(dataURL, ",")
	if !ok || len(data) <= aiLogoBytes*4/3 {
		return dataURL
	}
	raw, err := base64.StdEncoding.DecodeString(data)
	if err != nil || !strings.HasPrefix(header, "data:image/") {
		return dataURL
	}
	if small := decisions.ImageThumb(raw, aiLogoSide, logoQuality); small != "" {
		return small
	}
	return dataURL
}
