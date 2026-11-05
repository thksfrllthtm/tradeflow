package tnvedagent

// Клиент модели для агента подбора кода: OpenAI-совместимый Chat Completions с инструментами
// (tools / tool_calls). Две роли: модель рассуждений (DeepSeek Pro — цикл агента, фото не
// принимает) и модель для фото (DeepSeek Flash). Без отдельной модели для фото — одна модель на всё.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/ai"
	"github.com/shopspring/decimal"
)

// AgentModelConfig — провайдер и цены (USD за 1 млн токенов) для учёта расхода.
type AgentModelConfig struct {
	APIKey  string
	BaseURL string
	Model   string
	Vision  bool
	// Effort — уровень размышлений (reasoning_effort: low, high, max). Задан — режим размышлений DeepSeek
	// включён, "" — выключен (быстрее, как раньше). У Claude — output_config.effort.
	Effort string
	// Provider — "" (OpenAI-совместимый API) или "anthropic" (Claude: прогон eval и теневой подбор, см. claude.go).
	Provider string
	// EmbedModel — модель эмбеддингов на том же адресе и ключе (gemini-embedding-001): поиск похожих
	// товаров и записей по смыслу описания. "" — эмбеддинги выключены.
	EmbedModel string
	// Цены: обычный ввод, ввод из кэша, вывод.
	PriceInput  decimal.Decimal
	PriceCached decimal.Decimal
	PriceOutput decimal.Decimal
}

// agentToolCall — вызов инструмента в ответе модели.
type agentToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// agentMessage — сообщение переписки с моделью (OpenAI-формат). Content — строка или список
// частей (текст и картинки) для первого сообщения с фото. ReasoningContent — размышления модели:
// в режиме размышлений с инструментами DeepSeek требует возвращать их во всех следующих запросах.
type agentMessage struct {
	Role             string          `json:"role"`
	Content          any             `json:"content"`
	ReasoningContent string          `json:"reasoning_content,omitempty"`
	ToolCalls        []agentToolCall `json:"tool_calls,omitempty"`
	ToolCallID       string          `json:"tool_call_id,omitempty"`
	// ClaudeContent — ответ Claude целиком (с подписанными размышлениями): уходит ему обратно как есть.
	ClaudeContent json.RawMessage `json:"claude_content,omitempty"`
}

type agentTool struct {
	Type     string            `json:"type"`
	Function agentToolFunction `json:"function"`
}

type agentToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

type agentUsage struct {
	PromptTokens     int64
	CachedTokens     int64
	CompletionTokens int64
	// CacheWriteTokens — записано в кэш (Claude: дороже обычного ввода на 25%); входит в PromptTokens.
	CacheWriteTokens int64
}

type agentReply struct {
	Content       string
	Reasoning     string
	ToolCalls     []agentToolCall
	Usage         agentUsage
	ClaudeContent json.RawMessage
}

// agentModel — то, что нужно агенту от модели; в тестах подменяется сценарием.
type agentModel interface {
	Complete(ctx context.Context, messages []agentMessage, tools []agentTool, forceTool string) (agentReply, error)
	Configured() bool
	Name() string
}

type AgentModelClient struct {
	config     AgentModelConfig
	httpClient *http.Client
	retryDelay time.Duration
}

// С уровнем размышлений max ответ модели может идти дольше минуты.
const agentModelTimeout = 3 * time.Minute

// Сбой провайдера (5xx, «перегружен», обрыв связи) — запрос повторяется: за подбор их десятки, и один
// сбой не должен обрывать весь подбор. Тайм-аут — только один повтор (минуты ожидания).
const agentModelAttempts = 3

// retryable — запрос стоит повторить: сбой провайдера, а также ограничение запросов (429) — у DeepSeek оно
// кратковременное (под-агенты «Документов для ввоза» идут параллельно); у Gemini 429 — дневной лимит
// фото, ждать бессмысленно.
func retryable(err error, baseURL string) bool {
	return errors.Is(err, ai.ErrAIUnavailable) || (errors.Is(err, ai.ErrAIRateLimited) && !isGemini(baseURL))
}

