package utilfee

import (
	"errors"
	"strings"
	"testing"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
)

func calc(t *testing.T, input Input) Result {
	t.Helper()
	result, err := Calculate(input)
	if err != nil {
		t.Fatalf("Calculate(%+v): %v", input, err)
	}
	return result
}

func sections(result Result) []int {
	var out []int
	for _, match := range result.Matches {
		out = append(out, match.Section)
	}
	return out
}

// Количество позиций в каждом разделе перечня (приказ № 689, ред. 13.10.2023) — защита от
// пропущенного или лишнего кода при правке data.go.
func TestListHasEveryItem(t *testing.T) {
	want := map[int]int{1: 15, 2: 13, 3: 6, 4: 4, 5: 2, 6: 12, 7: 1, 8: 1, 9: 1, 10: 1, 11: 5, 12: 1, 13: 9, 14: 5, 15: 1, 16: 19, 17: 14, 18: 404, 19: 7}
	for _, sec := range list {
		if got := len(strings.Fields(sec.codes)); got != want[sec.number] {
			t.Errorf("раздел %d: %d кодов, в перечне %d", sec.number, got, want[sec.number])
		}
		for _, code := range strings.Fields(sec.codes) {
			if len(code) < 4 || len(code) > 10 || strings.Trim(code, "0123456789") != "" {
				t.Errorf("раздел %d: плохой код %q", sec.number, code)
			}
		}
	}
	if len(list) != len(want) {
		t.Errorf("разделов %d, ожидалось %d", len(list), len(want))
	}
}

// Микроволновка: товар (разд. 16, k = 2,00) и её упаковка (разд. 18).
func TestProductAndPackaging(t *testing.T) {
	result := calc(t, Input{Code: "8516 50 000 0", Mrp: "4325", MassKg: "500",
		Packaging: []PackagingInput{{Material: "paper", MassKg: "120"}, {Material: "polymer", MassKg: "15,5"}, {Material: "glass", MassKg: ""}}})
	if got := sections(result); len(got) != 2 || got[0] != 16 || got[1] != 18 {
		t.Fatalf("разделы: %v", got)
	}
	if result.Matches[0].Item != "16.13" || result.Matches[1].Item != "18.353" {
		t.Fatalf("пункты: %s, %s", result.Matches[0].Item, result.Matches[1].Item)
	}
	// 0,5 т × 4 325 × 2,00
	if result.Product == nil || result.Product.Amount != "4325.00" {
		t.Fatalf("товар: %+v", result.Product)
	}
	// 0,12 × 4 325 × 1,76 = 913,44; 0,0155 × 4 325 × 1,40 = 93,8525 → 93,85; пустая строка пропущена
	if len(result.Packaging) != 2 || result.Packaging[0].Amount != "913.44" || result.Packaging[1].Amount != "93.85" || result.Packaging[1].Index != 1 {
		t.Fatalf("упаковка: %+v", result.Packaging)
	}
	if result.Total != "5332.29" {
		t.Fatalf("итого %s", result.Total)
	}
}

// Плитка: сама не облагается, платится только за упаковку.
func TestTilesPayOnlyForPackaging(t *testing.T) {
	result := calc(t, Input{Code: "6907210000", Mrp: "4325", MassKg: "20000", Packaging: []PackagingInput{{Material: "paper", MassKg: "120"}}})
	if got := sections(result); len(got) != 1 || got[0] != 18 {
		t.Fatalf("разделы: %v", got)
	}
	if result.Product != nil || result.Total != "913.44" {
		t.Fatalf("товар %+v, итого %s", result.Product, result.Total)
	}
}

func TestVehicles(t *testing.T) {
	cases := []struct {
		name, code, category string
		eaeu                 bool
		want                 string
	}{
		{"легковой 1,5–2 л не из РБ/РФ", "8703231940", "m1-2000", false, "756875.00"}, // 50 × 4 325 × 3,5
		{"легковой 1,5–2 л из РФ", "8703231940", "m1-2000", true, "29410000.00"},      // 50 × 4 325 × 136
		{"электромобиль не из РБ/РФ", "8703800002", "m1-electric", false, "0.00"},
		{"трактор 100 л.с.", "8701930000", "tractor-130", false, "1189375.00"}, // 50 × 4 325 × 5,5
		{"комбайн свыше 400 л.с. из РБ", "8433510001", "combine-over400", true, "54062500.00"},
	}
	for _, tc := range cases {
		result := calc(t, Input{Code: tc.code, Mrp: "4325", VehicleCategory: tc.category, FromBelarusRussia: tc.eaeu})
		if result.Vehicle == nil || result.Vehicle.Amount != tc.want || result.Total != tc.want {
			t.Errorf("%s: %+v, итого %s", tc.name, result.Vehicle, result.Total)
		}
	}
}

