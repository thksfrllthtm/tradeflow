package decisions

import (
	"encoding/json"
	"testing"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
)

func savedDecision(id int64, code, final, invoice string, hashes ...string) repository.CodeDecision {
	raw, _ := json.Marshal(hashes)
	return repository.CodeDecision{ID: id, Code: code, FinalName: final, InvoiceName: invoice, PhotoHashes: raw, ItemID: "row-1", Status: "pending"}
}

// В одной смете тот же товар — тот же код, то же наименование (без регистра, «ё/е», знаков) и похожее
// чёрно-белое фото; другое фото того же наименования — другой товар.
func TestFindSameInEstimate(t *testing.T) {
	saved := []repository.CodeDecision{savedDecision(7, "9017800000", "Рулетка ёмкая, 5 м", "", "ffff0000ffff0000")}
	name := decisionNameKey("рулетка емкая 5 м", "")
	if match := findSame(saved, "9017800000", name, []string{"ffff0000ffff0001"}); match == nil || match.ID != 7 {
		t.Fatalf("тот же товар с почти тем же фото: %+v", match)
	}
	if match := findSame(saved, "9017800000", name, nil); match == nil {
		t.Fatal("без фото — по коду и наименованию")
	}
	if findSame(saved, "9017800000", name, []string{"0000ffff0000ffff"}) != nil {
		t.Fatal("другое фото — другой товар")
	}
	if findSame(saved, "9017809000", name, nil) != nil || findSame(saved, "9017800000", decisionNameKey("", "Рулетка 3 м"), nil) != nil {
		t.Fatal("другой код или наименование — другой товар")
	}
	// Наименование для декларации важнее названия из инвойса.
	if decisionNameKey("Рулетка", "卷尺") != "рулетка" || decisionNameKey("", "卷尺") != "卷尺" {
		t.Fatal("ключ наименования")
	}
}
