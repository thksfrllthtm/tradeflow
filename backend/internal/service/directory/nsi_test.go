package directory

import (
	"strings"
	"testing"
)

// Ответ API сборника (2026-11-04): коды с пробелами и неразрывными пробелами приводятся к 10 цифрам,
// решения группируются по коду, пример — страна, описание и обоснование.
func TestParseNSIDecisionsAndGroups(t *testing.T) {
	body := []byte(`[{"id":"a","data":{"CodeTNVED":"8467 29 800 0","Description":"Триммер садовый электрический сетевой с нейлоновой леской.","Country":{"id":"x","name":"РОССИЯ","type":"2021"},"Justification":"ОПИ 1 и 6"},"status":"actual"},
{"id":"b","data":{"CodeTNVED":"8424309000","Description":"Установка для удаления окалины.","Country":{"name":"КАЗАХСТАН"},"Justification":"ОПИ 1 и 6"}},
{"id":"c","data":{"CodeTNVED":"8467 29 800 0","Description":"Электрокоса бытовая.","Country":{"name":"БЕЛАРУСЬ"},"Justification":""}},
{"id":"d","data":{"CodeTNVED":"8467","Description":"код не полный","Country":{"name":"РОССИЯ"},"Justification":""}}]`)
	decisions, err := parseNSIDecisions(body)
	if err != nil || len(decisions) != 3 || decisions[0].Code != "8467298000" || decisions[0].Country != "РОССИЯ" || decisions[1].Code != "8424309000" {
		t.Fatalf("разбор: %v %+v", err, decisions)
	}
	groups := nsiGroups(decisions)
	if len(groups) != 2 || groups[0].Code != "8467298000" || groups[0].Count != 2 || groups[1].Code != "8424309000" || groups[1].Count != 1 {
		t.Fatalf("группы: %+v", groups)
	}
	example := groups[0].Examples[0]
	if example.Code != "8467298000" || !strings.HasPrefix(example.Text, "Триммер садовый") || !strings.Contains(example.Text, "Обоснование: ОПИ 1 и 6") ||
		example.Source != "предварительное решение · РОССИЯ (НСИ ЕАЭС)" || strings.Contains(groups[0].Examples[1].Text, "Обоснование") {
		t.Fatalf("пример: %+v", groups[0].Examples)
	}
	if within := groupsWithin(groups, "8424"); len(within) != 1 || within[0].Code != "8424309000" || len(groupsWithin(groups, "")) != 2 || len(groupsWithin(groups, "8467298000")) != 1 {
		t.Fatalf("отбор по позиции и коду: %+v", within)
	}
	if _, err := parseNSIDecisions([]byte(`{"type":"/problem-with-message"}`)); err == nil {
		t.Fatal("ошибка API — не список")
	}
	if NSIDecisionsURL("скатерть клеенка") != "https://nsi.eaeunion.org/portal/1999?searchText=%D1%81%D0%BA%D0%B0%D1%82%D0%B5%D1%80%D1%82%D1%8C+%D0%BA%D0%BB%D0%B5%D0%B5%D0%BD%D0%BA%D0%B0" {
		t.Fatalf("адрес: %s", NSIDecisionsURL("скатерть клеенка"))
	}
}
