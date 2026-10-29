package tnvedagent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// Путь кода, обрезанный в выгрузке TWS до двух уровней, помечается «… ›»; полный — без пометки.
func TestCodePathMarksTruncatedPath(t *testing.T) {
	if got := codePath("аппараты точильные и шлифовальные: 🠺 углошлифовальные"); got != "… › аппараты точильные и шлифовальные: › углошлифовальные" {
		t.Fatalf("обрезанный путь: %q", got)
	}
	full := "Инструменты ручные: 🠺 со встроенным электрическим двигателем: 🠺 прочие: 🠺 способные работать без внешнего источника питания"
	if got := codePath(full); strings.HasPrefix(got, "…") || !strings.Contains(got, "› способные работать") {
		t.Fatalf("полный путь: %q", got)
	}
}

// Изменено: блок классификационных решений остаётся на своём месте, перед ним — пометка, что это
// дополнительное доказательство после пояснений; остальной текст не меняется.
func TestDecisionsMarked(t *testing.T) {
	page := "Дерево кода:\n9015 30 100 0 электронные\nМеры нетарифного регулирования\nТР ТС\nКлассификационные решения (1)\n9015 30 100 0\nс 01.03.2026\nЛазерный нивелир классифицируется в подсубпозиции 9015 30 100 0\nДополнительная информация\nПояснения к позиции 9015"
	shown := decisionsMarked(page)
	if strings.Replace(shown, decisionsNote, "", 1) != page || strings.Index(shown, decisionsNote) > strings.Index(shown, "Классификационные решения (1)") || !strings.HasPrefix(shown, "Дерево кода") {
		t.Fatalf("пометка перед решениями, остальное как есть:\n%s", shown)
	}
	if same := decisionsMarked("Дерево кода:\n6116"); same != "Дерево кода:\n6116" {
		t.Fatal("без решений — страница как есть")
	}
}

// Подпись решения — вид и страна того решения, где есть цитата (формат блока как на TWS.BY).
func TestDecisionSource(t *testing.T) {
	page := "Дерево кода\n" + decisionsHeading + " (2)\nПредварительное\n8205 51 009 0\nОвощечистка, ручное приспособление из цинкового сплава.\nОснование : В соответствии с ОПИ 1 и 6\nСтрана : RU - Россия\n" +
		"Предварительное\n8205 51 009 0\nРожок для обуви стальной, термообработанный, используется в домашнем обиходе.\nОснование : В соответствии с ОПИ 1 и 6\nСтрана : KZ - Казахстан\n" + notesHeading + "\nРожки для обуви"
	cases := map[string]string{
		"Рожок для обуви стальной, термообработанный": "предварительное решение · KZ - Казахстан",
		"Овощечистка, ручное приспособление":          "предварительное решение · RU - Россия",
		"рожки для обуви и прочие":                    "", // пояснения — не решение
		"Рожок": "", // коротко
	}
	for quote, want := range cases {
		if got := decisionSource(page, quote); got != want {
			t.Errorf("%q: got %q, want %q", quote, got, want)
		}
	}
}

// Код из файла другой — в итоге оба кода с пошлинами (считает программа); тот же — ничего.
func TestFinishComparesFileCode(t *testing.T) {
	toolbox := newAgentToolbox(&fakeAgentData{})
	toolbox.fileCode = "4015190000"
	result, _, _ := toolbox.finish(context.Background(), `{"description":"Перчатки","name":"Перчатки","confidence":"high","candidates":[{"code":"6116102000","verdict":"fits","reason":"Да."}]}`)
	file := result.FileCode
	if file == nil || file.Code != "4015190000" || !file.Found || file.Duty != "10%" || file.BestCode != "6116102000" || file.BestDuty != "10%" || file.DutyDiffs {
		t.Fatalf("код из файла: %+v", file)
	}
	toolbox.fileCode = "6116102000"
	if result, _, _ := toolbox.finish(context.Background(), `{"description":"Перчатки","name":"Перчатки","confidence":"high","candidates":[{"code":"6116102000","verdict":"fits","reason":"Да."}]}`); result.FileCode != nil {
		t.Fatal("код совпал — сравнения нет")
	}
	toolbox.fileCode = "9999999999"
	if result, _, _ := toolbox.finish(context.Background(), `{"description":"x","name":"x","confidence":"high","candidates":[{"code":"6116102000","verdict":"fits","reason":"Да."}]}`); result.FileCode == nil || result.FileCode.Found || !result.FileCode.DutyDiffs {
		t.Fatalf("кода из файла нет в номенклатуре: %+v", result.FileCode)
	}
}

