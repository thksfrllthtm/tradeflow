package decisions

import (
	"encoding/json"
	"testing"
)

func TestDecisionRowValidatesAndHashesPhotos(t *testing.T) {
	photo := encodeTestImage(t, 200, 150, true, false)
	row, err := decisionRow(CodeDecisionInput{
		InvoiceName: " Перчатки ", Code: "6116 10 200 0", AgentCode: "4015190000", Photos: []string{photo},
		RejectedCodes: []RejectedCode{{Code: "4015190000", Why: "резина"}, {Code: "6116102000", Why: "тот же код"}}, UnitNetKg: "0,07",
	}, "agent")
	if err != nil {
		t.Fatal(err)
	}
	var hashes []string
	var rejected []RejectedCode
	_ = json.Unmarshal(row.PhotoHashes, &hashes)
	_ = json.Unmarshal(row.RejectedCodes, &rejected)
	if row.Code != "6116102000" || row.InvoiceName != "Перчатки" || row.Status != "pending" || len(hashes) != 1 || len(rejected) != 1 || row.UnitNetKg.String() != "0.07" {
		t.Fatalf("row: %+v hashes %v rejected %v", row, hashes, rejected)
	}
	if _, err := decisionRow(CodeDecisionInput{InvoiceName: "x", Code: "12345"}, "manual"); err == nil {
		t.Fatal("short code accepted")
	}
	if _, err := decisionRow(CodeDecisionInput{Code: "6116102000"}, "manual"); err == nil {
		t.Fatal("decision without any name accepted")
	}
	if _, err := decisionRow(CodeDecisionInput{InvoiceName: "x", Code: "6116102000", Photos: []string{"data:text/html;base64,AAAA"}}, "manual"); err == nil {
		t.Fatal("non-image photo accepted")
	}
}

func TestDecisionImportKeyIgnoresSpacesAndCase(t *testing.T) {
	a := decisionImportKey("8301100000", "Замок висячий  / 巨形锁", "", "巨形锁")
	b := decisionImportKey("8301100000", "замок висячий / 巨形锁", "", "巨形锁")
	if a != b || a == decisionImportKey("8301600009", "Замок висячий / 巨形锁", "", "巨形锁") {
		t.Fatal("import key")
	}
}

// Тот же код и наименование, но другое фото (16 разных отвёрток «Отвертка / 螺丝刀») — разные записи.
func TestImportDuplicateComparesPhotos(t *testing.T) {
	first := []string{PhotoHash(encodeTestImage(t, 200, 150, true, false))}
	other := []string{PhotoHash(encodeTestImage(t, 200, 150, true, true))}
	saved := [][]string{first}
	if !importDuplicate(saved, first) {
		t.Fatal("same photo must be a duplicate")
	}
	if importDuplicate(saved, other) {
		t.Fatal("another photo must not be a duplicate")
	}
	if !importDuplicate(saved, nil) || !importDuplicate([][]string{nil}, other) {
		t.Fatal("without a photo on one side the name decides")
	}
	if importDuplicate(nil, first) {
		t.Fatal("nothing saved yet")
	}
}

func TestOptionalDecimalTextForPriceEdit(t *testing.T) {
	price, err := optionalDecimalText(" 0,5 ", "unitPriceUsd")
	if err != nil || price == nil || price.String() != "0.5" {
		t.Fatalf("price %v err %v", price, err)
	}
	// Пустая строка — цену стёрли.
	if price, err := optionalDecimalText("  ", "unitPriceUsd"); err != nil || price != nil {
		t.Fatalf("empty: %v %v", price, err)
	}
	for _, bad := range []string{"-1", "abc", "100000000"} {
		if _, err := optionalDecimalText(bad, "unitPriceUsd"); err == nil {
			t.Fatalf("%q accepted", bad)
		}
	}
}
