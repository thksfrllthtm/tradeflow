package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

var ErrInvalidCredentials = errors.New("invalid credentials")
var ErrInvalidBootstrapUser = errors.New("bootstrap email and password are required")
var ErrEmailAlreadyExists = errors.New("email already exists")

type AuthService struct {
	repository authRepository
	sessionTTL time.Duration
}

type authRepository interface {
	UserByEmail(context.Context, string) (repository.User, error)
	CreateUser(context.Context, string, string, ...string) (repository.User, error)
	CreateSession(context.Context, string, int64, time.Time) error
	DeleteSession(context.Context, string) error
	UserBySessionToken(context.Context, string) (repository.User, error)
}

func NewAuthService(repository *repository.Repository, sessionTTL time.Duration) *AuthService {
	return &AuthService{repository: repository, sessionTTL: sessionTTL}
}

func newAuthService(repository authRepository, sessionTTL time.Duration) *AuthService {
	return &AuthService{repository: repository, sessionTTL: sessionTTL}
}

func (s *AuthService) Login(ctx context.Context, email, password string) (repository.User, string, time.Time, error) {
	user, err := s.repository.UserByEmail(ctx, email)
	if err != nil || !passwordMatches(user.PasswordHash, password) {
		return repository.User{}, "", time.Time{}, ErrInvalidCredentials
	}

	return s.startSession(ctx, user)
}

func (s *AuthService) Register(ctx context.Context, name, email, password string) (repository.User, string, time.Time, error) {
	name = strings.TrimSpace(name)
	email = strings.TrimSpace(email)
	if name == "" || len([]rune(name)) > 120 {
		return repository.User{}, "", time.Time{}, &ValidationError{Field: "name", Message: "Укажите имя (до 120 символов)"}
	}
	if !validEmail(email) {
		return repository.User{}, "", time.Time{}, &ValidationError{Field: "email", Message: "Введите корректный email"}
	}
	if len([]rune(password)) < 8 {
		return repository.User{}, "", time.Time{}, &ValidationError{Field: "password", Message: "Пароль должен содержать не менее 8 символов"}
	}
	if len([]byte(password)) > 1024 {
		return repository.User{}, "", time.Time{}, &ValidationError{Field: "password", Message: "Пароль слишком длинный (максимум 1024 байта)"}
	}
	if _, err := s.repository.UserByEmail(ctx, email); err == nil {
		return repository.User{}, "", time.Time{}, ErrEmailAlreadyExists
	} else if !errors.Is(err, repository.ErrNotFound) {
		return repository.User{}, "", time.Time{}, err
	}
	hash, err := hashPassword(password)
	if err != nil {
		return repository.User{}, "", time.Time{}, fmt.Errorf("hash password: %w", err)
	}
	user, err := s.repository.CreateUser(ctx, email, string(hash), name)
	if err != nil {
		// The database unique constraint is authoritative when two requests
		// race after the pre-check.
		if strings.Contains(strings.ToLower(err.Error()), "duplicate") || strings.Contains(strings.ToLower(err.Error()), "unique") {
			return repository.User{}, "", time.Time{}, ErrEmailAlreadyExists
		}
		return repository.User{}, "", time.Time{}, fmt.Errorf("create user: %w", err)
	}
	return s.startSession(ctx, user)
}

func (s *AuthService) startSession(ctx context.Context, user repository.User) (repository.User, string, time.Time, error) {
	token, err := newSessionToken()
	if err != nil {
		return repository.User{}, "", time.Time{}, fmt.Errorf("generate session token: %w", err)
	}
	expiresAt := time.Now().UTC().Add(s.sessionTTL)
	if err := s.repository.CreateSession(ctx, token, user.ID, expiresAt); err != nil {
		return repository.User{}, "", time.Time{}, fmt.Errorf("save session: %w", err)
	}
	return user, token, expiresAt, nil
}

func validEmail(value string) bool {
	address, err := mail.ParseAddress(value)
	return err == nil && address.Address == value && strings.Contains(value, "@") && len([]byte(value)) <= 320
}

func (s *AuthService) Logout(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.repository.DeleteSession(ctx, token)
}

func (s *AuthService) BootstrapUser(ctx context.Context, email, password string) (repository.User, error) {
	email = strings.TrimSpace(email)
	if email == "" || password == "" {
		return repository.User{}, ErrInvalidBootstrapUser
	}
	if len([]rune(password)) < 12 {
		return repository.User{}, errors.New("bootstrap password must be at least 12 characters")
	}
	if _, err := s.repository.UserByEmail(ctx, email); err == nil {
		return repository.User{}, errors.New("employee with this email already exists")
	} else if !errors.Is(err, repository.ErrNotFound) {
		return repository.User{}, err
	}

	hash, err := hashPassword(password)
	if err != nil {
		return repository.User{}, fmt.Errorf("hash password: %w", err)
	}
	return s.repository.CreateUser(ctx, email, string(hash), "")
}

func (s *AuthService) UserForToken(ctx context.Context, token string) (repository.User, error) {
	return s.repository.UserBySessionToken(ctx, token)
}

func newSessionToken() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

// bcrypt deliberately limits a password to 72 bytes. Keep the normal bcrypt
// representation for ordinary passwords (so existing hashes remain usable),
// and pre-hash only longer inputs so the registration contract can still allow
// long passphrases without silently truncating them.
func hashPassword(password string) ([]byte, error) {
	passwordBytes := []byte(password)
	if len(passwordBytes) <= 72 {
		return bcrypt.GenerateFromPassword(passwordBytes, bcrypt.DefaultCost)
	}
	digest := sha256.Sum256(passwordBytes)
	return bcrypt.GenerateFromPassword(digest[:], bcrypt.DefaultCost)
}

func passwordMatches(encodedHash, password string) bool {
	if bcrypt.CompareHashAndPassword([]byte(encodedHash), []byte(password)) == nil {
		return true
	}
	digest := sha256.Sum256([]byte(password))
	return bcrypt.CompareHashAndPassword([]byte(encodedHash), digest[:]) == nil
}
