package trademarks

import (
	"strings"
	"testing"
)

func testIndex() *Index {
	return NewIndex([]Record{
		{RegNo: "1/ТЗ", Name: "SONY", Codes: []string{"851830"}},
		{RegNo: "2/ТЗ", Name: "TOYOTA", Codes: []string{"870323"}},
		{RegNo: "3/ТЗ", Name: "PANASONIC", Codes: []string{"850610"}},
		{RegNo: "4/ТЗ", Name: "ADIDAS", Codes: []string{"640411"}},
		{RegNo: "5/ТЗ", Name: "АРАРАТ", Codes: []string{"220820"}},
		{RegNo: "6/ТЗ", Name: "ARIEL ЭЙРИЛ", Codes: []string{"340220"}},
		{RegNo: "7/ТЗ", Name: "CLEAR", Codes: []string{"330510"}},
		{RegNo: "8/ТЗ", LogoName: "NIVEA MEN", HasLogo: true, Codes: []string{"330499"}},
		{RegNo: "9/ТЗ", Name: "MOET & CHANDON", Codes: []string{"220410"}},
	})
}

func TestMatchBrandsAndLookalikes(t *testing.T) {
	index := testIndex()
	cases := []struct {
		text, code, want string // want — «рег.номер kind», "" — ничего
	}{
		{"Наушники SONV беспроводные", "8518300000", "1/ТЗ lookalike"},
		{"S0NY headphones", "", "1/ТЗ lookalike"},
		{"Sony headphones", "", "1/ТЗ brand"},
		{"ТОYОТА коврики", "", "2/ТЗ lookalike"}, // Т, О, О, Т, А — кириллица
		{"Battery PANASONlC AA", "", "3/ТЗ lookalike"},
		{"Кроссовки ADIBAS", "", "4/ТЗ lookalike"},
		{"Коньяк ARARAT 0,5 л", "", "5/ТЗ brand"},
		{"Коньяк «Арарат»", "", "5/ТЗ brand"},
		{"Порошок стиральный Ariel", "", "6/ТЗ brand"},
		{"Крем Nivea Men", "", "8/ТЗ brand"},
		{"Moet & Chandon brut", "", "9/ТЗ brand"},
		{"Кружка керамическая", "", ""},
		{"Clean cloth", "", ""},
		{"SONG book", "", ""},
		{"Toyo tires", "", ""},
	}
	for _, testCase := range cases {
		hits := index.Match([]string{testCase.text}, testCase.code)
		got := ""
		if len(hits) > 0 {
			got = hits[0].RegNo + " " + hits[0].Kind
		}
		if got != testCase.want {
			t.Errorf("%q: got %q (%+v), want %q", testCase.text, got, hits, testCase.want)
		}
	}
}

func TestMatchSameGoodsFirstAndLogoName(t *testing.T) {
	index := testIndex()
	hits := index.Match([]string{"Toyota Sony", "Nivea Men"}, "8518300000")
	if len(hits) != 3 || hits[0].RegNo != "1/ТЗ" || !hits[0].SameGoods || hits[1].SameGoods {
		t.Fatalf("%+v", hits)
	}
	for _, hit := range hits {
		if hit.RegNo == "8/ТЗ" && (!hit.FromLogo || hit.Name != "NIVEA MEN" || !hit.HasLogo) {
			t.Fatalf("logo hit %+v", hit)
		}
	}
}

func TestVisualKeyIgnoresCaseAndLookalikes(t *testing.T) {
	for _, pair := range [][2]string{{"clean", "CLEAN"}, {"PANASONlC", "PANASONIC"}, {"ТОYОТА", "TOYOTA"}, {"Kern", "KERN"}, {"VVOLF", "WOLF"}} {
		if visualKey(pair[0]) != visualKey(pair[1]) {
			t.Errorf("%q → %q, %q → %q", pair[0], visualKey(pair[0]), pair[1], visualKey(pair[1]))
		}
	}
}

