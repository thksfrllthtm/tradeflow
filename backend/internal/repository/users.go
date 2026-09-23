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

func (r *Repository) CreateUser(ctx context.Context, email, passwordHash string, name ...string) (User, error) {
	displayName := ""
	if len(name) > 0 {
		displayName = strings.TrimSpace(name[0])
	}
	user := User{Name: displayName, Email: normalizeEmail(email), PasswordHash: passwordHash}
	if err := r.db.WithContext(ctx).Create(&user).Error; err != nil {
		return User{}, err
	}
	return user, nil
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
	err = r.db.WithContext(ctx).First(&user, session.UserID).Error
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
