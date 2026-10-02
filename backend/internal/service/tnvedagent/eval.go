package tnvedagent

// Проверка точности агента на проверенных записях базы решений: агент подбирает код для товара
// записи, не видя самой записи, и итог сравнивается с проверенным кодом. Запускается командой
// cmd/tnved-agent-eval; ничего не сохраняет, расходует токены модели.

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/ai"
)

type AgentEvalSummary struct {
	Total int
	// Лучший код агента совпал с проверенным; проверенный код есть среди «подходит/возможно».
	BestCorrect      int
	CandidateCorrect int
	NoResult         int
	Errors           int
	CostUSD          float64
	Duration         time.Duration
}

// evalData — данные агента без проверяемой записи (иначе он найдёт готовый ответ в базе).
type evalData struct {
	agentData
	hiddenID int64
}

func (d evalData) SearchDecisions(ctx context.Context, query string, limit int) ([]repository.CodeDecision, error) {
	rows, err := d.agentData.SearchDecisions(ctx, query, limit+1)
	if err != nil {
		return nil, err
	}
	kept := rows[:0]
	for _, row := range rows {
		if row.ID != d.hiddenID && len(kept) < limit {
			kept = append(kept, row)
		}
	}
	return kept, nil
}

// Evaluate прогоняет агента на последних проверенных записях с фото или наименованием.
func (s *TNVEDAgentService) Evaluate(ctx context.Context, limit int, report func(string)) (AgentEvalSummary, error) {
	if !s.model.Configured() {
		return AgentEvalSummary{}, ai.ErrAINotConfigured
	}
	rows, _, err := s.repository.ListCodeDecisions(ctx, repository.CodeDecisionFilter{Status: "verified", Limit: limit})
	if err != nil {
		return AgentEvalSummary{}, err
	}
	summary := AgentEvalSummary{}
	started := time.Now()
	for _, listed := range rows {
		decision, err := s.repository.CodeDecisionByID(ctx, listed.ID)
		if err != nil {
			return summary, err
		}
		var photos []string
		_ = json.Unmarshal(decision.Photos, &photos)
		item := AgentItemInput{Name: evalName(decision), SupplierName: decision.SupplierName, Quantity: "1", Unit: "шт"}
		if decision.UnitNetKg != nil {
			item.NetKg = decision.UnitNetKg.String()
		}
		if decision.UnitPriceUSD != nil {
			item.PricePerUnit = decision.UnitPriceUSD.String()
		}
		initial, _ := json.Marshal([]map[string]any{initialAgentMessage(item, len(photos), nil)})
		var transcript []map[string]any
		_ = json.Unmarshal(initial, &transcript)
		var cost float64
		runner := &agentRunner{
			model: s.model, toolbox: newAgentToolbox(evalData{agentData: s.data, hiddenID: decision.ID}), system: tnvedAgentPrompt(s.settings.NamingRule),
			transcript: transcript, photos: photos, vision: s.model.Vision(),
			emit:   func(string, any) {},
			onStep: func(_ []map[string]any, usage agentUsage) { cost += s.model.Cost(usage) },
		}
		runCtx, cancel := context.WithTimeout(ctx, agentRunTimeout)
		outcome := runner.execute(runCtx)
		if outcome.status == "waiting" {
			// Ответить на вопрос некому — агент решает по имеющимся данным.
			runner.append(agentMessage{Role: "user", Content: "Сотрудника нет, ответить некому: реши по имеющимся данным и вызови finish."})
			outcome = runner.execute(runCtx)
		}
		cancel()
		summary.Total++
		summary.CostUSD += cost
		verdict := "нет итога"
		switch {
		case outcome.err != nil:
			summary.Errors++
			verdict = "ошибка: " + ai.AIErrorMessage(outcome.err)
		case outcome.result == nil:
			summary.NoResult++
		default:
			best := outcome.result.Best()
			inCandidates := false
			for _, candidate := range outcome.result.Candidates {
				if candidate.Code == decision.Code && candidate.Verdict != "no" {
					inCandidates = true
				}
			}
			if inCandidates {
				summary.CandidateCorrect++
			}
			switch {
			case best != nil && best.Code == decision.Code:
				summary.BestCorrect++
				verdict = "верно"
			case inCandidates:
				verdict = "верный код среди вариантов, лучший — " + FormatTNVEDCode(best.Code)
			case best != nil:
				verdict = "ошибка, предложил " + FormatTNVEDCode(best.Code)
			}
		}
		report(fmt.Sprintf("%3d. %s → %s: %s ($%s)", summary.Total, service.TruncateRunes(firstNonEmpty(item.Name, item.SupplierName), 60),
			FormatTNVEDCode(decision.Code), verdict, strconv.FormatFloat(cost, 'f', 4, 64)))
	}
	summary.Duration = time.Since(started)
	return summary, nil
}

// evalName — наименование, которое получает агент: из инвойса, а если его нет (импорт итоговых
// наименований) — русская часть итогового без обобщающего слова («Ручной инструмент: Отвертка /
// 螺丝刀» → «Отвертка»), чтобы не подсказывать группу ТН ВЭД.
func evalName(decision repository.CodeDecision) string {
	if strings.TrimSpace(decision.InvoiceName) != "" {
		return decision.InvoiceName
	}
	name, _, _ := strings.Cut(decision.FinalName, "/")
	if _, rest, ok := strings.Cut(name, ":"); ok {
		name = rest
	}
	return strings.TrimSpace(name)
}
