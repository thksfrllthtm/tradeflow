package tnvedagent

// Цикл агента «модель → инструменты → результат» без базы данных: переписка в памяти, шаги
// для чата — через emit, сохранение — через onStep. Отдельно от сервиса, чтобы проверять тестами
// со сценарием вместо модели.

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/directory"
)

type agentOutcome struct {
	status string // done или waiting
	result *AgentResult
	err    error
}

type agentRunner struct {
	model      agentModel
	toolbox    *agentToolbox
	system     string
	transcript []map[string]any
	photos     []string
	vision     bool
	emit       func(kind string, content any)
	onStep     func(transcript []map[string]any, usage agentUsage)
	maxSteps   int
	// tools — инструменты запуска (nil — как при подборе для строки сметы).
	tools []agentTool
	// documents — запуск по кнопке «Документы для ввоза»: веб-инструменты и больше шагов.
	documents bool
	// review — первый итог возвращается агенту на самопроверку («второй взгляд»), окончательный — второй.
	review bool
}

// stepLimit — шагов до принудительного итога: maxSteps, иначе по режиму.
func (r *agentRunner) stepLimit() int {
	switch {
	case r.maxSteps > 0:
		return r.maxSteps
	case r.documents:
		return agentDocumentsMaxSteps
	case r.toolbox.search != nil:
		return agentWebMaxSteps
	}
	return agentMaxSteps
}

// materialize — переписка для модели: системная подсказка, ссылки на фото заменены самими фото
// (только если модель видит фото — vision; модель рассуждений без зрения фото не получает, у неё
// описание фото текстом). Сообщения transcript всегда прошли через JSON (части — []any из map[string]any).
func (r *agentRunner) materialize() []agentMessage {
	messages := []agentMessage{{Role: "system", Content: r.system}}
	for _, raw := range r.transcript {
		data, _ := json.Marshal(raw)
		var message agentMessage
		if json.Unmarshal(data, &message) != nil {
			continue
		}
		if parts, ok := raw["content"].([]any); ok {
			content := make([]map[string]any, 0, len(parts))
			for _, part := range parts {
				object, ok := part.(map[string]any)
				if !ok {
					continue
				}
				if object["type"] == "image_ref" {
					index, _ := object["index"].(float64)
					if r.vision && int(index) >= 0 && int(index) < len(r.photos) {
						content = append(content, map[string]any{"type": "image_url", "image_url": map[string]string{"url": r.photos[int(index)]}})
					}
					continue
				}
				content = append(content, object)
			}
			message.Content = content
		}
		if message.Role == "assistant" && message.Content == nil {
			message.Content = ""
		}
		messages = append(messages, message)
	}
	return messages
}

// restoreOpened — страницы, открытые в прежних запусках этого подбора (для проверки цитат), а также
// справка, документы и ссылки с них: после ответа сотрудника на ask_user open_link иначе откажет.
func (r *agentRunner) restoreOpened(ctx context.Context) {
	names := map[string]string{}
	for _, raw := range r.transcript {
		if raw["role"] == "assistant" {
			calls, _ := raw["tool_calls"].([]any)
			for _, call := range calls {
				object, _ := call.(map[string]any)
				function, _ := object["function"].(map[string]any)
				id, _ := object["id"].(string)
				name, _ := function["name"].(string)
				names[id] = name
			}
		}
		if raw["role"] == "tool" {
			id, _ := raw["tool_call_id"].(string)
			content, _ := raw["content"].(string)
			switch names[id] {
			case "open_details":
				var page struct {
					Code string `json:"code"`
					Text string `json:"text"`
				}
				if json.Unmarshal([]byte(content), &page) == nil && page.Code != "" && page.Text != "" {
					// Пояснения, показанные модели ссылкой на страницу другого кода, — для цитат целиком.
					if text, other, ok := strings.Cut(page.Text, sameNotesPrefix); ok && len(other) >= 10 {
						page.Text = text + notesOf(r.toolbox.opened[other[:10]])
					}
					r.toolbox.opened[page.Code] = page.Text
				}
			case "search_declarations":
				// Как при вызове: поиск сделан (с результатом или сайт не ответил), а не ошибка в аргументах.
				if strings.Contains(content, `"groups"`) || strings.Contains(content, "не ответил") {
					r.toolbox.declarationsChecked = true
				}
				var found struct {
					Query   string                        `json:"query"`
					Sources []directory.DeclarationSource `json:"sources"`
				}
				if json.Unmarshal([]byte(content), &found) == nil && found.Query != "" {
					r.toolbox.rememberDeclarations(found.Query, found.Sources)
				}
			case "web_search":
				var found struct {
					Results []webResult `json:"results"`
				}
				if json.Unmarshal([]byte(content), &found) == nil {
					for _, item := range found.Results {
						r.toolbox.links[item.URL] = true
					}
				}
			case "web_page":
				// Страница из интернета: её адрес и ссылки с неё (текст — не источник цитат).
				var page struct {
					URL   string              `json:"url"`
					Links []directory.WebLink `json:"links"`
					Error string              `json:"error"`
				}
				if json.Unmarshal([]byte(content), &page) == nil && page.URL != "" && page.Error == "" {
					r.toolbox.rememberPage(page.URL, "", page.Links)
				}
			case "open_goods_help", "open_link", "search_in_document":
				var page struct {
					URL       string              `json:"url"`
					Text      string              `json:"text"`
					Fragments []string            `json:"fragments"`
					Links     []directory.WebLink `json:"links"`
					Error     string              `json:"error"`
					Source    string              `json:"source"`
				}
				if json.Unmarshal([]byte(content), &page) == nil && page.URL != "" && page.Error == "" {
					text := strings.TrimSpace(page.Text + "\n" + strings.Join(page.Fragments, "\n"))
					if page.Source == "web" {
						text = ""
					}
					r.toolbox.rememberPage(page.URL, text, page.Links)
				}
			}
		}
	}
}

