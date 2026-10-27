package directory

import (
	"strings"
	"testing"
)

// Настоящие страницы tks.ru (2026-10-24): пояснения к группе 82 (примечания и общие положения) и к
// товарной позиции 8204 — только текст блока, без меню, рекламы и тегов.
func TestParseTKSNotes(t *testing.T) {
	group := parseTKSNotes(decodeWindows1251(string(readTestdata(t, "tks_prim_c82.html"))))
	for _, want := range []string{"Группа 82", "Примечания:", "ОБЩИЕ ПОЛОЖЕНИЯ", "В данную группу не включаются:", "относятся к игрушкам (группа 95"} {
		if !strings.Contains(group, want) {
			t.Fatalf("группа 82 без «%s»: %.300s", want, group)
		}
	}
	heading := parseTKSNotes(decodeWindows1251(string(readTestdata(t, "tks_prim_c8204.html"))))
	for _, want := range []string{"8204 12", "- - разводные", "В данную товарную позицию включаются следующие ручные инструменты", "цепные гаечные ключи для труб"} {
		if !strings.Contains(heading, want) {
			t.Fatalf("позиция 8204 без «%s»: %.300s", want, heading)
		}
	}
	for _, text := range []string{group, heading} {
		if strings.Contains(text, "<") || strings.Contains(text, "Реклама") || strings.Contains(text, "jQuery") || strings.Contains(text, "&nbsp;") {
			t.Fatalf("мусор в тексте: %.400s", text)
		}
	}
	if parseTKSNotes("<html><body>нет блока</body></html>") != "" {
		t.Fatal("без блока пояснений — пусто")
	}
	if TKSNotesURL("8204") != "https://www.tks.ru/db/tnved/prim/c8204/" {
		t.Fatal(TKSNotesURL("8204"))
	}
}
