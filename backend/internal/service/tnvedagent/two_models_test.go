package tnvedagent

// Две модели: фото смотрит модель для фото (Flash), остальное — модель рассуждений (Pro), которая фото
// не получает. Настоящих вызовов нет: модель рассуждений — сценарий, клиент — против локального
// httptest-сервера, который отвечает в формате OpenAI.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

const testPhoto = "data:image/jpeg;base64,AAAA"

// Модель рассуждений не получает image_url: фото в переписке остаётся ссылкой, описание — текстом.
func TestReasoningModelGetsPhotoNoteNotImages(t *testing.T) {
	model := &scriptedModel{replies: []agentReply{{ToolCalls: []agentToolCall{finishWith(nil)}}}}
	runner, _ := newTestRunner(model, &fakeAgentData{})
	runner.vision = false
	runner.transcript = append(runner.transcript, map[string]any{"role": "user", "content": photoNotePrefix + ": секатор, лезвия из стали"})
	runner.toolbox.lookPhoto = func(context.Context, string) (string, error) { return "да", nil }
	if outcome := runner.execute(context.Background()); outcome.result == nil {
		t.Fatalf("итог: %+v", outcome)
	}
	data, _ := json.Marshal(model.lastSeen)
	if strings.Contains(string(data), "image_url") || strings.Contains(string(data), testPhoto) {
		t.Fatalf("модель рассуждений получила фото: %s", data)
	}
	if !strings.Contains(string(data), "лезвия из стали") {
		t.Fatal("описание фото не дошло до модели рассуждений")
	}
	if !strings.Contains(toolNames(model.lastTools), "look_at_photo") {
		t.Fatal("с моделью для фото есть look_at_photo")
	}
}

// look_at_photo уходит в модель для фото, не больше 3 вопросов; без модели для фото инструмента нет.
func TestLookAtPhotoLimit(t *testing.T) {
	toolbox := newAgentToolbox(&fakeAgentData{})
	if content, _ := toolbox.run(context.Background(), "look_at_photo", `{"question":"Есть пружина?"}`); !strings.Contains(content, "Нет такого инструмента") {
		t.Fatalf("без модели для фото: %s", content)
	}
	asked := []string{}
	toolbox.lookPhoto = func(_ context.Context, question string) (string, error) {
		asked = append(asked, question)
		return "Пружина видна между ручками.", nil
	}
	for index := 0; index < agentMaxPhotoQuestions; index++ {
		content, event := toolbox.run(context.Background(), "look_at_photo", `{"question":"Есть пружина?"}`)
		if !strings.Contains(content, "Пружина видна") || event.Tool != "look_at_photo" || event.Failed {
			t.Fatalf("ответ %d: %s %+v", index, content, event)
		}
	}
	if content, event := toolbox.run(context.Background(), "look_at_photo", `{"question":"А ещё?"}`); !strings.Contains(content, "Лимит вопросов") || !event.Failed {
		t.Fatalf("лимит: %s", content)
	}
	if len(asked) != agentMaxPhotoQuestions {
		t.Fatalf("вопросов в модель для фото: %d", len(asked))
	}
	model := &scriptedModel{replies: []agentReply{{ToolCalls: []agentToolCall{finishWith(nil)}}}}
	runner, _ := newTestRunner(model, &fakeAgentData{})
	runner.execute(context.Background())
	if strings.Contains(toolNames(model.lastTools), "look_at_photo") {
		t.Fatal("без модели для фото look_at_photo нет")
	}
}

