package riskindicators

// Разбор приложения к профилю риска КГД РК («24_прил CN.xlsx»): шапка с номером профиля,
// строка групп стран («AD, AG, …», «CN», «00») и строки «№ | код ТН ВЭД | индикатор по группам».

import (
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/xuri/excelize/v2"
)

const (
	maxFileBytes = 20 << 20
	maxRows      = 20000
)

// Workbook — профиль риска из файла: группы стран по колонкам и индикаторы кодов.
type Workbook struct {
	Number string
	Groups [][]string
	Rows   []Row
}

// Row — индикаторы кода по группам Workbook.Groups, USD за 1 кг; "" — индикатора нет («-»).
type Row struct {
	Code   string
	Prices []string
}

// RowError — строка файла (с 1, как в Excel) с ошибкой; с ошибками ничего не сохраняется.
type RowError struct {
	Row     int    `json:"row"`
	Message string `json:"message"`
}

var (
	profileNumber = regexp.MustCompile(`\d{3,}/\d{3,}/\d{3,}`)
	countryToken  = regexp.MustCompile(`^(?:[A-Z]{2}|00)$`)
	excelDecimal  = regexp.MustCompile(`^(\d{9,10})\.0+$`)
)

// Parse читает первый лист, на котором есть таблица индикаторов.
func Parse(file io.Reader, size int64, fileName string) (Workbook, []RowError, error) {
	if size > maxFileBytes {
		return Workbook{}, nil, &service.ValidationError{Field: "file", Message: fmt.Sprintf("Файл больше %d МБ", maxFileBytes>>20)}
	}
	book, err := excelize.OpenReader(file)
	if err != nil {
		return Workbook{}, nil, &service.ValidationError{Field: "file", Message: "Файл повреждён или не является XLSX"}
	}
	defer book.Close()
	for _, sheet := range book.GetSheetList() {
		rows, err := book.GetRows(sheet)
		if err != nil {
			continue
		}
		raw, err := book.GetRows(sheet, excelize.Options{RawCellValue: true})
		if err != nil || len(raw) != len(rows) {
			continue
		}
		if len(rows) > maxRows {
			return Workbook{}, nil, &service.ValidationError{Field: "file", Message: fmt.Sprintf("На листе «%s» больше %d строк", sheet, maxRows)}
		}
		if result, rowErrors, ok := parseSheet(rows, raw); ok {
			if result.Number == "" {
				result.Number = strings.TrimSuffix(filepath.Base(fileName), filepath.Ext(fileName))
			}
			if len(rowErrors) == 0 && len(result.Rows) == 0 {
				return Workbook{}, nil, &service.ValidationError{Field: "file", Message: "В файле нет строк с индикаторами"}
			}
			return result, rowErrors, nil
		}
	}
	return Workbook{}, nil, &service.ValidationError{Field: "file", Message: "Не найдена таблица индикаторов: нужны колонки «№ п/п», «Код товара по ТН ВЭД» и строка стран (AD, AG, … / CN / 00) над первой строкой с кодом"}
}