func (r *agentRunner) append(message agentMessage) {
	data, _ := json.Marshal(message)
	var raw map[string]any
	_ = json.Unmarshal(data, &raw)
	r.transcript = append(r.transcript, raw)
}

func (r *agentRunner) toolMessage(callID, content string) {
	r.append(agentMessage{Role: "tool", ToolCallID: callID, Content: content})
}

func (r *agentRunner) execute(ctx context.Context) agentOutcome {
	maxSteps := r.stepLimit()
	tools := r.tools
	if tools == nil {
		tools = agentTools(false, r.documents)
	}
	if r.toolbox.search != nil && !r.documents {
		tools = append(append([]agentTool{}, tools...), webTools()...)
	}
	if r.toolbox.lookPhoto != nil {
		tools = append(append([]agentTool{}, tools...), lookPhotoTool())
	}
	nudged, finishRetried, forced := false, false, false
	reviewed := !r.review
	for step := 0; ; step++ {
		if err := ctx.Err(); err != nil {
			return agentOutcome{err: err}
		}
		force := ""
		if step >= maxSteps {
			if forced {
				r.emit("notice", map[string]string{"text": "Агент не дал итог за отведённые шаги. Уточните задачу или подберите заново."})
				return agentOutcome{status: "done"}
			}
			forced = true
			force = "finish"
			r.append(agentMessage{Role: "user", Content: "Лимит шагов исчерпан. Сразу вызови finish по уже собранным данным."})
		}
		reply, err := r.model.Complete(ctx, r.materialize(), tools, force)
		if err != nil {
			return agentOutcome{err: err}
		}
		r.append(agentMessage{Role: "assistant", Content: reply.Content, ReasoningContent: reply.Reasoning, ToolCalls: reply.ToolCalls, ClaudeContent: reply.ClaudeContent})
		if reply.Content != "" {
			r.emit("text", map[string]string{"text": reply.Content})
		}
		if len(reply.ToolCalls) == 0 {
			r.onStep(r.transcript, reply.Usage)
			if nudged {
				r.emit("notice", map[string]string{"text": "Агент закончил без итога. Уточните задачу или подберите заново."})
				return agentOutcome{status: "done"}
			}
			nudged = true
			r.append(agentMessage{Role: "user", Content: "Продолжай: пользуйся инструментами и в конце обязательно вызови finish (или ask_user, если без ответа сотрудника не обойтись)."})
			continue
		}
		var finishCall, askCall *agentToolCall
		for index := range reply.ToolCalls {
			call := reply.ToolCalls[index]
			switch call.Function.Name {
			case "finish":
				if finishCall == nil {
					finishCall = &reply.ToolCalls[index]
					continue
				}
				r.toolMessage(call.ID, `{"status":"ignored"}`)
			case "ask_user":
				if askCall == nil {
					askCall = &reply.ToolCalls[index]
					continue
				}
				r.toolMessage(call.ID, `{"status":"ignored: один вопрос за раз"}`)
			default:
				content, event := r.toolbox.run(ctx, call.Function.Name, call.Function.Arguments)
				r.toolMessage(call.ID, content)
				r.emit("tool", event)
			}
		}
		if finishCall != nil {
			result, problem, err := r.toolbox.finish(ctx, finishCall.Function.Arguments)
			if err != nil {
				return agentOutcome{err: err}
			}
			if askCall != nil {
				r.toolMessage(askCall.ID, `{"status":"ignored: подбор завершён"}`)
			}
			// На принудительном итоге (лимит шагов) повторов нет: итог принимается как есть.
			if problem != "" && !finishRetried && !forced {
				finishRetried = true
				r.toolMessage(finishCall.ID, toolJSON(map[string]string{"error": problem}))
				r.onStep(r.transcript, reply.Usage)
				continue
			}
			// Самопроверка: итог ещё не принимается, агент проверяет себя и вызывает finish снова.
			if result != nil && !reviewed && !forced {
				reviewed = true
				r.toolMessage(finishCall.ID, toolJSON(map[string]string{"status": "review", "instruction": agentReviewPrompt}))
				r.onStep(r.transcript, reply.Usage)
				r.emit("tool", agentToolEvent{Tool: "review", Label: "Самопроверка вывода"})
				continue
			}
			if result == nil {
				r.toolMessage(finishCall.ID, `{"status":"rejected"}`)
				r.onStep(r.transcript, reply.Usage)
				r.emit("notice", map[string]string{"text": "Агент не нашёл кодов из номенклатуры. Уточните товар или подберите заново."})
				return agentOutcome{status: "done"}
			}
			r.toolMessage(finishCall.ID, `{"status":"ok"}`)
			r.onStep(r.transcript, reply.Usage)
			r.emit("result", result)
			return agentOutcome{status: "done", result: result}
		}
		if askCall != nil {
			var args struct {
				Question string `json:"question"`
			}
			_ = json.Unmarshal([]byte(askCall.Function.Arguments), &args)
			question := service.TruncateRunes(args.Question, 500)
			r.toolMessage(askCall.ID, `{"status":"Вопрос показан сотруднику. Его ответ придёт следующим сообщением."}`)
			r.onStep(r.transcript, reply.Usage)
			r.emit("question", map[string]string{"text": question})
			return agentOutcome{status: "waiting"}
		}
		r.onStep(r.transcript, reply.Usage)
	}
}
