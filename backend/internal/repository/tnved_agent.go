package repository

// Подбор кода ТН ВЭД ИИ-агентом: сессии (одна строка сметы), сообщения чата и сохранённые
// страницы кодов TWS.BY.

import (
	"context"
	"errors"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type TNVEDAgentSession struct {
	ID         int64  `gorm:"primaryKey"`
	UserID     int64  `gorm:"not null"`
	EstimateID *int64 `gorm:"column:estimate_id"`
	ItemID     string `gorm:"size:64;not null"`
	// Mode — row (строка сметы) или reference (чат-справочник на главной); Title — первый вопрос.
	Mode  string `gorm:"size:16;not null;default:row"`
	Title string `gorm:"not null"`
	// DocumentsCode — запуск в режиме «Документы для ввоза» для этого кода; "" — обычный подбор.
	DocumentsCode    string          `gorm:"size:10;not null"`
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
	// Доля модели для фото в токенах и стоимости выше (общие поля — сумма обеих моделей).
	VisionPromptTokens     int64           `gorm:"not null"`
	VisionCachedTokens     int64           `gorm:"not null"`
	VisionCompletionTokens int64           `gorm:"not null"`
	VisionCostUSD          decimal.Decimal `gorm:"column:vision_cost_usd;type:numeric(12,6);not null"`
	Model                  string          `gorm:"size:100;not null"`
	// TrademarkHits — вопрос «/троис» справочника: сколько знаков реестра найдено; nil — не проверка ТРОИС.
	TrademarkHits *int `gorm:"column:trademark_hits"`
	// AgentStartedAt — первый запуск агента (после описания фото и поиска в базе): от него считается время до итога.
	AgentStartedAt *time.Time `gorm:"column:agent_started_at"`
	CreatedAt      time.Time  `gorm:"not null"`
	UpdatedAt      time.Time  `gorm:"not null"`
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

// WebPageCache — сохранённая веб-страница для агента (справка tnved.info, акты): текст и ссылки.
type WebPageCache struct {
	URL       string    `gorm:"primaryKey;size:1000"`
	Title     string    `gorm:"not null"`
	Text      string    `gorm:"not null"`
	Links     JSON      `gorm:"type:jsonb;not null"`
	FetchedAt time.Time `gorm:"not null"`
}

func (WebPageCache) TableName() string { return "web_pages" }

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

// AddAgentSessionUsage прибавляет к подбору токены и стоимость отдельного запроса (перевод наименования);
// vision — запрос к модели для фото: прибавляется и к её доле.
func (r *Repository) AddAgentSessionUsage(ctx context.Context, id, promptTokens, cachedTokens, completionTokens int64, cost decimal.Decimal, vision bool) error {
	changes := map[string]any{
		"prompt_tokens": gorm.Expr("prompt_tokens + ?", promptTokens), "cached_tokens": gorm.Expr("cached_tokens + ?", cachedTokens),
		"completion_tokens": gorm.Expr("completion_tokens + ?", completionTokens), "cost_usd": gorm.Expr("cost_usd + ?", cost),
	}
	if vision {
		changes["vision_prompt_tokens"] = gorm.Expr("vision_prompt_tokens + ?", promptTokens)
		changes["vision_cached_tokens"] = gorm.Expr("vision_cached_tokens + ?", cachedTokens)
		changes["vision_completion_tokens"] = gorm.Expr("vision_completion_tokens + ?", completionTokens)
		changes["vision_cost_usd"] = gorm.Expr("vision_cost_usd + ?", cost)
	}
	return r.db.WithContext(ctx).Model(&TNVEDAgentSession{}).Where("id = ?", id).Updates(changes).Error
}

// SetAgentSessionStatusIf меняет статус, только если он сейчас from: два запуска одной сессии
// не идут параллельно. false — статус уже другой.
func (r *Repository) SetAgentSessionStatusIf(ctx context.Context, id int64, from []string, to string) (bool, error) {
	result := r.db.WithContext(ctx).Model(&TNVEDAgentSession{}).Where("id = ? AND status IN ?", id, from).
		Updates(map[string]any{"status": to, "updated_at": time.Now().UTC(), "error": ""})
	return result.RowsAffected > 0, result.Error
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

// CountAgentRunsSince — запуски агента сотрудником: старт после поиска в базе (или сразу —
// input с ai), ответ, «подобрать заново». Один поиск в базе без агента не считается.
func (r *Repository) CountAgentRunsSince(ctx context.Context, userID int64, since time.Time) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Table("tnved_agent_messages AS m").
		Joins("JOIN tnved_agent_sessions s ON s.id = m.session_id").
		Where("s.user_id = ? AND m.created_at >= ? AND (m.kind IN ('start', 'user', 'rerun', 'documents') OR (m.kind = 'input' AND m.content->>'ai' = 'true'))", userID, since).
		Count(&count).Error
	return count, err
}

// ReadyInterruptedSearches — поиск в базе, прерванный перезапуском сервера: агента можно запустить.
func (r *Repository) ReadyInterruptedSearches(ctx context.Context) error {
	return r.db.WithContext(ctx).Model(&TNVEDAgentSession{}).Where("status = 'searching'").
		Updates(map[string]any{"status": "ready", "updated_at": time.Now().UTC()}).Error
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
	// Доля модели для фото в суммах выше.
	VisionPromptTokens     int64
	VisionCachedTokens     int64
	VisionCompletionTokens int64
	VisionCostUSD          decimal.Decimal
}

// AgentUsageSince — расход агента за период (для администратора).
func (r *Repository) AgentUsageSince(ctx context.Context, since time.Time) (AgentUsage, error) {
	var usage AgentUsage
	err := r.db.WithContext(ctx).Raw(`SELECT COUNT(*) AS sessions, COALESCE(SUM(runs), 0) AS runs,
		COALESCE(SUM(prompt_tokens), 0) AS prompt_tokens, COALESCE(SUM(cached_tokens), 0) AS cached_tokens,
		COALESCE(SUM(completion_tokens), 0) AS completion_tokens, COALESCE(SUM(cost_usd), 0) AS cost_usd,
		COALESCE(SUM(vision_prompt_tokens), 0) AS vision_prompt_tokens, COALESCE(SUM(vision_cached_tokens), 0) AS vision_cached_tokens,
		COALESCE(SUM(vision_completion_tokens), 0) AS vision_completion_tokens, COALESCE(SUM(vision_cost_usd), 0) AS vision_cost_usd
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

// WebPage — сохранённая веб-страница; ErrNotFound, если её ещё не скачивали.
func (r *Repository) WebPage(ctx context.Context, url string) (WebPageCache, error) {
	var pages []WebPageCache
	if err := r.db.WithContext(ctx).Where("url = ?", url).Limit(1).Find(&pages).Error; err != nil {
		return WebPageCache{}, err
	}
	if len(pages) == 0 {
		return WebPageCache{}, ErrNotFound
	}
	return pages[0], nil
}

func (r *Repository) SaveWebPage(ctx context.Context, page WebPageCache) error {
	return r.db.WithContext(ctx).Save(&page).Error
}

// ReferenceSession — вопрос чата-справочника для списка прошлых вопросов.
type ReferenceSession struct {
	ID            int64
	Title         string
	Status        string
	Result        JSON
	Photos        JSON
	DocumentsCode string
	TrademarkHits *int
	CreatedAt     time.Time
	UpdatedAt     time.Time
	// SharedBy — имя автора, если вопрос открыли сотруднику (только в SharedReferenceSessions).
	SharedBy string
}

// ReferenceSessions — вопросы сотрудника в чате-справочнике, последние — первыми.
func (r *Repository) ReferenceSessions(ctx context.Context, userID int64, limit int) ([]ReferenceSession, error) {
	var rows []ReferenceSession
	err := r.db.WithContext(ctx).Table("tnved_agent_sessions").Select("id, title, status, result, photos, documents_code, trademark_hits, created_at, updated_at").
		Where("user_id = ? AND mode = 'reference'", userID).Order("updated_at DESC, id DESC").Limit(limit).Scan(&rows).Error
	return rows, err
}

// DeleteAgentSession удаляет сессию агента вместе с сообщениями.
func (r *Repository) DeleteAgentSession(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Where("id = ?", id).Delete(&TNVEDAgentSession{}).Error
}

// CodeDocuments — сохранённый ответ «Документы для ввоза» по коду: описание товара, резюме и меры
// (записи AgentDocument с видом документа). Показывается последний по коду.
type CodeDocuments struct {
	ID          int64     `gorm:"primaryKey"`
	Code        string    `gorm:"size:10;not null"`
	Product     string    `gorm:"not null"`
	Description string    `gorm:"not null"`
	Documents   JSON      `gorm:"type:jsonb;not null"`
	SessionID   *int64    `gorm:"column:session_id"`
	UserID      *int64    `gorm:"column:user_id"`
	CreatedAt   time.Time `gorm:"not null"`
}

func (CodeDocuments) TableName() string { return "tnved_code_documents" }

func (r *Repository) SaveCodeDocuments(ctx context.Context, row CodeDocuments) error {
	return r.db.WithContext(ctx).Create(&row).Error
}

// LatestCodeDocuments — последний сохранённый ответ по коду; ErrNotFound — ответа ещё не было.
func (r *Repository) LatestCodeDocuments(ctx context.Context, code string) (CodeDocuments, error) {
	var rows []CodeDocuments
	if err := r.db.WithContext(ctx).Where("code = ?", code).Order("created_at DESC, id DESC").Limit(1).Find(&rows).Error; err != nil {
		return CodeDocuments{}, err
	}
	if len(rows) == 0 {
		return CodeDocuments{}, ErrNotFound
	}
	return rows[0], nil
}

// GeneralAct — перечень СС/ДС общей базы «Документов для ввоза», добавленный из справки tnved.info.
type GeneralAct struct {
	URL     string    `gorm:"primaryKey;size:1000"`
	Title   string    `gorm:"not null"`
	AddedAt time.Time `gorm:"not null"`
}

func (GeneralAct) TableName() string { return "general_acts" }

// SaveGeneralActs — новые перечни; уже известные адреса не меняются.
func (r *Repository) SaveGeneralActs(ctx context.Context, acts []GeneralAct) error {
	if len(acts) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&acts).Error
}

// GeneralActs — все перечни, добавленные из справок, в порядке добавления.
func (r *Repository) GeneralActs(ctx context.Context) ([]GeneralAct, error) {
	var acts []GeneralAct
	return acts, r.db.WithContext(ctx).Order("added_at, url").Find(&acts).Error
}

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

// UpdateAgentMessageContent — содержимое сообщения чата (итог «Документов для ввоза» после второго мнения).
func (r *Repository) UpdateAgentMessageContent(ctx context.Context, id int64, content JSON) error {
	return r.db.WithContext(ctx).Model(&TNVEDAgentMessage{}).Where("id = ?", id).Update("content", content).Error
}

// MarkAgentStarted — момент первого запуска агента по сессии (повторные ходы его не сдвигают).
func (r *Repository) MarkAgentStarted(ctx context.Context, id int64) error {
	return r.db.WithContext(ctx).Model(&TNVEDAgentSession{}).Where("id = ? AND agent_started_at IS NULL", id).Update("agent_started_at", time.Now().UTC()).Error
}
