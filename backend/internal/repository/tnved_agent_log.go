package repository

// Журнал агента для администратора: все подборы всех сотрудников, рядом итог теневой модели
// (tnved_agent_shadow_runs) и запись базы решений по подбору. Лучший код считается в SQL так же, как
// AgentResult.Best(): первый «подходит», иначе первый не «не подходит». Время основной модели — от запуска
// агента (сообщение start или input) до первого итога, вопроса или ошибки.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// AgentLogFilter — период по created_at, сотрудник (0 — все), режим ("" — все), выборка Only:
// disagree — модели разошлись, verified — есть проверенная запись, wrong — основная ошиблась,
// shadow — есть итог тени, documents — модели разошлись по «Документам для ввоза». ID — одна сессия.
type AgentLogFilter struct {
	From, To time.Time
	UserID   int64
	Mode     string
	Only     string
	ID       int64
	Limit    int
	Offset   int
}

// AgentLogRow — сессия с итогом тени и записью базы решений по ней.
type AgentLogRow struct {
	ID           int64
	UserID       int64
	UserName     string
	UserEmail    string
	EstimateID   *int64
	ItemID       string
	Mode         string
	Title        string
	Status       string
	Error        string
	ItemName     string
	SupplierName string
	Model        string
	CostUSD      decimal.Decimal
	Steps        int
	Runs         int
	CreatedAt    time.Time
	BestCode     string
	StartedAt    *time.Time
	FinishedAt   *time.Time

	ShadowID         *int64
	ShadowStatus     string
	ShadowModel      string
	ShadowError      string
	ShadowCostUSD    decimal.Decimal
	ShadowSteps      int
	ShadowStartedAt  *time.Time
	ShadowFinishedAt *time.Time
	ShadowBestCode   string
	// ShadowDocumentsAgree — сошлись ли модели по «Документам для ввоза» (nil — не сравнивалось).
	ShadowDocumentsAgree *bool

	DecisionID           *int64
	DecisionCode         string
	DecisionStatus       string
	DecisionAgentCorrect *bool
}

// AgentLogSummary — сводка по той же выборке: сколько подборов, согласие моделей, правота среди
// проверенных, расход и среднее время до итога в секундах.
type AgentLogSummary struct {
	Total          int64
	WithResult     int64
	ShadowResults  int64
	Agree          int64
	Disagree       int64
	Verified       int64
	MainVerified   int64
	MainCorrect    int64
	ShadowVerified int64
	ShadowCorrect  int64
	// Документы для ввоза: сколько раз сравнивали ответы моделей и сколько раз они сошлись.
	DocumentsCompared int64
	DocumentsAgree    int64
	CostUSD           decimal.Decimal
	ShadowCostUSD     decimal.Decimal
	Seconds           float64
	ShadowSeconds     float64
}

// agentLogBest — лучший код из итога (jsonb) как в AgentResult.Best().
func agentLogBest(result string) string {
	return fmt.Sprintf(`(SELECT c.value->>'code' FROM jsonb_array_elements(CASE WHEN jsonb_typeof(%s->'candidates') = 'array' THEN %s->'candidates' ELSE '[]'::jsonb END) WITH ORDINALITY c
		WHERE COALESCE(c.value->>'verdict', '') <> 'no' ORDER BY (COALESCE(c.value->>'verdict', '') = 'fits') DESC, c.ordinality LIMIT 1)`, result, result)
}

// agentLogFrom — таблицы и условия журнала; аргументы — для WHERE.
func agentLogFrom(filter AgentLogFilter) (string, []any) {
	from := `FROM tnved_agent_sessions s
		JOIN users u ON u.id = s.user_id
		LEFT JOIN tnved_agent_shadow_runs sh ON sh.session_id = s.id
		LEFT JOIN LATERAL (SELECT id, code, status, agent_correct FROM code_decisions WHERE session_id = s.id ORDER BY id DESC LIMIT 1) d ON true
		LEFT JOIN LATERAL (SELECT ` + agentLogBest("s.result") + ` AS code) sb ON true
		LEFT JOIN LATERAL (SELECT ` + agentLogBest("sh.result") + ` AS code) shb ON true
		LEFT JOIN LATERAL (SELECT MIN(created_at) FILTER (WHERE kind IN ('start', 'input')) AS started,
			MIN(created_at) FILTER (WHERE kind IN ('result', 'question', 'error')) AS finished FROM tnved_agent_messages WHERE session_id = s.id) m ON true`
	where := []string{"s.created_at >= ?", "s.created_at < ?"}
	args := []any{filter.From, filter.To}
	if filter.ID > 0 {
		where, args = []string{"s.id = ?"}, []any{filter.ID}
	}
	if filter.UserID > 0 {
		where, args = append(where, "s.user_id = ?"), append(args, filter.UserID)
	}
	if filter.Mode != "" {
		where, args = append(where, "s.mode = ?"), append(args, filter.Mode)
	}
	switch filter.Only {
	case "disagree":
		where = append(where, "sb.code IS NOT NULL AND shb.code IS NOT NULL AND sb.code <> shb.code")
	case "verified":
		where = append(where, "d.status = 'verified'")
	case "wrong":
		where = append(where, "d.status = 'verified' AND sb.code IS DISTINCT FROM d.code")
	case "shadow":
		where = append(where, "sh.id IS NOT NULL")
	case "documents":
		where = append(where, "sh.documents_agree = false")
	}
	return from + " WHERE " + strings.Join(where, " AND "), args
}

