package tnvedagent

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/directory"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/utilfee"
)

const (
	testGeneralActURL = "https://adilet.zan.kz/rus/docs/H12EK000079"
	testEECCardURL    = "https://docs.eaeunion.org/docs/ru-ru/01530000/clcd_12102021_137"
	testEECAdiletURL  = "https://adilet.zan.kz/rus/docs/H21EK000137"
)

// Карточка решения Коллегии на docs.eaeunion.org — агент получает сводный текст с adilet, а не описание.
func TestEECCardOpensAdiletText(t *testing.T) {
	toolbox := newAgentToolbox(&fakeAgentData{})
	toolbox.documentsCode = "2903450000"
	toolbox.links[testEECCardURL] = true
	content, event := toolbox.run(context.Background(), "open_link", `{"url":"`+testEECCardURL+`"}`)
	if !strings.Contains(content, "из 2903 45 000 0") || event.Failed || !toolbox.links[testEECAdiletURL] {
		t.Fatalf("card: %s %+v", content, event)
	}
}

var testGeneralActs = []directory.WebLink{{URL: testGeneralActURL, Text: "Решение Коллегии ЕЭК №79 от 13.06.2012 (ТР ТС 019/2011)"}}

func (d *fakeAgentData) GeneralActs(context.Context) ([]directory.WebLink, error) {
	if !d.generalActs {
		return nil, nil
	}
	return testGeneralActs, nil
}