// parseSheet: первая строка с 10-значным кодом во второй колонке — начало таблицы, строка над ней —
// группы стран. ok=false — на листе нет такой таблицы.
func parseSheet(rows, raw [][]string) (Workbook, []RowError, bool) {
	start := -1
	for index, row := range rows {
		if index > 0 && len(row) > 2 && cellCode(row[1]) != "" {
			start = index
			break
		}
	}
	if start < 1 {
		return Workbook{}, nil, false
	}
	header := rows[start-1]
	result := Workbook{}
	columns := []int{}
	for col := 2; col < len(header); col++ {
		if countries := cellCountries(header[col]); len(countries) > 0 {
			result.Groups = append(result.Groups, countries)
			columns = append(columns, col)
		}
	}
	if len(columns) == 0 {
		return Workbook{}, nil, false
	}
	for _, row := range rows[:start] {
		for _, cell := range row {
			if match := profileNumber.FindString(cell); match != "" && result.Number == "" {
				result.Number = match
			}
		}
	}
	rowErrors := []RowError{}
	seen := map[string]string{}
	for index := start; index < len(rows); index++ {
		line := index + 1
		row := rows[index]
		if len(row) < 2 || strings.TrimSpace(row[1]) == "" {
			continue
		}
		code := cellCode(row[1])
		if code == "" {
			rowErrors = appendError(rowErrors, line, fmt.Sprintf("код ТН ВЭД «%s» — нужно 10 цифр", strings.TrimSpace(row[1])))
			continue
		}
		prices := make([]string, len(columns))
		valid, hasPrice := true, false
		for group, col := range columns {
			price, err := cellPrice(cellAt(row, col), cellAt(raw[index], col))
			if err != nil {
				rowErrors = appendError(rowErrors, line, fmt.Sprintf("код %s, колонка «%s»: %v", code, groupLabel(result.Groups[group]), err))
				valid = false
				continue
			}
			prices[group] = price
			hasPrice = hasPrice || price != ""
		}
		if !valid || !hasPrice {
			continue
		}
		key := strings.Join(prices, "|")
		if previous, exists := seen[code]; exists {
			if previous != key {
				rowErrors = appendError(rowErrors, line, fmt.Sprintf("код %s повторяется с другими индикаторами", code))
			}
			continue
		}
		seen[code] = key
		result.Rows = append(result.Rows, Row{Code: code, Prices: prices})
	}
	return result, rowErrors, true
}

// Сообщений об ошибках не больше 50: остальное — то же самое в других строках.
func appendError(list []RowError, row int, message string) []RowError {
	if len(list) >= 50 {
		return list
	}
	return append(list, RowError{Row: row, Message: message})
}

func cellAt(row []string, col int) string {
	if col < len(row) {
		return strings.TrimSpace(row[col])
	}
	return ""
}

// cellCode — 10-значный код из ячейки: «6107290000», «6107 29 000 0», число Excel без ведущего нуля.
func cellCode(value string) string {
	code := strings.Join(strings.Fields(strings.TrimPrefix(strings.TrimSpace(value), "'")), "")
	if match := excelDecimal.FindStringSubmatch(code); match != nil {
		code = match[1]
	}
	for _, r := range code {
		if r < '0' || r > '9' {
			return ""
		}
	}
	switch len(code) {
	case 10:
		return code
	case 9:
		// Код главы 01–09, сохранённый числом: Excel убрал ведущий ноль.
		return "0" + code
	}
	return ""
}

// cellCountries — коды стран через запятую («AD, AG, AI.», « PL», «00»); не список стран — nil.
func cellCountries(value string) []string {
	tokens := strings.FieldsFunc(strings.ToUpper(value), func(r rune) bool {
		return r == ',' || r == ';' || r == '.' || unicode.IsSpace(r)
	})
	countries := []string{}
	seen := map[string]bool{}
	for _, token := range tokens {
		if !countryToken.MatchString(token) {
			return nil
		}
		if !seen[token] {
			seen[token] = true
			countries = append(countries, token)
		}
	}
	return countries
}

// cellPrice — индикатор USD за 1 кг; «-», пусто и 0 (так бывает в файлах КГД) — индикатора нет.
func cellPrice(formatted, raw string) (string, error) {
	if formatted == "" || formatted == "-" || formatted == "—" || formatted == "–" {
		return "", nil
	}
	value, err := service.ParseDecimal(formatted)
	if err != nil {
		value, err = service.ParseDecimal(raw)
	}
	if err != nil {
		return "", fmt.Errorf("«%s» — не число", formatted)
	}
	if value.IsNegative() {
		return "", fmt.Errorf("индикатор не может быть отрицательным")
	}
	if value.IsZero() {
		return "", nil
	}
	return value.String(), nil
}

// groupLabel — группа стран для сообщений: «CN» или «AD, AG, AI … (95 стран)».
func groupLabel(countries []string) string {
	if len(countries) <= 4 {
		return strings.Join(countries, ", ")
	}
	return fmt.Sprintf("%s … (%d стран)", strings.Join(countries[:3], ", "), len(countries))
}
