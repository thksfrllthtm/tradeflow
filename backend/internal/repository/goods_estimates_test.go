package repository

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// sqlRecorder запоминает SQL, который GORM собрал бы для базы (DryRun ничего не выполняет).
type sqlRecorder struct {
	logger.Interface
	sql []string
}

func (r *sqlRecorder) Trace(_ context.Context, _ time.Time, fc func() (string, int64), _ error) {
	sql, _ := fc()
	r.sql = append(r.sql, sql)
}

func dryRunRepository(t *testing.T) (*Repository, *sqlRecorder) {
	t.Helper()
	recorder := &sqlRecorder{Interface: logger.Discard}
	db, err := gorm.Open(postgres.New(postgres.Config{DSN: "host=127.0.0.1 port=1 user=dry dbname=dry sslmode=disable"}), &gorm.Config{
		DryRun: true, DisableAutomaticPing: true, Logger: recorder,
	})
	if err != nil {
		t.Fatal(err)
	}
	return &Repository{db: db}, recorder
}

// «?» в регулярке цены GORM принимал за параметр: фильтр по сотруднику получал не то значение,
// и запрос инвойсера падал с синтаксической ошибкой (500 на /api/estimates/price-history).
func TestUnitPriceHistoryBindsEveryParameter(t *testing.T) {
	for _, tc := range []struct {
		name   string
		userID int64
		want   []string
	}{
		{"сотрудник", 7, []string{"IN ('8471300000','9503007000')", "e.id <> 42", "e.user_id = 7", `'^[0-9]+(\.[0-9]+){0,1}$'`}},
		{"администратор", AllUsers, []string{"IN ('8471300000','9503007000')", "e.id <> 42", `'^[0-9]+(\.[0-9]+){0,1}$'`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo, recorder := dryRunRepository(t)
			if _, err := repo.UnitPriceHistory(context.Background(), tc.userID, 42, []string{"8471300000", "9503007000"}); err != nil && !errors.Is(err, gorm.ErrDryRunModeUnsupported) {
				t.Fatal(err)
			}
			if len(recorder.sql) != 1 {
				t.Fatalf("ожидался один запрос, получено %d", len(recorder.sql))
			}
			sql := recorder.sql[0]
			for _, part := range tc.want {
				if !strings.Contains(sql, part) {
					t.Errorf("в SQL нет %q:\n%s", part, sql)
				}
			}
			if tc.userID == AllUsers && strings.Contains(sql, "user_id") {
				t.Errorf("администратору фильтр по сотруднику не нужен:\n%s", sql)
			}
		})
	}
}
