package directory

// Full TN VED EAEU nomenclature with EAEU tariff import duty rates, loaded from the free
// TWS.BY Excel export (https://www.tws.by/tws/tnved/download). The file has a «ТНВЭД» sheet
// with the columns «Код», «Наименование», «Тариф» and a «Система TWS» sheet with the date
// the data is current on. Each import replaces the list; the working directory is not touched.

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/antidumping"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/riskindicators"
	"github.com/shopspring/decimal"
	"github.com/xuri/excelize/v2"
)

const (
	maxNomenclatureRows   = 20000
	maxNomenclatureErrors = 50
	nomenclatureSource    = "TWS.BY"
)

// nomenclatureVAT — основная ставка НДС Казахстана (Налоговый кодекс РК с 2026 года); в
// выгрузке TWS НДС нет, поэтому он ставится каждому коду. Пониженные ставки (5% для
// лекарств и медицинских изделий) и освобождения задаются в справочнике вручную.
var nomenclatureVAT = decimal.NewFromInt(16)

type TNVEDNomenclatureService struct{ repository *repository.Repository }

func NewTNVEDNomenclatureService(r *repository.Repository) *TNVEDNomenclatureService {
	return &TNVEDNomenclatureService{repository: r}
}

// NomenclatureDuty is a tariff rate split into parts. Type: unknown, percentage, specific,
// combined_minimum («N%, но не менее …»), combined_plus («N% плюс …»).
type NomenclatureDuty struct {
	Type     string `json:"type"`
	Percent  string `json:"percent"`
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
	Unit     string `json:"unit"`
}

type NomenclatureCode struct {
	Code       string           `json:"code"`
	Name       string           `json:"name"`
	TariffText string           `json:"tariffText"`
	Duty       NomenclatureDuty `json:"duty"`
	VATPercent string           `json:"vatPercent"`
	// Страница кода в источнике («Подробнее» в выгрузке TWS); пусто, если ссылки нет.
	DetailsURL string `json:"detailsUrl"`
	// Ставка по перечню РК (ниже ЕТТ); nil — кода в перечне нет.
	KZ *KZRate `json:"kz"`
	// Антидемпинговые меры ЕАЭС, под которые попадает код (exact — по точному коду, иначе по
	// товарной позиции: решает описание товара).
	AntiDumping []AntiDumpingRef `json:"antiDumping"`
	// Стоимостной индикатор риска (СУР) для CN; nil — индикатора нет.
	SIR *SIRRef `json:"sir"`
}

// SIRRef — индикатор СУР кода: USD за 1 кг и профиль риска, как берёт смета по СУР.
type SIRRef struct {
	PriceUSD string `json:"priceUsd"`
	Profile  string `json:"profile"`
}

// AntiDumpingRef — мера реестра ЕЭК для кода номенклатуры.
type AntiDumpingRef struct {
	Number       string            `json:"number"`
	ShortName    string            `json:"shortName"`
	Countries    []string          `json:"countries"`
	CountryNames string            `json:"countryNames"`
	Rates        map[string]string `json:"rates"`
	Exact        bool              `json:"exact"`
	Suspended    bool              `json:"suspended"`
	EndDate      string            `json:"endDate"`
	DocumentURL  string            `json:"documentUrl"`
}

type NomenclatureImportInfo struct {
	Source         string `json:"source"`
	FileName       string `json:"fileName"`
	ActualDate     string `json:"actualDate"`
	CodeCount      int    `json:"codeCount"`
	AddedCount     int    `json:"addedCount"`
	RemovedCount   int    `json:"removedCount"`
	ChangedCount   int    `json:"changedCount"`
	ImportedAt     string `json:"importedAt"`
	ImportedByName string `json:"importedByName"`
}

// NomenclatureChange — код, который появился (added), пропал (removed) или сменил
// наименование или ставку (changed) при загрузке.
type NomenclatureChange struct {
	Code          string `json:"code"`
	Kind          string `json:"kind"`
	Name          string `json:"name,omitempty"`
	OldName       string `json:"oldName,omitempty"`
	TariffText    string `json:"tariffText,omitempty"`
	OldTariffText string `json:"oldTariffText,omitempty"`
}

