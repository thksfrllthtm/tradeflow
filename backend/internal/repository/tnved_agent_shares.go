package repository

import (
	"context"
	"time"

	"gorm.io/gorm/clause"
)

// TNVEDAgentSessionShare — вопрос чата на главной, открытый другому сотруднику (миграция 000053).
type TNVEDAgentSessionShare struct {
	SessionID int64 `gorm:"primaryKey"`
	UserID    int64 `gorm:"primaryKey"`
	SharedBy  *int64
	CreatedAt time.Time
}

func (TNVEDAgentSessionShare) TableName() string { return "tnved_agent_session_shares" }

// AgentShareUser — сотрудник в списке «Поделиться»: только номер и имя, без почты и роли.
type AgentShareUser struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// ShareUsers — кому можно открыть вопрос: работающие сотрудники с именем, кроме самого себя.
func (r *Repository) ShareUsers(ctx context.Context, excludeID int64) ([]AgentShareUser, error) {
	users := []AgentShareUser{}
	err := r.db.WithContext(ctx).Table("users").Select("id, name").
		Where("disabled_at IS NULL AND id <> ? AND name <> ''", excludeID).Order("name, id").Scan(&users).Error
	return users, err
}

// ExistingShareUsers — из номеров userIDs те, кому можно открыть вопрос (работающие сотрудники).
func (r *Repository) ExistingShareUsers(ctx context.Context, userIDs []int64) ([]int64, error) {
	var ids []int64
	if len(userIDs) == 0 {
		return ids, nil
	}
	err := r.db.WithContext(ctx).Table("users").Where("disabled_at IS NULL AND id IN ?", userIDs).Pluck("id", &ids).Error
	return ids, err
}

// ShareAgentSession открывает вопрос сотрудникам; уже открытым — без изменений.
func (r *Repository) ShareAgentSession(ctx context.Context, sessionID, sharedBy int64, userIDs []int64) error {
	if len(userIDs) == 0 {
		return nil
	}
	rows := make([]TNVEDAgentSessionShare, 0, len(userIDs))
	for _, userID := range userIDs {
		rows = append(rows, TNVEDAgentSessionShare{SessionID: sessionID, UserID: userID, SharedBy: &sharedBy, CreatedAt: time.Now()})
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&rows).Error
}

// UnshareAgentSession закрывает вопрос сотруднику.
func (r *Repository) UnshareAgentSession(ctx context.Context, sessionID, userID int64) error {
	return r.db.WithContext(ctx).Where("session_id = ? AND user_id = ?", sessionID, userID).Delete(&TNVEDAgentSessionShare{}).Error
}

// AgentSessionShares — кому открыт вопрос, по имени.
func (r *Repository) AgentSessionShares(ctx context.Context, sessionID int64) ([]AgentShareUser, error) {
	users := []AgentShareUser{}
	err := r.db.WithContext(ctx).Table("tnved_agent_session_shares AS sh").Select("u.id, u.name").
		Joins("JOIN users u ON u.id = sh.user_id").Where("sh.session_id = ?", sessionID).Order("u.name, u.id").Scan(&users).Error
	return users, err
}

// AgentSessionSharedWith — открыт ли вопрос сотруднику.
func (r *Repository) AgentSessionSharedWith(ctx context.Context, sessionID, userID int64) (bool, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&TNVEDAgentSessionShare{}).Where("session_id = ? AND user_id = ?", sessionID, userID).Count(&count).Error
	return count > 0, err
}

// SharedReferenceSessions — вопросы чата на главной, которые открыли сотруднику; SharedBy — имя автора.
func (r *Repository) SharedReferenceSessions(ctx context.Context, userID int64, limit int) ([]ReferenceSession, error) {
	var rows []ReferenceSession
	err := r.db.WithContext(ctx).Table("tnved_agent_session_shares AS sh").
		Select("s.id, s.title, s.status, s.result, s.photos, s.documents_code, s.trademark_hits, s.created_at, s.updated_at, COALESCE(u.name, '') AS shared_by").
		Joins("JOIN tnved_agent_sessions s ON s.id = sh.session_id").Joins("LEFT JOIN users u ON u.id = s.user_id").
		Where("sh.user_id = ? AND s.mode = 'reference'", userID).Order("s.updated_at DESC, s.id DESC").Limit(limit).Scan(&rows).Error
	return rows, err
}
