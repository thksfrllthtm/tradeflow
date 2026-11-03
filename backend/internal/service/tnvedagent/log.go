package tnvedagent

// Журнал агента для администратора: все подборы всех сотрудников, рядом итог теневой модели и запись
// базы решений — согласны ли модели и кто из них оказался прав после проверки администратором. Карточка
// подбора — обе ленты событий и полные переписки с моделями.

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
)

type AgentLogFilter = repository.AgentLogFilter

// AgentLogModelView — итог одной модели в строке журнала.
type AgentLogModelView struct {
	Status   string   `json:"status"`
	Model    string   `json:"model"`
	Error    string   `json:"error,omitempty"`
	BestCode string   `json:"bestCode"`
	CostUSD  string   `json:"costUsd"`
	Steps    int      `json:"steps"`
	Seconds  *float64 `json:"seconds"`
}

// AgentLogDecisionView — запись базы решений по подбору: что применил сотрудник и что подтвердил администратор.
type AgentLogDecisionView struct {
	ID           int64  `json:"id"`
	Code         string `json:"code"`
	Status       string `json:"status"`
	AgentCorrect *bool  `json:"agentCorrect"`
}

type AgentLogRowView struct {
	ID         int64                 `json:"id"`
	UserID     int64                 `json:"userId"`
	User       string                `json:"user"`
	EstimateID *int64                `json:"estimateId"`
	ItemID     string                `json:"itemId"`
	Mode       string                `json:"mode"`
	Name       string                `json:"name"`
	CreatedAt  time.Time             `json:"createdAt"`
	Runs       int                   `json:"runs"`
	Main       AgentLogModelView     `json:"main"`
	Shadow     *AgentLogModelView    `json:"shadow"`
	Decision   *AgentLogDecisionView `json:"decision"`
	// Agree — у обеих моделей есть лучший код и он один; MainCorrect и ShadowCorrect — по проверенной записи;
	// DocumentsAgree — сошлись ли модели по «Документам для ввоза» (nil — не сравнивалось).
	Agree          *bool `json:"agree"`
	MainCorrect    *bool `json:"mainCorrect"`
	ShadowCorrect  *bool `json:"shadowCorrect"`
	DocumentsAgree *bool `json:"documentsAgree"`
}

type AgentLogSummaryView struct {
	Total          int64 `json:"total"`
	WithResult     int64 `json:"withResult"`
	ShadowResults  int64 `json:"shadowResults"`
	Agree          int64 `json:"agree"`
	Disagree       int64 `json:"disagree"`
	Verified       int64 `json:"verified"`
	MainVerified   int64 `json:"mainVerified"`
	MainCorrect    int64 `json:"mainCorrect"`
	ShadowVerified int64 `json:"shadowVerified"`
	ShadowCorrect  int64 `json:"shadowCorrect"`
	// Документы для ввоза: сколько раз сравнивали ответы моделей и сколько раз они сошлись.
	DocumentsCompared int64   `json:"documentsCompared"`
	DocumentsAgree    int64   `json:"documentsAgree"`
	CostUSD           string  `json:"costUsd"`
	ShadowCostUSD     string  `json:"shadowCostUsd"`
	Seconds           float64 `json:"seconds"`
	ShadowSeconds     float64 `json:"shadowSeconds"`
}

// AgentShadowStatus — настройка тени и её расход против бюджета.
type AgentShadowStatus struct {
	Enabled    bool   `json:"enabled"`
	Model      string `json:"model"`
	Vision     bool   `json:"vision"`
	BudgetUSD  string `json:"budgetUsd"`
	SpentUSD   string `json:"spentUsd"`
	DailyLimit int    `json:"dailyLimit"`
	Today      int64  `json:"today"`
}

type AgentLogPage struct {
	Rows    []AgentLogRowView   `json:"rows"`
	Summary AgentLogSummaryView `json:"summary"`
	Shadow  AgentShadowStatus   `json:"shadow"`
}

// AgentShadowRunView — теневой подбор целиком: лента событий (как сообщения чата), итог и переписка с моделью.
type AgentShadowRunView struct {
	Status     string          `json:"status"`
	Model      string          `json:"model"`
	Error      string          `json:"error,omitempty"`
	Result     *AgentResult    `json:"result"`
	Messages   json.RawMessage `json:"messages"`
	Transcript json.RawMessage `json:"transcript"`
	Steps      int             `json:"steps"`
	CostUSD    string          `json:"costUsd"`
	Tokens     int64           `json:"tokens"`
	StartedAt  time.Time       `json:"startedAt"`
	FinishedAt *time.Time      `json:"finishedAt"`
}

// AgentLogEntry — карточка подбора: строка журнала, чат сотрудника с фото, переписка основной модели и тень.
type AgentLogEntry struct {
	Row        AgentLogRowView     `json:"row"`
	Session    AgentSessionView    `json:"session"`
	Transcript json.RawMessage     `json:"transcript"`
	Shadow     *AgentShadowRunView `json:"shadow"`
}