// NomenclatureImportResult: with Errors nothing was saved.
type NomenclatureImportResult struct {
	Import  *NomenclatureImportInfo `json:"import,omitempty"`
	Changes []NomenclatureChange    `json:"changes,omitempty"`
	Errors  []DirectoryImportError  `json:"errors"`
}

// ── Tariff text ──

var (
	nomenclatureNumber   = `(\d+(?:[.,]\d+)?)`
	nomenclatureSpecific = nomenclatureNumber + ` ([A-Z]{3}) за (.+)`
	percentOnlyPattern   = regexp.MustCompile(`^` + nomenclatureNumber + `%$`)
	specificPattern      = regexp.MustCompile(`^` + nomenclatureSpecific + `$`)
	minimumPattern       = regexp.MustCompile(`^` + nomenclatureNumber + `%,? но не менее ` + nomenclatureSpecific + `$`)
	plusPattern          = regexp.MustCompile(`^` + nomenclatureNumber + `% плюс ` + nomenclatureSpecific + `$`)
)

// ParseNomenclatureTariff splits «50%, но не менее 1 EUR за 1 кг». The unit drops the
// leading «1 »: «за 1 кг» → «кг», «за 1 1000 шт» → «1000 шт». Other forms stay unknown with
// the text kept for the operator.
func ParseNomenclatureTariff(text string) NomenclatureDuty {
	text = strings.Join(strings.Fields(text), " ")
	number := func(value string) string { return strings.ReplaceAll(value, ",", ".") }
	unit := func(value string) string { return strings.TrimPrefix(strings.TrimSpace(value), "1 ") }
	if m := percentOnlyPattern.FindStringSubmatch(text); m != nil {
		return NomenclatureDuty{Type: "percentage", Percent: number(m[1])}
	}
	if m := minimumPattern.FindStringSubmatch(text); m != nil {
		return NomenclatureDuty{Type: "combined_minimum", Percent: number(m[1]), Amount: number(m[2]), Currency: m[3], Unit: unit(m[4])}
	}
	if m := plusPattern.FindStringSubmatch(text); m != nil {
		return NomenclatureDuty{Type: "combined_plus", Percent: number(m[1]), Amount: number(m[2]), Currency: m[3], Unit: unit(m[4])}
	}
	if m := specificPattern.FindStringSubmatch(text); m != nil {
		return NomenclatureDuty{Type: "specific", Amount: number(m[1]), Currency: m[2], Unit: unit(m[3])}
	}
	return NomenclatureDuty{Type: "unknown"}
}

// ── File ──

type nomenclatureFile struct {
	codes      []repository.TNVEDNomenclatureCode
	actualDate *time.Time
	errors     []DirectoryImportError
}

var NomenclatureCodePattern = regexp.MustCompile(`^\d{10}$`)

