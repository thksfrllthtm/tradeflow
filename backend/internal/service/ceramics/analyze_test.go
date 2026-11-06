package ceramics

import (
	"context"
	"strings"
	"testing"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
)

func testContext() *ceramicsContext {
	row := func(mark, name, price string) repository.CeramicProduct {
		product, err := validateCeramicsProductInput(CeramicsProductInput{Mark: mark, Name: name, Price: price})
		if err != nil {
			panic(err)
		}
		return product
	}
	return newCeramicsContext([]repository.CeramicProduct{
		row("705-7DJ", "Керамическая тарелка салатница 7 дюймов/ 成人陶瓷沙拉盘7寸", "4.5"),
		row("705-9DJ", "Керамическая тарелка салатница 9 дюймов/ 成人陶瓷沙拉盘9寸", "5.0"),
		row("S-7A", "Керамическая тарелка салатница 7 дюймов", "4.5"),
		row("S-7B", "Керамическая тарелка салатница 7 дюймов", "4.50"),
		row("S-7C", "Керамическая тарелка салатница 7 дюймов", "5"),
		row("P-8", "Керамическая тарелка 8 дюймов/ 陶瓷平盘8寸", "3"),
		row("P-8B", "Керамическая тарелка 8 дюймов", "3.5"),
		row("B-8", "Керамическая тарелка табак 8 дюймов", "3.2"),
		row("T-1", "Керамический чайник/ 陶瓷茶壶", "12"),
		row("721-7", "Керамическая фруктовница 7 дюймов для взрослых/ 成年人陶瓷盘7高脚水果盘", "0,2"),
	})
}

func TestPriceFor(t *testing.T) {
	cc := testContext()
	salad := cc.typeByKey["керамическая тарелка салатница"]
	if got := cc.priceFor(salad, inch("7")); got.Status != CeramicsPriceMatched || got.Price != "4.5" || got.Total != 4 {
		t.Fatalf("7\" price = %+v", got)
	}
	if got := cc.priceFor(cc.typeByKey["керамическая тарелка"], inch("8")); got.Status != CeramicsPriceNeedReview || got.Price != "" {
		t.Fatalf("a tie must need review: %+v", got)
	}
	got := cc.priceFor(salad, inch("8"))
	if got.Status != CeramicsPriceNotFound || !strings.Contains(got.Message, `8"`) || len(got.AvailableSizes) != 2 || got.Name != "Керамическая тарелка салатница 8 дюймов" {
		t.Fatalf("a missing size must not borrow a neighbour price: %+v", got)
	}
	if got := cc.priceFor(cc.typeByKey["керамический чайник"], nil); got.Price != "12" || got.Name != "Керамический чайник" {
		t.Fatalf("unsized price = %+v", got)
	}
}

func TestAnalyzeFruitBowlOfTheBase(t *testing.T) {
	ai, prompts := fakeCeramicsAI(t, `{"candidates":[]}`, `{}`)
	service := &CeramicsService{ai: ai}
	// Same Chinese name and number as 721-7: type, size and price come from the base.
	result := service.analyzeOne(context.Background(), testContext(), CeramicsInputItem{Key: "k", Mark: "721-7B", ChineseName: "成年人陶瓷盘7高脚水果盘"})
	if len(*prompts) != 0 || result.Status != CeramicsAuto || result.Price.Price != "0.2" || result.DisplayName != "Керамическая фруктовница 7 дюймов для взрослых" {
		t.Fatalf("result = %+v", result)
	}
	// Another number: the type is known, the size is an assumption and 9" has no price.
	result = service.analyzeOne(context.Background(), testContext(), CeramicsInputItem{Key: "k", Mark: "721-9", ChineseName: "成年人陶瓷盘9高脚水果盘"})
	if len(*prompts) != 0 || result.ProductType != "Керамическая фруктовница для взрослых" || result.SizeStatus != CeramicsPartAssumed ||
		result.Size.key() != "9 inch" || result.Price.Status != CeramicsPriceNotFound || result.Status != CeramicsNeedReview ||
		result.DisplayName != "Керамическая фруктовница 9 дюймов для взрослых" {
		t.Fatalf("result = %+v", result)
	}
}

func TestAnalyzeUsesChineseNameMatchWithoutAI(t *testing.T) {
	ai, prompts := fakeCeramicsAI(t, `{"candidates":[]}`, `{}`)
	service := &CeramicsService{ai: ai}
	result := service.analyzeOne(context.Background(), testContext(), CeramicsInputItem{Key: "k", Mark: "NEW-705-9", ChineseName: "成人陶瓷沙拉盘9寸"})
	if len(*prompts) != 0 {
		t.Fatalf("the same Chinese name in the base must not call AI")
	}
	if result.ProductType != "Керамическая тарелка салатница" || result.Size == nil || result.Size.Value != "9" || result.SizeStatus != CeramicsPartMatched {
		t.Fatalf("result = %+v", result)
	}
	if result.Price.Price != "5" || result.Status != CeramicsAuto || result.DisplayName != "Керамическая тарелка салатница 9 дюймов" {
		t.Fatalf("price/status = %+v / %s / %s", result.Price, result.Status, result.DisplayName)
	}
}

