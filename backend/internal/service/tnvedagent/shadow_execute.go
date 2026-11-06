package tnvedagent

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/ai"
)

// shadowState — переписка тени, её лента событий и накопленный расход: с начала или из строки прошлого хода.
type shadowState struct {
	transcript, messages []map[string]any
	usage                sessionUsage
	steps                int
	// continued — повторный ход: finished_at (конец первого ответа, по нему считается время) не трогается.
	continued bool
}

// newShadowState — первый ход с копии переписки основной модели. Модель, которая смотрит фото сама, описание
// от модели для фото не получает.
func newShadowState(transcript []map[string]any, vision bool) shadowState {
	transcript = cloneTranscript(transcript)
	if vision {
		transcript = withoutPhotoNote(transcript)
	}
	return shadowState{transcript: transcript, messages: []map[string]any{}}
}

func loadShadowState(run repository.TNVEDAgentShadowRun) shadowState {
	state := shadowState{continued: true, steps: run.Steps, usage: sessionUsage{
		total: agentUsage{PromptTokens: run.PromptTokens, CachedTokens: run.CachedTokens, CompletionTokens: run.CompletionTokens},
		cost:  run.CostUSD, visionCost: run.VisionCostUSD,
	}}
	_ = json.Unmarshal(run.Transcript, &state.transcript)
	_ = json.Unmarshal(run.Messages, &state.messages)
	if state.transcript == nil {
		state.transcript = []map[string]any{}
	}
	if state.messages == nil {
		state.messages = []map[string]any{}
	}
	return state
}

func (st *shadowState) event(kind string, content any) {
	st.messages = append(st.messages, map[string]any{"kind": kind, "content": content, "createdAt": time.Now().UTC()})
}

// say — сообщение сотрудника в переписку тени и в ленту (как в чате: «подобрать заново» — разделителем).
func (st *shadowState) say(text string) {
	st.transcript = append(st.transcript, map[string]any{"role": "user", "content": text})
	if text == rerunPrompt {
		st.event("rerun", map[string]string{})
		return
	}
	st.event("user", map[string]string{"text": text})
}

// finish — итоговые поля хода: статус по исходу; finished_at — только у первого ответа.
func (st *shadowState) finish(ctx context.Context, outcome agentOutcome, timeoutMessage string) map[string]any {
	changes := shadowChanges(*st)
	if !st.continued {
		changes["finished_at"] = time.Now().UTC()
	}
	if outcome.err == nil {
		changes["status"] = "done"
		return changes
	}
	message := ai.AIErrorMessage(outcome.err)
	if errors.Is(outcome.err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
		message = timeoutMessage
	}
	changes["status"], changes["error"] = "error", message
	return changes
}

// executeShadow — один ход тени как у основной модели (та же подсказка и инструменты режима, база и интернет).
// save получает поля строки по ходу (расход, шаги, лента); возвращаются итоговые поля.
// Вопрос сотруднику остаётся без ответа — тень решает по имеющимся данным, как в прогоне точности.
func (s *TNVEDAgentService) executeShadow(ctx context.Context, session repository.TNVEDAgentSession, state shadowState, photos []string, save func(map[string]any)) map[string]any {
	reference := session.Mode == modeReference
	vision := s.shadow.Vision()
	toolbox := newAgentToolbox(s.data)
	toolbox.search = s.search
	if !vision {
		toolbox.lookPhoto = s.photoQuestions(photos, func(step agentUsage) { state.usage.add(step, s.visionModel.Cost(step), true) })
	}
	toolbox.requireDeclarations = true
	toolbox.fileCode = sessionFileCode(session)
	if s.trademarks != nil {
		toolbox.trademarks = s.trademarks.Match
	}
	runner := &agentRunner{
		model: s.shadow, toolbox: toolbox, system: s.systemPrompt(reference, toolbox.lookPhoto != nil), tools: agentTools(reference),
		transcript: state.transcript, photos: photos, vision: vision, emit: state.event,
		onStep: func(transcript []map[string]any, step agentUsage) {
			state.usage.add(step, s.shadow.Cost(step), false)
			state.steps++
			state.transcript = transcript
			save(shadowChanges(state))
		},
	}
	runner.restoreOpened(ctx)
	outcome := runner.execute(ctx)
	if outcome.status == "waiting" {
		state.event("notice", map[string]string{"text": "Ответить на вопрос некому: тень решает по имеющимся данным."})
		runner.append(agentMessage{Role: "user", Content: "Сотрудника нет, ответить некому: реши по имеющимся данным и вызови finish."})
		outcome = runner.execute(ctx)
	}
	state.transcript = runner.transcript
	changes := state.finish(ctx, outcome, "Теневой подбор занял слишком много времени и остановлен.")
	if outcome.err == nil && outcome.result != nil {
		data, _ := json.Marshal(outcome.result)
		changes["result"] = repository.JSON(data)
	}
	return changes
}

