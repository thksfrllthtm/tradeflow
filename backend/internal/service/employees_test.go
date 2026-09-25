package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
)

type fakeEmployeeRepository struct {
	users          map[int64]repository.User
	nextID         int64
	loggedOutUsers []int64
}

func newFakeEmployees(users ...repository.User) *fakeEmployeeRepository {
	f := &fakeEmployeeRepository{users: map[int64]repository.User{}}
	for _, user := range users {
		f.users[user.ID] = user
		if user.ID > f.nextID {
			f.nextID = user.ID
		}
	}
	return f
}

func (f *fakeEmployeeRepository) ListUsers(context.Context) ([]repository.User, error) {
	result := []repository.User{}
	for id := int64(1); id <= f.nextID; id++ {
		if user, ok := f.users[id]; ok {
			result = append(result, user)
		}
	}
	return result, nil
}

func (f *fakeEmployeeRepository) UserByID(_ context.Context, id int64) (repository.User, error) {
	user, ok := f.users[id]
	if !ok {
		return repository.User{}, repository.ErrNotFound
	}
	return user, nil
}

func (f *fakeEmployeeRepository) UserByEmail(_ context.Context, email string) (repository.User, error) {
	for _, user := range f.users {
		if user.Email == strings.ToLower(strings.TrimSpace(email)) {
			return user, nil
		}
	}
	return repository.User{}, repository.ErrNotFound
}

func (f *fakeEmployeeRepository) CreateUserWithRole(_ context.Context, email, hash, name, role string) (repository.User, error) {
	f.nextID++
	user := repository.User{ID: f.nextID, Email: strings.ToLower(email), PasswordHash: hash, Name: name, Role: role}
	f.users[user.ID] = user
	return user, nil
}

func (f *fakeEmployeeRepository) UpdateUser(_ context.Context, id int64, changes map[string]any) (repository.User, error) {
	user := f.users[id]
	for key, value := range changes {
		switch key {
		case "name":
			user.Name = value.(string)
		case "role":
			user.Role = value.(string)
		case "password_hash":
			user.PasswordHash = value.(string)
		case "disabled_at":
			if value == nil {
				user.DisabledAt = nil
			} else {
				at := value.(time.Time)
				user.DisabledAt = &at
			}
		}
	}
	f.users[id] = user
	return user, nil
}

func (f *fakeEmployeeRepository) CountActiveAdmins(context.Context) (int64, error) {
	var count int64
	for _, user := range f.users {
		if user.IsAdmin() && user.DisabledAt == nil {
			count++
		}
	}
	return count, nil
}

func (f *fakeEmployeeRepository) DeleteUserSessions(_ context.Context, id int64) error {
	f.loggedOutUsers = append(f.loggedOutUsers, id)
	return nil
}

var (
	testAdmin    = repository.User{ID: 1, Email: "admin@example.com", Role: repository.RoleAdmin}
	testInvoicer = repository.User{ID: 2, Email: "inv@example.com", Role: repository.RoleInvoicer}
)

func ptr[T any](value T) *T { return &value }

func validationField(err error) string {
	var validation *ValidationError
	if errors.As(err, &validation) {
		return validation.Field
	}
	return ""
}

func TestEmployeeCreateValidatesAndHashesPassword(t *testing.T) {
	repo := newFakeEmployees(testAdmin)
	service := &EmployeeService{repository: repo}

	if _, err := service.Create(context.Background(), EmployeeInput{Name: "Инвойсер", Email: "new@example.com", Password: "password1", Role: "boss"}); validationField(err) != "role" {
		t.Fatalf("unknown role must be rejected, got %v", err)
	}
	if _, err := service.Create(context.Background(), EmployeeInput{Name: "Инвойсер", Email: "new@example.com", Password: "short", Role: repository.RoleInvoicer}); validationField(err) != "password" {
		t.Fatalf("short password must be rejected, got %v", err)
	}
	if _, err := service.Create(context.Background(), EmployeeInput{Name: "Дубль", Email: "ADMIN@example.com", Password: "password1", Role: repository.RoleInvoicer}); !errors.Is(err, ErrEmailAlreadyExists) {
		t.Fatalf("existing email must be rejected, got %v", err)
	}
	created, err := service.Create(context.Background(), EmployeeInput{Name: " Переводчик ", Email: "tr@example.com", Password: "password1", Role: repository.RoleTranslator})
	if err != nil {
		t.Fatal(err)
	}
	stored := repo.users[created.ID]
	if created.Name != "Переводчик" || created.Role != repository.RoleTranslator || !passwordMatches(stored.PasswordHash, "password1") {
		t.Fatalf("unexpected employee %+v", created)
	}
}

