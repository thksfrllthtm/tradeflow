package calculation

// Ставки строки сметы — только из номенклатуры ТН ВЭД ЕАЭС: пошлина ЕТТ и НДС кода. Справочник
// ТН ВЭД и карточка товара дают строке название и ИТС, но не ставки; антидемпинга в
// номенклатуре нет, поэтому он не начисляется. Ставки перечня РК пока не применяются.

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/directory"
)

// rateErrorPrefix — начало блокирующего сообщения, когда ставки для кода взять неоткуда.
const RateErrorPrefix = "Ставки ТН ВЭД:"

type NomenclatureRates struct {
	Loaded     bool
	ActualDate string
	ByCode     map[string]repository.TNVEDNomenclatureCode
}

var excelDecimalCode = regexp.MustCompile(`^(\d{10})\.0+$`)

// rowTNVEDCode — код строки: из выбранной карточки, иначе из ячейки Excel («8206000000»,
// «8206 00 000 0», «8206000000.0» — число из Excel).
func RowTNVEDCode(snapshot map[string]any, cell string) string {
	if code := service.StringValue(snapshot, "code"); code != "" {
		cell = code
	}
	code := strings.Join(strings.Fields(strings.TrimPrefix(strings.TrimSpace(cell), "'")), "")
	if m := excelDecimalCode.FindStringSubmatch(code); m != nil {
		code = m[1]
	}
	if directory.NomenclatureCodePattern.MatchString(code) {
		return code
	}
	return ""
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
	return service.TruncateRunes(strings.Join(parts, " "), 300)
}

// applyNomenclatureRates возвращает снимок строки со ставками из номенклатуры. Название и ИТС
// выбранной карточки остаются; ставки и антидемпинг справочника отбрасываются. Кода нет в
// номенклатуре (или она не загружена) — ставки неизвестны, а rateError объясняет почему.
// Строка без кода и без карточки остаётся без снимка.
func ApplyNomenclatureRates(snapshot map[string]any, cell string, rates NomenclatureRates) map[string]any {
	code := RowTNVEDCode(snapshot, cell)
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
	row, found := rates.ByCode[code]
	if !found {
		result["duty"] = map[string]any{"type": "unknown"}
		result["vat"] = map[string]any{"type": "unknown"}
		if rates.Loaded {
			result["rateError"] = fmt.Sprintf("%s кода %s нет в номенклатуре ТН ВЭД ЕАЭС — проверьте код", RateErrorPrefix, code)
		} else {
			result["rateError"] = RateErrorPrefix + " номенклатура ТН ВЭД ЕАЭС не загружена — администратор загружает её на странице «Номенклатура»"
		}
		return result
	}
	source := "Номенклатура ТН ВЭД ЕАЭС (ЕТТ), TWS.BY"
	if rates.ActualDate != "" {
		source += " на " + rates.ActualDate
	}
	if service.StringValue(result, "name") == "" {
		result["name"] = nomenclatureName(row.Name)
	}
	result["tariffText"] = row.TariffText
	result["duty"] = map[string]any{
		"type": nomenclatureDutyType(row.DutyType), "percent": service.DecimalText(row.DutyPercent), "amount": service.DecimalText(row.DutyAmount),
		"currency": row.DutyCurrency, "unit": row.DutyUnit, "source": source, "sourceKind": "nomenclature",
	}
	result["vat"] = map[string]any{"type": "percentage", "percent": row.VATPercent.String(), "source": source, "sourceKind": "nomenclature"}
	return result
}
