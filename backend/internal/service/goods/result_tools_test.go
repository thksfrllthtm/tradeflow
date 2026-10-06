package goods

import (
	"encoding/json"
	"testing"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
)

// Подбор под бюджет и сверка по ИТП сохраняются в черновике сметы как есть.
func TestDraftPatchKeepsResultTools(t *testing.T) {
	tools := `{"budget":{"amount":"6700","gapMin":"20","gapMax":"30","gapMode":"under","summary":{"status":"ok","budget":6700,"total":6674.06,"gap":25.94,"fixedRows":0,"evaluations":3},` +
		`"notes":{"0":"Штыковая лопата"},"explanations":[{"code":"8201100000","names":"Лопата","price":"0.3865","reason":"Штыковая лопата","rows":[1,3]}]},"itp":{"value":"0.6","netKg":"49495","rate":"454.98"}}`
	var change repository.GoodsDraftPatch
	if err := prepareInputPatch(map[string]json.RawMessage{"resultTools": json.RawMessage(tools)}, &change); err != nil {
		t.Fatal(err)
	}
	var saved struct {
		ResultTools GoodsResultTools `json:"resultTools"`
	}
	if err := json.Unmarshal(change.Input, &saved); err != nil {
		t.Fatal(err)
	}
	budget := saved.ResultTools.Budget
	if budget == nil || budget.Amount != "6700" || budget.Summary.Total != 6674.06 || budget.Notes["0"] != "Штыковая лопата" || len(budget.Explanations[0].Rows) != 2 || saved.ResultTools.ITP.Value != "0.6" {
		t.Fatalf("saved = %s", change.Input)
	}
	if err := prepareInputPatch(map[string]json.RawMessage{"resultTools": json.RawMessage(`{"budget":{"summary":"много"}}`)}, &change); err == nil {
		t.Fatal("испорченное состояние должно отклоняться")
	}
}