func readNomenclatureFile(file io.Reader) (nomenclatureFile, error) {
	data, err := io.ReadAll(io.LimitReader(file, maxDirectoryImportBytes+1))
	if err != nil || len(data) > maxDirectoryImportBytes {
		return nomenclatureFile{}, &service.ValidationError{Field: "file", Message: "Выберите XLSX-файл размером до 20 МБ"}
	}
	workbook, err := excelize.OpenReader(bytes.NewReader(data))
	if err != nil {
		return nomenclatureFile{}, &service.ValidationError{Field: "file", Message: "Файл повреждён или не является XLSX"}
	}
	defer workbook.Close()
	result := nomenclatureFile{}
	var rows [][]string
	codesSheet := ""
	for _, sheet := range workbook.GetSheetList() {
		sheetRows, err := workbook.GetRows(sheet)
		if err != nil {
			continue
		}
		if len(sheetRows) > 0 && headerIndex(sheetRows[0], "код") >= 0 && headerIndex(sheetRows[0], "наименование") >= 0 {
			rows, codesSheet = sheetRows, sheet
			continue
		}
		// «Актуальность данных | 29.09.2026» on the service sheet.
		for _, row := range sheetRows {
			if len(row) >= 2 && strings.EqualFold(strings.TrimSpace(row[0]), "Актуальность данных") {
				if date, err := time.Parse("02.01.2006", strings.TrimSpace(row[1])); err == nil {
					result.actualDate = &date
				}
			}
		}
	}
	if rows == nil {
		return result, &service.ValidationError{Field: "file", Message: "Не найден лист с колонками «Код» и «Наименование». Загрузите выгрузку ТН ВЭД с сайта TWS.BY"}
	}
	if len(rows)-1 > maxNomenclatureRows {
		return result, &service.ValidationError{Field: "file", Message: fmt.Sprintf("В файле больше %d строк", maxNomenclatureRows)}
	}
	codeColumn, nameColumn, tariffColumn := headerIndex(rows[0], "код"), headerIndex(rows[0], "наименование"), headerIndex(rows[0], "тариф")
	links := nomenclatureLinks(data, codesSheet, headerIndex(rows[0], "подробности"))
	seen := map[string]int{}
	fail := func(row int, message string) {
		if len(result.errors) < maxNomenclatureErrors {
			result.errors = append(result.errors, DirectoryImportError{Row: row, Message: message})
		}
	}
	for index, row := range rows[1:] {
		line := index + 2
		code, name, tariff := cell(row, codeColumn), cell(row, nameColumn), cell(row, tariffColumn)
		if code == "" && name == "" {
			continue
		}
		if !NomenclatureCodePattern.MatchString(code) {
			fail(line, fmt.Sprintf("код «%s» — нужно 10 цифр", code))
			continue
		}
		if previous, ok := seen[code]; ok {
			fail(line, fmt.Sprintf("код %s повторяется (уже в строке %d)", code, previous))
			continue
		}
		seen[code] = line
		if name == "" {
			fail(line, fmt.Sprintf("у кода %s нет наименования", code))
			continue
		}
		if len([]rune(tariff)) > 200 {
			fail(line, fmt.Sprintf("у кода %s слишком длинный тариф", code))
			continue
		}
		duty := ParseNomenclatureTariff(tariff)
		result.codes = append(result.codes, repository.TNVEDNomenclatureCode{
			Code: code, Name: name, TariffText: strings.Join(strings.Fields(tariff), " "), DutyType: duty.Type,
			DutyPercent: optionalDecimal(duty.Percent), DutyAmount: optionalDecimal(duty.Amount), DutyCurrency: duty.Currency, DutyUnit: duty.Unit,
			VATPercent: nomenclatureVAT, DetailsURL: links[line],
		})
	}
	if len(result.codes) == 0 && len(result.errors) == 0 {
		return result, &service.ValidationError{Field: "file", Message: "В файле нет кодов"}
	}
	return result, nil
}

// nomenclatureLinks reads the «Подробности» hyperlinks by row number. Only http(s) links
// are kept: the page shows them as links. Column -1 (no such column) gives no links.
func nomenclatureLinks(data []byte, sheet string, column int) map[int]string {
	links := map[int]string{}
	if column < 0 {
		return links
	}
	columnName, err := excelize.ColumnNumberToName(column + 1)
	if err != nil {
		return links
	}
	cells, err := sheetHyperlinks(data, sheet)
	if err != nil {
		return links
	}
	for ref, target := range cells {
		name, row, err := excelize.SplitCellName(ref)
		if err != nil || name != columnName || len(target) > 500 {
			continue
		}
		if parsed, err := url.Parse(target); err == nil && (parsed.Scheme == "https" || parsed.Scheme == "http") && parsed.Host != "" {
			links[row] = target
		}
	}
	return links
}

func headerIndex(header []string, name string) int {
	for index, value := range header {
		if strings.EqualFold(strings.TrimSpace(value), name) {
			return index
		}
	}
	return -1
}

func cell(row []string, index int) string {
	if index < 0 || index >= len(row) {
		return ""
	}
	return strings.TrimSpace(row[index])
}