// Описание фото и ответы на вопросы — модель для фото с картинками; модель рассуждений не вызывается.
func TestPhotoNoteAndQuestionsGoToVisionModel(t *testing.T) {
	flashProvider, flashServer := newFakeProvider(t, textReply("- Вид товара: секатор.\n- Материал: сталь."))
	proProvider, proServer := newFakeProvider(t, textReply("не должен вызываться"))
	agent := NewTNVEDAgentService(nil, testClient(proServer, "deepseek-v4-pro", "high", false), testClient(flashServer, "deepseek-flash", "", true), nil, AgentSettings{})
	if agent.modelName() != "deepseek-flash + deepseek-v4-pro" {
		t.Fatalf("модели: %s", agent.modelName())
	}
	events := []agentToolEvent{}
	note, usage := agent.photoNote(context.Background(), []string{testPhoto}, []string{"Газовый ключ-синий 1* SL-900", ""}, nil, func(_ string, content any) { events = append(events, content.(agentToolEvent)) })
	text, _ := note["content"].(string)
	if !strings.HasPrefix(text, photoNotePrefix) || !strings.Contains(text, "секатор") || usage.PromptTokens != 1000 {
		t.Fatalf("описание фото: %v %+v", note, usage)
	}
	if len(events) != 1 || events[0].Tool != "photo" || events[0].Label != "Описание фото" || !strings.Contains(events[0].Detail, "секатор") {
		t.Fatalf("шаг «Описание фото»: %+v", events)
	}
	if again, _ := agent.photoNote(context.Background(), []string{testPhoto}, nil, []map[string]any{note}, func(string, any) {}); again != nil {
		t.Fatal("описание уже есть — второй раз не составляется")
	}
	spent := agentUsage{}
	ask := agent.photoQuestions([]string{testPhoto}, func(step agentUsage) { spent.PromptTokens += step.PromptTokens })
	if answer, err := ask(context.Background(), "Из чего лезвия?"); err != nil || answer == "" || spent.PromptTokens != 1000 {
		t.Fatalf("ответ по фото: %q %v, расход %+v", answer, err, spent)
	}
	if len(flashProvider.requests) != 2 || !strings.Contains(flashProvider.bodies[0], "image_url") || !strings.Contains(flashProvider.bodies[0], "«Газовый ключ-синий 1* SL-900»") || strings.Contains(flashProvider.bodies[0], "«»") || !strings.Contains(flashProvider.bodies[1], "Из чего лезвия?") || !strings.Contains(flashProvider.bodies[1], "image_url") {
		t.Fatalf("запросы модели для фото: %v", flashProvider.bodies)
	}
	if len(proProvider.requests) != 0 {
		t.Fatalf("модель рассуждений получила запрос по фото: %v", proProvider.bodies)
	}
	if agent.photoQuestions(nil, func(agentUsage) {}) != nil {
		t.Fatal("без фото — без look_at_photo")
	}
}

// Без TNVED_AI_VISION_MODEL — одна модель, как раньше: фото видит она сама, описания и вопросов нет.
func TestSingleModelWorksAsBefore(t *testing.T) {
	provider, server := newFakeProvider(t, textReply("x"))
	model := testClient(server, "deepseek-flash", "", true)
	agent := NewTNVEDAgentService(nil, model, nil, nil, AgentSettings{})
	if agent.twoModels() || agent.modelName() != "deepseek-flash" || agent.visionModel != model {
		t.Fatalf("одна модель: %s", agent.modelName())
	}
	// Одна модель (Flash) видит фото сама: отдельного шага с надписями нет — Flash их сочиняет.
	if note, _ := agent.photoNote(context.Background(), []string{testPhoto}, nil, nil, func(string, any) {}); note != nil {
		t.Fatal("одна модель видит фото — описание не нужно")
	}
	if agent.photoQuestions([]string{testPhoto}, func(agentUsage) {}) != nil || len(provider.requests) != 0 {
		t.Fatal("одна модель — без look_at_photo и лишних запросов")
	}
	if !strings.Contains(agent.systemPrompt(false, false), "Фото — главный источник") || strings.Contains(agent.systemPrompt(false, false), "look_at_photo") {
		t.Fatal("подсказка одной модели — прежняя")
	}
}