func TestVehicleGroupFollowsCode(t *testing.T) {
	result := calc(t, Input{Code: "8704211000"})
	if len(result.Matches) != 1 || result.Matches[0].VehicleGroup != "n" || !strings.Contains(result.Matches[0].Note, "50 т") {
		t.Fatalf("%+v", result.Matches)
	}
	for _, category := range result.VehicleCategories {
		if category.Group != "n" {
			t.Fatalf("чужая категория %s", category.ID)
		}
	}
	// Категория легковых для грузового кода — ошибка, а не расчёт по чужой таблице.
	_, err := Calculate(Input{Code: "8704211000", Mrp: "4325", VehicleCategory: "m1-2000"})
	if !isField(err, "vehicleCategory") {
		t.Fatalf("ожидалась ошибка категории, получено %v", err)
	}
}

func TestZeroCoefficientIsStillInList(t *testing.T) {
	result := calc(t, Input{Code: "3820000000", Mrp: "4325", MassKg: "1000"})
	if got := sections(result); len(got) != 2 || got[0] != 5 || got[1] != 18 {
		t.Fatalf("разделы: %v", got)
	}
	if result.Product == nil || result.Product.Amount != "0.00" || !strings.Contains(result.Matches[0].Note, "омыв") {
		t.Fatalf("%+v %+v", result.Product, result.Matches[0])
	}
}

// Упаковочный товар из разд. 6: k по материалу пункта, а не одна ставка на раздел.
func TestPackagingGoodsUseItemCoefficient(t *testing.T) {
	for code, want := range map[string]string{"3923210000": "1.40", "4819100000": "1.76", "7010901000": "1.14", "7612902000": "0.20"} {
		result := calc(t, Input{Code: code})
		if len(result.Matches) == 0 || result.Matches[0].Section != 6 || result.Matches[0].K != want {
			t.Errorf("%s: %+v", code, result.Matches)
		}
	}
}

func TestLongestPrefixAndExclusions(t *testing.T) {
	// 9504 50 000 — пункт 18.393, а не общий 9504 (18.392).
	if result := calc(t, Input{Code: "9504500000"}); result.Matches[len(result.Matches)-1].Item != "18.393" {
		t.Fatalf("%+v", result.Matches)
	}
	// 8510 90 000 0 исключён из 8510.
	if result := calc(t, Input{Code: "8510900000"}); len(result.Matches) != 0 {
		t.Fatalf("исключённый код найден: %+v", result.Matches)
	}
	if result := calc(t, Input{Code: "8510100000"}); len(result.Matches) != 2 {
		t.Fatalf("бритва: %+v", result.Matches)
	}
	// Не в перечне.
	if result := calc(t, Input{Code: "7208100000", Mrp: "4325", MassKg: "100"}); len(result.Matches) != 0 || result.Total != "" {
		t.Fatalf("%+v", result)
	}
}

func TestValidation(t *testing.T) {
	for _, tc := range []struct {
		input Input
		field string
	}{
		{Input{Code: "8516"}, "code"},
		{Input{Code: "85165000001"}, "code"},
		{Input{Code: "8516500000", MassKg: "500"}, "mrp"},
		{Input{Code: "8516500000", Mrp: "0", MassKg: "500"}, "mrp"},
		{Input{Code: "8516500000", Mrp: "4325", MassKg: "-1"}, "massKg"},
		{Input{Code: "8516500000", Mrp: "4325", Packaging: []PackagingInput{{Material: "wood", MassKg: "1"}}}, "packaging.0.material"},
		{Input{Code: "8703231940", Mrp: "4325", VehicleCategory: "nope"}, "vehicleCategory"},
	} {
		if _, err := Calculate(tc.input); !isField(err, tc.field) {
			t.Errorf("%+v: ожидалась ошибка поля %s, получено %v", tc.input, tc.field, err)
		}
	}
}

func isField(err error, field string) bool {
	var validation *service.ValidationError
	return errors.As(err, &validation) && validation.Field == field
}
