package tnvedagent

// Две модели: фото смотрит модель для фото (Flash), остальное — модель рассуждений (Pro), которая фото
// не получает. Настоящих вызовов нет: модель рассуждений — сценарий, клиент — против локального
// httptest-сервера, который отвечает в формате OpenAI.

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/ai"
	"github.com/shopspring/decimal"
)

// fakeProvider — OpenAI-совместимый сервер: запоминает запросы и отвечает reply.
type fakeProvider struct {
	mu       sync.Mutex
	requests []map[string]any
	bodies   []string
	reply    map[string]any
}

func newFakeProvider(t *testing.T, reply map[string]any) (*fakeProvider, *httptest.Server) {
	provider := &fakeProvider{reply: reply}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var request map[string]any
		_ = json.Unmarshal(body, &request)
		provider.mu.Lock()
		provider.requests = append(provider.requests, request)
		provider.bodies = append(provider.bodies, string(body))
		provider.mu.Unlock()
		_ = json.NewEncoder(w).Encode(provider.reply)
	}))
	t.Cleanup(server.Close)
	return provider, server
}

func textReply(content string) map[string]any {
	return map[string]any{
		"choices": []map[string]any{{"message": map[string]any{"content": content}}},
		"usage":   map[string]any{"prompt_tokens": 1000, "completion_tokens": 100, "prompt_cache_hit_tokens": 200},
	}
}

// testClient — клиент к локальному серверу. «/deepseek.com» в пути — чтобы клиент слал параметр thinking,
// как к настоящему DeepSeek.
func testClient(server *httptest.Server, model, effort string, vision bool) *AgentModelClient {
	return NewAgentModelClient(AgentModelConfig{APIKey: "test", BaseURL: server.URL + "/deepseek.com", Model: model, Vision: vision, Effort: effort,
		PriceInput: decimal.RequireFromString("0.30"), PriceCached: decimal.RequireFromString("0.006"), PriceOutput: decimal.RequireFromString("1.20")})
}

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
	if !strings.Contains(agent.systemPrompt(false, false, false), "Фото — главный источник") || strings.Contains(agent.systemPrompt(false, false, false), "look_at_photo") {
		t.Fatal("подсказка одной модели — прежняя")
	}
}