func optionalDecimal(value string) *decimal.Decimal {
	if value == "" {
		return nil
	}
	parsed, err := decimal.NewFromString(value)
	if err != nil {
		return nil
	}
	return &parsed
}

// ── Import, list ──

// Import checks the whole file first: one invalid row and nothing is saved. The result
// counts codes added, removed and with a changed name or rate against the previous list.
func (s *TNVEDNomenclatureService) Import(ctx context.Context, userID int64, fileName string, file io.Reader) (NomenclatureImportResult, error) {
	result := NomenclatureImportResult{Errors: []DirectoryImportError{}}
	parsed, err := readNomenclatureFile(file)
	if err != nil {
		return result, err
	}
	if len(parsed.errors) > 0 {
		result.Errors = parsed.errors
		return result, nil
	}
	previous, err := s.repository.AllTNVEDNomenclature(ctx)
	if err != nil {
		return result, err
	}
	record := repository.TNVEDNomenclatureImport{Source: nomenclatureSource, FileName: service.TruncateRunes(fileName, 255), ActualDate: parsed.actualDate, CodeCount: len(parsed.codes), ImportedBy: &userID}
	changes := diffNomenclature(previous, parsed.codes, &record)
	saved, err := s.repository.ReplaceTNVEDNomenclature(ctx, parsed.codes, record)
	if err != nil {
		return result, err
	}
	info := nomenclatureImportInfo(saved)
	result.Import = &info
	if len(changes) > maxNomenclatureChanges {
		changes = changes[:maxNomenclatureChanges]
	}
	result.Changes = changes
	return result, nil
}

// Список изменений в ответе на загрузку ограничен: при первой загрузке «новые» — все коды.
const maxNomenclatureChanges = 500

// diffNomenclature считает новые, удалённые и изменённые коды в record и возвращает их
// списком по порядку кодов.
func diffNomenclature(previous, current []repository.TNVEDNomenclatureCode, record *repository.TNVEDNomenclatureImport) []NomenclatureChange {
	old := make(map[string]repository.TNVEDNomenclatureCode, len(previous))
	for _, code := range previous {
		old[code.Code] = code
	}
	changes := []NomenclatureChange{}
	for _, code := range current {
		before, ok := old[code.Code]
		switch {
		case !ok:
			record.AddedCount++
			changes = append(changes, NomenclatureChange{Code: code.Code, Kind: "added", Name: code.Name, TariffText: code.TariffText})
		case before.Name != code.Name || before.TariffText != code.TariffText:
			record.ChangedCount++
			changes = append(changes, NomenclatureChange{Code: code.Code, Kind: "changed", Name: code.Name, OldName: before.Name, TariffText: code.TariffText, OldTariffText: before.TariffText})
		}
		delete(old, code.Code)
	}
	record.RemovedCount = len(old)
	for _, code := range old {
		changes = append(changes, NomenclatureChange{Code: code.Code, Kind: "removed", OldName: code.Name, OldTariffText: code.TariffText})
	}
	sort.Slice(changes, func(i, j int) bool { return changes[i].Code < changes[j].Code })
	return changes
}

func (s *TNVEDNomenclatureService) List(ctx context.Context, query string, limit, offset int) ([]NomenclatureCode, int64, error) {
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	if offset < 0 {
		offset = 0
	}
	rows, total, err := s.repository.ListTNVEDNomenclature(ctx, repository.TNVEDNomenclatureFilter{Query: query, Limit: limit, Offset: offset})
	if err != nil {
		return nil, 0, err
	}
	items, err := s.withKZRates(ctx, rows)
	return items, total, err
}

const maxNomenclatureLookup = 2000

