package repository

// Журнал действий сотрудников (user_actions): запись из middleware и список для администратора.

import (
	"context"
	"time"

	"gorm.io/gorm"
)

// UserAction — одно действие: маршрут gin (/api/goods-estimates/:id), значения параметров пути, код ответа.
// UserID nil — неудачный вход, тогда Email — введённый адрес.
type UserAction struct {
	UserID *int64
	Email  string
	Method string
	Route  string
	Target string
	Status int
	IP     string
}

// UserActionRow — строка журнала; UserName — текущее имя (пусто, если сотрудника удалили).
type UserActionRow struct {
	ID        int64
	UserID    *int64
	UserName  string
	Email     string
	Method    string
	Route     string
	Target    string
	Status    int
	IP        string
	Count     int
	CreatedAt time.Time
	UpdatedAt time.Time
}

type UserActionFilter struct {
	From, To time.Time
	UserID   int64
	Limit    int
	Offset   int
}

// RecordUserAction добавляет действие. Если последняя строка сотрудника — то же действие над той же записью
// с тем же итогом не старше 10 минут, у неё растёт счётчик: автосохранение сметы не плодит тысячи строк.
func (r *Repository) RecordUserAction(ctx context.Context, action UserAction) error {
	who, args := "user_id IS NULL", []any{}
	if action.UserID != nil {
		who, args = "user_id = ?", []any{*action.UserID}
	}
	args = append(args, action.Method, action.Route, action.Target, action.Status, action.Email,
		action.UserID, action.Email, action.Method, action.Route, action.Target, action.Status, action.IP)
	return r.db.WithContext(ctx).Exec(`WITH last AS (
			SELECT id FROM (SELECT * FROM user_actions WHERE `+who+` ORDER BY id DESC LIMIT 1) l
			WHERE method = ? AND route = ? AND target = ? AND status = ? AND email = ? AND updated_at > now() - interval '10 minutes'
		), bumped AS (
			UPDATE user_actions SET count = count + 1, updated_at = now() WHERE id IN (SELECT id FROM last) RETURNING id
		)
		INSERT INTO user_actions (user_id, email, method, route, target, status, ip)
		SELECT ?::bigint, ?, ?, ?, ?, ?::int, ? WHERE NOT EXISTS (SELECT 1 FROM bumped)`, args...).Error
}

// UserActions — строки за период по времени последнего повтора (новые первыми) и их общее число.
func (r *Repository) UserActions(ctx context.Context, filter UserActionFilter) ([]UserActionRow, int64, error) {
	query := func() *gorm.DB {
		q := r.db.WithContext(ctx).Table("user_actions a").Where("a.updated_at >= ? AND a.updated_at < ?", filter.From, filter.To)
		if filter.UserID > 0 {
			q = q.Where("a.user_id = ?", filter.UserID)
		}
		return q
	}
	var total int64
	if err := query().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []UserActionRow
	err := query().Select("a.*, COALESCE(u.name, '') AS user_name").Joins("LEFT JOIN users u ON u.id = a.user_id").
		Order("a.updated_at DESC, a.id DESC").Limit(filter.Limit).Offset(filter.Offset).Scan(&rows).Error
	return rows, total, err
}

// DeleteUserActionsBefore удаляет записи старше границы: журнал хранится полгода.
func (r *Repository) DeleteUserActionsBefore(ctx context.Context, before time.Time) error {
	return r.db.WithContext(ctx).Exec("DELETE FROM user_actions WHERE updated_at < ?", before).Error
}
