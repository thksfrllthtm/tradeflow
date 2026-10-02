package directory

// «Перечень РК»: коды, по которым Казахстан по обязательствам ВТО применяет ставки ввозной
// пошлины ниже ЕТТ ЕАЭС. PDF ЕЭК разбирает браузер (pdf.js) и присылает строки со сносками;
// из них сервер делает Excel для проверки или сразу сохраняет список. Excel такого же вида
// (в том числе поправленный вручную) тоже загружается. Каждая загрузка заменяет список.

import (
	"context"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
)

type TNVEDKZRateService struct{ repository *repository.Repository }

func NewTNVEDKZRateService(r *repository.Repository) *TNVEDKZRateService {
	return &TNVEDKZRateService{repository: r}
}

// KZListRow — строка перечня, прочитанная из PDF: ставка как в документе и номера сносок.
type KZListRow struct {
	Code  string   `json:"code"`
	Name  string   `json:"name"`
	Rate  string   `json:"rate"`
	Notes []string `json:"notes"`
	Page  int      `json:"page"`
}

// KZListExtract — всё, что браузер прочитал из PDF: строки и тексты сносок по номерам.
type KZListExtract struct {
	FileName  string            `json:"fileName"`
	Rows      []KZListRow       `json:"rows"`
	Footnotes map[string]string `json:"footnotes"`
}

// KZRate — ставка РК для кода и её действие на сегодня.
type KZRate struct {
	RateText  string           `json:"rateText"`
	Duty      NomenclatureDuty `json:"duty"`
	ValidFrom string           `json:"validFrom"`
	ValidTo   string           `json:"validTo"`
	Active    bool             `json:"active"`
	Status    string           `json:"status"`
	NotesText string           `json:"notesText"`
}

type KZImportInfo struct {
	Source         string `json:"source"`
	FileName       string `json:"fileName"`
	RowCount       int    `json:"rowCount"`
	ImportedAt     string `json:"importedAt"`
	ImportedByName string `json:"importedByName"`
}

type KZImportSummary struct {
	Active            int      `json:"active"`
	NotYet            int      `json:"notYet"`
	Expired           int      `json:"expired"`
	NotInNomenclature []string `json:"notInNomenclature"`
}

// KZImportResult: with Errors nothing was saved.
type KZImportResult struct {
	Import  *KZImportInfo          `json:"import,omitempty"`
	Summary *KZImportSummary       `json:"summary,omitempty"`
	Errors  []DirectoryImportError `json:"errors"`
}

const maxKZRows = 5000

// kzZone — время Казахстана (с 2024 года единый пояс UTC+5): «сегодня» для действия ставки.
var kzZone = time.FixedZone("UTC+5", 5*60*60)