// Lookup находит вставленные коды (колонку из Excel); кодов, которых нет в номенклатуре,
// в ответе нет.
func (s *TNVEDNomenclatureService) Lookup(ctx context.Context, codes []string) ([]NomenclatureCode, error) {
	if len(codes) > maxNomenclatureLookup {
		return nil, &service.ValidationError{Field: "codes", Message: fmt.Sprintf("Не больше %d кодов за раз", maxNomenclatureLookup)}
	}
	valid := make([]string, 0, len(codes))
	for _, code := range codes {
		if NomenclatureCodePattern.MatchString(code) {
			valid = append(valid, code)
		}
	}
	rows, err := s.repository.TNVEDNomenclatureByCodes(ctx, valid)
	if err != nil {
		return nil, err
	}
	return s.withKZRates(ctx, rows)
}

// withKZRates — строки номенклатуры для ответа вместе со ставками перечня РК.
func (s *TNVEDNomenclatureService) withKZRates(ctx context.Context, rows []repository.TNVEDNomenclatureCode) ([]NomenclatureCode, error) {
	codes := make([]string, 0, len(rows))
	for _, row := range rows {
		codes = append(codes, row.Code)
	}
	kzRates, err := kzRatesForCodes(ctx, s.repository, codes)
	if err != nil {
		return nil, err
	}
	measures, err := antidumping.NewService(s.repository).Measures(ctx)
	if err != nil {
		return nil, err
	}
	indicators, _, err := riskindicators.NewService(s.repository).Lookup(ctx, codes, riskindicators.DefaultCountry)
	if err != nil {
		return nil, err
	}
	items := make([]NomenclatureCode, 0, len(rows))
	for _, row := range rows {
		var sir *SIRRef
		if indicator, ok := indicators[row.Code]; ok {
			sir = &SIRRef{PriceUSD: indicator.PriceUSD.String(), Profile: indicator.Profile}
		}
		items = append(items, NomenclatureCode{
			SIR:  sir,
			Code: row.Code, Name: row.Name, TariffText: row.TariffText,
			VATPercent: row.VATPercent.String(), DetailsURL: row.DetailsURL, KZ: kzRates[row.Code], AntiDumping: antiDumpingRefs(measures, row.Code),
			Duty: NomenclatureDuty{Type: row.DutyType, Percent: service.DecimalText(row.DutyPercent), Amount: service.DecimalText(row.DutyAmount), Currency: row.DutyCurrency, Unit: row.DutyUnit},
		})
	}
	return items, nil
}

// LatestImport is nil while nothing has been imported.
func (s *TNVEDNomenclatureService) LatestImport(ctx context.Context) (*NomenclatureImportInfo, error) {
	record, err := s.repository.LatestTNVEDNomenclatureImport(ctx)
	if errors.Is(err, repository.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	info := nomenclatureImportInfo(record)
	return &info, nil
}

func nomenclatureImportInfo(record repository.TNVEDNomenclatureImport) NomenclatureImportInfo {
	info := NomenclatureImportInfo{
		Source: record.Source, FileName: record.FileName, CodeCount: record.CodeCount,
		AddedCount: record.AddedCount, RemovedCount: record.RemovedCount, ChangedCount: record.ChangedCount,
		ImportedAt: record.ImportedAt.UTC().Format(time.RFC3339), ImportedByName: record.ImportedByName,
	}
	if record.ActualDate != nil {
		info.ActualDate = record.ActualDate.Format("2006-01-02")
	}
	return info
}

// antiDumpingRefs — меры реестра ЕЭК для кода (пустой список, если мер нет).
func antiDumpingRefs(measures []antidumping.Measure, code string) []AntiDumpingRef {
	refs := []AntiDumpingRef{}
	for _, match := range antidumping.MatchCode(measures, code) {
		measure := match.Measure
		names := make([]string, 0, len(measure.Countries))
		for _, country := range measure.Countries {
			if name := antidumping.CountryNames[country]; name != "" {
				names = append(names, name)
			} else {
				names = append(names, country)
			}
		}
		refs = append(refs, AntiDumpingRef{
			Number: measure.Number, ShortName: measure.ShortName, Countries: measure.Countries, CountryNames: strings.Join(names, ", "),
			Rates: measure.Rates, Exact: match.Exact, Suspended: measure.Suspended, EndDate: measure.EndDate, DocumentURL: measure.DocumentURL,
		})
	}
	return refs
}
