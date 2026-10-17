package tnvedagent

import (
	"context"
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

// Блок классификационных решений — в начало страницы для модели; остальной текст не теряется.
func TestDecisionsFirst(t *testing.T) {
	page := "Дерево кода:\n9015 30 100 0 электронные\nМеры нетарифного регулирования\nТР ТС\nКлассификационные решения (1)\n9015 30 100 0\nс 01.03.2026\nЛазерный нивелир классифицируется в подсубпозиции 9015 30 100 0\nДополнительная информация\nПояснения к позиции 9015"
	shown, found := decisionsFirst(page)
	if !found || !strings.HasPrefix(shown, "Классификационные решения ЕЭК и таможни") || strings.Index(shown, "Лазерный нивелир") > strings.Index(shown, "Дерево кода") {
		t.Fatalf("решения не первыми:\n%s", shown)
	}
	for _, part := range []string{"Дерево кода", "ТР ТС", "с 01.03.2026", "Дополнительная информация\nПояснения к позиции 9015"} {
		if !strings.Contains(shown, part) {
			t.Errorf("потерялось %q", part)
		}
	}
	if same, found := decisionsFirst("Дерево кода:\n6116"); found || same != "Дерево кода:\n6116" {
		t.Fatal("без решений — страница как есть")
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
	for _, want := range []string{"1. Дерево", "2. Решения", "5. Источники", "6. Код из файла"} {
		if !strings.Contains(agentReviewPrompt, want) {
			t.Errorf("самопроверка без пункта %q", want)
		}
	}
}
