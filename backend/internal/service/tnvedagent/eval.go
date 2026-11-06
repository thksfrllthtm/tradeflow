package tnvedagent

// Проверка точности агента. Два режима (команда cmd/tnved-agent-eval, ничего не сохраняет, расходует
// токены моделей и запросы Tavily):
//   - Evaluate — последние проверенные записи базы решений: агент подбирает код, не видя самой записи;
//   - EvaluateItems — товары из Excel поставщика с правильными кодами: от агента прячутся записи базы
//     с тем же фото или наименованием, чтобы он не подсмотрел ответ.
// По каждому товару — подробная запись (итог, описание фото, ход подбора, стоимость, время) для отчёта.

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/ai"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/decisions"
	"github.com/shopspring/decimal"
)

type AgentEvalSummary struct {
	Total int
	// Лучший код агента совпал с проверенным; проверенный код есть среди «подходит/возможно».
	BestCorrect      int
	CandidateCorrect int
	NoResult         int
	Errors           int
	CostUSD          decimal.Decimal
	Duration         time.Duration
}

// AgentEvalItem — товар для прогона: наименования, фото и правильный код; Disputed — правильный код спорный.
type AgentEvalItem struct {
	Label    string
	Item     AgentItemInput
	Photos   []string
	Code     string
	Disputed bool
	hidden   map[int64]bool
}

// AgentEvalRecord — итог прогона по одному товару. Verdict: «верно», «среди вариантов», «ошибка»,
// «нет итога», «сбой».
type AgentEvalRecord struct {
	Label            string          `json:"label"`
	Name             string          `json:"name"`
	Code             string          `json:"code"`
	Disputed         bool            `json:"disputed,omitempty"`
	Verdict          string          `json:"verdict"`
	BestCode         string          `json:"bestCode,omitempty"`
	Result           *AgentResult    `json:"result,omitempty"`
	PhotoNote        string          `json:"photoNote,omitempty"`
	Steps            []string        `json:"steps"`
	Thoughts         []string        `json:"thoughts,omitempty"`
	Error            string          `json:"error,omitempty"`
	CostUSD          decimal.Decimal `json:"costUsd"`
	VisionCostUSD    decimal.Decimal `json:"visionCostUsd"`
	PromptTokens     int64           `json:"promptTokens"`
	CachedTokens     int64           `json:"cachedTokens"`
	CompletionTokens int64           `json:"completionTokens"`
	Seconds          float64         `json:"seconds"`
}

// Outcome — вердикт (при ошибке — с лучшим кодом агента), стоимость и время: строка отчёта и ячейка сравнения схем.
func (r AgentEvalRecord) Outcome() string {
	verdict := r.Verdict
	switch {
	case r.Error != "":
		verdict = "сбой: " + r.Error
	case r.Verdict == "ошибка" || r.Verdict == "среди вариантов":
		verdict += ", лучший — " + FormatTNVEDCode(r.BestCode)
	}
	return fmt.Sprintf("%s ($%s, %.0fs)", verdict, r.CostUSD.StringFixed(4), r.Seconds)
}

func (r AgentEvalRecord) line(number int) string {
	return fmt.Sprintf("%3d. %s → %s: %s", number, service.TruncateRunes(r.Name, 60), FormatTNVEDCode(r.Code), r.Outcome())
}

func (summary *AgentEvalSummary) add(record AgentEvalRecord) {
	summary.Total++
	summary.CostUSD = summary.CostUSD.Add(record.CostUSD)
	switch record.Verdict {
	case "верно":
		summary.BestCorrect++
		summary.CandidateCorrect++
	case "среди вариантов":
		summary.CandidateCorrect++
	case "нет итога":
		summary.NoResult++
	case "сбой":
		summary.Errors++
	}
}

// evalData — данные агента без скрытых записей базы решений (иначе он найдёт готовый ответ).
type evalData struct {
	agentData
	hidden map[int64]bool
}

