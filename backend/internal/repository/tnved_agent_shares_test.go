package repository

import (
	"context"
	"strings"
	"testing"
)

// Вопросы, открытые сотруднику, — только его доступы и только чат на главной; список для выбора — без себя
// и без отключённых сотрудников.
func TestShareQueriesFilterByUser(t *testing.T) {
	repo, recorder := dryRunRepository(t)
	_, _ = repo.SharedReferenceSessions(context.Background(), 42, 50)
	_, _ = repo.ShareUsers(context.Background(), 42)
	if len(recorder.sql) != 2 {
		t.Fatalf("запросов %d: %v", len(recorder.sql), recorder.sql)
	}
	shared, users := recorder.sql[0], recorder.sql[1]
	for _, want := range []string{"sh.user_id = 42", "s.mode = 'reference'", "JOIN tnved_agent_sessions s ON s.id = sh.session_id", "true AS shared", "AS shared_by", "LIMIT 50"} {
		if !strings.Contains(shared, want) {
			t.Errorf("открытые вопросы: нет %q в %s", want, shared)
		}
	}
	for _, want := range []string{"disabled_at IS NULL", "id <> 42", "name <> ''"} {
		if !strings.Contains(users, want) {
			t.Errorf("сотрудники: нет %q в %s", want, users)
		}
	}
	if strings.Contains(users, "email") || strings.Contains(users, "role") {
		t.Errorf("в списке для выбора только имя и номер: %s", users)
	}
}