func kzToday() time.Time {
	now := time.Now().In(kzZone)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

// ── Ставка ──

var (
	kzNumber   = `(\d+(?:[.,]\d+)?)`
	kzSpecific = kzNumber + ` (евро|долл(?:аров|\.)? США) за (.+)`
	kzPercent  = regexp.MustCompile(`^` + kzNumber + `%?$`)
	kzMinimum  = regexp.MustCompile(`^` + kzNumber + `%?, но не менее ` + kzSpecific + `$`)
	kzOnly     = regexp.MustCompile(`^` + kzSpecific + `$`)
)

// ParseKZRate разбирает ставку перечня: «8» и «6,5» — процент, «0,13 евро за 1 кг» — за
// единицу, «5, но не менее 0,03 евро за 1 кг» — комбинированная. Остальные формы
// («… или … евро за 1 л в зависимости, что ниже», «не более …») — other: только текст.
func ParseKZRate(text string) NomenclatureDuty {
	text = strings.Join(strings.Fields(text), " ")
	number := func(value string) string { return strings.ReplaceAll(value, ",", ".") }
	currency := func(value string) string {
		if value == "евро" {
			return "EUR"
		}
		return "USD"
	}
	unit := func(value string) string { return strings.TrimPrefix(strings.TrimSpace(value), "1 ") }
	if m := kzPercent.FindStringSubmatch(text); m != nil {
		return NomenclatureDuty{Type: "percentage", Percent: number(m[1])}
	}
	if m := kzMinimum.FindStringSubmatch(text); m != nil && !strings.Contains(m[4], ",") {
		return NomenclatureDuty{Type: "combined_minimum", Percent: number(m[1]), Amount: number(m[2]), Currency: currency(m[3]), Unit: unit(m[4])}
	}
	if m := kzOnly.FindStringSubmatch(text); m != nil && !strings.Contains(m[3], ",") {
		return NomenclatureDuty{Type: "specific", Amount: number(m[1]), Currency: currency(m[2]), Unit: unit(m[3])}
	}
	return NomenclatureDuty{Type: "other"}
}

// ── Сноски ──

var (
	kzMonths = map[string]time.Month{"января": 1, "февраля": 2, "марта": 3, "апреля": 4, "мая": 5, "июня": 6, "июля": 7, "августа": 8, "сентября": 9, "октября": 10, "ноября": 11, "декабря": 12}
	kzDate   = regexp.MustCompile(`с (\d{1,2}) ([а-я]+) (\d{4})`)
)

// footnoteValidity: «Ставка … применяется с 1 апреля 2023 г.» — ставка РК с этой даты;
// «Ставка … Единого таможенного тарифа … применяется с 1 января 2026 г.» — ставка РК до
// предыдущего дня (берётся последняя дата: «с 28 марта по 30 сентября 2022 г., а также с 1
// марта 2023 г.» — с 1 марта 2023). «Сноска утратила силу» ничего не меняет.
func footnoteValidity(text string) (from, to *time.Time) {
	all := kzDate.FindAllStringSubmatch(text, -1)
	if len(all) == 0 || strings.Contains(text, "утратила силу") {
		return nil, nil
	}
	m := all[len(all)-1]
	day, _ := strconv.Atoi(m[1])
	year, _ := strconv.Atoi(m[3])
	month, ok := kzMonths[m[2]]
	if !ok {
		return nil, nil
	}
	date := time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
	if strings.Contains(text, "Единого таможенного тарифа") {
		end := date.AddDate(0, 0, -1)
		return nil, &end
	}
	return &date, nil
}

// kzStatus — действует ли ставка РК на дату и как это показать.
func kzStatus(from, to *time.Time, today time.Time) (bool, string) {
	switch {
	case from != nil && from.After(today):
		return false, "с " + from.Format("02.01.2006") + ", до этого ЕТТ"
	case to != nil && to.Before(today):
		return false, "не действует с " + to.AddDate(0, 0, 1).Format("02.01.2006") + ", ставка ЕТТ"
	case to != nil:
		return true, "действует до " + to.Format("02.01.2006")
	}
	return true, "действует"
}

// ── Строки ──

type kzRowErrors struct{ list []DirectoryImportError }

func (e *kzRowErrors) add(row int, message string) {
	if len(e.list) < maxNomenclatureErrors {
		e.list = append(e.list, DirectoryImportError{Row: row, Message: message})
	}
}

// kzRate проверяет строку (row — номер строки для сообщения) и собирает запись базы.
func kzRate(row int, code, name, rateText string, from, to *time.Time, notes, notesText string, page *int, seen map[string]int, errs *kzRowErrors) (repository.TNVEDKZRate, bool) {
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	rateText = strings.Join(strings.Fields(rateText), " ")
	switch {
	case !NomenclatureCodePattern.MatchString(code):
		errs.add(row, fmt.Sprintf("код «%s» — нужно 10 цифр", code))
		return repository.TNVEDKZRate{}, false
	case seen[code] > 0:
		errs.add(row, fmt.Sprintf("код %s повторяется (уже в строке %d)", code, seen[code]))
		return repository.TNVEDKZRate{}, false
	case rateText == "":
		errs.add(row, fmt.Sprintf("у кода %s нет ставки", code))
		return repository.TNVEDKZRate{}, false
	case len([]rune(rateText)) > 300:
		errs.add(row, fmt.Sprintf("у кода %s слишком длинная ставка", code))
		return repository.TNVEDKZRate{}, false
	case from != nil && to != nil && from.After(*to):
		errs.add(row, fmt.Sprintf("у кода %s «Действует с» позже «Действует по»", code))
		return repository.TNVEDKZRate{}, false
	}
	seen[code] = row
	duty := ParseKZRate(rateText)
	return repository.TNVEDKZRate{
		Code: code, Name: strings.TrimSpace(name), RateText: rateText, DutyType: duty.Type,
		DutyPercent: optionalDecimal(duty.Percent), DutyAmount: optionalDecimal(duty.Amount), DutyCurrency: duty.Currency, DutyUnit: service.TruncateRunes(duty.Unit, 60),
		ValidFrom: from, ValidTo: to, Notes: service.TruncateRunes(notes, 60), NotesText: notesText, Page: page,
	}, true
}

func kzRatesFromExtract(extract KZListExtract) ([]repository.TNVEDKZRate, []DirectoryImportError) {
	errs := &kzRowErrors{}
	seen := map[string]int{}
	rates := make([]repository.TNVEDKZRate, 0, len(extract.Rows))
	for index, row := range extract.Rows {
		var from, to *time.Time
		texts := []string{}
		for _, note := range row.Notes {
			text := strings.TrimSpace(extract.Footnotes[note])
			texts = append(texts, note+") "+text)
			noteFrom, noteTo := footnoteValidity(text)
			if noteFrom != nil {
				from = noteFrom
			}
			if noteTo != nil {
				to = noteTo
			}
		}
		page := row.Page
		if rate, ok := kzRate(index+1, row.Code, row.Name, row.Rate, from, to, strings.Join(row.Notes, ", "), strings.Join(texts, " "), &page, seen, errs); ok {
			rates = append(rates, rate)
		}
	}
	return rates, errs.list
}

func (e KZListExtract) validate() error {
	if len(e.Rows) == 0 {
		return &service.ValidationError{Field: "file", Message: "В PDF не найдено ни одной строки перечня. Проверьте, что это «Перечень РК» с сайта ЕЭК"}
	}
	if len(e.Rows) > maxKZRows {
		return &service.ValidationError{Field: "file", Message: fmt.Sprintf("Больше %d строк", maxKZRows)}
	}
	return nil
}

// ── Excel ──

var kzColumns = []directoryColumn{
	{"Код ТН ВЭД", 14}, {"Наименование позиции", 60}, {"Ставка РК", 28}, {"Действует с", 13}, {"Действует по", 13},
	{"Ставка ЕТТ", 24}, {"Сноски", 9}, {"Текст сносок", 60}, {"Стр. PDF", 9},
}

func dateCell(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format("02.01.2006")
}

// Excel — перечень из PDF в виде таблицы для проверки: даты действия уже посчитаны по
// сноскам, рядом ставка ЕТТ из номенклатуры. Ошибки в строках возвращаются в Errors.
func (s *TNVEDKZRateService) Excel(ctx context.Context, extract KZListExtract) ([]byte, []DirectoryImportError, error) {
	if err := extract.validate(); err != nil {
		return nil, nil, err
	}
	rates, rowErrors := kzRatesFromExtract(extract)
	if len(rowErrors) > 0 {
		return nil, rowErrors, nil
	}
	ett, err := s.repository.TNVEDNomenclatureCodeSet(ctx)
	if err != nil {
		return nil, nil, err
	}
	rows := make([][]string, 0, len(rates))
	for _, rate := range rates {
		page := ""
		if rate.Page != nil {
			page = strconv.Itoa(*rate.Page)
		}
		rows = append(rows, []string{rate.Code, rate.Name, rate.RateText, dateCell(rate.ValidFrom), dateCell(rate.ValidTo), ett[rate.Code], rate.Notes, rate.NotesText, page})
	}
	data, err := writeDirectoryXLSX("Перечень РК", kzColumns, rows)
	return data, nil, err
}

// ── Загрузка ──

func (s *TNVEDKZRateService) ImportExtract(ctx context.Context, userID int64, extract KZListExtract) (KZImportResult, error) {
	result := KZImportResult{Errors: []DirectoryImportError{}}
	if err := extract.validate(); err != nil {
		return result, err
	}
	rates, rowErrors := kzRatesFromExtract(extract)
	if len(rowErrors) > 0 {
		result.Errors = rowErrors
		return result, nil
	}
	return s.save(ctx, userID, "pdf", extract.FileName, rates)
}

// ImportExcel читает Excel вида «Excel из PDF»: обязательны «Код ТН ВЭД» и «Ставка РК»,
// даты — «Действует с» / «Действует по» (ДД.ММ.ГГГГ или дата Excel).
func (s *TNVEDKZRateService) ImportExcel(ctx context.Context, userID int64, fileName string, file io.Reader, size int64) (KZImportResult, error) {
	result := KZImportResult{Errors: []DirectoryImportError{}}
	sheet, err := readDirectorySheet(file, size)
	if err != nil {
		return result, err
	}
	if !sheet.has("Код ТН ВЭД") || !sheet.has("Ставка РК") {
		return result, &service.ValidationError{Field: "file", Message: "В первой строке нужны колонки «Код ТН ВЭД» и «Ставка РК». Сделайте Excel из PDF и загрузите его"}
	}
	errs := &kzRowErrors{}
	seen := map[string]int{}
	rates := []repository.TNVEDKZRate{}
	for i := range sheet.rows {
		line := sheet.first + i
		code, _ := sheet.cell(i, "Код ТН ВЭД")
		rateText, _ := sheet.cell(i, "Ставка РК")
		if code == "" && rateText == "" {
			continue
		}
		dates := [2]*time.Time{}
		valid := true
		for index, header := range []string{"Действует с", "Действует по"} {
			stored, _ := sheet.at(sheet.raw, i, header)
			text, err := cellDate(stored)
			if err != nil {
				errs.add(line, fmt.Sprintf("«%s»: %v", header, err))
				valid = false
				continue
			}
			if text != "" {
				parsed, _ := time.Parse("2006-01-02", text)
				dates[index] = &parsed
			}
		}
		if !valid {
			continue
		}
		name, _ := sheet.cell(i, "Наименование позиции")
		notes, _ := sheet.cell(i, "Сноски")
		notesText, _ := sheet.cell(i, "Текст сносок")
		var page *int
		if value, _ := sheet.cell(i, "Стр. PDF"); value != "" {
			if number, err := strconv.Atoi(value); err == nil {
				page = &number
			}
		}
		if rate, ok := kzRate(line, code, name, rateText, dates[0], dates[1], notes, notesText, page, seen, errs); ok {
			rates = append(rates, rate)
		}
	}
	if len(errs.list) > 0 {
		result.Errors = errs.list
		return result, nil
	}
	if len(rates) == 0 {
		return result, &service.ValidationError{Field: "file", Message: "В файле нет строк перечня"}
	}
	return s.save(ctx, userID, "excel", fileName, rates)
}

func (s *TNVEDKZRateService) save(ctx context.Context, userID int64, source, fileName string, rates []repository.TNVEDKZRate) (KZImportResult, error) {
	result := KZImportResult{Errors: []DirectoryImportError{}}
	nomenclature, err := s.repository.TNVEDNomenclatureCodeSet(ctx)
	if err != nil {
		return result, err
	}
	summary := KZImportSummary{NotInNomenclature: []string{}}
	today := kzToday()
	for _, rate := range rates {
		switch active, _ := kzStatus(rate.ValidFrom, rate.ValidTo, today); {
		case active:
			summary.Active++
		case rate.ValidFrom != nil && rate.ValidFrom.After(today):
			summary.NotYet++
		default:
			summary.Expired++
		}
		// Сверка с номенклатурой имеет смысл, только когда она загружена.
		if _, ok := nomenclature[rate.Code]; !ok && len(nomenclature) > 0 {
			summary.NotInNomenclature = append(summary.NotInNomenclature, rate.Code)
		}
	}
	sort.Strings(summary.NotInNomenclature)
	record, err := s.repository.ReplaceTNVEDKZRates(ctx, rates, repository.TNVEDKZRateImport{Source: source, FileName: service.TruncateRunes(fileName, 255), RowCount: len(rates), ImportedBy: &userID})
	if err != nil {
		return result, err
	}
	info := kzImportInfo(record)
	result.Import, result.Summary = &info, &summary
	return result, nil
}

func (s *TNVEDKZRateService) LatestImport(ctx context.Context) (*KZImportInfo, error) {
	record, err := s.repository.LatestTNVEDKZRateImport(ctx)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	info := kzImportInfo(record)
	return &info, nil
}

func kzImportInfo(record repository.TNVEDKZRateImport) KZImportInfo {
	return KZImportInfo{Source: record.Source, FileName: record.FileName, RowCount: record.RowCount, ImportedAt: record.ImportedAt.UTC().Format(time.RFC3339), ImportedByName: record.ImportedByName}
}

// kzRatesForCodes — ставки РК для страницы номенклатуры.
func kzRatesForCodes(ctx context.Context, r *repository.Repository, codes []string) (map[string]*KZRate, error) {
	rows, err := r.TNVEDKZRatesByCodes(ctx, codes)
	if err != nil {
		return nil, err
	}
	today := kzToday()
	rates := make(map[string]*KZRate, len(rows))
	for _, row := range rows {
		active, status := kzStatus(row.ValidFrom, row.ValidTo, today)
		rates[row.Code] = &KZRate{
			RateText:  row.RateText,
			Duty:      NomenclatureDuty{Type: row.DutyType, Percent: service.DecimalText(row.DutyPercent), Amount: service.DecimalText(row.DutyAmount), Currency: row.DutyCurrency, Unit: row.DutyUnit},
			ValidFrom: isoDate(row.ValidFrom), ValidTo: isoDate(row.ValidTo), Active: active, Status: status, NotesText: row.NotesText,
		}
	}
	return rates, nil
}

func isoDate(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format("2006-01-02")
}
