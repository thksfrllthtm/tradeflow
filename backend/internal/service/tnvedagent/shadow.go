package tnvedagent

// Теневой подбор: тот же товар с теми же данными параллельно подбирает вторая модель (Claude), сотрудник
// её не видит и не ждёт. Первый ход (строка сметы или вопрос чата-справочника) идёт с той же переписки, что
// у основной модели; каждое следующее действие сотрудника — уточнение, ответ на вопрос, «подобрать заново»,
// «Документы для ввоза» — повторяется и у тени: сообщение дописывается в её переписку, документы проверяют
// её под-агенты по тому же коду. Так сравнивается весь разговор, а не только первый ответ. Итог, лента событий
// (как сообщения чата сотрудника) и расход — в tnved_agent_shadow_runs; в расход сессии, лимиты сотрудника и
// базу ответов по коду тень не входит, модели сравнивает администратор в журнале агента. Предохранители:
// общий бюджет в долларах и дневной лимит прогонов — исчерпаны, прогон записывается пропущенным с причиной.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/ai"
	"github.com/shopspring/decimal"
)

const (
	shadowParallelRuns = 2
	// Очередь на слот тени вместе с самим подбором (agentRunTimeout) — не дольше этого.
	shadowQueueTimeout = 20 * time.Minute
	// Повторный ход ждёт, пока тень закончит прошлый: проверка раз в столько.
	shadowPollInterval = 5 * time.Second
	// rerunPrompt — указание «подобрать заново» (Rerun); тень по нему пишет в ленту разделитель, как чат сотрудника.
	rerunPrompt = "Подбери код заново: проверь ещё раз и закончи вызовом finish."
)

// shadowModel — модель тени: как у агента, плюс цена ответа и видит ли она фото сама.
type shadowModel interface {
	agentModel
	Cost(usage agentUsage) decimal.Decimal
	Vision() bool
}

// ShadowSettings — предохранители тени: бюджет в USD за всё время (0 — тень выключена) и прогонов в сутки (0 — без лимита).
type ShadowSettings struct {
	BudgetUSD  decimal.Decimal
	DailyLimit int
}

// SetShadow включает теневой подбор второй моделью; без ключа модели или бюджета тень остаётся выключенной.
func (s *TNVEDAgentService) SetShadow(model *AgentModelClient, settings ShadowSettings) {
	if model == nil || !model.Configured() || !settings.BudgetUSD.IsPositive() {
		log.Printf("shadow: выключена — нужны модель с ключом и TNVED_AI_SHADOW_BUDGET_USD > 0")
		return
	}
	s.shadow, s.shadowSettings = model, settings
}

// launchShadow — ход тени для хода сессии: первый — с той же переписки, следующие — с тем же сообщением
// сотрудника в переписке тени, «Документы для ввоза» — своими под-агентами. В фоне, основной подбор тень не ждёт.
func (s *TNVEDAgentService) launchShadow(session repository.TNVEDAgentSession, transcript []map[string]any, photos []string) {
	if s.shadow == nil {
		return
	}
	switch {
	case session.DocumentsCode != "":
		go s.shadowDocuments(session, transcript)
	case session.Runs == 0:
		go s.runShadow(session, transcript, photos)
	default:
		if text := lastUserMessage(transcript); text != "" {
			go s.continueShadow(session, text, photos)
		}
	}
}

// lastUserMessage — текст последнего сообщения сотрудника в переписке ("" — переписка кончается не им).
func lastUserMessage(transcript []map[string]any) string {
	if len(transcript) == 0 {
		return ""
	}
	last := transcript[len(transcript)-1]
	text, _ := last["content"].(string)
	if last["role"] != "user" {
		return ""
	}
	return text
}

