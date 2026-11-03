package tnvedagent

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/shopspring/decimal"
)

// shadowScripted — сценарная модель в роли тени: 1 USD за 1 000 токенов вывода.
type shadowScripted struct {
	*scriptedModel
	vision bool
}

func (m shadowScripted) Cost(usage agentUsage) decimal.Decimal {
	return decimal.NewFromInt(usage.CompletionTokens).Div(decimal.NewFromInt(1000))
}

func (m shadowScripted) Vision() bool { return m.vision }

func messageKinds(changes map[string]any) string {
	var messages []map[string]any
	_ = json.Unmarshal([]byte(changes["messages"].(repository.JSON)), &messages)
	kinds := []string{}
	for _, message := range messages {
		kinds = append(kinds, message["kind"].(string))
	}
	return strings.Join(kinds, ",")
}

func TestShadowLimit(t *testing.T) {
	settings := ShadowSettings{BudgetUSD: decimal.RequireFromString("80"), DailyLimit: 2}
	for _, c := range []struct {
		spent string
		today int64
		want  string
	}{
		{"10", 1, ""},
		{"80", 0, "Бюджет тени исчерпан"},
		{"95.5", 0, "Бюджет тени исчерпан"},
		{"10", 2, "Дневной лимит тени исчерпан"},
	} {
		if got := shadowLimit(settings, decimal.RequireFromString(c.spent), c.today); !strings.HasPrefix(got, c.want) || (c.want == "" && got != "") {
			t.Fatalf("потрачено %s, сегодня %d: %q", c.spent, c.today, got)
		}
	}
	if got := shadowLimit(ShadowSettings{BudgetUSD: decimal.NewFromInt(1)}, decimal.Zero, 1000); got != "" {
		t.Fatalf("без дневного лимита: %q", got)
	}
}

// Первый ход тени: своя копия переписки; фото видит сама (описание от модели для фото убрано, картинки есть);
// на вопрос сотруднику отвечает себе сама и доводит до итога; итог, лента и расход уходят в save.
func TestExecuteShadow(t *testing.T) {
	model := &scriptedModel{replies: []agentReply{
		{ToolCalls: []agentToolCall{call("q", "ask_user", map[string]string{"question": "Есть покрытие?"})}, Usage: agentUsage{CompletionTokens: 500}},
		{ToolCalls: []agentToolCall{finishWith(nil)}, Usage: agentUsage{CompletionTokens: 1500}},
		{ToolCalls: []agentToolCall{finishWith(nil)}, Usage: agentUsage{CompletionTokens: 1000}},
	}}
	agent := &TNVEDAgentService{data: &fakeAgentData{}, shadow: shadowScripted{model, true}}
	runner, _ := newTestRunner(&scriptedModel{}, &fakeAgentData{})
	transcript := append(runner.transcript, map[string]any{"role": "user", "content": photoNotePrefix + ": перчатки с покрытием"})
	before, _ := json.Marshal(transcript)
	saves := []map[string]any{}
	final := agent.executeShadow(context.Background(), repository.TNVEDAgentSession{ID: 7}, newShadowState(transcript, true), runner.photos, func(changes map[string]any) { saves = append(saves, changes) })
	if after, _ := json.Marshal(transcript); string(after) != string(before) {
		t.Fatal("тень изменила переписку основной модели")
	}
	seen, _ := json.Marshal(model.lastSeen)
	if strings.Contains(string(seen), photoNotePrefix) || !strings.Contains(string(seen), "image_url") {
		t.Fatalf("тень с фото: описание убрано, картинки есть: %s", seen)
	}
	result, _ := final["result"].(repository.JSON)
	if final["status"] != "done" || !strings.Contains(string(result), "6116102000") || final["error"] != nil || final["finished_at"] == nil {
		t.Fatalf("итог: %v", final)
	}
	if kinds := messageKinds(final); !strings.Contains(kinds, "question,notice") || !strings.HasSuffix(kinds, "result") {
		t.Fatalf("лента: %s", kinds)
	}
	var tokens int64
	for _, reply := range model.replies[:model.calls] {
		tokens += reply.Usage.CompletionTokens
	}
	cost, _ := final["cost_usd"].(decimal.Decimal)
	if len(saves) != model.calls || final["completion_tokens"] != tokens || !cost.Equal(decimal.NewFromInt(tokens).Div(decimal.NewFromInt(1000))) || final["steps"] != model.calls {
		t.Fatalf("расход: вызовов %d, записей %d, токенов %v, цена %s, шагов %v", model.calls, len(saves), final["completion_tokens"], cost, final["steps"])
	}
}

