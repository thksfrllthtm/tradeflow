package tnvedagent

// Модель Claude (Anthropic Messages API, официальный Go SDK) в тех же ролях, что AgentModelClient на
// OpenAI-совместимом API: цикл агента с инструментами, описание фото, ответы по фото. Только для прогона
// cmd/tnved-agent-eval (Provider = "anthropic") и теневой подбор на сервере (shadow.go).
//
// Переписка агента хранится в OpenAI-формате; здесь она переводится в формат Claude: системная подсказка —
// отдельно, фото — base64-картинками, вызовы инструментов — tool_use, ответы — tool_result в сообщении
// user. Ответ Claude целиком (с блоками размышлений и их подписью) хранится в ClaudeContent и уходит
// модели обратно без изменений — так требует API при размышлениях с инструментами.

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/ai"
)

const (
	providerAnthropic = "anthropic"
	// Размышления входят в лимит ответа; без потоковой передачи SDK держит запрос до 10 минут.
	claudeMaxTokens = 16000
	claudeTimeout   = 5 * time.Minute
)

func (c *AgentModelClient) claudeClient() anthropic.Client {
	return anthropic.NewClient(option.WithAPIKey(c.config.APIKey), option.WithRequestTimeout(claudeTimeout), option.WithMaxRetries(2))
}

// claudeComplete — один запрос к Claude: с инструментами (цикл агента) или без (текст, JSON). Размышления —
// adaptive, глубина — effort из настроек; кэш подсказки — автоматический (cache_control на запросе).
func (c *AgentModelClient) claudeComplete(ctx context.Context, messages []agentMessage, tools []agentTool) (agentReply, error) {
	system, history, err := claudeMessages(messages)
	if err != nil {
		return agentReply{}, ai.ErrAIBadResponse
	}
	params := anthropic.MessageNewParams{
		Model:        anthropic.Model(c.config.Model),
		MaxTokens:    claudeMaxTokens,
		System:       system,
		Messages:     history,
		Thinking:     anthropic.ThinkingConfigParamUnion{OfAdaptive: &anthropic.ThinkingConfigAdaptiveParam{}},
		CacheControl: anthropic.NewCacheControlEphemeralParam(),
	}
	if c.config.Effort != "" {
		params.OutputConfig = anthropic.OutputConfigParam{Effort: anthropic.OutputConfigEffort(c.config.Effort)}
	}
	// Принудительный tool_choice эти модели не принимают (400): finish просит текст подсказки.
	for _, tool := range tools {
		properties, _ := tool.Function.Parameters["properties"].(map[string]any)
		required, _ := tool.Function.Parameters["required"].([]string)
		params.Tools = append(params.Tools, anthropic.ToolUnionParam{OfTool: &anthropic.ToolParam{
			Name: tool.Function.Name, Description: anthropic.String(tool.Function.Description),
			InputSchema: anthropic.ToolInputSchemaParam{Properties: properties, Required: required},
		}})
	}
	client := c.claudeClient()
	response, err := client.Messages.New(ctx, params)
	if err != nil {
		return agentReply{}, claudeError(ctx, err)
	}
	if response.StopReason == anthropic.StopReasonRefusal {
		return agentReply{}, ai.ErrAIRequestRejected
	}
	reply := agentReply{Usage: agentUsage{
		PromptTokens:     response.Usage.InputTokens + response.Usage.CacheReadInputTokens + response.Usage.CacheCreationInputTokens,
		CachedTokens:     response.Usage.CacheReadInputTokens,
		CacheWriteTokens: response.Usage.CacheCreationInputTokens,
		CompletionTokens: response.Usage.OutputTokens,
	}}
	texts := []string{}
	for _, block := range response.Content {
		switch variant := block.AsAny().(type) {
		case anthropic.TextBlock:
			texts = append(texts, variant.Text)
		case anthropic.ThinkingBlock:
			if variant.Thinking != "" {
				reply.Reasoning = strings.TrimSpace(reply.Reasoning + "\n" + variant.Thinking)
			}
		case anthropic.ToolUseBlock:
			call := agentToolCall{ID: variant.ID, Type: "function"}
			call.Function.Name, call.Function.Arguments = variant.Name, string(variant.Input)
			reply.ToolCalls = append(reply.ToolCalls, call)
		}
	}
	reply.Content = strings.TrimSpace(strings.Join(texts, "\n"))
	if raw, err := json.Marshal(response.ToParam()); err == nil {
		reply.ClaudeContent = raw
	}
	return reply, nil
}

