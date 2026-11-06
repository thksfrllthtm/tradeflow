package tnvedagent

import (
	"strings"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service/directory"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/trademarks"
)

// AgentCandidate — код из итога агента после проверки программой.
type AgentCandidate struct {
	Code    string `json:"code"`
	Verdict string `json:"verdict"`
	Quote   string `json:"quote"`
	// page — цитата найдена на открытой странице кода, help — в справке tnved.info или документе по
	// ссылке, name — в наименовании кода; "" — цитаты нет.
	QuoteSource string `json:"quoteSource"`
	// DecisionQuote — второе доказательство после пояснений: дословно из классификационного решения на
	// странице кода; DecisionSource — вид решения и страна («Предварительное · RU - Россия»).
	DecisionQuote  string                      `json:"decisionQuote,omitempty"`
	DecisionSource string                      `json:"decisionSource,omitempty"`
	Reason         string                      `json:"reason"`
	Check          string                      `json:"check"`
	PageOpened     bool                        `json:"pageOpened"`
	Nomenclature   *directory.NomenclatureCode `json:"nomenclature"`
	// Declarations — описания из деклараций под кодом (nil — агент примеры декларирования не искал).
	Declarations *AgentDeclarations `json:"declarations,omitempty"`
}

// AgentDeclarations — поиск ifcg.ru внутри кода по словам агента; Count — сколько записей под кодом
// нашлось в его поиске (0 — код в найденном не встретился).
type AgentDeclarations struct {
	URL   string `json:"url"`
	Query string `json:"query"`
	Count int    `json:"count,omitempty"`
	// Links — по сайту: описания под кодом по словам агента (ifcg.ru), поиск по словам (tks.ru), решения
	// сборника ЕАЭС; count — сколько записей под этим кодом нашлось там.
	Links []AgentDeclarationLink `json:"links,omitempty"`
}

// AgentDeclarationLink — ссылка на один сайт с примерами или решениями.
type AgentDeclarationLink struct {
	Site  string `json:"site"`
	URL   string `json:"url"`
	Count int    `json:"count,omitempty"`
}

type AgentResult struct {
	Description string           `json:"description"`
	Candidates  []AgentCandidate `json:"candidates"`
	Name        string           `json:"name"`
	// NameFull — подробный вариант наименования (только в справочнике; Name там — краткий).
	NameFull string `json:"nameFull,omitempty"`
	Notes    string `json:"notes"`
	// Confidence — уверенность агента в лучшем коде: high, medium, low ("" — не указал).
	Confidence string `json:"confidence,omitempty"`
	// ConfidenceReason — почему такая уверенность: что подтверждает код и что под вопросом, одна фраза.
	ConfidenceReason string `json:"confidenceReason,omitempty"`
	// Sources — страницы из интернета, на которые опирался агент; только адреса, которые он видел.
	Sources []AgentSource `json:"sources,omitempty"`
	// FileCode — код из файла поставщика, если лучший код агента другой: с пошлинами обоих (считает
	// программа, а не модель) — сотрудник видит, во что обходится расхождение.
	FileCode *AgentFileCode `json:"fileCode,omitempty"`
	// Коды, которых нет в номенклатуре, и сколько цитат не нашлось в источниках.
	DroppedCodes  []string `json:"droppedCodes"`
	QuotesRemoved int      `json:"quotesRemoved"`
	// Ответ по кнопке «Документы для ввоза»: для какого кода, меры и сколько чужих ссылок убрано.
	DocumentsCode string          `json:"documentsCode,omitempty"`
	Documents     []AgentDocument `json:"documents,omitempty"`
	LinksRemoved  int             `json:"linksRemoved,omitempty"`
	// Trademarks — знаки ТРОИС в наименованиях и бренде с фото (проверяет программа после итога).
	Trademarks []trademarks.Hit `json:"trademarks,omitempty"`
}

// trademarkSummary — «SONY (похоже), DOVE» для шага в чате.
func trademarkSummary(hits []trademarks.Hit) string {
	names := []string{}
	for _, hit := range hits {
		name := firstNonEmpty(hit.Name, hit.RegNo)
		if hit.Kind == trademarks.KindLookalike {
			name += " (похоже)"
		}
		names = append(names, name)
	}
	return strings.Join(names, ", ")
}

type AgentSource struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// AgentDocument — мера или документ для ввоза. Applies: yes — относится к товару, maybe — зависит от
// признака товара, no — по пояснению под актом мера для других товаров.
type AgentDocument struct {
	// Kind — вид документа: cc — сертификат соответствия, dc — декларация соответствия, sgr — свидетельство
	// о государственной регистрации, conditional — условный выпуск, other — прочие меры.
	Kind string `json:"kind"`
	// UtilFee — у меры утильсбора (РОП): за что платёж по перечню № 689 — «за товар», «за упаковку»,
	// «за товар и упаковку»; "" — не утильсбор или кода нет в перечне.
	UtilFee     string `json:"utilFee,omitempty"`
	Measure     string `json:"measure"`
	Act         string `json:"act"`
	URL         string `json:"url"`
	Requirement string `json:"requirement"`
	Applies     string `json:"applies"`
	// Basis — дословная цитата из справки, акта или страницы кода, на которой основан вывод (проверена
	// программой); NameHint — при каком реальном признаке товара мера не применяется и какое наименование
	// тогда указать в декларации.
	Basis string `json:"basis,omitempty"`
	// BasisSource — где найдена цитата: «справка tnved.info», название акта, «страница кода … (TWS.BY)».
	BasisSource string `json:"basisSource,omitempty"`
	NameHint    string `json:"nameHint,omitempty"`
}

// Best — первый подходящий код (или первый не «не подходит»).
func (r *AgentResult) Best() *AgentCandidate {
	if r == nil {
		return nil
	}
	for index := range r.Candidates {
		if r.Candidates[index].Verdict == "fits" {
			return &r.Candidates[index]
		}
	}
	for index := range r.Candidates {
		if r.Candidates[index].Verdict != "no" {
			return &r.Candidates[index]
		}
	}
	return nil
}

// AgentFileCode — код из файла поставщика рядом с подобранным: пошлины ЕТТ обоих.
type AgentFileCode struct {
	Code      string `json:"code"`
	Found     bool   `json:"found"`
	Duty      string `json:"duty"`
	BestCode  string `json:"bestCode"`
	BestDuty  string `json:"bestDuty"`
	DutyDiffs bool   `json:"dutyDiffers"`
}
