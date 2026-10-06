package repository

// Users and login sessions (session tokens are stored hashed).

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

func (r *Repository) UserByEmail(ctx context.Context, email string) (User, error) {
	var user User
	err := r.db.WithContext(ctx).Where("email = ?", normalizeEmail(email)).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return User{}, ErrNotFound
	}
	return user, err
}

// CreateUser creates an employee with the default invoicer role.
func (r *Repository) CreateUser(ctx context.Context, email, passwordHash string, name ...string) (User, error) {
	displayName := ""
	if len(name) > 0 {
		displayName = name[0]
	}
	return r.CreateUserWithRole(ctx, email, passwordHash, displayName, RoleInvoicer)
}

func (r *Repository) CreateUserWithRole(ctx context.Context, email, passwordHash, name, role string) (User, error) {
	user := User{Name: strings.TrimSpace(name), Email: normalizeEmail(email), PasswordHash: passwordHash, Role: role}
	if err := r.db.WithContext(ctx).Create(&user).Error; err != nil {
		return User{}, err
	}
	return user, nil
}

func (r *Repository) UserByID(ctx context.Context, id int64) (User, error) {
	var user User
	err := r.db.WithContext(ctx).First(&user, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return User{}, ErrNotFound
	}
	return user, err
}

func (r *Repository) ListUsers(ctx context.Context) ([]User, error) {
	var users []User
	err := r.db.WithContext(ctx).Order("id").Find(&users).Error
	return users, err
}

// EstimateCountsByUser — сколько смет товаров у каждого сотрудника.
func (r *Repository) EstimateCountsByUser(ctx context.Context) (map[int64]int64, error) {
	var rows []struct {
		UserID int64
		Count  int64
	}
	if err := r.db.WithContext(ctx).Model(&GoodsEstimate{}).Select("user_id, count(*) AS count").Group("user_id").Scan(&rows).Error; err != nil {
		return nil, err
	}
	counts := make(map[int64]int64, len(rows))
	for _, row := range rows {
		counts[row.UserID] = row.Count
	}
	return counts, nil
}

// DeleteUser удаляет сотрудника в одной транзакции. Его сметы и авторство записей справочников
// переходят к heirID (администратору, который удаляет): иначе сметы удалились бы вместе с
// сотрудником (ON DELETE CASCADE), а записи справочников не дали бы его удалить. Сессии
// удаляются каскадом, в керамике и журналах загрузок ссылка обнуляется. Возвращает число
// переданных смет.
func (r *Repository) DeleteUser(ctx context.Context, id, heirID int64) (int64, error) {
	var moved int64
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		result := tx.Model(&GoodsEstimate{}).Where("user_id = ?", id).Update("user_id", heirID)
		if result.Error != nil {
			return result.Error
		}
		moved = result.RowsAffected
		for _, table := range []string{"product_directory_entries"} {
			for _, column := range []string{"created_by", "updated_by", "archived_by"} {
				if err := tx.Table(table).Where(column+" = ?", id).Update(column, heirID).Error; err != nil {
					return err
				}
			}
		}
		deleted := tx.Delete(&User{}, id)
		if deleted.Error != nil {
			return deleted.Error
		}
		if deleted.RowsAffected == 0 {
			return ErrNotFound
		}
		return nil
	})
	return moved, err
}

// UpdateUser changes only the given columns (role, name, email, password_hash, disabled_at).
func (r *Repository) UpdateUser(ctx context.Context, id int64, changes map[string]any) (User, error) {
	if email, ok := changes["email"].(string); ok {
		changes["email"] = normalizeEmail(email)
	}
	changes["updated_at"] = time.Now().UTC()
	result := r.db.WithContext(ctx).Model(&User{}).Where("id = ?", id).Updates(changes)
	if result.Error != nil {
		return User{}, result.Error
	}
	if result.RowsAffected == 0 {
		return User{}, ErrNotFound
	}
	return r.UserByID(ctx, id)
}

func (r *Repository) CountActiveAdmins(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.WithContext(ctx).Model(&User{}).Where("role = ? AND disabled_at IS NULL", RoleAdmin).Count(&count).Error
	return count, err
}

// DeleteUserSessions logs the employee out everywhere (after blocking or a password change).
func (r *Repository) DeleteUserSessions(ctx context.Context, userID int64) error {
	return r.db.WithContext(ctx).Where("user_id = ?", userID).Delete(&Session{}).Error
}

func (r *Repository) CreateSession(ctx context.Context, token string, userID int64, expiresAt time.Time) error {
	return r.db.WithContext(ctx).Create(&Session{
		TokenHash: hashToken(token),
		UserID:    userID,
		ExpiresAt: expiresAt,
	}).Error
}

func (r *Repository) DeleteSession(ctx context.Context, token string) error {
	return r.db.WithContext(ctx).Where("token_hash = ?", hashToken(token)).Delete(&Session{}).Error
}

func (r *Repository) UserBySessionToken(ctx context.Context, token string) (User, error) {
	var session Session
	err := r.db.WithContext(ctx).Where("token_hash = ? AND expires_at > ?", hashToken(token), time.Now().UTC()).First(&session).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return User{}, ErrNotFound
	}
	if err != nil {
		return User{}, err
	}
	var user User
	// A blocked employee's sessions stop working immediately.
	err = r.db.WithContext(ctx).Where("disabled_at IS NULL").First(&user, session.UserID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return User{}, ErrNotFound
	}
	return user, err
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return stringEncodeHex(sum[:])
}

func stringEncodeHex(value []byte) string {
	const hex = "0123456789abcdef"
	result := make([]byte, len(value)*2)
	for i, b := range value {
		result[i*2] = hex[b>>4]
		result[i*2+1] = hex[b&0x0f]
	}
	return string(result)
}

func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}
