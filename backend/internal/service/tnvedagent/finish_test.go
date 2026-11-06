package tnvedagent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// Основание меры — только дословная цитата с открытых страниц: выдуманная убирается; «нужно» без
// основания возвращается агенту; подсказка по наименованию сохраняется.
func TestDocumentsBasisAndNameHint(t *testing.T) {
	toolbox := newAgentToolbox(&fakeAgentData{})
	toolbox.documentsCode = "6116102000"
	toolbox.run(context.Background(), "open_goods_help", `{"code":"6116102000"}`)
	hint := "Если перчатки действительно не контактируют с пищей (паспорт поставщика), наименование: «Перчатки рабочие трикотажные» — санэпиднадзор не нужен."
	finish := func(basis string) (*AgentResult, string) {
		args, _ := json.Marshal(map[string]any{"description": "Санэпиднадзор возможен.", "documents": []map[string]string{
			{"measure": "Санэпиднадзор", "act": "Решение КТС №299", "requirement": "Только изделия для контакта с пищей.", "applies": "yes", "basis": basis, "nameHint": hint},
		}})
		result, problem, _ := toolbox.finish(context.Background(), string(args))
		return result, problem
	}
	result, problem := finish("«только изделия, контактирующие с пищевыми продуктами»")
	if problem != "" || result.Documents[0].Basis != "только изделия, контактирующие с пищевыми продуктами" || result.Documents[0].BasisSource != "справка tnved.info" || result.Documents[0].NameHint != hint || result.QuotesRemoved != 0 {
		t.Fatalf("основание из справки: %+v %s", result.Documents, problem)
	}
	result, problem = finish("мера применяется ко всем перчаткам без исключения")
	if result.Documents[0].Basis != "" || result.QuotesRemoved != 1 || !strings.Contains(problem, "нет основания") {
		t.Fatalf("выдуманное основание: %+v %s", result.Documents, problem)
	}
}

func TestDocumentKind(t *testing.T) {
	for _, tc := range []struct{ kind, text, want string }{
		{"sgr", "что угодно", "sgr"},
		{"CC ", "", "cc"},
		{"", "Входит в перечень продукции, в отношении которой подача ДТ должна сопровождаться представлением там. органу сертификата соответствия согласно ТР ТС 004/2011", "cc"},
		{"", "подача ДТ должна сопровождаться представлением там. органу декларации о соответствии", "dc"},
		{"", "Государственная регистрация специализированной пищевой продукции", "sgr"},
		{"", "Требуется СГР", "sgr"},
		{"", "Условный выпуск по Правилам РК", "conditional"},
		{"", "Входит в перечень продукции, подлежащей государственному санитарно-эпидемиологическому надзору", "other"},
		{"странно", "Ветеринарный контроль", "other"},
	} {
		if got := documentKind(tc.kind, tc.text); got != tc.want {
			t.Errorf("documentKind(%q, %q) = %q, want %q", tc.kind, tc.text, got, tc.want)
		}
	}
}

// Изменено: цитата (quote) — только из пояснений страницы или открытых документов; дерево кода и
// наименование из номенклатуры не доказательство, классификационное решение — отдельное поле decisionQuote.
func TestQuoteSourceNeedsVerbatimText(t *testing.T) {
	toolbox := newAgentToolbox(&fakeAgentData{})
	toolbox.opened["6116102000"] = "Дерево кода:\n6116 10 200 0 перчатки, пропитанные или покрытые резиной\nМеры нетарифного регулирования\nСертификат соответствия на перчатки защитные\n" +
		decisionsHeading + "\nРешение ЕЭК № 12: перчатки хозяйственные с латексным покрытием — 6116 10 200 0\n" +
		notesHeading + "\nВ данную субпозицию включаются трикотажные перчатки с покрытием ладони."
	toolbox.web["https://tnved.info/help/6116102000"] = "Перчатки для защиты рук подлежат санитарному надзору"
	toolbox.web["https://adilet.zan.kz/rus/docs/V1800017400"] = "Перечень продукции, подлежащей обязательной сертификации"
	cases := map[string]string{
		"перчатки для защиты рук подлежат":             "help",     // справка tnved.info
		"перечень продукции, подлежащей обязательной":  "document", // другой документ — не справка tnved.info
		"«трикотажные перчатки, с покрытием ладони»":   "page",     // знаки и регистр не важны
		"перчатки хозяйственные с латексным покрытием": "",         // решение — не quote, а decisionQuote
		"перчатки, пропитанные или покрытые резиной":   "",         // наименование и дерево — не доказательство
		"сертификат соответствия на перчатки защитные": "",         // меры — тоже
		"перчатки из нитрила для медицины":             "",
		"прочие":                 "", // коротко
		"6116 10 200 0 перчатки": "", // номер кода и одно слово ничего не подтверждают
	}
	for quote, want := range cases {
		if got := toolbox.quoteSource("6116102000", strings.Trim(quote, "«»")); got != want {
			t.Errorf("%q: got %q, want %q", quote, got, want)
		}
	}
	// Основание меры в «Документах для ввоза»: решение — решением, страница — страницей, документ — документом.
	origins := map[string]string{
		"перчатки хозяйственные с латексным покрытием": "классификационное решение, код 6116 10 200 0 (TWS.BY)",
		"трикотажные перчатки с покрытием ладони":      "страница кода 6116 10 200 0 (TWS.BY)",
		"перечень продукции, подлежащей обязательной":  "документ " + shortURL("https://adilet.zan.kz/rus/docs/V1800017400"),
	}
	for quote, want := range origins {
		if got := toolbox.quoteOrigin(quote, "", ""); got != want {
			t.Errorf("основание %q: got %q, want %q", quote, got, want)
		}
	}
}
