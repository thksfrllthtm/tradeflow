// Package utilfee — калькулятор утилизационного платежа РК (Методика, приказ № 448):
//   - товар:     масса (т) × МРП × k раздела перечня;
//   - упаковка:  масса упаковки (т) × МРП × k материала;
//   - транспорт: 50 МРП × коэффициент 1 (или 2 — при ввозе из РБ и РФ).
package utilfee

import (
	"fmt"
	"strings"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/shopspring/decimal"
)

type PackagingInput struct {
	Material string `json:"material"`
	MassKg   string `json:"massKg"`
}

type Input struct {
	Code              string           `json:"code"`
	Mrp               string           `json:"mrp"`
	MassKg            string           `json:"massKg"`
	Packaging         []PackagingInput `json:"packaging"`
	VehicleCategory   string           `json:"vehicleCategory"`
	FromBelarusRussia bool             `json:"fromBelarusRussia"`
}

// Match — позиция перечня, в которую попал код.
type Match struct {
	Section      int    `json:"section"`
	Item         string `json:"item"`   // номер пункта перечня, «16.13»
	Prefix       string `json:"prefix"` // код позиции перечня
	Title        string `json:"title"`
	Kind         Kind   `json:"kind"`
	K            string `json:"k,omitempty"` // только у товара
	Note         string `json:"note,omitempty"`
	VehicleGroup string `json:"vehicleGroup,omitempty"`
}

type ProductAmount struct {
	MassKg string `json:"massKg"`
	K      string `json:"k"`
	Amount string `json:"amount"`
}

type PackagingAmount struct {
	Index    int    `json:"index"` // номер строки упаковки во входных данных
	Material string `json:"material"`
	Label    string `json:"label"`
	K        string `json:"k"`
	MassKg   string `json:"massKg"`
	Amount   string `json:"amount"`
}

type VehicleAmount struct {
	Category    string `json:"category"`
	Label       string `json:"label"`
	BaseMRP     string `json:"baseMrp"`
	Coefficient string `json:"coefficient"`
	Amount      string `json:"amount"`
}

type Result struct {
	Code              string            `json:"code"`
	Edition           string            `json:"edition"`
	Matches           []Match           `json:"matches"`
	Possible          []Match           `json:"possible"` // для неполного кода: позиции длиннее кода — зависят от полного кода, не считаются
	Materials         []Material        `json:"materials"`
	VehicleCategories []VehicleCategory `json:"vehicleCategories"` // категории группы найденного транспорта
	Product           *ProductAmount    `json:"product"`
	Packaging         []PackagingAmount `json:"packaging"`
	Vehicle           *VehicleAmount    `json:"vehicle"`
	Total             string            `json:"total"` // пусто, если ничего не посчитано
}

var thousand = decimal.NewFromInt(1000)