// WithEffort — тот же клиент с другим уровнем размышлений (под-агенты «Документов для ввоза»).
func (c *AgentModelClient) WithEffort(effort string) *AgentModelClient {
	clone := *c
	clone.config.Effort = strings.TrimSpace(effort)
	return &clone
}

func NewAgentModelClient(config AgentModelConfig) *AgentModelClient {
	// Ключ без пробелов и переводов строк внутри: при вставке в панель хостинга длинный ключ бывает разорван
	// переносом, и провайдер отвечает «invalid x-api-key». В настоящих ключах пробелов нет.
	config.APIKey = strings.Join(strings.Fields(config.APIKey), "")
	config.Model = strings.TrimSpace(config.Model)
	config.BaseURL = strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if config.BaseURL == "" {
		config.BaseURL = "https://api.deepseek.com"
	}
	return &AgentModelClient{config: config, httpClient: &http.Client{}, retryDelay: 3 * time.Second}
}

func (c *AgentModelClient) Configured() bool {
	return c != nil && c.config.APIKey != "" && c.config.Model != ""
}

func (c *AgentModelClient) Name() string { return c.config.Model }

func (c *AgentModelClient) Vision() bool { return c != nil && c.config.Vision }

var (
	million         = decimal.NewFromInt(1_000_000)
	cacheWriteExtra = decimal.RequireFromString("0.25")
)

// Cost — стоимость ответа в USD по ценам из настроек; запись в кэш — +25% к цене ввода (Claude).
func (c *AgentModelClient) Cost(usage agentUsage) decimal.Decimal {
	fresh := max(usage.PromptTokens-usage.CachedTokens, 0)
	return decimal.NewFromInt(fresh).Mul(c.config.PriceInput).
		Add(decimal.NewFromInt(usage.CacheWriteTokens).Mul(c.config.PriceInput).Mul(cacheWriteExtra)).
		Add(decimal.NewFromInt(usage.CachedTokens).Mul(c.config.PriceCached)).
		Add(decimal.NewFromInt(usage.CompletionTokens).Mul(c.config.PriceOutput)).
		Div(million)
}

func (c *AgentModelClient) Complete(ctx context.Context, messages []agentMessage, tools []agentTool, forceTool string) (agentReply, error) {
	if !c.Configured() {
		return agentReply{}, ai.ErrAINotConfigured
	}
	if c.config.Provider == providerAnthropic {
		return c.claudeComplete(ctx, messages, tools)
	}
	thinking := c.config.Effort != ""
	request := map[string]any{
		"model":      c.config.Model,
		"messages":   messages,
		"tools":      tools,
		"max_tokens": 4096,
	}
	if thinking {
		// Размышления входят в лимит ответа: на высоком уровне 4096 токенов обрезали бы итог.
		// temperature в режиме размышлений не действует; tool_choice в нём документацией DeepSeek не
		// описан — принудительный finish остаётся указанием в тексте («сразу вызови finish»).
		request["reasoning_effort"] = c.config.Effort
		request["max_tokens"] = 32768
	} else {
		request["temperature"] = 0.2
		if forceTool != "" {
			request["tool_choice"] = map[string]any{"type": "function", "function": map[string]string{"name": forceTool}}
		}
	}
	return c.send(ctx, request, thinking)
}

// CompleteJSON — один ответ модели в JSON без инструментов (описание и сверка фото).
func (c *AgentModelClient) CompleteJSON(ctx context.Context, messages []agentMessage) (agentReply, error) {
	if !c.Configured() {
		return agentReply{}, ai.ErrAINotConfigured
	}
	if c.config.Provider == providerAnthropic {
		return c.claudeComplete(ctx, messages, nil)
	}
	return c.send(ctx, map[string]any{
		"model":           c.config.Model,
		"messages":        messages,
		"response_format": map[string]string{"type": "json_object"},
		"max_tokens":      1024,
		"temperature":     0.1,
	}, false)
}

