package repository

import (
	"context"
	"strings"
	"testing"
)

// Поиск по базе документов: цифры — начало кода, слова — товар; акты в кэше — без справок tnved.info и без
// страниц поиска деклараций.
func TestDocumentsBaseQueries(t *testing.T) {
	repo, recorder := dryRunRepository(t)
	_, _ = repo.DocumentsBaseRows(context.Background(), "6302 51", 300)
	_, _ = repo.DocumentsBaseRows(context.Background(), "скатерть", 300)
	_, _ = repo.CachedActs(context.Background(), "", 300)
	if len(recorder.sql) != 3 {
		t.Fatalf("запросов %d", len(recorder.sql))
	}
	byCode, byProduct, acts := recorder.sql[0], recorder.sql[1], recorder.sql[2]
	if !strings.Contains(byCode, "d.code LIKE '630251%'") || !strings.Contains(byCode, "DISTINCT ON (d.code)") || !strings.Contains(byCode, "ORDER BY saved_at DESC") {
		t.Errorf("по коду: %s", byCode)
	}
	if !strings.Contains(byProduct, "d.product ILIKE '%скатерть%'") {
		t.Errorf("по товару: %s", byProduct)
	}
	for _, want := range []string{"url NOT LIKE 'https://tnved.info/Goodshelp/?Code=%'", "url NOT LIKE '%://www.ifcg.ru/%'", "url NOT LIKE '%://nsi.eaeunion.org/%'"} {
		if !strings.Contains(acts, want) {
			t.Errorf("акты: нет %q в %s", want, acts)
		}
	}
}