// Размышления модели сохраняются в переписке и уходят ей на следующем шаге.
func TestRunnerKeepsReasoningContent(t *testing.T) {
	model := &scriptedModel{replies: []agentReply{
		{Reasoning: "Похоже на 6116.", ToolCalls: []agentToolCall{call("c1", "list_heading", map[string]string{"prefix": "6116"})}},
		{ToolCalls: []agentToolCall{finishWith(nil)}},
	}}
	runner, _ := newTestRunner(model, &fakeAgentData{})
	runner.execute(context.Background())
	for _, message := range model.lastSeen {
		if message.Role == "assistant" && message.ReasoningContent == "Похоже на 6116." {
			return
		}
	}
	t.Fatalf("размышления не вернулись модели: %+v", model.lastSeen)
}

// Стоимость — decimal; расход сессии: всего и отдельно модель для фото.
func TestCostAndSessionUsage(t *testing.T) {
	_, server := newFakeProvider(t, textReply("x"))
	pro := NewAgentModelClient(AgentModelConfig{APIKey: "k", BaseURL: server.URL, Model: "pro",
		PriceInput: decimal.RequireFromString("1.32"), PriceCached: decimal.RequireFromString("0.044"), PriceOutput: decimal.RequireFromString("3.96")})
	cost := pro.Cost(agentUsage{PromptTokens: 1_000_000, CachedTokens: 500_000, CompletionTokens: 100_000})
	if !cost.Equal(decimal.RequireFromString("1.078")) {
		t.Fatalf("стоимость: %s", cost)
	}
	usage := sessionUsage{cost: decimal.Zero, visionCost: decimal.Zero}
	usage.add(agentUsage{PromptTokens: 100, CachedTokens: 10, CompletionTokens: 5}, decimal.RequireFromString("0.002"), false)
	usage.add(agentUsage{PromptTokens: 30, CachedTokens: 3, CompletionTokens: 2}, decimal.RequireFromString("0.0001"), true)
	changes := usage.changes()
	if changes["prompt_tokens"] != int64(130) || changes["vision_prompt_tokens"] != int64(30) || changes["completion_tokens"] != int64(7) ||
		!changes["cost_usd"].(decimal.Decimal).Equal(decimal.RequireFromString("0.0021")) || !changes["vision_cost_usd"].(decimal.Decimal).Equal(decimal.RequireFromString("0.0001")) {
		t.Fatalf("расход: %v", changes)
	}
}

// С интернетом — 24 шага, без него — 12, «Документы» — 16.
func TestStepLimits(t *testing.T) {
	runner, _ := newTestRunner(&scriptedModel{}, &fakeAgentData{})
	if runner.stepLimit() != agentMaxSteps {
		t.Fatalf("без интернета: %d", runner.stepLimit())
	}
	runner.toolbox.search = &fakeSearcher{}
	if runner.stepLimit() != agentWebMaxSteps || agentWebMaxSteps != 24 {
		t.Fatalf("с интернетом: %d", runner.stepLimit())
	}
}

// Модель, которая смотрит фото сама (Claude), описания фото от модели для фото не получает — решает по
// снимку; модель без зрения (DeepSeek) получает описание. В переписке описание остаётся для тени.
func TestOwnEyesSkipsPhotoNote(t *testing.T) {
	runner, _ := newTestRunner(&scriptedModel{}, &fakeAgentData{})
	runner.transcript = append(runner.transcript, map[string]any{"role": "user", "content": photoNotePrefix + ": перчатки с покрытием"})
	has := func() bool {
		raw, _ := json.Marshal(runner.materialize())
		return strings.Contains(string(raw), photoNotePrefix)
	}
	if !has() {
		t.Fatal("модель без своего зрения получает описание фото")
	}
	runner.ownEyes = true
	if has() {
		t.Fatal("модель со своим зрением описание фото не получает")
	}
	if last := runner.transcript[len(runner.transcript)-1]["content"]; last != photoNotePrefix+": перчатки с покрытием" {
		t.Fatalf("описание остаётся в переписке: %v", last)
	}
}
