package tnvedagent

import (
	"testing"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service/decisions"
)

func TestSortByVerdictPutsSameProductFirst(t *testing.T) {
	matches := []decisions.SimilarMatch{
		{Decision: decisions.CodeDecisionView{ID: 1}, Score: 90},
		{Decision: decisions.CodeDecisionView{ID: 2}, Score: 60, Verdict: "same"},
		{Decision: decisions.CodeDecisionView{ID: 3}, Score: 80, Verdict: "similar"},
		{Decision: decisions.CodeDecisionView{ID: 4}, Score: 70, Verdict: "same"},
	}
	sortByVerdict(matches)
	order := []int64{}
	for _, match := range matches {
		order = append(order, match.Decision.ID)
	}
	if order[0] != 4 || order[1] != 2 || order[2] != 3 || order[3] != 1 {
		t.Fatalf("order %v", order)
	}
}