// Повторный ход: сообщение сотрудника дописывается в переписку и ленту тени, расход и шаги копятся к прошлым,
// finished_at первого ответа не трогается.
func TestExecuteShadowContinues(t *testing.T) {
	model := &scriptedModel{replies: []agentReply{{ToolCalls: []agentToolCall{finishWith(nil)}, Usage: agentUsage{PromptTokens: 300, CompletionTokens: 2000}}, {ToolCalls: []agentToolCall{finishWith(nil)}}}}
	agent := &TNVEDAgentService{data: &fakeAgentData{}, shadow: shadowScripted{model, true}}
	runner, _ := newTestRunner(&scriptedModel{}, &fakeAgentData{})
	previous, _ := json.Marshal(append(runner.transcript, map[string]any{"role": "assistant", "content": "Перчатки 6116102000."}))
	messages, _ := json.Marshal([]map[string]any{{"kind": "result", "content": map[string]any{}}})
	state := loadShadowState(repository.TNVEDAgentShadowRun{Transcript: repository.JSON(previous), Messages: repository.JSON(messages), Steps: 4, PromptTokens: 1000, CompletionTokens: 500, CostUSD: decimal.RequireFromString("0.5")})
	state.say("Это резиновые перчатки без трикотажа")
	final := agent.executeShadow(context.Background(), repository.TNVEDAgentSession{ID: 7}, state, runner.photos, func(map[string]any) {})
	seen, _ := json.Marshal(model.lastSeen)
	if !strings.Contains(string(seen), "без трикотажа") || !strings.Contains(string(seen), "Перчатки 6116102000.") {
		t.Fatalf("тень не видит прошлую переписку и уточнение: %s", seen)
	}
	if kinds := messageKinds(final); !strings.HasPrefix(kinds, "result,user") || !strings.HasSuffix(kinds, "result") {
		t.Fatalf("лента: %s", kinds)
	}
	cost, _ := final["cost_usd"].(decimal.Decimal)
	if final["status"] != "done" || final["finished_at"] != nil || final["steps"].(int) < 5 || final["prompt_tokens"] != int64(1300) || !cost.Equal(decimal.RequireFromString("2.5")) {
		t.Fatalf("повторный ход: %v", final)
	}
	rerun := shadowState{}
	rerun.say(rerunPrompt)
	if rerun.messages[0]["kind"] != "rerun" {
		t.Fatalf("«подобрать заново» — разделитель: %v", rerun.messages)
	}
}

// Без тени ничего не запускается; повторный ход берёт последнее сообщение сотрудника.
func TestLaunchShadowRules(t *testing.T) {
	agent := &TNVEDAgentService{}
	agent.launchShadow(repository.TNVEDAgentSession{DocumentsCode: "6116102000"}, nil, nil)
	agent.shadow = shadowScripted{&scriptedModel{}, true}
	// Повтор без сообщения сотрудника: до continueShadow с nil-репозиторием не доходит.
	agent.launchShadow(repository.TNVEDAgentSession{Runs: 1}, []map[string]any{{"role": "assistant", "content": "x"}}, nil)
	if lastUserMessage([]map[string]any{{"role": "user", "content": "вопрос"}, {"role": "user", "content": "уточнение"}}) != "уточнение" {
		t.Fatal("последнее сообщение сотрудника")
	}
	if lastUserMessage(nil) != "" || lastUserMessage([]map[string]any{{"role": "user", "content": []any{}}}) != "" {
		t.Fatal("нет текста — нет хода")
	}
}

// shadowActs — модель под-агентов по актам в роли тени.
type shadowActs struct{ *actModel }

func (m shadowActs) Cost(usage agentUsage) decimal.Decimal {
	return decimal.NewFromInt(usage.CompletionTokens)
}
func (m shadowActs) Vision() bool { return true }

