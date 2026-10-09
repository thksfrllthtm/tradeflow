package tnvedagent

// Клиент модели для агента подбора кода: OpenAI-совместимый Chat Completions с инструментами
// (tools / tool_calls). По умолчанию DeepSeek Flash: он понимает фото и вызывает инструменты.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service/ai"
)

// AgentModelConfig — провайдер и цены (USD за 1 млн токенов) для учёта расхода.
type AgentModelConfig struct {
	APIKey  string
	BaseURL string
	Model   string
	Vision  bool
	// Effort — уровень размышлений модели (reasoning_effort: low, high, max …); "" — по умолчанию модели.
	Effort string
	// Цены: обычный ввод, ввод из кэша, вывод.
	PriceInput  float64
	PriceCached float64
	PriceOutput float64
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
// частей (текст и картинки) для первого сообщения с фото.
type agentMessage struct {
	Role       string          `json:"role"`
	Content    any             `json:"content"`
	ToolCalls  []agentToolCall `json:"tool_calls,omitempty"`
	ToolCallID string          `json:"tool_call_id,omitempty"`
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
}

type agentReply struct {
	Content   string
	ToolCalls []agentToolCall
	Usage     agentUsage
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
}

// С уровнем размышлений max ответ модели может идти дольше минуты.
const agentModelTimeout = 3 * time.Minute

func NewAgentModelClient(config AgentModelConfig) *AgentModelClient {
	config.APIKey = strings.TrimSpace(config.APIKey)
	config.Model = strings.TrimSpace(config.Model)
	config.BaseURL = strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	if config.BaseURL == "" {
		config.BaseURL = "https://api.deepseek.com"
	}
	return &AgentModelClient{config: config, httpClient: &http.Client{}}
}

func (c *AgentModelClient) Configured() bool {
	return c != nil && c.config.APIKey != "" && c.config.Model != ""
}

func (c *AgentModelClient) Name() string { return c.config.Model }

func (c *AgentModelClient) Vision() bool { return c != nil && c.config.Vision }

// Cost — стоимость ответа в USD по ценам из настроек.
func (c *AgentModelClient) Cost(usage agentUsage) float64 {
	fresh := usage.PromptTokens - usage.CachedTokens
	if fresh < 0 {
		fresh = 0
	}
	return (float64(fresh)*c.config.PriceInput + float64(usage.CachedTokens)*c.config.PriceCached + float64(usage.CompletionTokens)*c.config.PriceOutput) / 1e6
}

func (c *AgentModelClient) Complete(ctx context.Context, messages []agentMessage, tools []agentTool, forceTool string) (agentReply, error) {
	if !c.Configured() {
		return agentReply{}, ai.ErrAINotConfigured
	}
	request := map[string]any{
		"model":       c.config.Model,
		"messages":    messages,
		"tools":       tools,
		"max_tokens":  4096,
		"temperature": 0.2,
	}
	if c.config.Effort != "" {
		// Размышления входят в лимит ответа: на высоком уровне 4096 токенов обрезали бы итог.
		request["reasoning_effort"] = c.config.Effort
		request["max_tokens"] = 32768
	}
	if forceTool != "" {
		request["tool_choice"] = map[string]any{"type": "function", "function": map[string]string{"name": forceTool}}
	}
	return c.send(ctx, request)
}

// CompleteJSON — один ответ модели в JSON без инструментов (описание и сверка фото).
func (c *AgentModelClient) CompleteJSON(ctx context.Context, messages []agentMessage) (agentReply, error) {
	if !c.Configured() {
		return agentReply{}, ai.ErrAINotConfigured
	}
	return c.send(ctx, map[string]any{
		"model":           c.config.Model,
		"messages":        messages,
		"response_format": map[string]string{"type": "json_object"},
		"max_tokens":      1024,
		"temperature":     0.1,
	})
}

func (c *AgentModelClient) send(ctx context.Context, request map[string]any) (agentReply, error) {
	if strings.Contains(c.config.BaseURL, "deepseek.com") {
		// Отдельный режим рассуждений DeepSeek в разы медленнее; шаги рассуждения — в системной подсказке.
		request["thinking"] = map[string]string{"type": "disabled"}
	}
	body, err := json.Marshal(request)
	if err != nil {
		return agentReply{}, ai.ErrAIBadResponse
	}
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
	switch {
	case resp.StatusCode == http.StatusUnauthorized, resp.StatusCode == http.StatusForbidden, resp.StatusCode == http.StatusPaymentRequired:
		return agentReply{}, ai.ErrAIUnauthorized
	case resp.StatusCode == http.StatusTooManyRequests:
		return agentReply{}, ai.ErrAIRateLimited
	case resp.StatusCode == http.StatusBadRequest, resp.StatusCode == http.StatusUnprocessableEntity:
		return agentReply{}, ai.ErrAIRequestRejected
	case resp.StatusCode == http.StatusNotFound:
		return agentReply{}, ai.ErrAIModelUnavailable
	case resp.StatusCode == http.StatusRequestTimeout:
		return agentReply{}, ai.ErrAITimeout
	case resp.StatusCode >= 500:
		return agentReply{}, ai.ErrAIUnavailable
	case resp.StatusCode < 200 || resp.StatusCode >= 300:
		return agentReply{}, ai.ErrAIBadResponse
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content   *string         `json:"content"`
				ToolCalls []agentToolCall `json:"tool_calls"`
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
	reply := agentReply{ToolCalls: message.ToolCalls}
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
