package utilfee

// Таблица утильсбора по Excel поставщика: в каждой строке код ТН ВЭД, брутто и (необязательно) нетто.
// Нетто не указано — брутто минус процент упаковки (по умолчанию 20 %). Масса упаковки = брутто − нетто,
// материал один на всю таблицу. Каждая строка считается тем же Calculate, что и калькулятор одного кода.

import (
	"fmt"
	"strings"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/shopspring/decimal"
)

const MaxRows = 1000

var (
	defaultPackagingPercent = decimal.NewFromInt(20)
	hundred                 = decimal.NewFromInt(100)
)

type RowInput struct {
	Code    string `json:"code"`
	GrossKg string `json:"grossKg"`
	NetKg   string `json:"netKg"`
}

type RowsInput struct {
	Mrp              string     `json:"mrp"`
	PackagingPercent string     `json:"packagingPercent"` // % упаковки от брутто, когда нетто не указано
	Material         string     `json:"material"`
	ProductWeight    string     `json:"productWeight"` // от какого веса сбор за товар: "net" (по умолчанию) или "gross"
	UsdRate          string     `json:"usdRate"`       // курс НБРК, ₸ за 1 USD; пусто — суммы в долларах не считаются
	Rows             []RowInput `json:"rows"`
}

type RowResult struct {
	Code          string   `json:"code"`
	NetKg         string   `json:"netKg"`
	NetCalculated bool     `json:"netCalculated"`
	PackagingKg   string   `json:"packagingKg"`
	Matches       []Match  `json:"matches"`
	Possible      []Match  `json:"possible"`
	Product       string   `json:"product"`   // сумма за товар; пусто — не платится
	Packaging     string   `json:"packaging"` // сумма за упаковку; пусто — не платится
	Total         string   `json:"total"`     // пусто — строка не посчитана (ошибка, неполный код, транспорт)
	TotalUsd      string   `json:"totalUsd"`  // Total по курсу; пусто — нет курса или суммы
	Error         string   `json:"error"`
	Warnings      []string `json:"warnings"`
}

type RowsResult struct {
	Rows     []RowResult `json:"rows"`
	Total    string      `json:"total"`
	TotalUsd string      `json:"totalUsd"` // итог в тенге по курсу (не сумма строк в долларах)
}

// CalculateRows — утильсбор по строкам. Ошибки таблицы (МРП, материал, %) отклоняют запрос, ошибка строки
// остаётся в строке, остальные считаются.
func CalculateRows(input RowsInput) (RowsResult, error) {
	if _, err := service.ParsePositive(input.Mrp, "mrp", false); err != nil {
		return RowsResult{}, &service.ValidationError{Field: "mrp", Message: "Укажите МРП — положительное число"}
	}
	if _, ok := findMaterial(input.Material); !ok {
		return RowsResult{}, &service.ValidationError{Field: "material", Message: "Выберите материал упаковки"}
	}
	percent, err := service.ParseNonNegativeDefault(input.PackagingPercent, defaultPackagingPercent, "packagingPercent")
	if err != nil || !percent.LessThan(hundred) {
		return RowsResult{}, &service.ValidationError{Field: "packagingPercent", Message: "Процент упаковки — от 0 до 99"}
	}
	byGross := input.ProductWeight == "gross"
	if !byGross && input.ProductWeight != "" && input.ProductWeight != "net" {
		return RowsResult{}, &service.ValidationError{Field: "productWeight", Message: "Сбор за товар — от нетто или от брутто"}
	}
	if len(input.Rows) > MaxRows {
		return RowsResult{}, &service.ValidationError{Field: "rows", Message: fmt.Sprintf("Не больше %d строк за раз", MaxRows)}
	}
	usdRate, err := service.ParseOptionalPositive(input.UsdRate, "usdRate")
	if err != nil {
		return RowsResult{}, &service.ValidationError{Field: "usdRate", Message: "Курс USD — положительное число"}
	}
	inUsd := func(kzt string) string {
		if usdRate == nil || kzt == "" {
			return ""
		}
		return service.MoneyText(service.MustDecimal(kzt).Div(*usdRate))
	}

	netShare := hundred.Sub(percent).Div(hundred)
	result := RowsResult{Rows: make([]RowResult, len(input.Rows))}
	total := decimal.Zero
	for index, row := range input.Rows {
		result.Rows[index] = calculateRow(row, input.Mrp, input.Material, netShare, byGross)
		if amount := result.Rows[index].Total; amount != "" {
			total = total.Add(service.MustDecimal(amount))
			result.Rows[index].TotalUsd = inUsd(amount)
		}
	}
	result.Total = service.MoneyText(total)
	result.TotalUsd = inUsd(result.Total)
	return result, nil
}