// createShadowRun — строка тени для сессии: пропущенная с причиной, если бюджет или дневной лимит исчерпаны.
func (s *TNVEDAgentService) createShadowRun(store context.Context, session repository.TNVEDAgentSession) (repository.TNVEDAgentShadowRun, error) {
	now := time.Now().UTC()
	run := repository.TNVEDAgentShadowRun{SessionID: session.ID, Model: s.shadowRunName(), Status: "running", Transcript: repository.JSON("[]"),
		Messages: repository.JSON("[]"), Result: repository.JSON("null"), CostUSD: decimal.Zero, VisionCostUSD: decimal.Zero, StartedAt: now}
	reason, err := s.shadowLimit(store)
	if err != nil {
		return run, err
	}
	if reason != "" {
		run.Status, run.Error, run.FinishedAt = "skipped", reason, &now
	}
	if run, err = s.repository.CreateShadowRun(store, run); err != nil {
		log.Printf("shadow: запись: %v", err)
	}
	return run, err
}

func (s *TNVEDAgentService) runShadow(session repository.TNVEDAgentSession, transcript []map[string]any, photos []string) {
	run, err := s.createShadowRun(context.Background(), session)
	if err != nil || run.Status == "skipped" {
		return
	}
	state := newShadowState(transcript, s.shadow.Vision())
	s.shadowTurn(run.ID, func(ctx context.Context, save func(map[string]any)) map[string]any {
		return s.executeShadow(ctx, session, state, photos, save)
	})
}

// continueShadow — повторный ход тени: сообщение сотрудника дописывается в её переписку, и она отвечает.
func (s *TNVEDAgentService) continueShadow(session repository.TNVEDAgentSession, text string, photos []string) {
	store := context.Background()
	run, state, ok := s.claimShadow(store, session)
	if !ok {
		return
	}
	state.say(text)
	s.shadowTurn(run.ID, func(ctx context.Context, save func(map[string]any)) map[string]any {
		return s.executeShadow(ctx, session, state, photos, save)
	})
}

// shadowDocuments — «Документы для ввоза» у тени: по тому же коду, товару и описанию фото, что у основной
// модели. Если подбора кода у тени не было (включили позже), строка создаётся только под документы.
func (s *TNVEDAgentService) shadowDocuments(session repository.TNVEDAgentSession, mainTranscript []map[string]any) {
	store := context.Background()
	run, state, ok := s.claimShadow(store, session)
	if !ok {
		if _, err := s.repository.ShadowRunBySession(store, session.ID); !errors.Is(err, repository.ErrNotFound) {
			return
		}
		var err error
		if run, err = s.createShadowRun(store, session); err != nil || run.Status == "skipped" {
			return
		}
		state = newShadowState(nil, s.shadow.Vision())
	}
	if text := lastUserMessage(mainTranscript); text != "" {
		state.transcript = append(state.transcript, map[string]any{"role": "user", "content": text})
	}
	state.event("documents", map[string]string{"code": session.DocumentsCode})
	s.shadowTurn(run.ID, func(ctx context.Context, save func(map[string]any)) map[string]any {
		return s.executeShadowDocuments(ctx, session, mainTranscript, state, save)
	})
	s.compareShadowDocuments(store, session.ID, session.DocumentsCode)
}

// shadowOpinionWait — сколько основная модель ждёт ответ тени по документам, чтобы учесть второе мнение до
// показа сотруднику (тень обычно отвечает быстрее).
const shadowOpinionWait = 90 * time.Second

func (s *TNVEDAgentService) shadowName() string {
	if s.shadow == nil {
		return ""
	}
	return s.shadow.Name()
}

// shadowResultContents — содержимое итогов (result) из ленты тени.
func shadowResultContents(messages repository.JSON) []json.RawMessage {
	var events []struct {
		Kind    string          `json:"kind"`
		Content json.RawMessage `json:"content"`
	}
	_ = json.Unmarshal(messages, &events)
	contents := []json.RawMessage{}
	for _, event := range events {
		if event.Kind == "result" {
			contents = append(contents, event.Content)
		}
	}
	return contents
}