func (d evalData) SearchDecisions(ctx context.Context, query string, limit int) ([]repository.CodeDecision, error) {
	rows, err := d.agentData.SearchDecisions(ctx, query, limit+len(d.hidden))
	if err != nil {
		return nil, err
	}
	kept := rows[:0]
	for _, row := range rows {
		if !d.hidden[row.ID] && len(kept) < limit {
			kept = append(kept, row)
		}
	}
	return kept, nil
}

// Evaluate прогоняет агента на последних проверенных записях — по одной на код, чтобы товары были разные
// (13 одинаковых перчаток мерят одно и то же). Записи о том же товаре (по наименованию и фото) от
// агента скрыты, иначе он берёт готовый ответ из базы.
func (s *TNVEDAgentService) Evaluate(ctx context.Context, limit int, report func(string)) (AgentEvalSummary, error) {
	summary, _, err := s.EvaluateDecisions(ctx, limit, report)
	return summary, err
}

// EvaluateDecisions — Evaluate с подробными записями по каждому товару (для отчёта).
func (s *TNVEDAgentService) EvaluateDecisions(ctx context.Context, limit int, report func(string)) (AgentEvalSummary, []AgentEvalRecord, error) {
	if !s.model.Configured() {
		return AgentEvalSummary{}, nil, ai.ErrAINotConfigured
	}
	rows, _, err := s.repository.ListCodeDecisions(ctx, repository.CodeDecisionFilter{Status: "verified", Limit: min(limit*20, 1000)})
	if err != nil {
		return AgentEvalSummary{}, nil, err
	}
	summary, records := AgentEvalSummary{}, []AgentEvalRecord{}
	started := time.Now()
	codes := map[string]bool{}
	for _, listed := range rows {
		if summary.Total == limit {
			break
		}
		if codes[listed.Code] {
			continue
		}
		codes[listed.Code] = true
		decision, err := s.repository.CodeDecisionByID(ctx, listed.ID)
		if err != nil {
			return summary, records, err
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
		hidden, err := s.sameDecisions(ctx, item, photos)
		if err != nil {
			return summary, records, err
		}
		hidden[decision.ID] = true
		record := s.evalOne(ctx, AgentEvalItem{Label: "запись " + fmt.Sprint(decision.ID), Item: item, Photos: photos, Code: decision.Code, hidden: hidden})
		summary.add(record)
		records = append(records, record)
		report(record.line(summary.Total))
	}
	summary.Duration = time.Since(started)
	return summary, records, nil
}

// sameDecisions — записи базы о том же товаре (по наименованию и фото): от агента их прячут.
func (s *TNVEDAgentService) sameDecisions(ctx context.Context, item AgentItemInput, photos []string) (map[int64]bool, error) {
	hidden := map[int64]bool{}
	if s.decisions == nil {
		return hidden, nil
	}
	hashes := []string{}
	for _, photo := range photos {
		if hash := decisions.PhotoHash(photo); hash != "" {
			hashes = append(hashes, hash)
		}
	}
	same, err := s.decisions.FindSame(ctx, item.names(), hashes)
	for _, match := range same {
		hidden[match.Decision.ID] = true
	}
	return hidden, err
}

// EvaluateItems прогоняет агента на товарах из файла поставщика. Записи базы о том же товаре (по фото и
// наименованию) от агента скрыты.
func (s *TNVEDAgentService) EvaluateItems(ctx context.Context, items []AgentEvalItem, report func(string)) (AgentEvalSummary, []AgentEvalRecord, error) {
	if !s.model.Configured() {
		return AgentEvalSummary{}, nil, ai.ErrAINotConfigured
	}
	summary, records := AgentEvalSummary{}, []AgentEvalRecord{}
	started := time.Now()
	for _, item := range items {
		hidden, err := s.sameDecisions(ctx, item.Item, item.Photos)
		if err != nil {
			return summary, records, err
		}
		item.hidden = hidden
		record := s.evalOne(ctx, item)
		summary.add(record)
		records = append(records, record)
		report(record.line(summary.Total))
	}
	summary.Duration = time.Since(started)
	return summary, records, nil
}

// evalOne — один подбор как в программе (описание фото, вопросы по фото, интернет и
// декларации); вопрос сотруднику — ответа нет, агент решает сам.
func (s *TNVEDAgentService) evalOne(ctx context.Context, item AgentEvalItem) AgentEvalRecord {
	record := AgentEvalRecord{Label: item.Label, Name: firstNonEmpty(item.Item.Name, item.Item.SupplierName), Code: item.Code, Disputed: item.Disputed, Steps: []string{},
		CostUSD: decimal.Zero, VisionCostUSD: decimal.Zero}
	started := time.Now()
	emit := func(kind string, content any) {
		switch value := content.(type) {
		case agentToolEvent:
			step := value.Label
			if value.Detail != "" {
				step += " — " + service.TruncateRunes(value.Detail, 400)
			}
			if value.Failed {
				step += " (не удалось)"
			}
			record.Steps = append(record.Steps, step)
		case map[string]string:
			if kind == "text" || kind == "notice" || kind == "question" {
				record.Thoughts = append(record.Thoughts, kind+": "+value["text"])
			}
		}
	}
	spentVision := func(usage agentUsage) {
		cost := s.visionModel.Cost(usage)
		record.CostUSD, record.VisionCostUSD = record.CostUSD.Add(cost), record.VisionCostUSD.Add(cost)
	}
	runCtx, cancel := context.WithTimeout(ctx, agentRunTimeout)
	defer cancel()
	initial := []map[string]any{initialAgentMessage(item.Item, len(item.Photos), nil)}
	if note, usage := s.photoNote(runCtx, item.Photos, item.Item.names(), initial, emit); note != nil || usage != (agentUsage{}) {
		spentVision(usage)
		if note != nil {
			initial = append(initial, note)
			record.PhotoNote, _ = note["content"].(string)
		}
	}
	raw, _ := json.Marshal(initial)
	var transcript []map[string]any
	_ = json.Unmarshal(raw, &transcript)
	toolbox := newAgentToolbox(evalData{agentData: s.data, hidden: item.hidden})
	toolbox.search = s.search
	toolbox.lookPhoto = s.photoQuestions(item.Photos, spentVision)
	toolbox.requireDeclarations = true
	toolbox.fileCode = item.Item.Code
	runner := &agentRunner{
		model: s.model, toolbox: toolbox, system: s.systemPrompt(false, toolbox.lookPhoto != nil),
		transcript: transcript, photos: item.Photos, vision: s.model.Vision(), emit: emit,
		onStep: func(_ []map[string]any, usage agentUsage) {
			record.CostUSD = record.CostUSD.Add(s.model.Cost(usage))
			record.PromptTokens += usage.PromptTokens
			record.CachedTokens += usage.CachedTokens
			record.CompletionTokens += usage.CompletionTokens
		},
	}
	outcome := runner.execute(runCtx)
	if outcome.status == "waiting" {
		// Ответить на вопрос некому — агент решает по имеющимся данным.
		runner.append(agentMessage{Role: "user", Content: "Сотрудника нет, ответить некому: реши по имеющимся данным и вызови finish."})
		outcome = runner.execute(runCtx)
	}
	record.Seconds = time.Since(started).Seconds()
	record.Result = outcome.result
	switch {
	case outcome.err != nil:
		record.Verdict, record.Error = "сбой", ai.AIErrorMessage(outcome.err)
	case outcome.result == nil:
		record.Verdict = "нет итога"
	default:
		record.Verdict = "ошибка"
		if best := outcome.result.Best(); best != nil {
			record.BestCode = best.Code
		}
		for _, candidate := range outcome.result.Candidates {
			if candidate.Code == item.Code && candidate.Verdict != "no" {
				record.Verdict = "среди вариантов"
			}
		}
		if record.BestCode == item.Code {
			record.Verdict = "верно"
		}
	}
	return record
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
