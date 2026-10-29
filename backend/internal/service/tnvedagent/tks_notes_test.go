package tnvedagent

import (
	"context"
	"strings"
	"testing"
)

// open_details без страницы TWS.BY: пояснения с tks.ru как резерв, цитата из них принимается; страница
// TWS есть — tks.ru не трогается; нет и tks.ru — прежняя ошибка.
func TestOpenDetailsFallsBackToTKSNotes(t *testing.T) {
	const notes = "Пояснения к ТН ВЭД (tks.ru), группа 40:\nПримечания:\nВ данную группу включаются перчатки из вулканизованной резины для бытовых работ."
	data := &fakeAgentData{tksNotes: notes}
	toolbox := newAgentToolbox(data)
	content, event := toolbox.run(context.Background(), "open_details", `{"code":"4015190000"}`)
	if !strings.Contains(content, "Резерв: страница TWS.BY не открылась") || !strings.Contains(content, "перчатки из вулканизованной резины") || event.Failed {
		t.Fatalf("резерв tks.ru: %s %+v", content, event)
	}
	if !strings.Contains(event.Detail, "пояснения tks.ru") || event.URL != "https://www.tks.ru/db/tnved/prim/c4015/" {
		t.Fatalf("шаг: %+v", event)
	}
	if source := toolbox.quoteSource("4015190000", "перчатки из вулканизованной резины для бытовых работ"); source != "tks" {
		t.Fatalf("цитата из пояснений tks.ru: %q", source)
	}
	if origin := toolbox.quoteOrigin("перчатки из вулканизованной резины для бытовых работ", "", ""); origin != "пояснения к коду 4015 19 000 0 (tks.ru)" {
		t.Fatalf("основание из пояснений tks.ru: %q", origin)
	}

	toolbox.run(context.Background(), "open_details", `{"code":"6116102000"}`)
	if len(data.tksOpened) != 1 {
		t.Fatalf("со страницей TWS tks.ru не нужен: %v", data.tksOpened)
	}

	toolbox = newAgentToolbox(&fakeAgentData{})
	content, event = toolbox.run(context.Background(), "open_details", `{"code":"4015190000"}`)
	if !strings.Contains(content, "Страница не открылась") || !strings.Contains(content, "tks.ru недоступны") || !event.Failed {
		t.Fatalf("без TWS и tks.ru: %s %+v", content, event)
	}
}
