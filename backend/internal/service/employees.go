package service

// Employee management for administrators: list, create, change role/name/password, block.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
)

type Employee struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	Role      string `json:"role"`
	Disabled  bool   `json:"disabled"`
	CreatedAt string `json:"createdAt"`
}

type EmployeeInput struct {
	Name     string `json:"name"`
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

// EmployeeUpdate changes only the fields that are set.
type EmployeeUpdate struct {
	Name     *string `json:"name"`
	Role     *string `json:"role"`
	Disabled *bool   `json:"disabled"`
	Password *string `json:"password"`
}

type employeeRepository interface {
	ListUsers(context.Context) ([]repository.User, error)
	UserByID(context.Context, int64) (repository.User, error)
	UserByEmail(context.Context, string) (repository.User, error)
	CreateUserWithRole(context.Context, string, string, string, string) (repository.User, error)
	UpdateUser(context.Context, int64, map[string]any) (repository.User, error)
	CountActiveAdmins(context.Context) (int64, error)
	DeleteUserSessions(context.Context, int64) error
}

type EmployeeService struct {
	repository employeeRepository
}

func NewEmployeeService(repo *repository.Repository) *EmployeeService {
	if repo == nil {
		return &EmployeeService{}
	}
	return &EmployeeService{repository: repo}
}

func (s *EmployeeService) List(ctx context.Context) ([]Employee, error) {
	users, err := s.repository.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]Employee, 0, len(users))
	for _, user := range users {
		result = append(result, employeeFromUser(user))
	}
	return result, nil
}

func (s *EmployeeService) Create(ctx context.Context, input EmployeeInput) (Employee, error) {
	name, err := validateEmployeeName(input.Name)
	if err != nil {
		return Employee{}, err
	}
	email := strings.TrimSpace(input.Email)
	if !validEmail(email) {
		return Employee{}, &ValidationError{Field: "email", Message: "Введите корректный email"}
	}
	if !repository.ValidRole(input.Role) {
		return Employee{}, &ValidationError{Field: "role", Message: "Выберите роль"}
	}
	if err := validateNewPassword(input.Password); err != nil {
		return Employee{}, err
	}
	if _, err := s.repository.UserByEmail(ctx, email); err == nil {
		return Employee{}, ErrEmailAlreadyExists
	} else if !errors.Is(err, repository.ErrNotFound) {
		return Employee{}, err
	}
	hash, err := hashPassword(input.Password)
	if err != nil {
		return Employee{}, fmt.Errorf("hash password: %w", err)
	}
	user, err := s.repository.CreateUserWithRole(ctx, email, string(hash), name, input.Role)
	if err != nil {
		if isUniqueViolation(err) {
			return Employee{}, ErrEmailAlreadyExists
		}
		return Employee{}, fmt.Errorf("create employee: %w", err)
	}
	return employeeFromUser(user), nil
}

// Update applies an administrator's changes. The administrator cannot take
// the admin role away from themselves or block themselves, and the last active
// administrator cannot be demoted or blocked — otherwise nobody could manage
// employees any more.
func (s *EmployeeService) Update(ctx context.Context, actor repository.User, id int64, update EmployeeUpdate) (Employee, error) {
	target, err := s.repository.UserByID(ctx, id)
	if err != nil {
		return Employee{}, err
	}
	changes := map[string]any{}
	logout := false
	losesAdmin := false

	if update.Name != nil {
		name, err := validateEmployeeName(*update.Name)
		if err != nil {
			return Employee{}, err
		}
		changes["name"] = name
	}
	if update.Role != nil && *update.Role != target.Role {
		if !repository.ValidRole(*update.Role) {
			return Employee{}, &ValidationError{Field: "role", Message: "Выберите роль"}
		}
		if id == actor.ID {
			return Employee{}, &ValidationError{Field: "role", Message: "Нельзя снять роль администратора с себя"}
		}
		changes["role"] = *update.Role
		losesAdmin = target.IsAdmin()
	}
	if update.Disabled != nil && *update.Disabled != (target.DisabledAt != nil) {
		if *update.Disabled {
			if id == actor.ID {
				return Employee{}, &ValidationError{Field: "disabled", Message: "Нельзя заблокировать себя"}
			}
			changes["disabled_at"] = time.Now().UTC()
			logout = true
			losesAdmin = losesAdmin || target.IsAdmin()
		} else {
			changes["disabled_at"] = nil
		}
	}
	if update.Password != nil {
		if err := validateNewPassword(*update.Password); err != nil {
			return Employee{}, err
		}
		hash, err := hashPassword(*update.Password)
		if err != nil {
			return Employee{}, fmt.Errorf("hash password: %w", err)
		}
		changes["password_hash"] = string(hash)
		logout = true
	}
	if len(changes) == 0 {
		return employeeFromUser(target), nil
	}
	if losesAdmin && target.DisabledAt == nil {
		admins, err := s.repository.CountActiveAdmins(ctx)
		if err != nil {
			return Employee{}, err
		}
		if admins <= 1 {
			return Employee{}, &ValidationError{Field: "role", Message: "Это последний администратор: сначала назначьте другого"}
		}
	}

	updated, err := s.repository.UpdateUser(ctx, id, changes)
	if err != nil {
		return Employee{}, err
	}
	if logout {
		if err := s.repository.DeleteUserSessions(ctx, id); err != nil {
			return Employee{}, fmt.Errorf("end employee sessions: %w", err)
		}
	}
	return employeeFromUser(updated), nil
}

func employeeFromUser(user repository.User) Employee {
	return Employee{
		ID: user.ID, Name: user.Name, Email: user.Email, Role: user.Role,
		Disabled: user.DisabledAt != nil, CreatedAt: user.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func validateEmployeeName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" || len([]rune(name)) > 120 {
		return "", &ValidationError{Field: "name", Message: "Укажите имя (до 120 символов)"}
	}
	return name, nil
}

func validateNewPassword(password string) error {
	if len([]rune(password)) < 8 {
		return &ValidationError{Field: "password", Message: "Пароль должен содержать не менее 8 символов"}
	}
	if len([]byte(password)) > 1024 {
		return &ValidationError{Field: "password", Message: "Пароль слишком длинный (максимум 1024 байта)"}
	}
	return nil
}

func isUniqueViolation(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "duplicate") || strings.Contains(message, "unique")
}
