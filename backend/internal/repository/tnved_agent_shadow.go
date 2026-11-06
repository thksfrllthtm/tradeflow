package repository

// Теневой подбор второй моделью: одна строка на сессию агента, расход — отдельно от сессии.

import (
	"context"
	"errors"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

// TNVEDAgentShadowRun — теневой подбор второй моделью для сессии (tnvedagent/shadow.go): одна строка на
// сессию. Messages — лента событий, как сообщения чата сотрудника; расход — отдельно от сессии.
type TNVEDAgentShadowRun struct {
	ID               int64           `gorm:"primaryKey"`
	SessionID        int64           `gorm:"not null"`
	Model            string          `gorm:"size:100;not null"`
	Status           string          `gorm:"size:16;not null"`
	Transcript       JSON            `gorm:"type:jsonb;not null"`
	Messages         JSON            `gorm:"type:jsonb;not null"`
	Result           JSON            `gorm:"type:jsonb;not null"`
	Error            string          `gorm:"not null"`
	Steps            int             `gorm:"not null"`
	PromptTokens     int64           `gorm:"not null"`
	CachedTokens     int64           `gorm:"not null"`
	CompletionTokens int64           `gorm:"not null"`
	CostUSD          decimal.Decimal `gorm:"column:cost_usd;type:numeric(12,6);not null"`
	VisionCostUSD    decimal.Decimal `gorm:"column:vision_cost_usd;type:numeric(12,6);not null"`
	StartedAt        time.Time       `gorm:"not null"`
	FinishedAt       *time.Time
	// DocumentsCode и DocumentsAgree — сошлись ли модели по мерам «нужно» в «Документах для ввоза» для кода (nil — не сравнивалось).
	DocumentsCode  string `gorm:"size:10;not null"`
	DocumentsAgree *bool
}

func (TNVEDAgentShadowRun) TableName() string { return "tnved_agent_shadow_runs" }

func (r *Repository) CreateShadowRun(ctx context.Context, run TNVEDAgentShadowRun) (TNVEDAgentShadowRun, error) {
	return run, r.db.WithContext(ctx).Create(&run).Error
}

func (r *Repository) UpdateShadowRun(ctx context.Context, id int64, changes map[string]any) error {
	return r.db.WithContext(ctx).Model(&TNVEDAgentShadowRun{}).Where("id = ?", id).Updates(changes).Error
}

func (r *Repository) ShadowRunBySession(ctx context.Context, sessionID int64) (TNVEDAgentShadowRun, error) {
	var run TNVEDAgentShadowRun
	err := r.db.WithContext(ctx).Where("session_id = ?", sessionID).Take(&run).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return TNVEDAgentShadowRun{}, ErrNotFound
	}
	return run, err
}

// ShadowSpend — расход тени за всё время и прогонов с момента since (пропущенные не считаются).
func (r *Repository) ShadowSpend(ctx context.Context, since time.Time) (decimal.Decimal, int64, error) {
	var row struct {
		Spent decimal.Decimal
		Runs  int64
	}
	err := r.db.WithContext(ctx).Raw(`SELECT COALESCE(SUM(cost_usd), 0) AS spent,
		COUNT(*) FILTER (WHERE started_at >= ? AND status <> 'skipped') AS runs FROM tnved_agent_shadow_runs`, since).Scan(&row).Error
	return row.Spent, row.Runs, err
}

// FailRunningShadowRuns — после перезапуска сервера незаконченные теневые подборы помечаются ошибкой.
func (r *Repository) FailRunningShadowRuns(ctx context.Context, message string) error {
	return r.db.WithContext(ctx).Model(&TNVEDAgentShadowRun{}).Where("status = 'running'").
		Updates(map[string]any{"status": "error", "error": message, "finished_at": time.Now().UTC()}).Error
}

// ClaimShadowRun — тень берётся за новый ход, только если прошлый закончен (done или error): два хода одной
// тени не идут параллельно. false — тень ещё отвечает или была пропущена.
func (r *Repository) ClaimShadowRun(ctx context.Context, id int64) (bool, error) {
	result := r.db.WithContext(ctx).Model(&TNVEDAgentShadowRun{}).Where("id = ? AND status IN ('done', 'error')", id).
		Updates(map[string]any{"status": "running", "error": ""})
	return result.RowsAffected > 0, result.Error
}
