package repository

import (
	"context"
	"errors"
	"strings"
	"testing"

	"gorm.io/gorm"
)

// Повтор сливается только с последней строкой того же сотрудника; неудачный вход (без сотрудника) — с
// последней строкой без сотрудника и тем же email.
func TestRecordUserActionMergesWithLastRowOfSameUser(t *testing.T) {
	id := int64(7)
	for name, tc := range map[string]struct {
		action UserAction
		want   []string
	}{
		"сотрудник":      {UserAction{UserID: &id, Email: "a@b.kz", Method: "PATCH", Route: "/api/goods-estimates/:id/draft", Target: "5", Status: 200}, []string{"WHERE user_id = 7 ORDER BY id DESC LIMIT 1", "target = '5'", "status = 200", "count = count + 1", "SELECT 7::bigint, 'a@b.kz', 'PATCH'"}},
		"неудачный вход": {UserAction{Email: "x@y.kz", Method: "POST", Route: "/api/auth/login", Status: 401}, []string{"WHERE user_id IS NULL ORDER BY id DESC LIMIT 1", "email = 'x@y.kz'", "SELECT NULL::bigint, 'x@y.kz', 'POST'"}},
	} {
		t.Run(name, func(t *testing.T) {
			repo, recorder := dryRunRepository(t)
			if err := repo.RecordUserAction(context.Background(), tc.action); err != nil && !errors.Is(err, gorm.ErrDryRunModeUnsupported) {
				t.Fatal(err)
			}
			sql := strings.Join(recorder.sql, "\n")
			for _, want := range tc.want {
				if !strings.Contains(sql, want) {
					t.Errorf("нет %q в %s", want, sql)
				}
			}
		})
	}
}