// CompleteText — один текстовый ответ без инструментов (описание фото, ответ на вопрос по фото).
func (c *AgentModelClient) CompleteText(ctx context.Context, messages []agentMessage, maxTokens int) (agentReply, error) {
	if !c.Configured() {
		return agentReply{}, ai.ErrAINotConfigured
	}
	if c.config.Provider == providerAnthropic {
		return c.claudeComplete(ctx, messages, nil)
	}
	return c.send(ctx, map[string]any{"model": c.config.Model, "messages": messages, "max_tokens": maxTokens, "temperature": 0.1}, false)
}

// geminiThinkingHeadroom — запас токенов на размышления Gemini 3: выключить их нельзя, и они входят в
// max_tokens; без запаса короткие ответы (описание фото, JSON) обрезаются.
const geminiThinkingHeadroom = 2048

func isGemini(baseURL string) bool {
	return strings.Contains(baseURL, "generativelanguage.googleapis.com")
}

func (c *AgentModelClient) send(ctx context.Context, request map[string]any, thinking bool) (agentReply, error) {
	switch {
	case strings.Contains(c.config.BaseURL, "deepseek.com"):
		// Режим размышлений DeepSeek включён по умолчанию и в разы медленнее: только для цикла агента
		// с заданным уровнем размышлений, остальные запросы — без него.
		mode := "disabled"
		if thinking {
			mode = "enabled"
		}
		request["thinking"] = map[string]string{"type": mode}
	case isGemini(c.config.BaseURL):
		// Gemini 3 размышляет всегда: вне цикла агента — минимально, и с запасом токенов на размышления.
		if !thinking {
			request["reasoning_effort"] = "low"
		}
		if limit, ok := request["max_tokens"].(int); ok {
			request["max_tokens"] = limit + geminiThinkingHeadroom
		}
	}
	body, err := json.Marshal(request)
	if err != nil {
		return agentReply{}, ai.ErrAIBadResponse
	}
	for attempt := 1; ; attempt++ {
		reply, err := c.post(ctx, body)
		// Тайм-аут одного ответа повторяется один раз: обычно это перегрузка провайдера, а не вечная задача.
		timedOut := errors.Is(err, ai.ErrAITimeout) && attempt == 1
		if (!retryable(err, c.config.BaseURL) && !timedOut) || attempt == agentModelAttempts {
			return reply, err
		}
		select {
		case <-ctx.Done():
			return reply, err
		case <-time.After(time.Duration(attempt) * c.retryDelay):
		}
	}
}

// post — один запрос к провайдеру.
func (c *AgentModelClient) post(ctx context.Context, body []byte) (agentReply, error) {
	requestCtx, cancel := context.WithTimeout(ctx, agentModelTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, c.config.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return agentReply{}, ai.ErrAIBadResponse
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		if errors.Is(requestCtx.Err(), context.DeadlineExceeded) {
			return agentReply{}, ai.ErrAITimeout
		}
		log.Printf("ai %s: %v", c.config.Model, err)
		return agentReply{}, ai.ErrAIUnavailable
	}
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	resp.Body.Close()
	if readErr != nil {
		if errors.Is(requestCtx.Err(), context.DeadlineExceeded) {
			return agentReply{}, ai.ErrAITimeout
		}
		return agentReply{}, ai.ErrAIBadResponse
	}
	if err := statusError(resp.StatusCode); err != nil {
		log.Printf("ai %s: HTTP %d %s", c.config.Model, resp.StatusCode, service.TruncateRunes(string(data), 300))
		return agentReply{}, err
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content          *string         `json:"content"`
				ReasoningContent string          `json:"reasoning_content"`
				ToolCalls        []agentToolCall `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens         int64 `json:"prompt_tokens"`
			CompletionTokens     int64 `json:"completion_tokens"`
			PromptCacheHitTokens int64 `json:"prompt_cache_hit_tokens"`
			PromptTokensDetails  struct {
				CachedTokens int64 `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
		} `json:"usage"`
	}
	if json.Unmarshal(data, &parsed) != nil || len(parsed.Choices) == 0 {
		return agentReply{}, ai.ErrAIBadResponse
	}
	message := parsed.Choices[0].Message
	reply := agentReply{ToolCalls: message.ToolCalls, Reasoning: message.ReasoningContent}
	if message.Content != nil {
		reply.Content = strings.TrimSpace(*message.Content)
	}
	cached := parsed.Usage.PromptCacheHitTokens
	if cached == 0 {
		cached = parsed.Usage.PromptTokensDetails.CachedTokens
	}
	reply.Usage = agentUsage{PromptTokens: parsed.Usage.PromptTokens, CachedTokens: cached, CompletionTokens: parsed.Usage.CompletionTokens}
	return reply, nil
}