// claudeMessages — переписка агента в формате Claude. Подряд идущие сообщения одной роли склеиваются:
// ответы инструментов и следующее указание — одно сообщение user, первым — tool_result.
func claudeMessages(messages []agentMessage) ([]anthropic.TextBlockParam, []anthropic.MessageParam, error) {
	system := []anthropic.TextBlockParam{}
	history := []anthropic.MessageParam{}
	add := func(role anthropic.MessageParamRole, blocks ...anthropic.ContentBlockParamUnion) {
		if len(blocks) == 0 {
			return
		}
		if last := len(history) - 1; last >= 0 && history[last].Role == role {
			history[last].Content = append(history[last].Content, blocks...)
			return
		}
		history = append(history, anthropic.MessageParam{Role: role, Content: blocks})
	}
	for _, message := range messages {
		switch message.Role {
		case "system":
			if text, _ := message.Content.(string); text != "" {
				system = append(system, anthropic.TextBlockParam{Text: text})
			}
		case "user":
			add(anthropic.MessageParamRoleUser, claudeUserBlocks(message.Content)...)
		case "tool":
			text, _ := message.Content.(string)
			add(anthropic.MessageParamRoleUser, anthropic.NewToolResultBlock(message.ToolCallID, firstNonEmpty(text, "{}"), false))
		case "assistant":
			if len(message.ClaudeContent) > 0 {
				var saved anthropic.MessageParam
				if err := json.Unmarshal(message.ClaudeContent, &saved); err != nil {
					return nil, nil, err
				}
				add(anthropic.MessageParamRoleAssistant, saved.Content...)
				continue
			}
			// Ответ другой модели (в прогоне не бывает) — текстом и вызовами без размышлений.
			blocks := []anthropic.ContentBlockParamUnion{}
			if text, _ := message.Content.(string); strings.TrimSpace(text) != "" {
				blocks = append(blocks, anthropic.NewTextBlock(text))
			}
			for _, call := range message.ToolCalls {
				blocks = append(blocks, anthropic.NewToolUseBlock(call.ID, json.RawMessage(firstNonEmpty(call.Function.Arguments, "{}")), call.Function.Name))
			}
			add(anthropic.MessageParamRoleAssistant, blocks...)
		}
	}
	return system, history, nil
}

// claudeUserBlocks — текст и фото сообщения: строка или части (text, image_url с data URL).
func claudeUserBlocks(content any) []anthropic.ContentBlockParamUnion {
	if text, ok := content.(string); ok {
		if strings.TrimSpace(text) == "" {
			return nil
		}
		return []anthropic.ContentBlockParamUnion{anthropic.NewTextBlock(text)}
	}
	parts := []map[string]any{}
	switch value := content.(type) {
	case []map[string]any:
		parts = value
	case []any:
		for _, part := range value {
			if object, ok := part.(map[string]any); ok {
				parts = append(parts, object)
			}
		}
	}
	blocks := []anthropic.ContentBlockParamUnion{}
	for _, part := range parts {
		switch part["type"] {
		case "text":
			if text, _ := part["text"].(string); strings.TrimSpace(text) != "" {
				blocks = append(blocks, anthropic.NewTextBlock(text))
			}
		case "image_url":
			address := ""
			switch image := part["image_url"].(type) {
			case map[string]string:
				address = image["url"]
			case map[string]any:
				address, _ = image["url"].(string)
			}
			// data:image/jpeg;base64,…
			header, data, ok := strings.Cut(address, ",")
			mediaType, isBase64 := strings.CutSuffix(strings.TrimPrefix(header, "data:"), ";base64")
			if ok && isBase64 && strings.HasPrefix(mediaType, "image/") {
				blocks = append(blocks, anthropic.NewImageBlockBase64(mediaType, data))
			}
		}
	}
	return blocks
}

// claudeError — ошибка API Claude в ошибки ИИ программы (как у OpenAI-совместимого клиента).
func claudeError(ctx context.Context, err error) error {
	// Текст ошибки API — в журнал: в отчёте прогона видна только общая причина.
	log.Printf("claude: %v", err)
	var apiError *anthropic.Error
	if errors.As(err, &apiError) {
		switch status := apiError.StatusCode; {
		case status == http.StatusUnauthorized, status == http.StatusForbidden:
			return ai.ErrAIUnauthorized
		case status == http.StatusTooManyRequests:
			return ai.ErrAIRateLimited
		case status == http.StatusBadRequest, status == http.StatusUnprocessableEntity:
			return ai.ErrAIRequestRejected
		case status == http.StatusNotFound:
			return ai.ErrAIModelUnavailable
		case status >= 500:
			return ai.ErrAIUnavailable
		}
		return ai.ErrAIBadResponse
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return ai.ErrAITimeout
	}
	return ai.ErrAIUnavailable
}
