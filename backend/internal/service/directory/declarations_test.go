package directory

import (
	"strings"
	"testing"
)

// Настоящая страница ifcg.ru (2026-10-16): поиск «ключ газовый» — позиции по убыванию числа записей
// деклараций, примеры описаний без пустых «(ФИРМА) ; (TM)».
func TestParseDeclarationsByHeading(t *testing.T) {
	groups := parseDeclarations(string(readTestdata(t, "ifcg_search_klyuch_gazovyi.html")))
	if len(groups) < 10 || groups[0].Code != "8204" || groups[0].Count != 113 || groups[1].Code != "7318" || groups[1].Count != 61 {
		t.Fatalf("позиции: %d, первые %+v %+v", len(groups), groups[0], groups[1])
	}
	first := groups[0]
	if !strings.HasPrefix(first.Name, "Ключи гаечные ручные") || len(first.Examples) == 0 || first.Examples[0].Code != "8204110000" {
		t.Fatalf("позиция 8204: %+v", first)
	}
	for _, group := range groups {
		for _, example := range group.Examples {
			if strings.Contains(example.Text, "<b>") || strings.Contains(example.Text, "</") || strings.HasSuffix(example.Text, "(TM)") || len(example.Code) != 10 || !strings.HasPrefix(example.Code, group.Code) {
				t.Fatalf("пример %s: %+v", group.Code, example)
			}
		}
	}
}

// Внутри позиции 8204 — 10-значные коды с числом записей.
func TestParseDeclarationsWithinHeading(t *testing.T) {
	groups := parseDeclarations(string(readTestdata(t, "ifcg_search_klyuch_gazovyi_8204.html")))
	want := map[string]int{"8204120000": 53, "8204110000": 32, "8204200000": 28}
	if len(groups) != len(want) {
		t.Fatalf("коды: %+v", groups)
	}
	for _, group := range groups {
		if want[group.Code] != group.Count || len(group.Examples) == 0 {
			t.Errorf("%s: %d записей, примеров %d", group.Code, group.Count, len(group.Examples))
		}
	}
	if groups[0].Name != "Ключи гаечные ручные разводные" || !strings.Contains(groups[0].Examples[0].Text, "КЛЮЧ ТРУБНЫЙ (ГАЗОВЫЙ)") {
		t.Fatalf("код 8204120000: %+v", groups[0])
	}
}

func TestParseDeclarationsNothingFound(t *testing.T) {
	if groups := parseDeclarations(string(readTestdata(t, "ifcg_search_nothing.html"))); groups == nil || len(groups) != 0 {
		t.Fatalf("ничего не нашлось: %+v", groups)
	}
	if got := DeclarationsURL("ключ & газовый", "8204"); got != "https://www.ifcg.ru/kb/tnved/search/?s=stat&q=%D0%BA%D0%BB%D1%8E%D1%87+%26+%D0%B3%D0%B0%D0%B7%D0%BE%D0%B2%D1%8B%D0%B9&g=8204" {
		t.Fatalf("адрес поиска: %s", got)
	}
}

// Настоящие страницы tks.ru (windows-1251, 2026-10-16): «блендер» — коды с числом записей по убыванию,
// описания первой страницы — к своим кодам.
func TestParseTKS(t *testing.T) {
	groups := parseTKS(decodeWindows1251(string(readTestdata(t, "tks_goods_blender_grouped.html"))), decodeWindows1251(string(readTestdata(t, "tks_goods_blender.html"))))
	if len(groups) < 20 || groups[0].Code != "8509400000" || groups[0].Count != 98 || groups[1].Code != "8509900000" || groups[1].Count != 15 {
		t.Fatalf("коды: %d, первые %+v %+v", len(groups), groups[0], groups[1])
	}
	if !strings.HasPrefix(groups[0].Name, "МАШИНЫ БЫТОВЫЕ ИЗМЕЛЬЧИТЕЛИ") || len(groups[0].Examples) < 10 || !strings.Contains(groups[0].Examples[0].Text, "БЛЕНДЕР SILENTMIXX") {
		t.Fatalf("код 8509400000: %+v", groups[0])
	}
	examples := 0
	for _, group := range groups {
		for _, example := range group.Examples {
			examples++
			if example.Code != group.Code || strings.Contains(example.Text, "<") {
				t.Fatalf("пример %s: %+v", group.Code, example)
			}
		}
	}
	if examples != 20 {
		t.Fatalf("описаний первой страницы: %d", examples)
	}
	if address, ok := tksURL("блендер ручной", true); !ok || address != "https://www.tks.ru/db/tnved/goods/?searchstr=%E1%EB%E5%ED%E4%E5%F0+%F0%F3%F7%ED%EE%E9&g=on" {
		t.Fatalf("адрес tks.ru: %s", address)
	}
	if _, ok := tksURL("搅拌机", false); ok {
		t.Fatal("китайский запрос tks.ru не ищет")
	}
}