// documentsSecondOpinion — второе мнение по документам для основной модели: ждёт ответ тени (до
// shadowOpinionWait), сверяет наборы мер «нужно» (итог — в строку тени) и отдаёт её документы, чтобы спорные
// меры стали «возможно» ещё до показа. nil — тени нет.
func (s *TNVEDAgentService) documentsSecondOpinion(store context.Context, session repository.TNVEDAgentSession) func(context.Context, *AgentResult) *AgentResult {
	if s.shadow == nil {
		return nil
	}
	return func(ctx context.Context, main *AgentResult) *AgentResult {
		deadline := time.Now().Add(shadowOpinionWait)
		for {
			run, err := s.repository.ShadowRunBySession(store, session.ID)
			if err == nil {
				if run.Status == "skipped" {
					return nil
				}
				if other := lastDocumentsResult(shadowResultContents(run.Messages), session.DocumentsCode); other != nil {
					s.saveShadow(store, run.ID, map[string]any{"documents_code": session.DocumentsCode, "documents_agree": documentsAgree(main.Documents, other.Documents)})
					return other
				}
			}
			if ctx.Err() != nil || time.Now().After(deadline) {
				return nil
			}
			select {
			case <-ctx.Done():
				return nil
			case <-time.After(shadowPollInterval):
			}
		}
	}
}

// compareShadowDocuments — тень ответила по документам позже основной модели (второе мнение до показа не
// успело): сверить наборы мер «нужно», итог — в строку тени; при расхождении спорные меры в уже показанном
// ответе становятся «возможно», в чат — заметка, в базу ответов по коду — обновлённый итог.
func (s *TNVEDAgentService) compareShadowDocuments(store context.Context, sessionID int64, code string) {
	if s.shadow == nil || code == "" {
		return
	}
	run, err := s.repository.ShadowRunBySession(store, sessionID)
	if err != nil || run.DocumentsCode == code {
		return
	}
	shadowResult := lastDocumentsResult(shadowResultContents(run.Messages), code)
	messages, err := s.repository.AgentMessagesAfter(store, sessionID, 0)
	if err != nil {
		return
	}
	var mainMessage repository.TNVEDAgentMessage
	var mainResult *AgentResult
	for _, message := range messages {
		if message.Kind != "result" {
			continue
		}
		if result := lastDocumentsResult([]json.RawMessage{json.RawMessage(message.Content)}, code); result != nil {
			mainMessage, mainResult = message, result
		}
	}
	if shadowResult == nil || mainResult == nil {
		return
	}
	agree := documentsAgree(mainResult.Documents, shadowResult.Documents)
	s.saveShadow(store, run.ID, map[string]any{"documents_code": code, "documents_agree": agree})
	if agree {
		return
	}
	rules, _ := s.data.WebPage(store, conditionalReleaseURL)
	if applySecondOpinion(mainResult, shadowResult.Documents, s.shadow.Name(), rules.Text, nil, 0) == 0 {
		return
	}
	data, _ := json.Marshal(mainResult)
	_ = s.repository.UpdateAgentMessageContent(store, mainMessage.ID, repository.JSON(data))
	if session, err := s.repository.AgentSessionByID(store, sessionID); err == nil {
		s.saveCodeDocuments(store, session, mainResult)
	}
	s.emit(store, sessionID, "notice", map[string]string{"text": fmt.Sprintf("Вторая модель (%s) не согласна с частью мер «Документов для ввоза» для кода %s: они отмечены «возможно» с её доводом — откройте ответ заново и проверьте по актам.", s.shadow.Name(), FormatTNVEDCode(code))})
}