func readAct(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// Выдержки из настоящих сводных текстов adilet: документ после кодов (ТР ТС 010), перед кодами (004),
// коды через запятую, документ в две строки и примечание (017), «из 61» и наименование в несколько строк (019).
func TestFindInAct(t *testing.T) {
	act := directory.WebLink{URL: "https://adilet.zan.kz/rus/docs/H18EK000006", Text: "Решение Коллегии ЕЭК №6"}
	garage := findInAct("8467292000", act, readAct(t, "act_010.txt"))
	if len(garage) != 1 || !strings.HasPrefix(garage[0].heading, "3. Оборудование гаражное") || !garage[0].partial ||
		garage[0].document != "сертификат соответствия" || garage[0].kind != documentKindCertificate {
		t.Fatalf("010: %+v", garage)
	}
	if none := findInAct("8471300000", act, readAct(t, "act_010.txt")); len(none) != 0 {
		t.Fatalf("010 other code: %+v", none)
	}

	dishwasher := findInAct("8422110000", act, readAct(t, "act_004.txt"))
	if len(dishwasher) != 1 || dishwasher[0].product != "машины посудомоечные" || dishwasher[0].document != "сертификат соответствия" ||
		dishwasher[0].partial || !strings.HasPrefix(dishwasher[0].heading, "1. Электрические аппараты") {
		t.Fatalf("004: %+v", dishwasher)
	}

	cotton := findInAct("5208110000", act, readAct(t, "act_017.txt"))
	if len(cotton) < 2 || !strings.HasPrefix(cotton[0].product, "бельевые") || cotton[0].kind != documentKindDeclaration ||
		cotton[0].document != "декларация о соответствии или сертификат соответствия" || !strings.HasPrefix(cotton[0].note, "кроме суровых") {
		t.Fatalf("017: %+v", cotton)
	}

	coat := findInAct("6201300000", act, readAct(t, "act_019.txt"))
	if len(coat) == 0 || !strings.Contains(coat[0].product, "Костюмы мужские и женские для защиты от общих") || coat[0].document != "декларация о соответствии" {
		t.Fatalf("019: %+v", coat)
	}
}

// №299: только раздел II (СГР); код из раздела III не находится.
func TestFindInActSGRSectionOnly(t *testing.T) {
	act := directory.WebLink{URL: directory.SGRActURL, Text: "Решение КТС №299"}
	acids := findInAct("2915900000", act, readAct(t, "act_299.txt"))
	if len(acids) != 1 || acids[0].kind != documentKindRegistration || !acids[0].partial || !strings.HasPrefix(acids[0].product, "Кислоты ациклические") {
		t.Fatalf("299: %+v", acids)
	}
	if paint := findInAct("3208109000", act, readAct(t, "act_299.txt")); len(paint) != 0 {
		t.Fatalf("section III matched: %+v", paint)
	}
}

func TestGeneralDocuments(t *testing.T) {
	act := directory.WebLink{URL: "https://adilet.zan.kz/rus/docs/H18EK000006", Text: "Решение Коллегии ЕЭК №6 от 16.01.2018 (ТР ТС 010/2011)"}
	garage := generalDocuments("8467292000", findInAct("8467292000", act, readAct(t, "act_010.txt")), nil, 14)
	if !strings.Contains(garage.Description, "СС/ДС: нужны, если товар подходит под позицию — №6 (ТР ТС 010/2011)") ||
		!strings.Contains(garage.Description, "СГР: не нужно") || garage.Documents[0].Measure != "Сертификат соответствия — 3. Оборудование гаражное для автотранспортных средств и прицепов" ||
		garage.Documents[0].Applies != "maybe" || garage.Documents[1].Kind != documentKindConditional {
		t.Fatalf("garage %+v", garage)
	}
	// Шины: в перечнях СС/ДС кода нет, утильсбор нужен; непрочитанный перечень — в примечании.
	tyres := generalDocuments("4011100003", nil, []string{"Решение №77"}, 13)
	if !strings.Contains(tyres.Description, "СС/ДС: не нужны — кода нет в перечнях общей базы (прочитано 13)") ||
		!strings.Contains(tyres.Description, "утильсбор: код в перечне") || !strings.Contains(tyres.Notes, "Не удалось прочитать: Решение №77") {
		t.Fatalf("tyres %+v", tyres)
	}
}

// Лимит tnved.info: open_goods_help отдаёт агенту позиции общей базы; перечень открыт — цитаты из него проходят.
func TestDocumentsHelpLimitFallsBackToGeneralBase(t *testing.T) {
	toolbox := newAgentToolbox(&fakeAgentData{helpLimit: true, generalActs: true})
	toolbox.documentsCode = "6116102000"
	content, event := toolbox.run(context.Background(), "open_goods_help", `{"code":"6116102000"}`)
	if !strings.Contains(content, "общая база") || !strings.Contains(content, "из 61, из 62") || !event.Failed || event.URL != testHelpURL ||
		!strings.Contains(event.Detail, "лимит tnved.info") || !strings.Contains(event.Detail, "в 1 перечнях") {
		t.Fatalf("limit answer %s event %+v", content, event)
	}
	if origin := toolbox.quoteOrigin("декларация о соответствии", testGeneralActURL, ""); origin == "" {
		t.Fatal("quote from general act not accepted")
	}
}

// Сверка с общей базой: перечень №79 есть, а в ответе его нет — первый итог возвращается агенту, после
// повтора без него — предупреждение «⚠» в ответе; учтённый перечень (по номеру решения) — без предупреждений.
func TestDocumentsGeneralCheck(t *testing.T) {
	toolbox := newAgentToolbox(&fakeAgentData{generalActs: true})
	toolbox.documentsCode = "6116102000"
	without := `{"description":"Нужен санэпиднадзор.","documents":[{"kind":"other","measure":"Санэпиднадзор","act":"Решение КТС №299","requirement":"Уточнить контакт с пищей.","applies":"maybe"}]}`
	result, problem, err := toolbox.finish(context.Background(), without)
	if err != nil || !strings.Contains(problem, "Сверка с общей базой") || !strings.Contains(problem, testGeneralActURL) {
		t.Fatalf("first finish: %q %v", problem, err)
	}
	result, problem, _ = toolbox.finish(context.Background(), without)
	last := result.Documents[len(result.Documents)-1]
	if problem != "" || !strings.HasPrefix(last.Measure, "⚠ Код есть в перечне №79 (ТР ТС 019/2011)") || last.Applies != "maybe" || last.URL != testGeneralActURL {
		t.Fatalf("second finish: %q %+v", problem, result.Documents)
	}
	with := `{"description":"Декларация по ТР ТС 019.","documents":[{"kind":"dc","measure":"Декларация соответствия ТР ТС 019/2011","act":"Решение Коллегии ЕЭК №79","requirement":"Только если перчатки защитные.","applies":"maybe"}]}`
	fresh := newAgentToolbox(&fakeAgentData{generalActs: true})
	fresh.documentsCode = "6116102000"
	result, problem, _ = fresh.finish(context.Background(), with)
	if problem != "" || len(result.Documents) != 1 {
		t.Fatalf("covered: %q %+v", problem, result.Documents)
	}
}

// Лимит подборов, ответ агента по коду есть — он и показывается, с датой и товаром.
func TestSavedDocumentsResult(t *testing.T) {
	row := repository.CodeDocuments{Code: "8467292000", Product: "дрель", Description: "нужен СС",
		Documents: repository.JSON(`[{"kind":"cc","measure":"сертификат"}]`), CreatedAt: time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)}
	result := savedDocumentsResult(row)
	if result.DocumentsCode != "8467292000" || len(result.Documents) != 1 || result.Documents[0].Kind != "cc" || !strings.Contains(result.Notes, "от 20.10.2026 по товару «дрель»") {
		t.Fatalf("saved %+v", result)
	}
}

func TestUtilFeeObject(t *testing.T) {
	for code, want := range map[string]string{"0401100000": "за упаковку", "4011100003": "за товар", "8703231981": "за товар", "8418102001": "за товар и упаковку", "7308100000": ""} {
		fees, _ := utilfee.Lookup(code)
		if got := utilFeeObject(fees); got != want {
			t.Errorf("%s: %q, want %q", code, got, want)
		}
	}
	if !isUtilFeeMeasure("Расширенные обязательства производителей (импортеров) — утилизация отходов (РОП)") || isUtilFeeMeasure("Санэпиднадзор") {
		t.Fatal("utilfee measure detection")
	}
}