// «Документы для ввоза» у тени: под-агенты по тому же коду с описанием фото из переписки основной модели; ответ —
// в ленту и переписку тени, итог подбора кода (result) не трогается, finished_at повторного хода тоже.
func TestExecuteShadowDocuments(t *testing.T) {
	model := &actModel{scripts: map[string][]agentReply{
		"№299": {
			{ToolCalls: []agentToolCall{call("s1", "search_in_document", map[string]string{"url": "http://adilet.zan.kz/rus/docs/H10T0000299#z5", "query": "перчатки контактирующие пищевыми"})}},
			{ToolCalls: []agentToolCall{call("f", "finish", map[string]any{
				"description": "Санэпиднадзор, если перчатки контактируют с пищей.",
				"documents":   []map[string]string{{"measure": "Санитарно-эпидемиологический надзор", "act": "Решение КТС №299 от 28.05.2010", "url": testActURL, "requirement": "Только для изделий, контактирующих с пищей.", "applies": "maybe"}},
			})}},
		},
		"№79": {
			{ToolCalls: []agentToolCall{call("l3", "open_link", map[string]string{"url": testPDFURL})}},
			{ToolCalls: []agentToolCall{call("f", "finish", map[string]any{
				"description": "Нужна декларация соответствия.",
				"documents":   []map[string]string{{"kind": "dc", "measure": "Декларация соответствия ТР ТС 019/2011", "act": "Решение Коллегии ЕЭК №79", "url": testPDFURL, "requirement": "Декларация о соответствии.", "applies": "yes", "basis": "Требуется декларация соответствия ТР ТС 019/2011"}},
			})}},
		},
	}}
	agent := &TNVEDAgentService{data: &fakeAgentData{generalActs: true}, shadow: shadowActs{model}}
	session := repository.TNVEDAgentSession{ID: 9, DocumentsCode: "6116102000"}
	state := loadShadowState(repository.TNVEDAgentShadowRun{Transcript: repository.JSON(`[]`), Messages: repository.JSON(`[]`)})
	state.event("documents", map[string]string{"code": session.DocumentsCode})
	main := []map[string]any{{"role": "user", "content": photoNotePrefix + ": перчатки трикотажные с резиновым покрытием ладони"}}
	saves := 0
	final := agent.executeShadowDocuments(context.Background(), session, main, state, func(map[string]any) { saves++ })
	if final["status"] != "done" || final["result"] != nil || final["finished_at"] != nil || saves == 0 {
		t.Fatalf("документы: %v", final)
	}
	if kinds := messageKinds(final); !strings.HasPrefix(kinds, "documents,tool") || !strings.HasSuffix(kinds, "result") {
		t.Fatalf("лента: %s", kinds)
	}
	var transcript []map[string]any
	_ = json.Unmarshal([]byte(final["transcript"].(repository.JSON)), &transcript)
	if last := transcript[len(transcript)-1]; len(transcript) != 1 || last["role"] != "assistant" || !strings.Contains(last["content"].(string), "Документы для ввоза для кода 6116 10 200 0") {
		t.Fatalf("переписка: %v", transcript)
	}
	if task, _ := model.seen["№79"][1].Content.(string); !strings.Contains(task, "резиновым покрытием ладони") {
		t.Fatalf("под-агент не получил описание фото: %s", task)
	}
}

// В чате-справочнике тень получает подсказку и инструменты справочника: без ask_user.
func TestExecuteShadowReferenceMode(t *testing.T) {
	model := &scriptedModel{replies: []agentReply{{ToolCalls: []agentToolCall{finishWith(nil)}}, {ToolCalls: []agentToolCall{finishWith(nil)}}}}
	agent := &TNVEDAgentService{data: &fakeAgentData{}, shadow: shadowScripted{model, true}}
	runner, _ := newTestRunner(&scriptedModel{}, &fakeAgentData{})
	final := agent.executeShadow(context.Background(), repository.TNVEDAgentSession{ID: 8, Mode: modeReference}, newShadowState(runner.transcript, true), runner.photos, func(map[string]any) {})
	if final["status"] != "done" || strings.Contains(toolNames(model.lastTools), "ask_user") {
		t.Fatalf("справочник: %v, инструменты %s", final["status"], toolNames(model.lastTools))
	}
	if system, _ := model.lastSeen[0].Content.(string); system != agent.systemPrompt(true, false) {
		t.Fatal("подсказка тени в справочнике — не справочная")
	}
}