// claimShadow — строка тени сессии, занятая под новый ход, и её состояние. Ждёт, пока тень закончит прошлый
// ход; false — тени у сессии нет, она пропущена, не дождались или бюджет исчерпан (тогда заметка в ленте).
func (s *TNVEDAgentService) claimShadow(store context.Context, session repository.TNVEDAgentSession) (repository.TNVEDAgentShadowRun, shadowState, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), shadowQueueTimeout)
	defer cancel()
	var run repository.TNVEDAgentShadowRun
	for {
		var err error
		if run, err = s.repository.ShadowRunBySession(store, session.ID); err != nil {
			if !errors.Is(err, repository.ErrNotFound) {
				log.Printf("shadow: сессия %d: %v", session.ID, err)
			}
			return run, shadowState{}, false
		}
		claimed, err := s.repository.ClaimShadowRun(store, run.ID)
		if err != nil {
			log.Printf("shadow %d: %v", run.ID, err)
			return run, shadowState{}, false
		}
		if claimed {
			break
		}
		if run.Status != "running" {
			return run, shadowState{}, false
		}
		select {
		case <-ctx.Done():
			return run, shadowState{}, false
		case <-time.After(shadowPollInterval):
		}
	}
	state := loadShadowState(run)
	reason, err := s.shadowLimit(store)
	if err == nil && reason == "" {
		return run, state, true
	}
	if reason != "" {
		state.event("notice", map[string]string{"text": "Тень не ответила на это действие. " + reason})
	}
	changes := shadowChanges(state)
	changes["status"] = run.Status
	s.saveShadow(store, run.ID, changes)
	return run, state, false
}

// shadowTurn — один ход тени в своём слоте: итог и расход — в строку run.
func (s *TNVEDAgentService) shadowTurn(runID int64, execute func(ctx context.Context, save func(map[string]any)) map[string]any) {
	store := context.Background()
	ctx, cancel := context.WithTimeout(context.Background(), shadowQueueTimeout)
	defer cancel()
	select {
	case s.shadowSlots <- struct{}{}:
		defer func() { <-s.shadowSlots }()
	case <-ctx.Done():
		s.saveShadow(store, runID, map[string]any{"status": "error", "error": "Тень не дождалась очереди.", "finished_at": time.Now().UTC()})
		return
	}
	runCtx, cancelRun := context.WithTimeout(ctx, agentRunTimeout)
	defer cancelRun()
	s.saveShadow(store, runID, execute(runCtx, func(changes map[string]any) { s.saveShadow(store, runID, changes) }))
}

func (s *TNVEDAgentService) saveShadow(ctx context.Context, id int64, changes map[string]any) {
	if err := s.repository.UpdateShadowRun(ctx, id, changes); err != nil {
		log.Printf("shadow %d: %v", id, err)
	}
}

// shadowLimit — почему тень сейчас не запускается ("" — запускается): бюджет исчерпан или дневной лимит.
func (s *TNVEDAgentService) shadowLimit(ctx context.Context) (string, error) {
	spent, today, err := s.repository.ShadowSpend(ctx, dayStart(time.Now()))
	if err != nil {
		log.Printf("shadow: расход: %v", err)
		return "", err
	}
	return shadowLimit(s.shadowSettings, spent, today), nil
}

// shadowLimit — причина не запускать тень при таком расходе ("" — запускать).
func shadowLimit(settings ShadowSettings, spent decimal.Decimal, today int64) string {
	switch {
	case spent.GreaterThanOrEqual(settings.BudgetUSD):
		return fmt.Sprintf("Бюджет тени исчерпан: потрачено $%s из $%s.", spent.StringFixed(2), settings.BudgetUSD.StringFixed(2))
	case settings.DailyLimit > 0 && today >= int64(settings.DailyLimit):
		return fmt.Sprintf("Дневной лимит тени исчерпан: %d прогонов.", settings.DailyLimit)
	}
	return ""
}

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

// executeShadow — один ход тени как у основной модели (та же подсказка и инструменты режима, база, интернет и
// самопроверка). save получает поля строки по ходу (расход, шаги, лента); возвращаются итоговые поля.
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
		review: s.settings.SelfCheck, transcript: state.transcript, photos: photos, vision: vision, emit: state.event,
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