// Записи реестра с картинкой и текстом в одной ячейке (этикетка + «BALLANTINE’S FINEST, Geo. Ballantine B»).
func TestMatchBallantinesVariants(t *testing.T) {
	index := NewIndex([]Record{
		{RegNo: "00727/ТЗ-160916", Name: "BALLANTINE’S", Codes: []string{"220830"}},
		{RegNo: "00728/ТЗ-160916", Name: "BALLANTINE’S FINEST, Geo. Ballantine B", HasLogo: true, Codes: []string{"220830"}},
		{RegNo: "00729/ТЗ-160916", Name: "BALLANTINE’S FINEST B", HasLogo: true, Codes: []string{"220830"}},
	})
	cases := []struct{ text, wantKind string }{
		{"Виски Ballantine's Finest 0,7 л", KindBrand},
		{"Виски BALLANTINES 12 лет", KindBrand},
		{"Виски Balantines", KindLookalike},
		{"Ballantine's Finest", KindBrand},
	}
	for _, testCase := range cases {
		hits := index.Match([]string{testCase.text}, "2208301000")
		if len(hits) == 0 || hits[0].Kind != testCase.wantKind || !hits[0].SameGoods {
			t.Errorf("%q: %+v", testCase.text, hits)
		}
	}
	// «Ballantine's Finest» без буквы B — тот же знак 00729, а не имитация.
	for _, hit := range index.Match([]string{"Ballantine's Finest"}, "") {
		if hit.RegNo == "00729/ТЗ-160916" && hit.Kind != KindBrand {
			t.Errorf("00729: %+v", hit)
		}
	}
}

// Без кода группа товаров — по описанию товаров записи; с кодом другой позиции — «для других товаров».
func TestMatchGoodsByDescription(t *testing.T) {
	index := NewIndex([]Record{{RegNo: "1/ТЗ", Name: "SONY", Goods: "Наушники; микрофоны; изделия прочие / 9 / 851830", Codes: []string{"851830"}}})
	cases := []struct {
		text, code, match string
		other             bool
	}{
		{"Наушники беспроводные SONV", "", "description", false},
		{"Наушники SONY", "8518300000", "code", false},
		{"Кабель SONY", "8544429000", "", true},
		{"Изделия SONY", "", "", false}, // «изделия» — общее слово, не группа
	}
	for _, c := range cases {
		hits := index.Match([]string{c.text}, c.code)
		if len(hits) != 1 || hits[0].GoodsMatch != c.match || hits[0].SameGoods != (c.match != "") || hits[0].OtherGoods != c.other {
			t.Errorf("%q: %+v", c.text, hits)
		}
	}
}

// Кандидаты для сверки фото: похожий рисунок важнее группы товаров; без логотипа и уже найденные — нет.
func TestPictureCandidates(t *testing.T) {
	index := NewIndex([]Record{
		{RegNo: "1/ТЗ", HasLogo: true, LogoPicture: "поезд, рельсы", Goods: "Игрушки / 28 / 950300"},
		{RegNo: "2/ТЗ", HasLogo: true, LogoPicture: "лев, корона", Goods: "Вагоны железнодорожные / 12 / 860500"},
		{RegNo: "3/ТЗ", HasLogo: true, LogoPicture: "круг, полосы", Goods: "Кофе / 30 / 090121"},
		{RegNo: "4/ТЗ", HasLogo: false, LogoPicture: "поезд", Goods: "Вагоны / 12 / 860500"},
		{RegNo: "5/ТЗ", HasLogo: true, LogoPicture: "поезд", Goods: "Кофе / 30 / 090121"},
	})
	hits := index.PictureCandidates([]string{"Поезд", "полосы"}, []string{"вагоны пассажирские"}, "", map[string]bool{"5/ТЗ": true}, 10)
	got := []string{}
	for _, hit := range hits {
		got = append(got, hit.RegNo)
		if !hit.ByPicture {
			t.Fatalf("%+v", hit)
		}
	}
	// 1 — тот же рисунок; 2 — та же группа товаров по описанию; 3 — только «полосы» (общее слово) — нет.
	if strings.Join(got, ",") != "1/ТЗ,2/ТЗ" {
		t.Fatalf("%v", got)
	}
}

// Ложные тревоги на реальных наименованиях компании (база решений, 2026-10): русские окончания и число из
// знака «ВД-40» — не совпадения.
func TestMatchRealFalsePositives(t *testing.T) {
	index := NewIndex([]Record{
		{RegNo: "01163/ТЗ", Name: "ДЕТСКОЕ"}, {RegNo: "00015/ТЗ", Name: "КОРОВКА"}, {RegNo: "02403/ТЗ", Name: "ВД-40"},
	})
	for _, text := range []string{"Детские музыкальные инструменты", "Коронка регулируемая двурезцовая", "方片锁 40-081"} {
		if hits := index.Match([]string{text}, ""); len(hits) != 0 {
			t.Errorf("%q: %+v", text, hits)
		}
	}
	// Сам знак и подделка похожими буквами — находятся.
	for text, want := range map[string]string{"Смазка ВД-40": KindBrand, "Конфеты КОРОBКА": KindLookalike, "Молоко «Детское»": KindBrand} {
		if hits := index.Match([]string{text}, ""); len(hits) != 1 || hits[0].Kind != want {
			t.Errorf("%q: %+v", text, hits)
		}
	}
}
