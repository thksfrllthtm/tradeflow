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

// genericWords — обобщающие слова правила наименования и начала кодов, для которых они приняты.
// ponytail: повторяет пункт 3 правила по умолчанию; поменяли слова в TNVED_AI_NAMING_RULE — поправить и
// здесь. «Принадлежности столовые и кухонные», «Кухонные принадлежности» — без проверки: группы шире.
var genericWords = []struct {
	word     string
	prefixes []string
}{
	{"Ручной инструмент", []string{"8201", "8202", "8203", "8204", "8205"}},
	{"Сменная часть для ручного инструмента", []string{"8202", "8207", "6804"}},
	{"Столовые приборы", []string{"8211", "8215"}},
	{"Прибор электронагревательный", []string{"8516"}},
	{"Машины электромеханические бытовые со встроенным электродвигателем", []string{"8509"}},
	{"Провод электрический", []string{"8544"}},
}

func genericWordFits(prefixes []string, code string) bool {
	for _, prefix := range prefixes {
		if strings.HasPrefix(code, prefix) {
			return true
		}
	}
	return false
}

// fixGenericWord — обобщающее слово наименования по коду: не из своих позиций («Ручной инструмент:» у
// триммера 8467) — заменяется словом группы кода или убирается, если у группы его нет.
func fixGenericWord(name, code string) string {
	word, rest, ok := strings.Cut(name, ":")
	rest = strings.TrimSpace(rest)
	if !ok || rest == "" {
		return name
	}
	for _, generic := range genericWords {
		if !strings.EqualFold(strings.TrimSpace(word), generic.word) {
			continue
		}
		if genericWordFits(generic.prefixes, code) {
			return name
		}
		for _, other := range genericWords {
			if genericWordFits(other.prefixes, code) {
				return other.word + ": " + rest
			}
		}
		runes := []rune(rest)
		return strings.ToUpper(string(runes[0])) + string(runes[1:])
	}
	return name
}

// declarationName — русская часть наименования из итога агента не длиннее maxRussianNameRunes: длинное
// обрезается по границе слова (а не посреди слова), без висящей запятой или скобки на конце.
func declarationName(name string) string {
	name = russianNamePart(name)
	runes := []rune(name)
	if len(runes) <= maxRussianNameRunes {
		return name
	}
	cut := string(runes[:maxRussianNameRunes])
	if space := strings.LastIndexAny(cut, "  "); space > 0 {
		cut = cut[:space]
	}
	return strings.TrimRight(cut, " ,;:-–—(/")
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
	reply, err := s.visionModel.CompleteJSON(callCtx, []agentMessage{{Role: "system", Content: translateNamePrompt}, {Role: "user", Content: text}})
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
	if !s.visionModel.Configured() {
		return "", ai.ErrAINotConfigured
	}
	var item AgentItemInput
	_ = json.Unmarshal(session.Item, &item)
	chinese, usage, err := s.translateName(ctx, russian, item.SupplierName)
	if usage.PromptTokens > 0 || usage.CompletionTokens > 0 {
		_ = s.repository.AddAgentSessionUsage(ctx, id, usage.PromptTokens, usage.CachedTokens, usage.CompletionTokens, s.visionModel.Cost(usage).Round(6), true)
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
