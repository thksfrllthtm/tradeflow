package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
)

func TestValidEmail(t *testing.T) {
	for _, value := range []string{"employee@example.com", " user@example.com"} {
		if value == " user@example.com" {
			if validEmail(value) {
				t.Fatalf("validEmail(%q) = true; outer whitespace must be normalized before validation", value)
			}
			continue
		}
		if !validEmail(value) {
			t.Fatalf("validEmail(%q) = false", value)
		}
	}
	for _, value := range []string{"", "employee", "employee@", "Name <employee@example.com>"} {
		if validEmail(value) {
			t.Fatalf("validEmail(%q) = true", value)
		}
	}
}

func TestPasswordHashSupportsLongPassphrases(t *testing.T) {
	password := "длинная фраза с пробелами, которую нельзя обрезать при сохранении" + "!"
	hash, err := hashPassword(password)
	if err != nil {
		t.Fatalf("hashPassword() error = %v", err)
	}
	if !passwordMatches(string(hash), password) {
		t.Fatal("passwordMatches() rejected the original long passphrase")
	}
	if passwordMatches(string(hash), password+"x") {
		t.Fatal("passwordMatches() accepted a different passphrase")
	}
}

func TestRegisterValidatesInputBeforeRepository(t *testing.T) {
	service := NewAuthService(nil, time.Hour)
	cases := []struct {
		name     string
		email    string
		password string
		field    string
	}{
		{name: "", email: "employee@example.com", password: "long enough", field: "name"},
		{name: "Employee", email: "not-an-email", password: "long enough", field: "email"},
		{name: "Employee", email: "employee@example.com", password: "short", field: "password"},
	}
	for _, tc := range cases {
		_, _, _, err := service.Register(context.Background(), tc.name, tc.email, tc.password)
		var validation *ValidationError
		if !errors.As(err, &validation) || validation.Field != tc.field {
			t.Errorf("Register(%q, %q) field = %v, want %q", tc.name, tc.email, validation, tc.field)
		}
	}
}

type fakeAuthRepository struct {
	users          map[string]repository.User
	sessions       map[string]repository.User
	createUserErr  error
	createdSession bool
	nextID         int64
}

func (f *fakeAuthRepository) UserByEmail(_ context.Context, email string) (repository.User, error) {
	user, ok := f.users[strings.ToLower(strings.TrimSpace(email))]
	if !ok {
		return repository.User{}, repository.ErrNotFound
	}
	return user, nil
}

func (f *fakeAuthRepository) CreateUser(ctx context.Context, email, passwordHash string, name ...string) (repository.User, error) {
	displayName := ""
	if len(name) > 0 {
		displayName = name[0]
	}
	return f.CreateUserWithRole(ctx, email, passwordHash, displayName, repository.RoleInvoicer)
}

func (f *fakeAuthRepository) CreateUserWithRole(_ context.Context, email, passwordHash, displayName, role string) (repository.User, error) {
	if f.createUserErr != nil {
		return repository.User{}, f.createUserErr
	}
	f.nextID++
	user := repository.User{ID: f.nextID, Name: displayName, Email: strings.ToLower(strings.TrimSpace(email)), PasswordHash: passwordHash, Role: role}
	if f.users == nil {
		f.users = make(map[string]repository.User)
	}
	f.users[user.Email] = user
	return user, nil
}

func (f *fakeAuthRepository) CreateSession(_ context.Context, token string, userID int64, _ time.Time) error {
	f.createdSession = true
	for _, user := range f.users {
		if user.ID == userID {
			if f.sessions == nil {
				f.sessions = make(map[string]repository.User)
			}
			f.sessions[token] = user
			return nil
		}
	}
	return errors.New("user not found")
}

func (f *fakeAuthRepository) DeleteSession(_ context.Context, token string) error {
	delete(f.sessions, token)
	return nil
}

func (f *fakeAuthRepository) UserBySessionToken(_ context.Context, token string) (repository.User, error) {
	user, ok := f.sessions[token]
	if !ok {
		return repository.User{}, repository.ErrNotFound
	}
	return user, nil
}

func TestRegisterCreatesNormalUserAndSession(t *testing.T) {
	store := &fakeAuthRepository{users: make(map[string]repository.User)}
	service := newAuthService(store, time.Hour)
	user, token, expiresAt, err := service.Register(context.Background(), "  Анна  ", " EMPLOYEE@EXAMPLE.COM ", "пароль с пробелами")
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if user.Name != "Анна" || user.Email != "employee@example.com" {
		t.Fatalf("unexpected user: %+v", user)
	}
	if token == "" || !store.createdSession || !expiresAt.After(time.Now()) {
		t.Fatalf("registration did not create a valid session: token=%q expires=%v", token, expiresAt)
	}
}

func TestRegisterRejectsDuplicateEmail(t *testing.T) {
	store := &fakeAuthRepository{users: map[string]repository.User{
		"employee@example.com": {ID: 1, Email: "employee@example.com", PasswordHash: "unused"},
	}}
	service := newAuthService(store, time.Hour)
	_, _, _, err := service.Register(context.Background(), "Employee", "EMPLOYEE@example.com", "long enough")
	if !errors.Is(err, ErrEmailAlreadyExists) {
		t.Fatalf("Register() error = %v, want ErrEmailAlreadyExists", err)
	}
}

func TestLoginReturnsGenericInvalidCredentialsError(t *testing.T) {
	hash, err := hashPassword("correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}
	store := &fakeAuthRepository{users: map[string]repository.User{
		"employee@example.com": {ID: 1, Email: "employee@example.com", PasswordHash: string(hash)},
	}}
	service := newAuthService(store, time.Hour)
	_, _, _, err = service.Login(context.Background(), "employee@example.com", "wrong password")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("Login() error = %v, want ErrInvalidCredentials", err)
	}
}
