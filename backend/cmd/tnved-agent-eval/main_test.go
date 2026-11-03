package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service/tnvedagent"
	"github.com/shopspring/decimal"
)

// Сравнение схем: строка на товар, под ней итог каждой схемы; у прерванной схемы вместо итога — пометка.
func TestPrintComparison(t *testing.T) {
	record := func(verdict, best string) tnvedagent.AgentEvalRecord {
		return tnvedagent.AgentEvalRecord{Name: "Секатор", Code: "8201500000", Verdict: verdict, BestCode: best, CostUSD: decimal.RequireFromString("0.0123"), Seconds: 40}
	}
	runs := []schemeRun{
		{Scheme: "P", Records: []tnvedagent.AgentEvalRecord{record("верно", "8201500000"), record("ошибка", "8203200000")}},
		{Scheme: "E", Records: []tnvedagent.AgentEvalRecord{record("среди вариантов", "8203200000")}},
	}
	var out bytes.Buffer
	printComparison(&out, runs)
	for _, want := range []string{
		"  1. Секатор → " + tnvedagent.FormatTNVEDCode("8201500000"),
		"P: верно ($0.0123, 40s)",
		"E: среди вариантов, лучший — " + tnvedagent.FormatTNVEDCode("8203200000"),
		"  2. Секатор",
		"P: ошибка, лучший — " + tnvedagent.FormatTNVEDCode("8203200000"),
		"E: прогон прерван",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("нет «%s»:\n%s", want, out.String())
		}
	}
}
