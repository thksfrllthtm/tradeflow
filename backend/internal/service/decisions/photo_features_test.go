package decisions

import (
	"strings"
	"testing"
)

func TestFeatureSimilarityPrefersSameKindAndMaterial(t *testing.T) {
	gloves := PhotoFeatures{Kind: "перчатки", Material: []string{"трикотаж", "полиуретан"}, Shape: []string{"пятипалые", "манжета-резинка"}, Color: []string{"серый"}, Features: []string{"покрытие ладони", "рабочие"}}
	sameOtherShot := PhotoFeatures{Kind: "перчатки рабочие", Material: []string{"трикотажное полотно", "полиуретановое покрытие"}, Shape: []string{"манжета на резинке"}, Color: []string{"серо-черный"}, Features: []string{"покрытие ладоней и пальцев"}}
	nitrile := PhotoFeatures{Kind: "перчатки", Material: []string{"нитрил"}, Shape: []string{"пятипалые"}, Color: []string{"синий"}, Features: []string{"одноразовые", "медицинские"}}
	mug := PhotoFeatures{Kind: "кружка", Material: []string{"керамика"}, Shape: []string{"с ручкой"}, Color: []string{"серый"}}

	same := FeatureSimilarity(gloves, sameOtherShot)
	other := FeatureSimilarity(gloves, nitrile)
	unrelated := FeatureSimilarity(gloves, mug)
	if !(same > other && other > unrelated) {
		t.Fatalf("order: same %.2f, nitrile %.2f, mug %.2f", same, other, unrelated)
	}
	if same*100 < SimilarMinScore {
		t.Fatalf("the same gloves on another photo must be offered: %.2f", same)
	}
	if unrelated*100 >= SimilarMinScore {
		t.Fatalf("a mug must not be offered for gloves: %.2f", unrelated)
	}
	if FeatureSimilarity(PhotoFeatures{}, gloves) != 0 {
		t.Fatal("empty description is not similar to anything")
	}
}

// Описания ИИ двух разных белых кружек: подробное описание набора не должно утопить похожесть.
func TestFeatureSimilarityOffersSameKindWithVerboseDescription(t *testing.T) {
	stored := PhotoFeatures{Kind: "кружка", Material: []string{"керамика"}, Shape: []string{"цилиндрическая", "расширяется кверху", "с ручкой"}, Features: []string{"с ложкой", "гладкая", "для напитков"}, Color: []string{"белый"}}
	set := PhotoFeatures{Kind: "кружки", Material: []string{"керамика"}, Shape: []string{"цилиндрическая", "с ручкой", "сужение к низу"}, Features: []string{"глянцевая поверхность", "рельефный рисунок", "набор из четырёх штук"}, Color: []string{"белый"}}
	bowl := PhotoFeatures{Kind: "миска", Material: []string{"керамика"}, Shape: []string{"круглая", "глубокая"}, Features: []string{"гладкая", "глянцевая поверхность"}, Color: []string{"белый"}}
	if score := FeatureSimilarity(set, stored); score*100 < SimilarMinScore {
		t.Fatalf("another white ceramic mug must be offered: %.2f", score)
	}
	if score := FeatureSimilarity(bowl, stored); score*100 >= SimilarMinScore {
		t.Fatalf("a bowl must not be offered for a mug: %.2f", score)
	}
}

func TestPhotoSimilarityLooseThreshold(t *testing.T) {
	base := "f0f0f0f0f0f0f0f0"
	// 8 отличающихся бит — уже не «тот же снимок» (до 6), но похожий.
	near := "0ff0f0f0f0f0f0f0"
	if photoHashDistance(base, near) != 8 {
		t.Fatalf("distance %d", photoHashDistance(base, near))
	}
	score, ok := photoSimilarity([]string{base}, []string{near})
	if !ok || score <= 0.7 || score >= 0.8 {
		t.Fatalf("near photo: %.2f %v", score, ok)
	}
	if _, ok := photoSimilarity([]string{base}, []string{"0f0f0f0f0f0f0f0f"}); ok {
		t.Fatal("inverted picture must not be similar")
	}
}

func TestPhotoFeaturesSummaryAndParse(t *testing.T) {
	features := PhotoFeatures{Kind: "перчатки", Material: []string{"трикотаж", " "}, Color: []string{"серый"}, Features: []string{"покрытие ладони"}}
	if got := features.Summary(); got != "перчатки · трикотаж · покрытие ладони · серый" {
		t.Fatalf("summary %q", got)
	}
	if _, ok := ParsePhotoFeatures([]byte("null")); ok {
		t.Fatal("null is no description")
	}
	parsed, ok := ParsePhotoFeatures([]byte(`{"kind":"кружка","material":["керамика"]}`))
	if !ok || !strings.Contains(parsed.Summary(), "керамика") {
		t.Fatalf("parsed %+v %v", parsed, ok)
	}
}