// Режим размышлений: включается уровнем размышлений, без temperature и tool_choice; reasoning_content
// из ответа сохраняется и уходит модели в следующих запросах.
func TestClientThinkingModeAndReasoningContent(t *testing.T) {
	provider, server := newFakeProvider(t, map[string]any{
		"choices": []map[string]any{{"message": map[string]any{"content": "", "reasoning_content": "Сначала проверю позицию 8201.",
			"tool_calls": []map[string]any{{"id": "c1", "type": "function", "function": map[string]any{"name": "list_heading", "arguments": `{"prefix":"8201"}`}}}}}},
		"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 5},
	})
	thinking := testClient(server, "deepseek-v4-pro", "high", false)
	reply, err := thinking.Complete(context.Background(), []agentMessage{
		{Role: "user", Content: "товар"},
		{Role: "assistant", Content: "", ReasoningContent: "прежние размышления", ToolCalls: []agentToolCall{call("c0", "get_codes", map[string]any{"codes": []string{}})}},
		{Role: "tool", ToolCallID: "c0", Content: "{}"},
	}, agentTools(false, false), "finish")
	if err != nil || reply.Reasoning != "Сначала проверю позицию 8201." || len(reply.ToolCalls) != 1 {
		t.Fatalf("ответ: %+v %v", reply, err)
	}
	request := provider.requests[0]
	if mode, _ := request["thinking"].(map[string]any); mode["type"] != "enabled" || request["reasoning_effort"] != "high" {
		t.Fatalf("размышления включены: %v", request["thinking"])
	}
	if _, ok := request["tool_choice"]; ok {
		t.Fatal("tool_choice в режиме размышлений не шлём")
	}
	if _, ok := request["temperature"]; ok {
		t.Fatal("temperature в режиме размышлений не шлём")
	}
	if !strings.Contains(provider.bodies[0], `"reasoning_content":"прежние размышления"`) {
		t.Fatal("размышления прежних шагов возвращаются модели")
	}

	plain := testClient(server, "deepseek-flash", "", true)
	if _, err := plain.Complete(context.Background(), []agentMessage{{Role: "user", Content: "x"}}, agentTools(false, false), "finish"); err != nil {
		t.Fatal(err)
	}
	if mode, _ := provider.requests[1]["thinking"].(map[string]any); mode["type"] != "disabled" || provider.requests[1]["tool_choice"] == nil {
		t.Fatalf("без уровня размышлений — как раньше: %v", provider.requests[1])
	}
	if _, err := thinking.CompleteText(context.Background(), []agentMessage{{Role: "user", Content: "x"}}, 100); err != nil {
		t.Fatal(err)
	}
	if mode, _ := provider.requests[2]["thinking"].(map[string]any); mode["type"] != "disabled" || provider.requests[2]["tools"] != nil {
		t.Fatalf("текстовый ответ — без размышлений и инструментов: %v", provider.requests[2])
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
	runner.documents = true
	if runner.stepLimit() != agentDocumentsMaxSteps {
		t.Fatalf("документы: %d", runner.stepLimit())
	}
}

// longPagesData — страницы кодов целиком: у обоих кодов одинаковые длинные пояснения к ТН ВЭД.
type longPagesData struct{ fakeAgentData }

const testGroupNotes = "Общие положения группы: перчатки трикотажные с покрытием из резины включаются в группу 61. "

func (d *longPagesData) DetailsPage(_ context.Context, code, _ string) (string, bool, error) {
	return "Дерево кода:\n" + code + " перчатки\n" + notesHeading + "\n" + strings.Repeat(testGroupNotes, 20), false, nil
}

// Страница кода целиком; одинаковые пояснения второй страницы — модели ссылкой на первую, а цитаты из
// них проверяются по полному тексту, в том числе после ответа сотрудника.
func TestOpenDetailsSendsRepeatedNotesOnce(t *testing.T) {
	model := &scriptedModel{replies: []agentReply{{ToolCalls: []agentToolCall{
		call("a", "open_details", map[string]string{"code": "6116102000"}),
		call("b", "open_details", map[string]string{"code": "4015190000"}),
		call("q", "ask_user", map[string]string{"question": "Покрытие из резины?"}),
	}}}}
	runner, _ := newTestRunner(model, &fakeAgentData{})
	runner.toolbox = newAgentToolbox(&longPagesData{})
	if outcome := runner.execute(context.Background()); outcome.status != "waiting" {
		t.Fatalf("первый запуск: %+v", outcome)
	}
	first, second := toolAnswer(t, runner, "a")["text"].(string), toolAnswer(t, runner, "b")["text"].(string)
	if strings.Count(first, testGroupNotes) != 20 || strings.Contains(second, testGroupNotes) || !strings.Contains(second, sameNotesPrefix+"6116102000") {
		t.Fatalf("пояснения второй страницы — ссылкой: %q", service.TruncateRunes(second, 200))
	}
	raw, _ := json.Marshal(runner.transcript)
	var restored []map[string]any
	_ = json.Unmarshal(raw, &restored)
	next, _ := newTestRunner(&scriptedModel{}, &fakeAgentData{})
	next.toolbox = newAgentToolbox(&longPagesData{})
	next.transcript = restored
	next.restoreOpened(context.Background())
	if source := next.toolbox.quoteSource("4015190000", "перчатки трикотажные с покрытием из резины включаются в группу 61"); source != "page" {
		t.Fatalf("цитата из пояснений второй страницы после ответа сотрудника: %q", source)
	}
}

// Gemini 3 (OpenAI-совместимый адрес generativelanguage.googleapis.com): размышления не выключаются —
// вне цикла агента reasoning_effort low, max_tokens с запасом на размышления; параметр thinking DeepSeek не шлём.
func TestClientGeminiRequests(t *testing.T) {
	provider, server := newFakeProvider(t, textReply("кельма"))
	gemini := NewAgentModelClient(AgentModelConfig{APIKey: "test", BaseURL: server.URL + "/generativelanguage.googleapis.com", Model: "gemini-3.8-flash", Vision: true})
	if _, err := gemini.CompleteText(context.Background(), []agentMessage{{Role: "user", Content: "x"}}, 100); err != nil {
		t.Fatal(err)
	}
	request := provider.requests[0]
	if request["reasoning_effort"] != "low" || request["max_tokens"] != float64(100+geminiThinkingHeadroom) || request["thinking"] != nil {
		t.Fatalf("запрос к Gemini: %v", request)
	}
	if _, err := gemini.CompleteJSON(context.Background(), []agentMessage{{Role: "user", Content: "x"}}); err != nil {
		t.Fatal(err)
	}
	if request := provider.requests[1]; request["reasoning_effort"] != "low" || request["max_tokens"] != float64(1024+geminiThinkingHeadroom) {
		t.Fatalf("JSON-запрос к Gemini: %v", request)
	}

	thinking := NewAgentModelClient(AgentModelConfig{APIKey: "test", BaseURL: server.URL + "/generativelanguage.googleapis.com", Model: "gemini-3.8-flash", Effort: "high"})
	if _, err := thinking.Complete(context.Background(), []agentMessage{{Role: "user", Content: "x"}}, agentTools(false, false), "finish"); err != nil {
		t.Fatal(err)
	}
	if request := provider.requests[2]; request["reasoning_effort"] != "high" || request["max_tokens"] != float64(32768+geminiThinkingHeadroom) {
		t.Fatalf("цикл агента на Gemini сохраняет свой уровень размышлений: %v", request)
	}
}

// Эмбеддинги: запрос на /embeddings того же адреса с размерностью колонки; без модели — не настроено.
func TestClientEmbed(t *testing.T) {
	vector := make([]float64, embeddingDimensions)
	vector[0], vector[1] = 0.25, -0.5
	provider, server := newFakeProvider(t, map[string]any{"data": []map[string]any{{"embedding": vector}}})
	client := NewAgentModelClient(AgentModelConfig{APIKey: "test", BaseURL: server.URL + "/generativelanguage.googleapis.com", Model: "gemini-3.8-flash", EmbedModel: "gemini-embedding-001"})
	got, err := client.Embed(context.Background(), "кельма, мастерок")
	if err != nil || len(got) != embeddingDimensions || got[0] != 0.25 || got[1] != -0.5 {
		t.Fatalf("embedding: %v %v", len(got), err)
	}
	request := provider.requests[0]
	if request["model"] != "gemini-embedding-001" || request["input"] != "кельма, мастерок" || request["dimensions"] != float64(embeddingDimensions) {
		t.Fatalf("request: %v", request)
	}
	plain := NewAgentModelClient(AgentModelConfig{APIKey: "test", BaseURL: server.URL, Model: "gemini-3.8-flash"})
	if plain.Embeds() {
		t.Fatal("no embed model — embeddings off")
	}
	if _, err := plain.Embed(context.Background(), "x"); !errors.Is(err, ai.ErrAINotConfigured) {
		t.Fatalf("err = %v", err)
	}
	short, shortServer := newFakeProvider(t, map[string]any{"data": []map[string]any{{"embedding": []float64{1, 2}}}})
	_ = short
	bad := NewAgentModelClient(AgentModelConfig{APIKey: "test", BaseURL: shortServer.URL, Model: "m", EmbedModel: "e"})
	if _, err := bad.Embed(context.Background(), "x"); !errors.Is(err, ai.ErrAIBadResponse) {
		t.Fatalf("wrong dimensions must be a bad response: %v", err)
	}
}
