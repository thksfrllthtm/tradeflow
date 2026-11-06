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

// launch — запуск в очереди. Лимит времени считается с выхода из очереди: в большой смете последние строки
// ждут свободного места долго, и ожидание не должно съедать их время на подбор. Остановка сотрудником
// (cancel) работает и в очереди.
func (s *TNVEDAgentService) launch(sessionID int64, timeout time.Duration) {
	queued, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.cancels[sessionID] = cancel
	s.mu.Unlock()
	go func() {
		defer func() {
			cancel()
			s.mu.Lock()
			delete(s.cancels, sessionID)
			s.mu.Unlock()
		}()
		select {
		case s.slots <- struct{}{}:
			defer func() { <-s.slots }()
		case <-queued.Done():
		}
		ctx, stop := context.WithTimeout(queued, timeout)
		defer stop()
		s.run(ctx, sessionID)
	}()
}

// run — один запуск агента до итога, вопроса сотруднику, ошибки или остановки.
func (s *TNVEDAgentService) run(ctx context.Context, sessionID int64) {
	// Записи в базу — не через ctx запуска: после остановки или тайм-аута итог всё равно сохраняется.
	store := context.Background()
	session, err := s.repository.AgentSessionByID(store, sessionID)
	if err != nil {
		return
	}
	var transcript []map[string]any
	var photos []string
	_ = json.Unmarshal(session.Transcript, &transcript)
	_ = json.Unmarshal(session.Photos, &photos)
	usage := usageOf(session)
	steps := session.Steps
	documents := session.DocumentsCode != ""
	emit := func(kind string, content any) { s.emit(store, sessionID, kind, content) }
	// Товар нашёлся в базе без ИИ, а сотрудник подбирает заново: описания фото ещё нет.
	if note, step := s.photoNote(ctx, photos, sessionNames(session), transcript, emit); note != nil || step != (agentUsage{}) {
		usage.add(step, s.visionModel.Cost(step), true)
		if note != nil {
			transcript = append(transcript, note)
		}
	}
	// Отсюда считается время до итога — у основной модели и у тени одинаково: описание фото и поиск в базе
	// позади, обе начинают с одной переписки и идут параллельно.
	_ = s.repository.MarkAgentStarted(store, sessionID)
	s.launchShadow(session, transcript, photos)
	var outcome agentOutcome
	if documents {
		outcome = s.runDocuments(ctx, store, session, transcript, &usage, &steps, emit)
	} else {
		toolbox := newAgentToolbox(s.data)
		toolbox.search = s.search
		// Модель, которая видит фото сама (Claude), вопросов модели для фото не задаёт.
		if !s.model.Vision() {
			toolbox.lookPhoto = s.photoQuestions(photos, func(step agentUsage) { usage.add(step, s.visionModel.Cost(step), true) })
		}
		toolbox.requireDeclarations = true
		toolbox.fileCode = sessionFileCode(session)
		if s.trademarks != nil {
			toolbox.trademarks = s.trademarks.Match
		}
		runner := &agentRunner{
			model:      s.model,
			toolbox:    toolbox,
			system:     s.systemPrompt(session.Mode == modeReference, toolbox.lookPhoto != nil),
			tools:      agentTools(session.Mode == modeReference),
			ownEyes:    s.model.Vision() && s.twoModels(),
			transcript: transcript,
			photos:     photos,
			vision:     s.model.Vision(),
			emit:       emit,
			onStep: func(transcript []map[string]any, step agentUsage) {
				usage.add(step, s.model.Cost(step), false)
				steps++
				data, _ := json.Marshal(transcript)
				changes := usage.changes()
				changes["transcript"], changes["steps"] = repository.JSON(data), steps
				_ = s.repository.UpdateAgentSession(store, sessionID, changes)
			},
			finalize: func(result *AgentResult) { result.Trademarks = s.checkTrademarks(store, session, transcript, result) },
		}
		runner.restoreOpened(store)
		outcome = runner.execute(ctx)
	}
	changes := map[string]any{"runs": session.Runs + 1}
	switch {
	case errors.Is(ctx.Err(), context.Canceled):
		// Остановлено сотрудником: статус уже «остановлен».
	case outcome.err != nil:
		message := ai.AIErrorMessage(outcome.err)
		if errors.Is(outcome.err, context.DeadlineExceeded) || errors.Is(ctx.Err(), context.DeadlineExceeded) {
			message = "Подбор занял слишком много времени и остановлен. Нажмите «Подобрать заново» или уточните задачу."
		}
		changes["status"], changes["error"] = "error", message
		s.emit(store, sessionID, "error", map[string]string{"message": message})
	case outcome.status == "waiting":
		changes["status"] = "waiting"
	default:
		changes["status"] = "done"
		// Ответ про документы — в чате и в таблице документов по коду: итог подбора (код строки,
		// история) остаётся прежним.
		if outcome.result != nil && !documents {
			data, _ := json.Marshal(outcome.result)
			changes["result"] = repository.JSON(data)
			if title := resultTitle(outcome.result); session.Mode == modeReference && title != "" {
				changes["title"] = title
			}
		}
		// Итог с пробелами (под-агент не ответил) показывается, но ответом по коду в базе не становится.
		if outcome.result != nil && documents && !outcome.partial {
			s.saveCodeDocuments(store, session, outcome.result)
		}
	}
	if documents {
		// Ответ (или причина неудачи) — в переписку: уточнение в том же чате видит его, и два сообщения
		// сотрудника подряд не встают (переписку под-агентов в сессию не пишем).
		transcript = append(transcript, map[string]any{"role": "assistant", "content": documentsReply(session.DocumentsCode, outcome)})
		data, _ := json.Marshal(transcript)
		changes["transcript"] = repository.JSON(data)
	}
	_ = s.repository.UpdateAgentSession(store, sessionID, changes)
	if documents {
		s.compareShadowDocuments(store, sessionID, session.DocumentsCode)
	}
}

// systemPrompt — подсказка запуска: подбор или справочник, плюс интернет и вопросы по фото.
func (s *TNVEDAgentService) systemPrompt(reference, photoQuestions bool) string {
	prompt := tnvedAgentPrompt(s.settings.NamingRule)
	if reference {
		prompt = tnvedReferencePrompt(s.settings.NamingRule)
	}
	if s.search != nil {
		prompt += "\n\n" + tnvedWebPrompt
	}
	if photoQuestions {
		photo := tnvedPhotoPrompt
		if reference {
			photo = strings.Replace(photo, "- Сотрудника (ask_user) спрашивай только о том, чего на фото не видно: состав, мощность, назначение.", "- Чего на фото не видно (состав, мощность, назначение) — не спрашивай сотрудника, а напиши в check.", 1)
		}
		prompt += "\n\n" + photo
	}
	return prompt
}
