package repository

// Подбор кода ТН ВЭД ИИ-агентом: сессии (одна строка сметы), сообщения чата и сохранённые
// страницы кодов TWS.BY.

import (
	"context"
	"errors"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
)

type TNVEDAgentSession struct {
	ID               int64           `gorm:"primaryKey"`
	UserID           int64           `gorm:"not null"`
	EstimateID       *int64          `gorm:"column:estimate_id"`
	ItemID           string          `gorm:"size:64;not null"`
	Status           string          `gorm:"size:16;not null"`
	Item             JSON            `gorm:"type:jsonb;not null"`
	Photos           JSON            `gorm:"type:jsonb;not null"`
	Transcript       JSON            `gorm:"type:jsonb;not null"`
	Result           JSON            `gorm:"type:jsonb;not null"`
	Error            string          `gorm:"not null"`
	Steps            int             `gorm:"not null"`
	Runs             int             `gorm:"not null"`
	PromptTokens     int64           `gorm:"not null"`
	CachedTokens     int64           `gorm:"not null"`
	CompletionTokens int64           `gorm:"not null"`
	CostUSD          decimal.Decimal `gorm:"column:cost_usd;type:numeric(12,6);not null"`
	Model            string          `gorm:"size:100;not null"`
	CreatedAt        time.Time       `gorm:"not null"`
	UpdatedAt        time.Time       `gorm:"not null"`
}

func (TNVEDAgentSession) TableName() string { return "tnved_agent_sessions" }

type TNVEDAgentMessage struct {
	ID        int64     `gorm:"primaryKey"`
	SessionID int64     `gorm:"not null"`
	Kind      string    `gorm:"size:16;not null"`
	Content   JSON      `gorm:"type:jsonb;not null"`
	CreatedAt time.Time `gorm:"not null"`
}

func (TNVEDAgentMessage) TableName() string { return "tnved_agent_messages" }

type TNVEDDetailsPage struct {
	Code      string    `gorm:"primaryKey;size:10"`
	URL       string    `gorm:"size:500;not null"`
	Text      string    `gorm:"not null"`
	FetchedAt time.Time `gorm:"not null"`
}

func (TNVEDDetailsPage) TableName() string { return "tnved_details_pages" }

func (r *Repository) CreateAgentSession(ctx context.Context, session TNVEDAgentSession) (TNVEDAgentSession, error) {
	return session, r.db.WithContext(ctx).Create(&session).Error
}

func (r *Repository) AgentSessionByID(ctx context.Context, id int64) (TNVEDAgentSession, error) {
	var session TNVEDAgentSession
	err := r.db.WithContext(ctx).Where("id = ?", id).Take(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return TNVEDAgentSession{}, ErrNotFound
	}
	return session, err
}

// AgentSessionSummary — состояние подбора строки без переписки и фото.
type AgentSessionSummary struct {
	ID        int64
	UserID    int64
	ItemID    string
	Status    string
	Result    JSON
	UpdatedAt time.Time
}

// LatestAgentSessions — последний подбор каждой строки сметы (или одной строки, если itemID задан).
func (r *Repository) LatestAgentSessions(ctx context.Context, estimateID int64, itemID string) ([]AgentSessionSummary, error) {
	query := r.db.WithContext(ctx).Table("tnved_agent_sessions").
		Select("DISTINCT ON (item_id) id, user_id, item_id, status, result, updated_at").
		Where("estimate_id = ?", estimateID)
	if itemID != "" {
		query = query.Where("item_id = ?", itemID)
	}
	var rows []AgentSessionSummary
	err := query.Order("item_id, id DESC").Scan(&rows).Error
	return rows, err
}

func (r *Repository) UpdateAgentSession(ctx context.Context, id int64, changes map[string]any) error {
	changes["updated_at"] = time.Now().UTC()
	return r.db.WithContext(ctx).Model(&TNVEDAgentSession{}).Where("id = ?", id).Updates(changes).Error
}

// AddAgentSessionUsage прибавляет к подбору токены и стоимость отдельного запроса (перевод наименования).
func (r *Repository) AddAgentSessionUsage(ctx context.Context, id, promptTokens, cachedTokens, completionTokens int64, cost decimal.Decimal) error {
	return r.db.WithContext(ctx).Model(&TNVEDAgentSession{}).Where("id = ?", id).Updates(map[string]any{
		"prompt_tokens": gorm.Expr("prompt_tokens + ?", promptTokens), "cached_tokens": gorm.Expr("cached_tokens + ?", cachedTokens),
		"completion_tokens": gorm.Expr("completion_tokens + ?", completionTokens), "cost_usd": gorm.Expr("cost_usd + ?", cost),
	}).Error
}

