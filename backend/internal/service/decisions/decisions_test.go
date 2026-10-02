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

func TestNormalizeDecisionNameAndSortMatches(t *testing.T) {
	if NormalizeDecisionName("  Перчатки, х/б — ЁЛКА!! ") != "перчатки х б елка" {
		t.Fatalf("got %q", NormalizeDecisionName("  Перчатки, х/б — ЁЛКА!! "))
	}
	matches := []SameProductMatch{
		{Decision: CodeDecisionView{ID: 1, Status: "pending"}, ByPhoto: true},
		{Decision: CodeDecisionView{ID: 2, Status: "verified"}, ByName: true},
		{Decision: CodeDecisionView{ID: 3, Status: "verified"}, ByPhoto: true},
	}
	sortMatches(matches)
	if matches[0].Decision.ID != 3 || matches[1].Decision.ID != 2 || matches[2].Decision.ID != 1 {
		t.Fatalf("order: %d %d %d", matches[0].Decision.ID, matches[1].Decision.ID, matches[2].Decision.ID)
	}
}

func TestDecisionNameVariantsSplitRussianAndChinese(t *testing.T) {
	variants := decisionNameVariants("Перчатки рабочие трикотажные покрытые резиной / 聚氨酯橡胶针织工作手套")
	want := map[string]bool{"перчатки рабочие трикотажные покрытые резиной": true, "聚氨酯橡胶针织工作手套": true}
	found := 0
	for _, variant := range variants {
		if want[variant] {
			found++
		}
	}
	if found != 2 {
		t.Fatalf("variants: %q", variants)
	}
	// Короткие части по отдельности («лом», «撬棍») слишком общие; узнаётся только целиком.
	if got := decisionNameVariants("Лом / 撬棍"); len(got) != 1 || got[0] != "лом 撬棍" {
		t.Fatalf("short parts must not match alone: %q", got)
	}
}

func TestAgreedVerifiedCodeNeedsOneCode(t *testing.T) {
	verified := func(code string) SameProductMatch {
		return SameProductMatch{Decision: CodeDecisionView{Status: "verified", Code: code}}
	}
	if code, ok := AgreedVerifiedCode([]SameProductMatch{verified("8205200000"), verified("8205200000"), {Decision: CodeDecisionView{Status: "pending", Code: "1111111111"}}}); !ok || code != "8205200000" {
		t.Fatalf("got %q %v", code, ok)
	}
	if _, ok := AgreedVerifiedCode([]SameProductMatch{verified("8203200009"), verified("8205598099")}); ok {
		t.Fatal("different verified codes must not skip AI")
	}
	if _, ok := AgreedVerifiedCode([]SameProductMatch{{Decision: CodeDecisionView{Status: "pending", Code: "8205200000"}}}); ok {
		t.Fatal("unverified match must not skip AI")
	}
}

func TestDecisionImportKeyIgnoresSpacesAndCase(t *testing.T) {
	a := decisionImportKey("8301100000", "Замок висячий  / 巨形锁", "", "巨形锁")
	b := decisionImportKey("8301100000", "замок висячий / 巨形锁", "", "巨形锁")
	if a != b || a == decisionImportKey("8301600009", "Замок висячий / 巨形锁", "", "巨形锁") {
		t.Fatal("import key")
	}
}

// Фото главнее наименования: запись с тем же наименованием, но другим фото, не даёт обойтись без ИИ.
func TestAgreedVerifiedCodeIgnoresNameMatchWithOtherPhoto(t *testing.T) {
	verified := SameProductMatch{Decision: CodeDecisionView{Status: "verified", Code: "8205200000"}, ByName: true, PhotoDiffers: true}
	if _, ok := AgreedVerifiedCode([]SameProductMatch{verified}); ok {
		t.Fatal("a name match with a different photo must not skip the agent")
	}
	verified.PhotoDiffers = false
	if code, ok := AgreedVerifiedCode([]SameProductMatch{verified}); !ok || code != "8205200000" {
		t.Fatalf("a name match without a photo conflict skips the agent: %q %v", code, ok)
	}
}
