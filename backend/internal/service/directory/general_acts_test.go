package directory

import (
	"testing"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
)

// Перечень к ТР ЕАЭС 048/2019 (вступает в силу не ранее 01.09.2028) не участвует в поиске до этой даты —
// ни из основного списка, ни из справки; повторы адресов из справок не дублируются.
func TestGeneralActsInForce(t *testing.T) {
	const tr048 = "https://adilet.zan.kz/rus/docs/H22EK000023"
	rows := []repository.GeneralAct{
		{URL: tr048, Title: "из справки"},
		{URL: SGRActURL, Title: "повтор"},
		{URL: "https://adilet.zan.kz/rus/docs/H14EK000076", Title: "Решение Коллегии ЕЭК №76 (ТР ТС 023/2011)"},
	}
	count := func(acts []WebLink, address string) int {
		n := 0
		for _, act := range acts {
			if act.URL == address {
				n++
			}
		}
		return n
	}
	before := generalActsOn(rows, time.Date(2028, time.August, 31, 23, 0, 0, 0, time.UTC))
	if count(before, tr048) != 0 {
		t.Error("ТР ЕАЭС 048/2019 must not be searched before 2028-09-01")
	}
	if count(before, SGRActURL) != 1 || count(before, "https://adilet.zan.kz/rus/docs/H14EK000076") != 1 || len(before) != len(generalActs) {
		t.Errorf("acts before: %d", len(before))
	}
	after := generalActsOn(rows, time.Date(2028, time.September, 1, 0, 0, 0, 0, time.UTC))
	if count(after, tr048) != 1 || len(after) != len(generalActs)+1 {
		t.Errorf("acts after: %d", len(after))
	}
}
