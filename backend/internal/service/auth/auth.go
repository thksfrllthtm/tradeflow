package auth

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
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"golang.org/x/crypto/bcrypt"
)

var ErrInvalidCredentials = errors.New("invalid credentials")
var ErrInvalidBootstrapUser = errors.New("bootstrap email and password are required")
var ErrEmailAlreadyExists = errors.New("email already exists")

type AuthService struct {
	repository AuthRepository
	sessionTTL time.Duration
}

// AuthRepository — то, что нужно входу и сессиям от хранилища (в тестах — подделка).
type AuthRepository interface {
	UserByEmail(context.Context, string) (repository.User, error)
	CreateUser(context.Context, string, string, ...string) (repository.User, error)
	CreateUserWithRole(context.Context, string, string, string, string) (repository.User, error)
	CreateSession(context.Context, string, int64, time.Time) error
	DeleteSession(context.Context, string) error
	UserBySessionToken(context.Context, string) (repository.User, error)
}

func NewAuthService(repository AuthRepository, sessionTTL time.Duration) *AuthService {
	return &AuthService{repository: repository, sessionTTL: sessionTTL}
}

func (s *AuthService) Login(ctx context.Context, email, password string) (repository.User, string, time.Time, error) {
	user, err := s.repository.UserByEmail(ctx, email)
	// A blocked employee gets the same answer as a wrong password.
	if err != nil || user.DisabledAt != nil || !PasswordMatches(user.PasswordHash, password) {
		return repository.User{}, "", time.Time{}, ErrInvalidCredentials
	}

	return s.startSession(ctx, user)
}

func (s *AuthService) Register(ctx context.Context, name, email, password string) (repository.User, string, time.Time, error) {
	name = strings.TrimSpace(name)
	email = strings.TrimSpace(email)
	if name == "" || len([]rune(name)) > 120 {
		return repository.User{}, "", time.Time{}, &service.ValidationError{Field: "name", Message: "Укажите имя (до 120 символов)"}
	}
	if !ValidEmail(email) {
		return repository.User{}, "", time.Time{}, &service.ValidationError{Field: "email", Message: "Введите корректный email"}
	}
	if err := ValidateNewPassword(password); err != nil {
		return repository.User{}, "", time.Time{}, err
	}
	if _, err := s.repository.UserByEmail(ctx, email); err == nil {
		return repository.User{}, "", time.Time{}, ErrEmailAlreadyExists
	} else if !errors.Is(err, repository.ErrNotFound) {
		return repository.User{}, "", time.Time{}, err
	}
	hash, err := HashPassword(password)
	if err != nil {
		return repository.User{}, "", time.Time{}, fmt.Errorf("hash password: %w", err)
	}
	user, err := s.repository.CreateUser(ctx, email, string(hash), name)
	if err != nil {
		// The database unique constraint is authoritative when two requests
		// race after the pre-check.
		if IsUniqueViolation(err) {
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

func ValidEmail(value string) bool {
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

	hash, err := HashPassword(password)
	if err != nil {
		return repository.User{}, fmt.Errorf("hash password: %w", err)
	}
	// The bootstrap employee manages everyone else, so it is an administrator.
	return s.repository.CreateUserWithRole(ctx, email, string(hash), "", repository.RoleAdmin)
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
func HashPassword(password string) ([]byte, error) {
	passwordBytes := []byte(password)
	if len(passwordBytes) <= 72 {
		return bcrypt.GenerateFromPassword(passwordBytes, bcrypt.DefaultCost)
	}
	digest := sha256.Sum256(passwordBytes)
	return bcrypt.GenerateFromPassword(digest[:], bcrypt.DefaultCost)
}

func PasswordMatches(encodedHash, password string) bool {
	if bcrypt.CompareHashAndPassword([]byte(encodedHash), []byte(password)) == nil {
		return true
	}
	digest := sha256.Sum256([]byte(password))
	return bcrypt.CompareHashAndPassword([]byte(encodedHash), digest[:]) == nil
}

func ValidateNewPassword(password string) error {
	if len([]rune(password)) < 8 {
		return &service.ValidationError{Field: "password", Message: "Пароль должен содержать не менее 8 символов"}
	}
	if len([]byte(password)) > 1024 {
		return &service.ValidationError{Field: "password", Message: "Пароль слишком длинный (максимум 1024 байта)"}
	}
	return nil
}

func IsUniqueViolation(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "duplicate") || strings.Contains(message, "unique")
}