// statusError — ошибка ИИ по коду ответа провайдера; nil — успех.
func statusError(code int) error {
	switch {
	case code == http.StatusUnauthorized, code == http.StatusForbidden, code == http.StatusPaymentRequired:
		return ai.ErrAIUnauthorized
	case code == http.StatusTooManyRequests:
		return ai.ErrAIRateLimited
	case code == http.StatusBadRequest, code == http.StatusUnprocessableEntity:
		return ai.ErrAIRequestRejected
	case code == http.StatusNotFound:
		return ai.ErrAIModelUnavailable
	case code == http.StatusRequestTimeout:
		return ai.ErrAITimeout
	case code >= 500:
		return ai.ErrAIUnavailable
	case code < 200 || code >= 300:
		return ai.ErrAIBadResponse
	}
	return nil
}

// Размерность эмбеддингов: столько же в колонке code_decisions.photo_embedding (миграция 000042).
// Gemini по умолчанию даёт 3072, но для сотен записей хватает 768, и индекс pgvector выше 2000 не строится.
const embeddingDimensions = 768

const embedTimeout = 30 * time.Second

// Embeds — есть модель эмбеддингов.
func (c *AgentModelClient) Embeds() bool { return c.Configured() && c.config.EmbedModel != "" }

// Embed — вектор текста (OpenAI-совместимый /embeddings) для поиска по смыслу.
func (c *AgentModelClient) Embed(ctx context.Context, text string) ([]float32, error) {
	if !c.Embeds() {
		return nil, ai.ErrAINotConfigured
	}
	body, err := json.Marshal(map[string]any{"model": c.config.EmbedModel, "input": text, "dimensions": embeddingDimensions})
	if err != nil {
		return nil, ai.ErrAIBadResponse
	}
	requestCtx, cancel := context.WithTimeout(ctx, embedTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(requestCtx, http.MethodPost, c.config.BaseURL+"/embeddings", bytes.NewReader(body))
	if err != nil {
		return nil, ai.ErrAIBadResponse
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.config.APIKey)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		if errors.Is(requestCtx.Err(), context.DeadlineExceeded) {
			return nil, ai.ErrAITimeout
		}
		return nil, ai.ErrAIUnavailable
	}
	data, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	resp.Body.Close()
	if readErr != nil {
		return nil, ai.ErrAIBadResponse
	}
	if err := statusError(resp.StatusCode); err != nil {
		return nil, err
	}
	var parsed struct {
		Data []struct {
			Embedding []float32 `json:"embedding"`
		} `json:"data"`
	}
	if json.Unmarshal(data, &parsed) != nil || len(parsed.Data) == 0 || len(parsed.Data[0].Embedding) != embeddingDimensions {
		return nil, ai.ErrAIBadResponse
	}
	return parsed.Data[0].Embedding, nil
}
