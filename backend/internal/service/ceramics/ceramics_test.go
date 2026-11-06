package ceramics

import (
	"strings"
	"testing"

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

func TestFormatCeramicSizeLong(t *testing.T) {
	for value, want := range map[string]string{"1": "1 дюйм", "3": "3 дюйма", "7": "7 дюймов", "11": "11 дюймов", "21": "21 дюйм", "7.5": "7.5 дюйма"} {
		if got := formatCeramicSizeLong(CeramicSize{Value: value, Unit: SizeUnitInch}); got != want {
			t.Errorf("%s → %q, want %q", value, got, want)
		}
	}
}