// AgentLog — строки журнала (новые первыми) и сводка по всей выборке.
func (r *Repository) AgentLog(ctx context.Context, filter AgentLogFilter) ([]AgentLogRow, AgentLogSummary, error) {
	from, args := agentLogFrom(filter)
	var rows []AgentLogRow
	err := r.db.WithContext(ctx).Raw(`SELECT s.id, s.user_id, u.name AS user_name, u.email AS user_email, s.estimate_id, s.item_id, s.mode, s.title,
		s.status, s.error, COALESCE(s.item->>'name', '') AS item_name, COALESCE(s.item->>'supplierName', '') AS supplier_name, s.model,
		s.cost_usd, s.steps, s.runs, s.created_at, COALESCE(sb.code, '') AS best_code, m.started AS started_at, m.finished AS finished_at,
		sh.id AS shadow_id, COALESCE(sh.status, '') AS shadow_status, COALESCE(sh.model, '') AS shadow_model, COALESCE(sh.error, '') AS shadow_error,
		COALESCE(sh.cost_usd, 0) AS shadow_cost_usd, COALESCE(sh.steps, 0) AS shadow_steps, sh.started_at AS shadow_started_at,
		sh.finished_at AS shadow_finished_at, COALESCE(shb.code, '') AS shadow_best_code, sh.documents_agree AS shadow_documents_agree,
		d.id AS decision_id, COALESCE(d.code, '') AS decision_code, COALESCE(d.status, '') AS decision_status, d.agent_correct AS decision_agent_correct
		`+from+` ORDER BY s.id DESC LIMIT ? OFFSET ?`, append(args, filter.Limit, filter.Offset)...).Scan(&rows).Error
	if err != nil {
		return nil, AgentLogSummary{}, err
	}
	var summary AgentLogSummary
	err = r.db.WithContext(ctx).Raw(`SELECT COUNT(*) AS total,
		COUNT(*) FILTER (WHERE sb.code IS NOT NULL) AS with_result,
		COUNT(*) FILTER (WHERE shb.code IS NOT NULL) AS shadow_results,
		COUNT(*) FILTER (WHERE sb.code IS NOT NULL AND sb.code = shb.code) AS agree,
		COUNT(*) FILTER (WHERE sb.code IS NOT NULL AND shb.code IS NOT NULL AND sb.code <> shb.code) AS disagree,
		COUNT(*) FILTER (WHERE d.status = 'verified') AS verified,
		COUNT(*) FILTER (WHERE d.status = 'verified' AND sb.code IS NOT NULL) AS main_verified,
		COUNT(*) FILTER (WHERE d.status = 'verified' AND sb.code = d.code) AS main_correct,
		COUNT(*) FILTER (WHERE d.status = 'verified' AND shb.code IS NOT NULL) AS shadow_verified,
		COUNT(*) FILTER (WHERE d.status = 'verified' AND shb.code = d.code) AS shadow_correct,
		COUNT(*) FILTER (WHERE sh.documents_agree IS NOT NULL) AS documents_compared,
		COUNT(*) FILTER (WHERE sh.documents_agree) AS documents_agree,
		COALESCE(SUM(s.cost_usd), 0) AS cost_usd, COALESCE(SUM(sh.cost_usd), 0) AS shadow_cost_usd,
		COALESCE(AVG(EXTRACT(EPOCH FROM (m.finished - m.started))::float8) FILTER (WHERE m.finished IS NOT NULL), 0) AS seconds,
		COALESCE(AVG(EXTRACT(EPOCH FROM (sh.finished_at - sh.started_at))::float8) FILTER (WHERE sh.status = 'done'), 0) AS shadow_seconds
		`+from, args...).Scan(&summary).Error
	return rows, summary, err
}

// ArchiveAgentSessionsForItem — прежние подборы строки остаются в журнале, но без фото (они занимают
// больше всего места; фото применённого кода хранит база решений).
func (r *Repository) ArchiveAgentSessionsForItem(ctx context.Context, estimateID int64, itemID string) error {
	return r.db.WithContext(ctx).Model(&TNVEDAgentSession{}).Where("estimate_id = ? AND item_id = ?", estimateID, itemID).
		Updates(map[string]any{"photos": JSON("[]"), "updated_at": time.Now().UTC()}).Error
}
