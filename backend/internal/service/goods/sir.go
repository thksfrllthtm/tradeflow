package goods

// Режим сметы «ТС по СИР»: параметры из запроса и индикатор каждой строки.

import (
	"fmt"
	"strings"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/calculation"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/riskindicators"
	"github.com/shopspring/decimal"
)

type sirSettings struct {
	enabled  bool
	weight   string
	discount decimal.Decimal
	country  string
}

// parseSIRSettings проверяет параметры СИР; они сохраняются и в режиме «по инвойсу», чтобы
// переключение режима не сбрасывало вес, снижение и страну.
func parseSIRSettings(input GoodsCalculationInput) (sirSettings, error) {
	settings := sirSettings{weight: "net", country: riskindicators.NormalizeCountry(input.SirCountry)}
	switch mode := strings.ToLower(strings.TrimSpace(input.CustomsValueMode)); mode {
	case "", "invoice":
	case "sir":
		settings.enabled = true
	default:
		return settings, &service.ValidationError{Field: "customsValueMode", Message: "Выберите, как считать таможенную стоимость: по инвойсу или по СИР"}
	}
	switch weight := strings.ToLower(strings.TrimSpace(input.SirWeight)); weight {
	case "", "net":
	case "gross":
		settings.weight = "gross"
	default:
		return settings, &service.ValidationError{Field: "sirWeight", Message: "Вес для СИР — нетто или брутто"}
	}
	if text := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(input.SirDiscountPercent), "%")); text != "" {
		value, err := service.ParseDecimal(strings.TrimPrefix(text, "-"))
		if err != nil || value.IsNegative() || value.GreaterThanOrEqual(decimal.NewFromInt(100)) {
			return settings, &service.ValidationError{Field: "sirDiscountPercent", Message: "Снижение СИР — число от 0 до 99,99 %"}
		}
		settings.discount = value
	}
	if !riskindicators.ValidCountry(settings.country) {
		return settings, &service.ValidationError{Field: "sirCountry", Message: "Страна для СИР — двухбуквенный код, например CN"}
	}
	return settings, nil
}

func (s sirSettings) mode() string {
	if s.enabled {
		return "sir"
	}
	return "invoice"
}

// basis — индикатор строки с кодом code; без индикатора строка считается по инвойсу с пояснением.
func (s sirSettings) basis(code string, indicators map[string]riskindicators.Indicator, loaded bool) *calculation.SIRBasis {
	basis := &calculation.SIRBasis{Country: s.country, Weight: s.weight, DiscountPercent: s.discount}
	switch indicator, found := indicators[code]; {
	case code == "":
		// Строка без кода уже помечена ошибкой «укажите код».
	case !loaded:
		basis.Missing = "СИР: индикаторы не загружены (администратор загружает файлы профилей риска на странице «Номенклатура») — ТС по инвойсу"
	case !found:
		basis.Missing = fmt.Sprintf("СИР: для кода %s и страны %s индикатора нет — ТС по инвойсу", code, s.country)
	default:
		price := indicator.PriceUSD
		basis.PriceUSD, basis.Profile = &price, indicator.Profile
	}
	return basis
}