// В итоге: quote — из пояснений, decisionQuote — из решения со страницы кода с подписью; выдуманное
// решение убирается, а решение в quote не принимается.
func TestFinishDecisionQuote(t *testing.T) {
	toolbox := newAgentToolbox(&fakeAgentData{})
	toolbox.opened["6116102000"] = "Дерево кода\n" + decisionsHeading + " (1)\nПредварительное\n6116 10 200 0\nПерчатки трикотажные с латексным покрытием ладони для хозяйственных работ.\nОснование : В соответствии с ОПИ 1 и 6\nСтрана : RU - Россия\n" +
		notesHeading + "\nВ данную субпозицию включаются трикотажные перчатки с покрытием ладони; одежные крючки; рожки для обуви; стальные и другие мет аллические ножеточки."
	finish := func(quote, decision string) (AgentCandidate, string) {
		raw, _ := json.Marshal(map[string]any{"description": "Перчатки", "name": "Перчатки", "confidence": "high",
			"candidates": []map[string]string{{"code": "6116102000", "verdict": "fits", "reason": "Да.", "quote": quote, "decisionQuote": decision}}})
		result, problem, _ := toolbox.finish(context.Background(), string(raw))
		return result.Candidates[0], problem
	}
	got, problem := finish("включаются трикотажные перчатки с покрытием ладони", "Перчатки трикотажные с латексным покрытием ладони")
	if got.QuoteSource != "page" || got.DecisionQuote == "" || got.DecisionSource != "предварительное решение · RU - Россия" || problem != "" {
		t.Fatalf("пояснение и решение: %+v %q", got, problem)
	}
	if got, _ := finish("Перчатки трикотажные с латексным покрытием ладони", "перчатки для сварщиков из спилка кожи"); got.Quote != "" || got.DecisionQuote != "" || got.DecisionSource != "" {
		t.Fatalf("решение в quote и выдуманное решение убираются: %+v", got)
	}
	// Решение без цитаты пояснений не показывается, модели — замечание переделать итог.
	if got, problem := finish("", "Перчатки трикотажные с латексным покрытием ладони"); got.DecisionQuote != "" || !strings.Contains(problem, "6116 10 200 0") || !strings.Contains(problem, "без цитаты из пояснений") {
		t.Fatalf("решение без пояснений: %+v %q", got, problem)
	}
	// Слово, разорванное на TWS.BY («мет аллические»), не мешает дословной цитате.
	if got, _ := finish("рожки для обуви; стальные и другие металлические ножеточки", ""); got.QuoteSource != "page" {
		t.Fatalf("цитата с разорванным словом: %+v", got)
	}
}

// Порядок доверия и чтение дерева — в подсказке подбора и справочника; самопроверка — по пунктам.
func TestPromptHasSourcePriority(t *testing.T) {
	for name, prompt := range map[string]string{"подбор": tnvedAgentPrompt(""), "справочник": tnvedReferencePrompt("")} {
		for _, want := range []string{"Источники и порядок доверия", "Как читать дерево ТН ВЭД", "Конфликт источников: решай по старшему", "«прочие» — только если"} {
			if !strings.Contains(prompt, want) {
				t.Errorf("%s: нет %q", name, want)
			}
		}
	}
	if strings.Contains(tnvedReferencePrompt(""), "Данные товара: фото") {
		t.Error("справочнику не нужны данные строки сметы")
	}
	for _, want := range []string{"1. Дерево", "2. Позиция", "4. Источники", "5. Код из файла"} {
		if !strings.Contains(agentReviewPrompt, want) {
			t.Errorf("самопроверка без пункта %q", want)
		}
	}
	// Классификационные решения — после пояснений и отдельным полем decisionQuote.
	prompt := tnvedAgentPrompt("")
	if !strings.Contains(prompt, "4. Классификационные решения") || strings.Index(prompt, "3. Пояснения") > strings.Index(prompt, "4. Классификационные решения") || !strings.Contains(prompt, "- decisionQuote") {
		t.Error("решения — пункт 4 после пояснений, цитата решения — в decisionQuote")
	}
}
