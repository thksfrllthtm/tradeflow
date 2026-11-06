package decisions

import (
	"testing"
)

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