// SetAgentSessionStatusIf меняет статус, только если он сейчас from: два запуска одной сессии
// не идут параллельно. false — статус уже другой.
func (r *Repository) SetAgentSessionStatusIf(ctx context.Context, id int64, from []string, to string) (bool, error) {
	result := r.db.WithContext(ctx).Model(&TNVEDAgentSession{}).Where("id = ? AND status IN ?", id, from).
		Updates(map[string]any{"status": to, "updated_at": time.Now().UTC(), "error": ""})
	return result.RowsAffected > 0, result.Error
}

func (r *Repository) DeleteAgentSessionsForItem(ctx context.Context, estimateID int64, itemID string) error {
	return r.db.WithContext(ctx).Where("estimate_id = ? AND item_id = ?", estimateID, itemID).Delete(&TNVEDAgentSession{}).Error
}

func (r *Repository) AddAgentMessage(ctx context.Context, sessionID int64, kind string, content JSON) (TNVEDAgentMessage, error) {
	message := TNVEDAgentMessage{SessionID: sessionID, Kind: kind, Content: content, CreatedAt: time.Now().UTC()}
	return message, r.db.WithContext(ctx).Create(&message).Error
}

func (r *Repository) AgentMessagesAfter(ctx context.Context, sessionID, afterID int64) ([]TNVEDAgentMessage, error) {
	var rows []TNVEDAgentMessage
	err := r.db.WithContext(ctx).Where("session_id = ? AND id > ?", sessionID, afterID).Order("id ASC").Find(&rows).Error
	return rows, err
}

// CountAgentRunsSince — запуски агента сотрудником (первый подбор, ответ, «подобрать заново»).
func (r *Repository) CountAgentRunsSince(ctx context.Context, userID int64, since time.Time) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("tnved_agent_messages AS m").
		Joins("JOIN tnved_agent_sessions s ON s.id = m.session_id").
		Where("s.user_id = ? AND m.created_at >= ? AND m.kind IN ('input', 'user', 'rerun')", userID, since).
		Count(&count).Error
	return count, err
}

// FailRunningAgentSessions — после перезапуска сервера незаконченные подборы помечаются ошибкой.
func (r *Repository) FailRunningAgentSessions(ctx context.Context, message string) ([]int64, error) {
	var ids []int64
	if err := r.db.WithContext(ctx).Table("tnved_agent_sessions").Where("status = 'running'").Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	if len(ids) == 0 {
		return ids, nil
	}
	return ids, r.db.WithContext(ctx).Model(&TNVEDAgentSession{}).Where("id IN ?", ids).
		Updates(map[string]any{"status": "error", "error": message, "updated_at": time.Now().UTC()}).Error
}

type AgentUsage struct {
	Runs             int64
	Sessions         int64
	PromptTokens     int64
	CachedTokens     int64
	CompletionTokens int64
	CostUSD          decimal.Decimal
}

// AgentUsageSince — расход агента за период (для администратора).
func (r *Repository) AgentUsageSince(ctx context.Context, since time.Time) (AgentUsage, error) {
	var usage AgentUsage
	err := r.db.WithContext(ctx).Raw(`SELECT COUNT(*) AS sessions, COALESCE(SUM(runs), 0) AS runs,
		COALESCE(SUM(prompt_tokens), 0) AS prompt_tokens, COALESCE(SUM(cached_tokens), 0) AS cached_tokens,
		COALESCE(SUM(completion_tokens), 0) AS completion_tokens, COALESCE(SUM(cost_usd), 0) AS cost_usd
		FROM tnved_agent_sessions WHERE created_at >= ?`, since).Scan(&usage).Error
	return usage, err
}

// DetailsPage — сохранённая страница кода; ErrNotFound, если её ещё не скачивали (Find, а не Take:
// отсутствие страницы — обычный случай, и GORM не пишет его в лог как ошибку).
func (r *Repository) DetailsPage(ctx context.Context, code string) (TNVEDDetailsPage, error) {
	var pages []TNVEDDetailsPage
	if err := r.db.WithContext(ctx).Where("code = ?", code).Limit(1).Find(&pages).Error; err != nil {
		return TNVEDDetailsPage{}, err
	}
	if len(pages) == 0 {
		return TNVEDDetailsPage{}, ErrNotFound
	}
	return pages[0], nil
}

func (r *Repository) DetailsPages(ctx context.Context, codes []string) ([]TNVEDDetailsPage, error) {
	var pages []TNVEDDetailsPage
	if len(codes) == 0 {
		return pages, nil
	}
	return pages, r.db.WithContext(ctx).Where("code IN ?", codes).Find(&pages).Error
}

func (r *Repository) SaveDetailsPage(ctx context.Context, page TNVEDDetailsPage) error {
	return r.db.WithContext(ctx).Save(&page).Error
}