// executeShadowDocuments — «Документы для ввоза» под-агентами тени: по тому же коду, товару и описанию фото.
// Ответ — в ленту и переписку тени; итог подбора кода (result) и база ответов по коду не трогаются.
func (s *TNVEDAgentService) executeShadowDocuments(ctx context.Context, session repository.TNVEDAgentSession, mainTranscript []map[string]any, state shadowState, save func(map[string]any)) map[string]any {
	run := &documentsRun{
		model: s.shadow, data: s.data, search: s.search, code: session.DocumentsCode,
		product: sessionProduct(session), photo: photoNoteText(mainTranscript), emit: state.event,
		onStep: func(step agentUsage) {
			state.usage.add(step, s.shadow.Cost(step), false)
			state.steps++
			save(shadowChanges(state))
		},
	}
	outcome := run.execute(ctx)
	state.transcript = append(state.transcript, map[string]any{"role": "assistant", "content": documentsReply(session.DocumentsCode, outcome)})
	return state.finish(ctx, outcome, "Теневая проверка документов заняла слишком много времени и остановлена.")
}

// shadowChanges — поля строки тени: переписка, лента, шаги и расход.
func shadowChanges(state shadowState) map[string]any {
	transcriptJSON, _ := json.Marshal(state.transcript)
	messagesJSON, _ := json.Marshal(state.messages)
	return map[string]any{
		"transcript": repository.JSON(transcriptJSON), "messages": repository.JSON(messagesJSON), "steps": state.steps,
		"prompt_tokens": state.usage.total.PromptTokens, "cached_tokens": state.usage.total.CachedTokens, "completion_tokens": state.usage.total.CompletionTokens,
		"cost_usd": state.usage.cost.Round(6), "vision_cost_usd": state.usage.visionCost.Round(6),
	}
}

// cloneTranscript — своя копия переписки: основной исполнитель свою дописывает, тень не должна её трогать.
func cloneTranscript(transcript []map[string]any) []map[string]any {
	data, _ := json.Marshal(transcript)
	copied := []map[string]any{}
	_ = json.Unmarshal(data, &copied)
	return copied
}

// withoutPhotoNote — переписка без описания фото от модели для фото.
func withoutPhotoNote(transcript []map[string]any) []map[string]any {
	kept := make([]map[string]any, 0, len(transcript))
	for _, message := range transcript {
		if text, ok := message["content"].(string); ok && message["role"] == "user" && strings.HasPrefix(text, photoNotePrefix) {
			continue
		}
		kept = append(kept, message)
	}
	return kept
}

// shadowRunName — название тени в журнале: тень без зрения (DeepSeek) видит фото через модель для фото —
// «gemini-3.8-flash + deepseek-v4-pro»; Claude смотрит сам — только своё название.
func (s *TNVEDAgentService) shadowRunName() string {
	if !s.shadow.Vision() && s.twoModels() && s.visionModel.Configured() && s.visionModel.Vision() {
		return s.visionModel.Name() + " + " + s.shadow.Name()
	}
	return s.shadow.Name()
}