func TestEmployeeUpdateProtectsAdministrators(t *testing.T) {
	repo := newFakeEmployees(testAdmin, testInvoicer)
	service := &EmployeeService{repository: repo}
	ctx := context.Background()

	if _, err := service.Update(ctx, testAdmin, testAdmin.ID, EmployeeUpdate{Role: ptr(repository.RoleInvoicer)}); validationField(err) != "role" {
		t.Fatalf("admin must not demote themselves, got %v", err)
	}
	if _, err := service.Update(ctx, testAdmin, testAdmin.ID, EmployeeUpdate{Disabled: ptr(true)}); validationField(err) != "disabled" {
		t.Fatalf("admin must not block themselves, got %v", err)
	}

	// A second admin may demote the first while another admin remains.
	promoted, err := service.Update(ctx, testAdmin, testInvoicer.ID, EmployeeUpdate{Role: ptr(repository.RoleAdmin)})
	if err != nil || promoted.Role != repository.RoleAdmin {
		t.Fatalf("promote: %+v %v", promoted, err)
	}
	if _, err := service.Update(ctx, repo.users[testInvoicer.ID], testAdmin.ID, EmployeeUpdate{Role: ptr(repository.RoleInvoicer)}); err != nil {
		t.Fatalf("demoting one of two admins must work: %v", err)
	}
	// Now testInvoicer is the only admin; testAdmin (no longer admin) tries to block them.
	if _, err := service.Update(ctx, repo.users[testAdmin.ID], testInvoicer.ID, EmployeeUpdate{Disabled: ptr(true)}); validationField(err) != "role" {
		t.Fatalf("the last active admin must not be blocked, got %v", err)
	}
}

func TestEmployeeBlockAndPasswordChangeEndSessions(t *testing.T) {
	repo := newFakeEmployees(testAdmin, testInvoicer)
	service := &EmployeeService{repository: repo}
	ctx := context.Background()

	blocked, err := service.Update(ctx, testAdmin, testInvoicer.ID, EmployeeUpdate{Disabled: ptr(true)})
	if err != nil || !blocked.Disabled {
		t.Fatalf("block: %+v %v", blocked, err)
	}
	if len(repo.loggedOutUsers) != 1 || repo.loggedOutUsers[0] != testInvoicer.ID {
		t.Fatalf("blocking must end the employee's sessions, got %v", repo.loggedOutUsers)
	}
	unblocked, err := service.Update(ctx, testAdmin, testInvoicer.ID, EmployeeUpdate{Disabled: ptr(false)})
	if err != nil || unblocked.Disabled {
		t.Fatalf("unblock: %+v %v", unblocked, err)
	}
	if _, err := service.Update(ctx, testAdmin, testInvoicer.ID, EmployeeUpdate{Password: ptr("new-password")}); err != nil {
		t.Fatal(err)
	}
	if !passwordMatches(repo.users[testInvoicer.ID].PasswordHash, "new-password") || len(repo.loggedOutUsers) != 2 {
		t.Fatalf("password change must store the new hash and end sessions, sessions ended: %v", repo.loggedOutUsers)
	}
}

func TestLoginRejectsBlockedEmployee(t *testing.T) {
	hash, _ := hashPassword("password1")
	blockedAt := time.Now()
	fake := &fakeAuthRepository{users: map[string]repository.User{
		"blocked@example.com": {ID: 5, Email: "blocked@example.com", PasswordHash: string(hash), DisabledAt: &blockedAt},
	}}
	if _, _, _, err := newAuthService(fake, time.Hour).Login(context.Background(), "blocked@example.com", "password1"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("blocked employee must not log in, got %v", err)
	}
}