// Calculate находит код в перечне и считает платёж по тем полям, что заполнены. Только код —
// только сведения из перечня.
func Calculate(input Input) (Result, error) {
	code := strings.Join(strings.Fields(input.Code), "")
	if len(code) < 4 || len(code) > 10 || strings.Trim(code, "0123456789") != "" {
		return Result{}, &service.ValidationError{Field: "code", Message: "Введите код ТН ВЭД: от 4 до 10 цифр"}
	}
	matches, possible := Lookup(code)
	result := Result{Code: code, Edition: Edition, Matches: matches, Possible: possible, Materials: Materials, Packaging: []PackagingAmount{}}

	var mrp *decimal.Decimal
	needMRP := func() (decimal.Decimal, error) {
		if mrp != nil {
			return *mrp, nil
		}
		value, err := service.ParsePositive(input.Mrp, "mrp", false)
		if err != nil {
			return decimal.Zero, &service.ValidationError{Field: "mrp", Message: "Укажите МРП — положительное число"}
		}
		mrp = &value
		return value, nil
	}
	total := decimal.Zero
	computed := false

	for _, match := range result.Matches {
		switch match.Kind {
		case KindProduct:
			mass, err := service.ParseOptionalPositive(input.MassKg, "massKg")
			if err != nil || mass == nil {
				if err != nil {
					return Result{}, err
				}
				continue
			}
			mrpValue, err := needMRP()
			if err != nil {
				return Result{}, err
			}
			amount := service.RoundMoney(mass.Div(thousand).Mul(mrpValue).Mul(service.MustDecimal(match.K)))
			result.Product = &ProductAmount{MassKg: mass.String(), K: match.K, Amount: service.MoneyText(amount)}
			total, computed = total.Add(amount), true

		case KindPackaging:
			for index, row := range input.Packaging {
				material, ok := findMaterial(row.Material)
				if !ok {
					return Result{}, &service.ValidationError{Field: fmt.Sprintf("packaging.%d.material", index), Message: "Выберите материал упаковки"}
				}
				mass, err := service.ParseOptionalPositive(row.MassKg, fmt.Sprintf("packaging.%d.massKg", index))
				if err != nil {
					return Result{}, err
				}
				if mass == nil {
					continue
				}
				mrpValue, err := needMRP()
				if err != nil {
					return Result{}, err
				}
				amount := service.RoundMoney(mass.Div(thousand).Mul(mrpValue).Mul(service.MustDecimal(material.K)))
				result.Packaging = append(result.Packaging, PackagingAmount{Index: index, Material: material.ID, Label: material.Label, K: material.K, MassKg: mass.String(), Amount: service.MoneyText(amount)})
				total, computed = total.Add(amount), true
			}

		case KindVehicle:
			for _, category := range VehicleCategories {
				if category.Group == match.VehicleGroup {
					result.VehicleCategories = append(result.VehicleCategories, category)
				}
			}
			if strings.TrimSpace(input.VehicleCategory) == "" {
				continue
			}
			category, ok := findCategory(input.VehicleCategory)
			if !ok || category.Group != match.VehicleGroup {
				return Result{}, &service.ValidationError{Field: "vehicleCategory", Message: "Выберите категорию из списка для этого кода"}
			}
			mrpValue, err := needMRP()
			if err != nil {
				return Result{}, err
			}
			coefficient := category.Coef1
			if input.FromBelarusRussia {
				coefficient = category.Coef2
			}
			amount := service.RoundMoney(service.MustDecimal(BaseMRP).Mul(mrpValue).Mul(service.MustDecimal(coefficient)))
			result.Vehicle = &VehicleAmount{Category: category.ID, Label: category.Label, BaseMRP: BaseMRP, Coefficient: coefficient, Amount: service.MoneyText(amount)}
			total, computed = total.Add(amount), true
		}
	}
	if computed {
		result.Total = service.MoneyText(total)
	}
	return result, nil
}

// Lookup — позиции перечня для кода из 4–10 цифр. matches — в каждом разделе самая длинная позиция,
// которой код начинается (по ним считается платёж). possible — для неполного кода в разделах без
// такой позиции: позиции, которые начинаются с кода, то есть зависят от его продолжения.
func Lookup(code string) (matches, possible []Match) {
	matches, possible = []Match{}, []Match{}
	for _, sec := range list {
		prefixes := strings.Fields(sec.codes)
		best, bestIndex := "", -1
		for index, prefix := range prefixes {
			if hasPrefix(code, prefix) && len(prefix) > len(best) && !isExcluded(code, prefix) {
				best, bestIndex = prefix, index
			}
		}
		if bestIndex >= 0 {
			matches = append(matches, newMatch(sec, bestIndex, best))
			continue
		}
		for index, prefix := range prefixes {
			if len(prefix) > len(code) && hasPrefix(prefix, code) {
				possible = append(possible, newMatch(sec, index, prefix))
			}
		}
	}
	return matches, possible
}

func newMatch(sec section, index int, prefix string) Match {
	match := Match{Section: sec.number, Item: fmt.Sprintf("%d.%d", sec.number, index+1), Prefix: prefix, Title: sec.title, Kind: sec.kind, Note: noteFor(sec.number, prefix)}
	switch sec.kind {
	case KindProduct:
		match.K = sec.k
		if k, ok := itemK[prefix]; ok {
			match.K = k
		}
	case KindVehicle:
		match.VehicleGroup = vehicleGroup(prefix)
	}
	return match
}

func hasPrefix(code, prefix string) bool { return strings.HasPrefix(code, prefix) }

func isExcluded(code, prefix string) bool {
	for _, excludedCode := range excluded[prefix] {
		if hasPrefix(code, excludedCode) {
			return true
		}
	}
	return false
}

func findMaterial(id string) (Material, bool) {
	for _, material := range Materials {
		if material.ID == id {
			return material, true
		}
	}
	return Material{}, false
}

func findCategory(id string) (VehicleCategory, bool) {
	for _, category := range VehicleCategories {
		if category.ID == id {
			return category, true
		}
	}
	return VehicleCategory{}, false
}
