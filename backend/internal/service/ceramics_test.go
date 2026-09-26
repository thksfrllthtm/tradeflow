package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/shopspring/decimal"
)

func TestNormalizeCeramicMark(t *testing.T) {
	for input, want := range map[string]string{
		" 705-7dj ": "705-7DJ",
		"７０５－７ＤＪ":   "705-7DJ",
		"705–7 DJ":  "705-7DJ",
		"new-705-7": "NEW-705-7",
	} {
		if got := NormalizeCeramicMark(input); got != want {
			t.Errorf("NormalizeCeramicMark(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestExtractSizeMentions(t *testing.T) {
	cases := []struct {
		text     string
		want     []string
		compound bool
	}{
		{"成人陶瓷沙拉盘7寸", []string{"7 inch"}, false},
		{"成人陶瓷沙拉盘9寸", []string{"9 inch"}, false},
		{"陶瓷汤碗七寸", []string{"7 inch"}, false},
		{"陶瓷盘十二寸", []string{"12 inch"}, false},
		{"陶瓷盘 7.5英寸", []string{"7.5 inch"}, false},
		{"陶瓷杯350ml", []string{"350 ml"}, false},
		{"茶壶1.2升", []string{"1.2 l"}, false},
		{"Тарелка 9 дюймов", []string{"9 inch"}, false},
		{"Plate 10\"", []string{"10 inch"}, false},
		{"陶瓷盘20cm", []string{"20 cm"}, false},
		{"陶瓷盘20*15cm", nil, true},
		{"陶瓷餐具12头套装", nil, false},
		{"3 in 1 set", nil, false},
		{"A705-7寸", nil, false},
	}
	for _, tc := range cases {
		mentions, compound := extractSizeMentions(tc.text)
		got := []string{}
		for _, mention := range mentions {
			got = append(got, mention.Size.key())
		}
		if strings.Join(got, ",") != strings.Join(tc.want, ",") || compound != tc.compound {
			t.Errorf("extractSizeMentions(%q) = %v compound=%v, want %v compound=%v", tc.text, got, compound, tc.want, tc.compound)
		}
	}
}

func TestChineseBaseName(t *testing.T) {
	if chineseBaseName("成人陶瓷沙拉盘7寸") != chineseBaseName("成人陶瓷沙拉盘 9寸") {
		t.Fatal("names differing only by size must share the base name")
	}
	if got := chineseBaseName("成人陶瓷沙拉盘7寸"); got != "成人陶瓷沙拉盘" {
		t.Fatalf("base name = %q", got)
	}
}

func inch(value string) *CeramicSize { return &CeramicSize{Value: value, Unit: SizeUnitInch} }

func TestMarkSeries(t *testing.T) {
	rows := []ceramicCatalogEntry{
		{Mark: "705-7DJ", MarkKey: "705-7DJ", TypeKey: "a", Size: inch("7")},
		{Mark: "705-8DJ", MarkKey: "705-8DJ", TypeKey: "a", Size: inch("8")},
		{Mark: "705-9DJ", MarkKey: "705-9DJ", TypeKey: "a", Size: inch("9")},
		{Mark: "705-10DJ", MarkKey: "705-10DJ", TypeKey: "a", Size: inch("10")},
		{Mark: "607-7", MarkKey: "607-7", TypeKey: "b", Size: inch("7")},
	}
	series := buildMarkSeries(rows)
	suggestions := inferSizeFromMarkPattern("705-11DJ", series)
	if len(suggestions) != 1 || suggestions[0].Size.Value != "11" || suggestions[0].Template != "705-{SIZE}DJ" || suggestions[0].TypeKeys[0] != "a" {
		t.Fatalf("705-11DJ suggestions = %+v", suggestions)
	}
	// One mark of one size does not make a series: 607-9 gets no size from its digits.
	if got := inferSizeFromMarkPattern("607-9", series); len(got) != 0 {
		t.Fatalf("607-9 must not get a size, got %+v", got)
	}
	// 705 is not a size although it is a number of the mark.
	if got := inferSizeFromMarkPattern("999-7DJ", series); len(got) != 0 {
		t.Fatalf("999-7DJ is another series, got %+v", got)
	}
}

func TestPriceMode(t *testing.T) {
	prices := func(values ...string) []decimal.Decimal {
		result := []decimal.Decimal{}
		for _, value := range values {
			result = append(result, decimal.RequireFromString(value))
		}
		return result
	}
	mode, options := priceMode(prices("4.5", "4.50", "4.5", "5.0", "4.5"))
	if mode != "4.5" || options[0].Count != 4 || options[1].Price != "5" {
		t.Fatalf("mode = %q options = %+v", mode, options)
	}
	if mode, options := priceMode(prices("4.5", "4.5", "5.0", "5.0")); mode != "" || len(options) != 2 {
		t.Fatalf("a tie must not choose a price: mode = %q options = %+v", mode, options)
	}
}

func TestParseCeramicName(t *testing.T) {
	parsed := parseCeramicName("Керамическая фруктовница 7 дюймов для взрослых/ 成年人陶瓷盘7高脚水果盘")
	if parsed.ProductType != "Керамическая фруктовница для взрослых" || parsed.Chinese != "成年人陶瓷盘7高脚水果盘" ||
		parsed.Size == nil || parsed.Size.key() != "7 inch" || parsed.Template != "Керамическая фруктовница {SIZE} для взрослых" {
		t.Fatalf("parsed = %+v", parsed)
	}
	if got := ceramicRussianName(parsed.Template, parsed.ProductType, inch("9")); got != "Керамическая фруктовница 9 дюймов для взрослых" {
		t.Fatalf("name for 9\" = %q", got)
	}
	for name, want := range map[string]string{
		"Керамический чайник/ 陶瓷茶壶":       "Керамический чайник||陶瓷茶壶",
		"Тарелка 20 см 陶瓷盘20cm":           "Тарелка|20 cm|陶瓷盘20cm",
		"Тарелка/миска 8\"/ 陶瓷碗8寸":        "Тарелка/миска|8 inch|陶瓷碗8寸",
		"Керамическая салатница 9 дюймов": "Керамическая салатница|9 inch|",
	} {
		parsed := parseCeramicName(name)
		size := ""
		if parsed.Size != nil {
			size = parsed.Size.key()
		}
		if got := parsed.ProductType + "|" + size + "|" + parsed.Chinese; got != want {
			t.Errorf("%q → %q, want %q", name, got, want)
		}
	}
}

func TestValidateCeramicsProductInput(t *testing.T) {
	product, err := validateCeramicsProductInput(CeramicsProductInput{Mark: " 721-7 ", Name: "Керамическая фруктовница 7 дюймов для взрослых/ 成年人陶瓷盘7高脚水果盘", Price: "0,2"})
	if err != nil {
		t.Fatal(err)
	}
	if product.MarkKey != "721-7" || product.Price.String() != "0.2" || product.ProductTypeKey != "керамическая фруктовница для взрослых" || product.SizeValue.String() != "7" || *product.SizeUnit != SizeUnitInch {
		t.Fatalf("product = %+v", product)
	}
}

func TestBareSizeNumbers(t *testing.T) {
	for text, want := range map[string]string{
		"成年人陶瓷盘7高脚水果盘": "7",
		"陶瓷餐具12头套装":    "",
		"陶瓷盘7寸":        "",
		"A7陶瓷盘":        "",
		"陶瓷盘20*15":     "",
	} {
		if got := strings.Join(bareSizeNumbers(text), ","); got != want {
			t.Errorf("%s → %q, want %q", text, got, want)
		}
	}
}

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

func TestFormatCeramicSizeLong(t *testing.T) {
	for value, want := range map[string]string{"1": "1 дюйм", "3": "3 дюйма", "7": "7 дюймов", "11": "11 дюймов", "21": "21 дюйм", "7.5": "7.5 дюйма"} {
		if got := formatCeramicSizeLong(CeramicSize{Value: value, Unit: SizeUnitInch}); got != want {
			t.Errorf("%s → %q, want %q", value, got, want)
		}
	}
}

func TestParseCeramicsCandidatesRejectsUnknownTypes(t *testing.T) {
	allowed := []string{"Керамическая тарелка", "Керамическая тарелка салатница"}
	result, err := parseCeramicsCandidates("```json\n"+`{"candidates":[
		{"productType":"Керамическая миска","confidence":0.9,"reason":"x","sources":["image"]},
		{"productType":"керамическая  тарелка САЛАТНИЦА","confidence":"0.8","reason":"沙拉盘","sources":["chinese_name","image","bogus"]},
		{"productType":"Керамическая тарелка","confidence":55,"reason":"плоская"}
	]}`+"\n```", allowed, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Candidates) != 2 || result.Candidates[0].ProductType != "Керамическая тарелка салатница" || result.Candidates[1].Confidence != 0.55 {
		t.Fatalf("candidates = %+v", result.Candidates)
	}
	if strings.Join(result.Candidates[0].Sources, ",") != "chinese_name" {
		t.Fatalf("image must be dropped when no photo was sent: %v", result.Candidates[0].Sources)
	}
	if _, err := parseCeramicsCandidates("not json", allowed, false); !errors.Is(err, ErrAIBadResponse) {
		t.Fatalf("expected bad response, got %v", err)
	}
}

func TestParseCeramicsComparison(t *testing.T) {
	candidates := []string{"Керамическая тарелка", "Керамическая тарелка салатница"}
	result, err := parseCeramicsComparison(`{"decision":"MATCHED","productType":"Керамическая тарелка салатница","confidence":0.9,"reason":"沙拉盘"}`, candidates, true)
	if err != nil || !result.Matched || result.ProductType != "Керамическая тарелка салатница" {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	for _, answer := range []string{
		`{"decision":"MATCHED","productType":"Керамическая миска","confidence":0.95}`,
		`{"decision":"MATCHED","productType":"Керамическая тарелка","confidence":0.4}`,
		`{"decision":"UNSURE","productType":"","confidence":0.5}`,
	} {
		result, err := parseCeramicsComparison(answer, candidates, true)
		if err != nil || result.Matched {
			t.Fatalf("%s must not match: %+v, %v", answer, result, err)
		}
	}
}

// fakeCeramicsAI answers stage A with the given candidates and stage B with the given decision.
func fakeCeramicsAI(t *testing.T, stageA, stageB string) (*CeramicsAIClient, *[]string) {
	t.Helper()
	prompts := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Messages []struct {
				Content any `json:"content"`
			} `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&request)
		system, _ := request.Messages[0].Content.(string)
		prompts = append(prompts, system)
		content := stageA
		if strings.Contains(system, "из нескольких похожих") {
			content = stageB
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"content": content}}}})
	}))
	t.Cleanup(server.Close)
	return NewCeramicsAIClient(CeramicsAIConfig{APIKey: "test", Model: "test", BaseURL: server.URL}), &prompts
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