func TestAnalyzeSizeConflictNeedsReview(t *testing.T) {
	ai, _ := fakeCeramicsAI(t, `{"candidates":[]}`, `{}`)
	service := &CeramicsService{ai: ai}
	// The name says 9寸, the series 705-{SIZE}DJ reads 11 from the mark.
	result := service.analyzeOne(context.Background(), testContext(), CeramicsInputItem{Key: "k", Mark: "705-11DJ", ChineseName: "成人陶瓷沙拉盘9寸"})
	if result.Size == nil || result.Size.Value != "9" || result.SizeStatus != CeramicsPartNeedReview || result.Status != CeramicsNeedReview || len(result.SizeOptions) != 2 {
		t.Fatalf("result = %+v", result)
	}
}

func TestAnalyzeSizeConfirmedBySeries(t *testing.T) {
	ai, _ := fakeCeramicsAI(t, `{"candidates":[]}`, `{}`)
	service := &CeramicsService{ai: ai}
	result := service.analyzeOne(context.Background(), testContext(), CeramicsInputItem{Key: "k", Mark: "705-7DJ", ChineseName: "成人陶瓷沙拉盘7寸"})
	confirmed := false
	for _, evidence := range result.Evidence {
		confirmed = confirmed || strings.Contains(evidence.Text, "Также совпадает с шаблоном марки 705-{SIZE}DJ")
	}
	if !confirmed || result.SizeStatus != CeramicsPartMatched || result.Status != CeramicsAuto || result.Price.Price != "4.5" {
		t.Fatalf("result = %+v", result)
	}
}

func TestAnalyzeComparesSimilarTypes(t *testing.T) {
	ai, prompts := fakeCeramicsAI(t,
		`{"candidates":[{"productType":"Керамическая тарелка","confidence":0.8,"reason":"на фото тарелка"}]}`,
		`{"decision":"MATCHED","productType":"Керамическая тарелка салатница","confidence":0.85,"reason":"沙拉盘 — салатная","sources":["chinese_name"],"ranking":[{"productType":"Керамическая тарелка салатница","confidence":0.85},{"productType":"Керамическая тарелка","confidence":0.1}]}`)
	service := &CeramicsService{ai: ai}
	result := service.analyzeOne(context.Background(), testContext(), CeramicsInputItem{Key: "k", Mark: "NEW-1", ChineseName: "陶瓷沙拉盘子7寸"})
	if len(*prompts) != 2 {
		t.Fatalf("expected two AI stages, got %d", len(*prompts))
	}
	if result.ProductType != "Керамическая тарелка салатница" || result.Price.Price != "4.5" || result.Status != CeramicsAuto {
		t.Fatalf("result = %+v", result)
	}
	if len(result.TypeCandidates) < 3 {
		t.Fatalf("similar types (салатница, табак) must be compared: %+v", result.TypeCandidates)
	}
}

func TestAnalyzeUnsureNeedsReview(t *testing.T) {
	ai, _ := fakeCeramicsAI(t,
		`{"candidates":[{"productType":"Керамическая тарелка","confidence":0.6},{"productType":"Керамическая тарелка табак","confidence":0.5}]}`,
		`{"decision":"UNSURE","productType":"","confidence":0.5,"reason":"по фото не отличить"}`)
	service := &CeramicsService{ai: ai}
	result := service.analyzeOne(context.Background(), testContext(), CeramicsInputItem{Key: "k", Mark: "X-1", ChineseName: "陶瓷盘8寸"})
	if result.Status != CeramicsNeedReview || result.TypeStatus != CeramicsPartNeedReview || result.ProductType != "" || result.Price.Status != CeramicsPriceNotChecked {
		t.Fatalf("result = %+v", result)
	}
}

func TestAnalyzeRejectsInventedType(t *testing.T) {
	ai, _ := fakeCeramicsAI(t, `{"candidates":[{"productType":"Керамическая миска","confidence":0.99}]}`, `{}`)
	service := &CeramicsService{ai: ai}
	result := service.analyzeOne(context.Background(), testContext(), CeramicsInputItem{Key: "k", Mark: "X-2", ChineseName: "陶瓷碗"})
	if result.Status != CeramicsNotDetermined || result.ProductType != "" {
		t.Fatalf("a type outside the base must not be accepted: %+v", result)
	}
}

func TestAnalyzeSizeFromMarkSeriesIsAssumption(t *testing.T) {
	ai, _ := fakeCeramicsAI(t, `{"candidates":[{"productType":"Керамическая тарелка салатница","confidence":0.9}]}`, `{"decision":"MATCHED","productType":"Керамическая тарелка салатница","confidence":0.9}`)
	service := &CeramicsService{ai: ai}
	// The series 705-{SIZE}DJ is known from 705-7DJ and 705-9DJ; the name has no size.
	result := service.analyzeOne(context.Background(), testContext(), CeramicsInputItem{Key: "k", Mark: "705-8DJ", ChineseName: "沙拉盘"})
	if result.Size == nil || result.Size.Value != "8" || result.SizeStatus != CeramicsPartAssumed || result.Status != CeramicsNeedReview {
		t.Fatalf("size from a series must be an assumption for review: %+v", result)
	}
	if result.Price.Status != CeramicsPriceNotFound {
		t.Fatalf("8\" has no price in the base: %+v", result.Price)
	}
}
