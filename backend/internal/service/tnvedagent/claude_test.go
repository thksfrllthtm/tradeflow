package tnvedagent

import (
	"encoding/json"
	"strings"
	"testing"
)

// Переписка агента в формате Claude: системная подсказка отдельно, фото — base64-картинкой, вызовы —
// tool_use, ответы инструментов и следующее указание — одно сообщение user (tool_result первыми),
// сохранённый ответ Claude (с подписанными размышлениями) — без изменений.
func TestClaudeMessages(t *testing.T) {
	saved := json.RawMessage(`{"role":"assistant","content":[{"type":"thinking","thinking":"","signature":"sig-1"},{"type":"tool_use","id":"t2","name":"get_codes","input":{"codes":["6116102000"]}}]}`)
	messages := []agentMessage{
		{Role: "system", Content: "Ты — декларант."},
		{Role: "user", Content: []map[string]any{{"type": "text", "text": "Подбери код"}, {"type": "image_url", "image_url": map[string]string{"url": "data:image/jpeg;base64,AAAA"}}}},
		{Role: "user", Content: "Описание фото товара: перчатки"},
		{Role: "assistant", Content: "Ищу.", ToolCalls: []agentToolCall{call("t1", "search_nomenclature", map[string]string{"query": "перчатки"})}},
		{Role: "tool", ToolCallID: "t1", Content: `{"total":1}`},
		{Role: "user", Content: "Продолжай."},
		{Role: "assistant", ClaudeContent: saved},
		{Role: "tool", ToolCallID: "t2", Content: `{"codes":[]}`},
	}
	system, history, err := claudeMessages(messages)
	if err != nil {
		t.Fatal(err)
	}
	data, _ := json.Marshal(history)
	body := string(data)
	if len(system) != 1 || system[0].Text != "Ты — декларант." || len(history) != 5 {
		t.Fatalf("system %v, сообщений %d: %s", system, len(history), body)
	}
	for _, want := range []string{`"source":{"data":"AAAA","media_type":"image/jpeg","type":"base64"},"type":"image"`, `"id":"t1","input":{"query":"перчатки"},"name":"search_nomenclature","type":"tool_use"`,
		`"tool_use_id":"t1"`, `"signature":"sig-1"`, `"tool_use_id":"t2"`} {
		if !strings.Contains(body, want) {
			t.Errorf("нет %s в %s", want, body)
		}
	}
	// Два первых сообщения user склеены; ответ инструмента и «Продолжай» — одно сообщение, tool_result первым.
	if history[0].Role != "user" || len(history[0].Content) != 3 || history[2].Role != "user" || len(history[2].Content) != 2 || history[2].Content[0].OfToolResult == nil {
		t.Fatalf("склейка сообщений: %s", body)
	}
}

// Ключ, разорванный переносом при вставке в панель хостинга, склеивается обратно.
func TestAPIKeyWithoutInnerWhitespace(t *testing.T) {
	client := NewAgentModelClient(AgentModelConfig{Provider: providerAnthropic, APIKey: " sk-ant-api03-\nAbCd \r\n123\t"})
	if client.config.APIKey != "sk-ant-api03-AbCd123" {
		t.Fatalf("ключ: %q", client.config.APIKey)
	}
}
