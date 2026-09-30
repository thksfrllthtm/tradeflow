package service

// Ставки строки сметы — только из номенклатуры ТН ВЭД ЕАЭС: пошлина ЕТТ и НДС кода. Справочник
// ТН ВЭД и карточка товара дают строке название и ИТС, но не ставки; антидемпинга в
// номенклатуре нет, поэтому он не начисляется. Ставки перечня РК пока не применяются.

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
)

// rateErrorPrefix — начало блокирующего сообщения, когда ставки для кода взять неоткуда.
const rateErrorPrefix = "Ставки ТН ВЭД:"

type nomenclatureRates struct {
	loaded     bool
	actualDate string
	byCode     map[string]repository.TNVEDNomenclatureCode
}

var excelDecimalCode = regexp.MustCompile(`^(\d{10})\.0+$`)

// rowTNVEDCode — код строки: из выбранной карточки, иначе из ячейки Excel («8206000000»,
// «8206 00 000 0», «8206000000.0» — число из Excel).
func rowTNVEDCode(snapshot map[string]any, cell string) string {
	if code := stringValue(snapshot, "code"); code != "" {
		cell = code
	}
	code := strings.Join(strings.Fields(strings.TrimPrefix(strings.TrimSpace(cell), "'")), "")
	if m := excelDecimalCode.FindStringSubmatch(code); m != nil {
		code = m[1]
	}
	if nomenclatureCodePattern.MatchString(code) {
		return code
	}
	return ""
}

// loadNomenclatureRates читает номенклатуру для всех кодов сметы одним запросом.
func (s *GlovesService) loadNomenclatureRates(ctx context.Context, codes []string) (nomenclatureRates, error) {
	rates := nomenclatureRates{byCode: map[string]repository.TNVEDNomenclatureCode{}}
	latest, err := s.repository.LatestTNVEDNomenclatureImport(ctx)
	if IsNotFound(err) {
		return rates, nil
	}
	if err != nil {
		return rates, err
	}
	rates.loaded = true
	if latest.ActualDate != nil {
		rates.actualDate = latest.ActualDate.Format("02.01.2006")
	}
	rows, err := s.repository.TNVEDNomenclatureByCodes(ctx, codes)
	if err != nil {
		return rates, err
	}
	for _, row := range rows {
		rates.byCode[row.Code] = row
	}
	return rates, nil
}

// nomenclatureDutyType — схема пошлины для расчёта; «процент плюс сумма» и пустая — неизвестна.
func nomenclatureDutyType(value string) string {
	switch value {
	case "percentage", "specific", "combined_minimum":
		return value
	}
	return "unknown"
}

// nomenclatureName — наименование кода без разделителей уровней дерева («… 🠺 прочие»).
func nomenclatureName(name string) string {
	parts := []string{}
	for _, part := range strings.Split(name, "🠺") {
		if part = strings.TrimSpace(part); part != "" {
			parts = append(parts, part)
		}
	}
	return truncateRunes(strings.Join(parts, " "), 300)
}

// applyNomenclatureRates возвращает снимок строки со ставками из номенклатуры. Название и ИТС
// выбранной карточки остаются; ставки и антидемпинг справочника отбрасываются. Кода нет в
// номенклатуре (или она не загружена) — ставки неизвестны, а rateError объясняет почему.
// Строка без кода и без карточки остаётся без снимка.
func applyNomenclatureRates(snapshot map[string]any, cell string, rates nomenclatureRates) map[string]any {
	code := rowTNVEDCode(snapshot, cell)
	if code == "" {
		return snapshot
	}
	result := map[string]any{}
	for key, value := range snapshot {
		result[key] = value
	}
	delete(result, "antiDumpingPercent")
	delete(result, "verificationStatus")
	result["code"] = code
	result["rateSource"] = "nomenclature"
	row, found := rates.byCode[code]
	if !found {
		result["duty"] = map[string]any{"type": "unknown"}
		result["vat"] = map[string]any{"type": "unknown"}
		if rates.loaded {
			result["rateError"] = fmt.Sprintf("%s кода %s нет в номенклатуре ТН ВЭД ЕАЭС — проверьте код", rateErrorPrefix, code)
		} else {
			result["rateError"] = rateErrorPrefix + " номенклатура ТН ВЭД ЕАЭС не загружена — администратор загружает её на странице «Номенклатура»"
		}
		return result
	}
	source := "Номенклатура ТН ВЭД ЕАЭС (ЕТТ), TWS.BY"
	if rates.actualDate != "" {
		source += " на " + rates.actualDate
	}
	if stringValue(result, "name") == "" {
		result["name"] = nomenclatureName(row.Name)
	}
	result["tariffText"] = row.TariffText
	result["duty"] = map[string]any{
		"type": nomenclatureDutyType(row.DutyType), "percent": decimalText(row.DutyPercent), "amount": decimalText(row.DutyAmount),
		"currency": row.DutyCurrency, "unit": row.DutyUnit, "source": source, "sourceKind": "nomenclature",
	}
	result["vat"] = map[string]any{"type": "percentage", "percent": row.VATPercent.String(), "source": source, "sourceKind": "nomenclature"}
	return result
}
