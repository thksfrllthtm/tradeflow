package repository

import (
	"context"
	"errors"
	"strings"
	"testing"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// Order(gorm.Expr(...)) GORM молча пропускает: агент получал примеры без сортировки по числу
// совпавших слов, и под лимит попадали случайные.
func TestSearchCodeDecisionsOrdersByMatchedWords(t *testing.T) {
	repo, recorder := dryRunRepository(t)
	if _, err := repo.SearchCodeDecisions(context.Background(), "отвертка крестовая", 10); err != nil && !errors.Is(err, gorm.ErrDryRunModeUnsupported) {
		t.Fatal(err)
	}
	sql := recorder.sql[len(recorder.sql)-1]
	if !strings.Contains(sql, "ORDER BY ((CASE WHEN") || !strings.Contains(sql, "CASE d.status WHEN 'verified' THEN 0 ELSE 1 END") {
		t.Errorf("нет сортировки по совпавшим словам:\n%s", sql)
	}
}

// «карта» находила 188 записей: слово искалось во всём описании фото («картонная упаковка» у рулеток
// и наждачки), а список шёл по дате, и «Карты игральные» оказывались в середине.
func TestDecisionTextSearchRanksNameMatchesFirst(t *testing.T) {
	repo, _ := dryRunRepository(t)
	where, order := decisionTextSearch("карта", []int64{5})
	sql := repo.db.ToSQL(func(tx *gorm.DB) *gorm.DB {
		return tx.Table("code_decisions AS d").Where(where.SQL, where.Vars...).Order(clause.OrderBy{Expression: order}).Find(&[]CodeDecision{})
	})
	for _, part := range []string{
		"WHERE ((d.invoice_name ILIKE '%карт%'",
		"OR concat_ws(' ', d.photo_features->>'kind', d.photo_features->>'purpose', d.photo_features->'synonyms', d.photo_features->'keywords') ILIKE '%карт%')) OR d.id IN (5)",
		"ORDER BY CASE WHEN (d.invoice_name ILIKE '%карт%' OR d.supplier_name ILIKE '%карт%' OR d.final_name ILIKE '%карт%') THEN 0 WHEN",
		"THEN 1 ELSE 2 END",
	} {
		if !strings.Contains(sql, part) {
			t.Errorf("в SQL нет %q:\n%s", part, sql)
		}
	}
	for _, part := range []string{"photo_features::text", "material", "'features'"} {
		if strings.Contains(sql, part) {
			t.Errorf("поиск не должен идти по %q:\n%s", part, sql)
		}
	}
}