// Log — страница журнала со сводкой по всей выборке и состоянием тени.
func (s *TNVEDAgentService) Log(ctx context.Context, filter AgentLogFilter) (AgentLogPage, error) {
	rows, summary, err := s.repository.AgentLog(ctx, filter)
	if err != nil {
		return AgentLogPage{}, err
	}
	page := AgentLogPage{Rows: make([]AgentLogRowView, 0, len(rows)), Summary: AgentLogSummaryView{
		Total: summary.Total, WithResult: summary.WithResult, ShadowResults: summary.ShadowResults, Agree: summary.Agree, Disagree: summary.Disagree,
		Verified: summary.Verified, MainVerified: summary.MainVerified, MainCorrect: summary.MainCorrect, ShadowVerified: summary.ShadowVerified, ShadowCorrect: summary.ShadowCorrect,
		DocumentsCompared: summary.DocumentsCompared, DocumentsAgree: summary.DocumentsAgree,
		CostUSD: summary.CostUSD.StringFixed(4), ShadowCostUSD: summary.ShadowCostUSD.StringFixed(4), Seconds: summary.Seconds, ShadowSeconds: summary.ShadowSeconds,
	}}
	for _, row := range rows {
		page.Rows = append(page.Rows, logRowView(row))
	}
	page.Shadow, err = s.shadowStatus(ctx)
	return page, err
}

func (s *TNVEDAgentService) shadowStatus(ctx context.Context) (AgentShadowStatus, error) {
	if s.shadow == nil {
		return AgentShadowStatus{}, nil
	}
	spent, today, err := s.repository.ShadowSpend(ctx, dayStart(time.Now()))
	return AgentShadowStatus{
		Enabled: true, Model: s.shadow.Name(), Vision: s.shadow.Vision(), BudgetUSD: s.shadowSettings.BudgetUSD.StringFixed(2),
		SpentUSD: spent.StringFixed(2), DailyLimit: s.shadowSettings.DailyLimit, Today: today,
	}, err
}

func logRowView(row repository.AgentLogRow) AgentLogRowView {
	view := AgentLogRowView{
		ID: row.ID, UserID: row.UserID, User: firstNonEmpty(row.UserName, row.UserEmail), EstimateID: row.EstimateID, ItemID: row.ItemID, Mode: row.Mode,
		Name: firstNonEmpty(row.ItemName, row.SupplierName, row.Title), CreatedAt: row.CreatedAt, Runs: row.Runs,
		Main: AgentLogModelView{Status: row.Status, Model: row.Model, Error: row.Error, BestCode: row.BestCode, CostUSD: row.CostUSD.StringFixed(4), Steps: row.Steps, Seconds: seconds(row.StartedAt, row.FinishedAt)},
	}
	if row.ShadowID != nil {
		view.Shadow = &AgentLogModelView{Status: row.ShadowStatus, Model: row.ShadowModel, Error: row.ShadowError, BestCode: row.ShadowBestCode,
			CostUSD: row.ShadowCostUSD.StringFixed(4), Steps: row.ShadowSteps, Seconds: seconds(row.ShadowStartedAt, row.ShadowFinishedAt)}
	}
	if row.DecisionID != nil {
		view.Decision = &AgentLogDecisionView{ID: *row.DecisionID, Code: row.DecisionCode, Status: row.DecisionStatus, AgentCorrect: row.DecisionAgentCorrect}
	}
	view.Agree, view.MainCorrect, view.ShadowCorrect = compareCodes(row.BestCode, row.ShadowBestCode, row.DecisionCode, row.DecisionStatus == "verified")
	view.DocumentsAgree = row.ShadowDocumentsAgree
	return view
}

// compareCodes — согласие моделей (у обеих есть лучший код) и правота каждой по проверенному коду.
func compareCodes(main, shadow, verifiedCode string, verified bool) (agree, mainCorrect, shadowCorrect *bool) {
	flag := func(value bool) *bool { return &value }
	if main != "" && shadow != "" {
		agree = flag(main == shadow)
	}
	if verified && main != "" {
		mainCorrect = flag(main == verifiedCode)
	}
	if verified && shadow != "" {
		shadowCorrect = flag(shadow == verifiedCode)
	}
	return agree, mainCorrect, shadowCorrect
}

func seconds(started, finished *time.Time) *float64 {
	if started == nil || finished == nil || finished.Before(*started) {
		return nil
	}
	value := finished.Sub(*started).Seconds()
	return &value
}

// LogEntry — карточка подбора для администратора.
func (s *TNVEDAgentService) LogEntry(ctx context.Context, user repository.User, id int64) (AgentLogEntry, error) {
	rows, _, err := s.repository.AgentLog(ctx, AgentLogFilter{ID: id, Limit: 1})
	if err != nil {
		return AgentLogEntry{}, err
	}
	if len(rows) == 0 {
		return AgentLogEntry{}, repository.ErrNotFound
	}
	session, err := s.Get(ctx, user, id, 0)
	if err != nil {
		return AgentLogEntry{}, err
	}
	stored, err := s.repository.AgentSessionByID(ctx, id)
	if err != nil {
		return AgentLogEntry{}, err
	}
	entry := AgentLogEntry{Row: logRowView(rows[0]), Session: session, Transcript: json.RawMessage(stored.Transcript)}
	shadow, err := s.repository.ShadowRunBySession(ctx, id)
	if errors.Is(err, repository.ErrNotFound) {
		return entry, nil
	}
	if err != nil {
		return AgentLogEntry{}, err
	}
	entry.Shadow = &AgentShadowRunView{
		Status: shadow.Status, Model: shadow.Model, Error: shadow.Error, Messages: json.RawMessage(shadow.Messages), Transcript: json.RawMessage(shadow.Transcript),
		Steps: shadow.Steps, CostUSD: shadow.CostUSD.StringFixed(4), Tokens: shadow.PromptTokens + shadow.CompletionTokens, StartedAt: shadow.StartedAt, FinishedAt: shadow.FinishedAt,
	}
	var result AgentResult
	if len(shadow.Result) > 0 && string(shadow.Result) != "null" && json.Unmarshal(shadow.Result, &result) == nil {
		entry.Shadow.Result = &result
	}
	return entry, nil
}