func calculateRow(row RowInput, mrp, material string, netShare decimal.Decimal, byGross bool) RowResult {
	out := RowResult{Code: strings.Join(strings.Fields(row.Code), ""), Matches: []Match{}, Possible: []Match{}, Warnings: []string{}}
	if out.Code == "" && strings.TrimSpace(row.GrossKg) == "" && strings.TrimSpace(row.NetKg) == "" {
		return out
	}
	gross, err := service.ParseOptionalPositive(row.GrossKg, "grossKg")
	if err != nil {
		out.Error = "Брутто должно быть положительным числом"
		return out
	}
	if gross == nil {
		out.Error = "Укажите брутто"
		return out
	}
	net, err := service.ParseOptionalPositive(row.NetKg, "netKg")
	if err != nil {
		out.Error = "Нетто должно быть положительным числом"
		return out
	}
	if net == nil {
		value := service.RoundWeightKG(gross.Mul(netShare))
		net, out.NetCalculated = &value, true
	}
	if net.GreaterThan(*gross) {
		out.Error = "Нетто больше брутто"
		return out
	}
	packagingKg := service.RoundWeightKG(gross.Sub(*net))
	out.NetKg, out.PackagingKg = service.WeightKGText(*net), service.WeightKGText(packagingKg)

	// Упаковка считается отдельно и при сборе за товар от брутто — так решено (вес упаковки входит в оба).
	productMass := *net
	if byGross {
		productMass = *gross
	}
	input := Input{Code: out.Code, Mrp: mrp, MassKg: productMass.String(), Packaging: []PackagingInput{}}
	if packagingKg.IsPositive() {
		input.Packaging = append(input.Packaging, PackagingInput{Material: material, MassKg: packagingKg.String()})
	}
	calc, err := Calculate(input)
	if err != nil {
		out.Error = err.Error()
		return out
	}
	out.Matches, out.Possible = calc.Matches, calc.Possible

	incomplete := len(calc.Matches) == 0 && len(calc.Possible) > 0
	sum := decimal.Zero
	for _, match := range calc.Matches {
		if match.Note != "" {
			out.Warnings = append(out.Warnings, match.Note)
		}
		switch match.Kind {
		case KindVehicle:
			incomplete = true
			out.Warnings = append(out.Warnings, "Транспорт и техника: посчитайте в режиме «Один код» — нужна категория")
		case KindPackaging:
			if !packagingKg.IsPositive() {
				out.Warnings = append(out.Warnings, "Масса упаковки 0: брутто равно нетто")
			}
		}
	}
	if len(calc.Possible) > 0 {
		out.Warnings = append(out.Warnings, "Зависит от полного кода: введите код из 10 цифр")
	}
	if calc.Product != nil {
		out.Product = calc.Product.Amount
		sum = sum.Add(service.MustDecimal(calc.Product.Amount))
	}
	for _, packaging := range calc.Packaging {
		out.Packaging = packaging.Amount
		sum = sum.Add(service.MustDecimal(packaging.Amount))
	}
	if !incomplete {
		out.Total = service.MoneyText(sum)
	}
	return out
}
