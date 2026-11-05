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

// agentForcedTries — сколько раз после лимита шагов модели предлагается вызвать finish, прежде чем
// запуск заканчивается без итога.
const agentForcedTries = 2

type agentOutcome struct {
	status string // done или waiting
	result *AgentResult
	err    error
	// partial — итог «Документов для ввоза» с пробелами (под-агент не ответил): в базу по коду не сохраняется.
	partial bool
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
	// documents — под-агент «Документов для ввоза»: без поиска в интернете.
	documents bool
	// review — первый итог возвращается агенту на самопроверку («второй взгляд»), окончательный — второй.
	review bool
	// finalize — дополнение принятого итога программой перед показом (знаки ТРОИС); nil — нет.
	finalize func(result *AgentResult)
	// ownEyes — модель смотрит фото сама и описание фото от модели для фото не получает (оно остаётся в
	// переписке для тени без зрения и следующих ходов).
	ownEyes bool
}

// stepLimit — шагов до принудительного итога: maxSteps, иначе по режиму.
func (r *agentRunner) stepLimit() int {
	switch {
	case r.maxSteps > 0:
		return r.maxSteps
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
		if text, ok := raw["content"].(string); ok && r.ownEyes && raw["role"] == "user" && strings.HasPrefix(text, photoNotePrefix) {
			continue
		}
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
					text := strings.Replace(r.toolbox.expandNotes(page.Text), decisionsNote, "", 1)
					r.toolbox.opened[page.Code] = text
					r.toolbox.rememberNotes(page.Code, text)
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
		tools = agentTools(false)
	}
	if r.toolbox.search != nil && !r.documents {
		tools = append(append([]agentTool{}, tools...), webTools()...)
	}
	if r.toolbox.lookPhoto != nil {
		tools = append(append([]agentTool{}, tools...), lookPhotoTool())
	}
	nudged, finishRetried := false, false
	// forcedTries — попытки принудительного итога после лимита шагов: на них другие инструменты не
	// выполняются, модель получает отказ и зовёт finish (модель с размышлениями tool_choice не принимает).
	forcedTries := 0
	reviewed := !r.review
	canAsk := false
	for _, tool := range tools {
		canAsk = canAsk || tool.Function.Name == "ask_user"
	}
	nudge := "Продолжай: пользуйся инструментами и в конце обязательно вызови finish."
	if canAsk {
		nudge = "Продолжай: пользуйся инструментами и в конце обязательно вызови finish (или ask_user, если без ответа сотрудника не обойтись)."
	}
	for step := 0; ; step++ {
		if err := ctx.Err(); err != nil {
			return agentOutcome{err: err}
		}
		force := ""
		if step >= maxSteps {
			if forcedTries == agentForcedTries {
				r.emit("notice", map[string]string{"text": "Агент не дал итог за отведённые шаги. Уточните задачу или подберите заново."})
				return agentOutcome{status: "done"}
			}
			forcedTries++
			force = "finish"
			if forcedTries == 1 {
				r.append(agentMessage{Role: "user", Content: "Лимит шагов исчерпан. Сразу вызови finish по уже собранным данным."})
			}
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
			r.append(agentMessage{Role: "user", Content: nudge})
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
				if !canAsk {
					r.toolMessage(call.ID, `{"error":"В справочнике вопросов сотруднику нет: дай лучший код по умолчанию, второй вариант с verdict doubtful, а что уточнить — в check; затем finish."}`)
					continue
				}
				if askCall != nil {
					r.toolMessage(call.ID, `{"status":"ignored: один вопрос за раз"}`)
					continue
				}
				if problem := r.questionProblem(ctx, call.Function.Arguments); problem != "" {
					r.toolMessage(call.ID, toolJSON(map[string]string{"error": problem}))
					continue
				}
				askCall = &reply.ToolCalls[index]
			default:
				if force != "" {
					r.toolMessage(call.ID, `{"error":"Лимит шагов исчерпан: инструменты больше недоступны — сразу вызови finish по уже собранным данным (чего не хватает — applies maybe или verdict doubtful)."}`)
					continue
				}
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
			if problem != "" && !finishRetried && forcedTries == 0 {
				finishRetried = true
				r.toolMessage(finishCall.ID, toolJSON(map[string]string{"error": problem}))
				r.onStep(r.transcript, reply.Usage)
				continue
			}
			// Самопроверка: итог ещё не принимается, агент проверяет себя и вызывает finish снова.
			if result != nil && !reviewed && forcedTries == 0 {
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
			if r.finalize != nil {
				r.finalize(result)
			}
			r.emit("result", result)
			return agentOutcome{status: "done", result: result}
		}
		if askCall != nil {
			args := parseQuestion(askCall.Function.Arguments)
			question := questionText(args)
			r.toolMessage(askCall.ID, `{"status":"`+askedNote+` Его ответ придёт следующим сообщением."}`)
			r.onStep(r.transcript, reply.Usage)
			r.emit("question", map[string]string{"text": question})
			return agentOutcome{status: "waiting"}
		}
		r.onStep(r.transcript, reply.Usage)
	}
}

// agentMaxQuestions — вопросов сотруднику за подбор; askedNote — ответ модели на принятый вопрос.
const (
	agentMaxQuestions = 2
	askedNote         = "Вопрос показан сотруднику."
)

// agentQuestion — аргументы ask_user: вопрос и варианты ответа, каждый со своим кодом.
type agentQuestion struct {
	Question string `json:"question"`
	Options  []struct {
		Answer string `json:"answer"`
		Code   string `json:"code"`
	} `json:"options"`
}

func parseQuestion(arguments string) agentQuestion {
	var question agentQuestion
	_ = json.Unmarshal([]byte(arguments), &question)
	question.Question = service.TruncateRunes(strings.TrimSpace(question.Question), 500)
	for index := range question.Options {
		question.Options[index].Answer = service.TruncateRunes(strings.TrimSpace(question.Options[index].Answer), 120)
		question.Options[index].Code = nonDigits.ReplaceAllString(question.Options[index].Code, "")
	}
	return question
}

// questionText — вопрос для сотрудника с вариантами и кодом, к которому ведёт каждый.
func questionText(question agentQuestion) string {
	lines := []string{question.Question}
	for _, option := range question.Options {
		lines = append(lines, "— "+option.Answer+" → "+FormatTNVEDCode(option.Code))
	}
	return strings.Join(append(lines, "— не знаю → код по умолчанию, второй вариант «возможно»"), "\n")
}

// questionProblem — почему вопрос нельзя задавать ("" — можно). Спрашивать стоит, только если от ответа
// зависит код: у вариантов минимум два разных кода из номенклатуры. И не больше agentMaxQuestions за подбор.
func (r *agentRunner) questionProblem(ctx context.Context, arguments string) string {
	asked := 0
	for _, message := range r.transcript {
		if content, _ := message["content"].(string); message["role"] == "tool" && strings.Contains(content, askedNote) {
			asked++
		}
	}
	if asked >= agentMaxQuestions {
		return "Вопросов уже два: больше не спрашивай. Дай лучший код по умолчанию, второй вариант с verdict doubtful, что уточнить — в check; затем finish."
	}
	question := parseQuestion(arguments)
	codes := map[string]bool{}
	for _, option := range question.Options {
		if len(option.Code) == 10 && option.Answer != "" {
			codes[option.Code] = true
		}
	}
	if question.Question == "" || len(codes) < 2 {
		return "Вопрос не меняет код: задавай его, только если от ответа зависит код — в options 2–4 ответа, у каждого свой 10-значный код. Иначе не спрашивай, реши сам и вызови finish (что уточнить — в check)."
	}
	list := make([]string, 0, len(codes))
	for code := range codes {
		list = append(list, code)
	}
	found, err := r.toolbox.data.LookupCodes(ctx, list)
	if err == nil && len(found) < len(list) {
		return "В options есть коды, которых нет в номенклатуре: проверь их через get_codes и задай вопрос с настоящими кодами."
	}
	return ""
}
