package tnvedagent

// Наименование для декларации: агент предлагает русскую часть, сотрудник может её исправить,
// а перевод на китайский запрашивается только по кнопке «Применить наименование» — с уже
// исправленного текста. Итог — «русское / 中文».

import (
	"context"
	"encoding/json"
	"strings"
	"unicode"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/ai"
	"github.com/shopspring/decimal"
)

const maxRussianNameRunes = 200

func hasHan(value string) bool {
	for _, r := range value {
		if unicode.Is(unicode.Han, r) {
			return true
		}
	}
	return false
}

// russianNamePart — русская часть наименования «русское / 中文» (китайская часть отбрасывается).
func russianNamePart(name string) string {
	name = strings.TrimSpace(name)
	if index := strings.LastIndex(name, "/"); index > 0 && hasHan(name[index:]) {
		name = strings.TrimSpace(name[:index])
	}
	return name
}

// translateName — перевод русской части на китайский; supplierName — подсказка, каким термином
// называют товар.
func (s *TNVEDAgentService) translateName(ctx context.Context, russian, supplierName string) (string, agentUsage, error) {
	callCtx, cancel := context.WithTimeout(ctx, photoCallTimeout)
	defer cancel()
	text := "Наименование: " + russian
	if supplierName = strings.TrimSpace(supplierName); supplierName != "" {
		text += "\nНаименование поставщика (подсказка): " + service.TruncateRunes(supplierName, 300)
	}
	reply, err := s.model.CompleteJSON(callCtx, []agentMessage{{Role: "system", Content: translateNamePrompt}, {Role: "user", Content: text}})
	if err != nil {
		return "", agentUsage{}, err
	}
	var answer struct {
		ZH string `json:"zh"`
	}
	if json.Unmarshal([]byte(unwrapJSON(reply.Content)), &answer) != nil {
		return "", reply.Usage, ai.ErrAIBadResponse
	}
	chinese := service.TruncateRunes(strings.Trim(strings.TrimSpace(answer.ZH), "«»\"/ "), 120)
	if !hasHan(chinese) {
		return "", reply.Usage, ai.ErrAIBadResponse
	}
	return chinese, reply.Usage, nil
}

// AgentNameInput — русская часть наименования, которую сотрудник применяет (возможно, исправленная).
type AgentNameInput struct {
	Name string `json:"name"`
}

// ApplyName переводит русскую часть на китайский и возвращает «русское / 中文». Если код подбора
// уже записан в базу решений и ещё не проверен, наименование записи обновляется.
func (s *TNVEDAgentService) ApplyName(ctx context.Context, user repository.User, id int64, input AgentNameInput) (string, error) {
	session, err := s.ownedSession(ctx, user, id)
	if err != nil {
		return "", err
	}
	russian := service.TruncateRunes(russianNamePart(input.Name), maxRussianNameRunes)
	if russian == "" {
		return "", &service.ValidationError{Field: "name", Message: "Укажите наименование"}
	}
	if !s.model.Configured() {
		return "", ai.ErrAINotConfigured
	}
	var item AgentItemInput
	_ = json.Unmarshal(session.Item, &item)
	chinese, usage, err := s.translateName(ctx, russian, item.SupplierName)
	if usage.PromptTokens > 0 || usage.CompletionTokens > 0 {
		_ = s.repository.AddAgentSessionUsage(ctx, id, usage.PromptTokens, usage.CachedTokens, usage.CompletionTokens, decimal.NewFromFloat(s.model.Cost(usage)).Round(6))
	}
	if err != nil {
		return "", err
	}
	name := russian + " / " + chinese
	if err := s.repository.SetPendingDecisionNameForSession(ctx, id, name); err != nil {
		return "", err
	}
	return name, nil
}
