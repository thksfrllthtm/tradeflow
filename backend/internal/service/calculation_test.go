package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/shopspring/decimal"
)

func TestCalculateItemUsesPairsAndAutomaticNet(t *testing.T) {
	item := calculateItem(CalculationItemInput{
		ProductID: 1, StandardName: "Перчатки", GrossKG: "3100", GrossWeightUnit: "kg", Places: "100", PairsPerPlace: "10", Coefficient: "0.65",
		DirectorySnapshot: map[string]any{"duty": map[string]any{"type": "percentage", "percent": "0"}, "vat": map[string]any{"type": "exempt"}},
	}, decimal.NewFromInt(500), decimal.NewFromInt(550), decimal.NewFromFloat(.3))
	if item.Pairs != "1000" {
		t.Fatalf("pairs = %s", item.Pairs)
	}
	if item.NetKG != "3070" || item.NetSource != "calculated" {
		t.Fatalf("net = %s source=%s", item.NetKG, item.NetSource)
	}
	if item.CostUSD != "1995.50" || item.PairPriceUSD != "1.9955" {
		t.Fatalf("cost=%s pair=%s", item.CostUSD, item.PairPriceUSD)
	}
}

func TestCalculateItemDerivesPairsPerPlace(t *testing.T) {
	item := calculateItem(CalculationItemInput{StandardName: "Перчатки", Pairs: "170", Places: "85", GrossKG: "10", GrossWeightUnit: "kg", NetKG: "10", NetWeightUnit: "kg", Coefficient: "1", DirectorySnapshot: map[string]any{
		"duty": map[string]any{"type": "unknown"}, "vat": map[string]any{"type": "unknown"},
	}}, decimal.NewFromInt(500), decimal.Zero, decimal.NewFromFloat(.3))
	if item.PairsPerPlace != "2" {
		t.Fatalf("pairs per place = %s", item.PairsPerPlace)
	}
}

func TestNonPercentageDutyIsUnsupported(t *testing.T) {
	item := calculateItem(CalculationItemInput{StandardName: "Перчатки", GrossKG: "10", GrossWeightUnit: "kg", NetKG: "10", NetWeightUnit: "kg", NetSource: "excel", Pairs: "100", Coefficient: "1", DirectorySnapshot: map[string]any{
		"duty": map[string]any{"type": "combined_minimum", "percent": "5", "amount": "0.001", "currency": "USD", "unit": "пара"},
		"vat":  map[string]any{"type": "percentage", "percent": "12"},
	}}, decimal.NewFromInt(100), decimal.NewFromInt(0), decimal.NewFromFloat(.3))
	if item.DutyKZT != "" || item.VATKZT != "" || item.Status != "needs_review" {
		t.Fatalf("non-percentage duty must not produce an amount: duty=%q vat=%q status=%s", item.DutyKZT, item.VATKZT, item.Status)
	}
	found := false
	for _, issue := range item.Errors {
		if strings.Contains(issue, "не поддерживается") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected unsupported-scheme error, got %v", item.Errors)
	}
}

func TestCalculationDecimalInputKeepsSmallThreeDigitFraction(t *testing.T) {
	value, err := parseDecimal("0.041")
	if err != nil || value.String() != "0.041" {
		t.Fatalf("small decimal = %s, err=%v", value, err)
	}
	if _, err := parseDecimal("1,000"); err == nil {
		t.Fatal("ambiguous three-digit fraction should be rejected")
	}
}

func TestUnknownDutyDoesNotProduceVATAmount(t *testing.T) {
	item := calculateItem(CalculationItemInput{StandardName: "Перчатки", GrossKG: "10", GrossWeightUnit: "kg", NetKG: "10", NetWeightUnit: "kg", Pairs: "10", Coefficient: "1", DirectorySnapshot: map[string]any{
		"duty": map[string]any{"type": "unknown"}, "vat": map[string]any{"type": "percentage", "percent": "12"},
	}}, decimal.NewFromInt(100), decimal.Zero, decimal.NewFromFloat(.3))
	if item.DutyKZT != "" || item.VATKZT != "" || item.Status != "needs_review" {
		t.Fatalf("unknown duty should block dependent payments: duty=%q vat=%q status=%s", item.DutyKZT, item.VATKZT, item.Status)
	}
}

func TestCalculatedNetRecomputesWhenSourceInputsChange(t *testing.T) {
	item := calculateItem(CalculationItemInput{
		StandardName: "Перчатки", GrossKG: "3100", GrossWeightUnit: "kg", NetKG: "3070", NetWeightUnit: "kg", NetSource: "calculated",
		Places: "100", PairsPerPlace: "10", Coefficient: "0.65",
		DirectorySnapshot: map[string]any{"duty": map[string]any{"type": "exempt"}, "vat": map[string]any{"type": "exempt"}},
	}, decimal.NewFromInt(500), decimal.Zero, decimal.NewFromFloat(.4))
	if item.NetKG != "3060" || item.NetSource != "calculated" {
		t.Fatalf("calculated net was not refreshed: net=%s source=%s", item.NetKG, item.NetSource)
	}
}

func TestMonetaryValuesUseDisplayPrecisionForDependentCalculations(t *testing.T) {
	item := calculateItem(CalculationItemInput{
		StandardName: "Перчатки", GrossKG: "10", GrossWeightUnit: "kg", NetKG: "10", NetWeightUnit: "kg", NetSource: "excel",
		Pairs: "96000", Coefficient: "0.9", DeliveryUSD: "60.6356968215158924", DeliveryIncluded: true,
		DirectorySnapshot: map[string]any{"duty": map[string]any{"type": "unknown"}, "vat": map[string]any{"type": "unknown"}},
	}, decimal.NewFromFloat(447.88), decimal.Zero, decimal.NewFromFloat(.3))
	if item.CostUSD != "9.60" || item.PairPriceUSD != "0.0001" || item.DeliveryUSD != "60.64" || item.DeliveryKZT != "27159.44" {
		t.Fatalf("rounded values = cost=%s pair=%s delivery=%s deliveryKzt=%s", item.CostUSD, item.PairPriceUSD, item.DeliveryUSD, item.DeliveryKZT)
	}
}

func TestCalculatedNetIsStoredInKilograms(t *testing.T) {
	item := calculateItem(CalculationItemInput{
		StandardName: "Перчатки", GrossKG: "100", GrossWeightUnit: "kg", Places: "10", PackageWeightKG: "0.3",
		Coefficient: "1", DirectorySnapshot: map[string]any{"duty": map[string]any{"type": "percentage", "percent": "0"}, "vat": map[string]any{"type": "exempt"}},
	}, decimal.NewFromInt(500), decimal.Zero, decimal.NewFromFloat(.3))
	if item.NetKG != "97" || item.NetWeightUnit != "kg" || item.NetSource != "calculated" {
		t.Fatalf("calculated net = %s %s source=%s", item.NetKG, item.NetWeightUnit, item.NetSource)
	}
	if len(item.Errors) != 0 {
		t.Fatalf("calculated net produced blocking errors: %v", item.Errors)
	}
}

func TestExplicitNetIsNotReplacedByAutomaticFormula(t *testing.T) {
	item := calculateItem(CalculationItemInput{
		StandardName: "Перчатки", GrossKG: "100", GrossWeightUnit: "kg", NetKG: "95", NetWeightUnit: "kg", NetSource: "excel", Places: "10", PackageWeightKG: "0.3",
		Coefficient: "1", DirectorySnapshot: map[string]any{"duty": map[string]any{"type": "unknown"}, "vat": map[string]any{"type": "unknown"}},
	}, decimal.NewFromInt(500), decimal.Zero, decimal.NewFromFloat(.3))
	if item.NetKG != "95" || item.NetWeightUnit != "kg" || item.NetSource != "excel" {
		t.Fatalf("explicit net was replaced: net=%s unit=%s source=%s", item.NetKG, item.NetWeightUnit, item.NetSource)
	}
}

func TestInvalidNetUnitDoesNotSilentlyBecomeKilograms(t *testing.T) {
	item := calculateItem(CalculationItemInput{
		StandardName: "Перчатки", GrossKG: "100", GrossWeightUnit: "kg", NetKG: "95", NetWeightUnit: "", NetSource: "excel", Places: "10",
		Coefficient: "1", DirectorySnapshot: map[string]any{"duty": map[string]any{"type": "unknown"}, "vat": map[string]any{"type": "unknown"}},
	}, decimal.NewFromInt(500), decimal.Zero, decimal.NewFromFloat(.3))
	if item.NetKG != "" || len(item.Errors) == 0 {
		t.Fatalf("unknown net unit was accepted: net=%s errors=%v", item.NetKG, item.Errors)
	}
}

func TestDeliveryAllocationTotalsMatchRoundedPartyAmount(t *testing.T) {
	items := []CalculationItem{
		{CalculationItemInput: CalculationItemInput{GrossKG: "42945"}},
		{CalculationItemInput: CalculationItemInput{GrossKG: "1"}},
	}
	total := decimal.NewFromInt(840)
	allocateDelivery(items, &total, decimal.NewFromFloat(447.88))
	var usd, kzt decimal.Decimal
	for _, item := range items {
		usd = usd.Add(mustDecimal(item.DeliveryUSD))
		kzt = kzt.Add(mustDecimal(item.DeliveryKZT))
	}
	if usd.StringFixed(2) != "840.00" || kzt.StringFixed(2) != "376219.20" {
		t.Fatalf("delivery totals = %s USD / %s KZT", usd, kzt)
	}
}

func TestDeliveryModeRequiresExplicitActiveValue(t *testing.T) {
	if !hasDeliveryInput(CalculationInput{Items: []CalculationItemInput{{DeliveryUSD: "840.00"}}}) {
		t.Fatal("stale row delivery should require explicit mode")
	}
	if hasDeliveryInput(CalculationInput{}) {
		t.Fatal("empty delivery should remain unconfigured")
	}
}

func TestAuditDeliveryAllocationForTwelveRows(t *testing.T) {
	weights := []string{"3100", "3500", "1600", "8000", "8000", "4600", "4600", "4370", "608", "1365", "1890", "1312"}
	items := make([]CalculationItem, len(weights))
	for i, weight := range weights {
		items[i].GrossKG = weight
		items[i].GrossWeightUnit = "kg"
	}
	total := decimal.NewFromInt(840)
	allocateDelivery(items, &total, decimal.NewFromFloat(447.88))
	var usd, kzt decimal.Decimal
	for _, item := range items {
		usd = usd.Add(mustDecimal(item.DeliveryUSD))
		kzt = kzt.Add(mustDecimal(item.DeliveryKZT))
	}
	if usd.StringFixed(2) != "840.00" || kzt.StringFixed(2) != "376219.20" {
		t.Fatalf("audit allocation totals = %s USD / %s KZT", usd, kzt)
	}
}

func TestLegacyResultReadDoesNotApplyNewRounding(t *testing.T) {
	items, _ := json.Marshal([]CalculationItem{{CalculationItemInput: CalculationItemInput{ProductID: 1, DeliveryUSD: "2.3456"}, CostUSD: "1.2345", DeliveryKZT: "1049.99", Status: "needs_review"}})
	result, err := resultFromModel(repository.CustomsCalculation{
		ID: 1, EstimateID: 1, AssessmentDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), USDKZT: decimal.NewFromInt(447), WagonTariffUSD: decimal.NewFromInt(420), PackageWeightKG: decimal.NewFromFloat(.3), Items: repository.JSON(items), UpdatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Items[0].CostUSD != "1.2345" || result.FormulaVersion != "legacy/unknown" {
		t.Fatalf("legacy snapshot was changed: cost=%s formula=%s", result.Items[0].CostUSD, result.FormulaVersion)
	}
}

func TestFeeIncludedFalseIsRecognizedOnlyWhenExplicit(t *testing.T) {
	var omitted CalculationInput
	if err := json.Unmarshal([]byte(`{"usdKzt":"1"}`), &omitted); err != nil {
		t.Fatal(err)
	}
	if omitted.feeIncludedSet {
		t.Fatal("omitted feeIncluded should not be treated as a switch")
	}
	var disabled CalculationInput
	if err := json.Unmarshal([]byte(`{"feeIncluded":false}`), &disabled); err != nil {
		t.Fatal(err)
	}
	if !disabled.feeIncludedSet || disabled.FeeIncluded {
		t.Fatalf("explicit false was not retained: set=%v value=%v", disabled.feeIncludedSet, disabled.FeeIncluded)
	}
}

func TestDefaultDeliveryInclusionRequiresExplicitPartyMode(t *testing.T) {
	if !defaultDeliveryInCustoms(nil) {
		t.Fatal("a new calculation should include separately entered delivery by default")
	}
	if !defaultDeliveryInCustoms([]CalculationItemInput{{ProductID: 1}, {ProductID: 2}}) {
		t.Fatal("rows without an explicit choice should use the new default")
	}
	if defaultDeliveryInCustoms([]CalculationItemInput{{ProductID: 1, DeliveryIncluded: true}}) {
		t.Fatal("an explicit row choice must not be replaced by the default")
	}
}

func TestExtraExpensesAreIncludedInCustomsValue(t *testing.T) {
	base := CalculationItemInput{
		StandardName: "Перчатки", GrossKG: "100", GrossWeightUnit: "kg", NetKG: "100", NetWeightUnit: "kg", NetSource: "excel",
		Pairs: "100", Coefficient: "1",
		DirectorySnapshot: map[string]any{
			"duty": map[string]any{"type": "percentage", "percent": "10"},
			"vat":  map[string]any{"type": "percentage", "percent": "16"},
		},
	}

	withoutExtra := calculateItem(base, decimal.NewFromInt(100), decimal.Zero, decimal.NewFromFloat(.3))
	if withoutExtra.CustomsValueKZT != "10000.00" || withoutExtra.DutyKZT != "1000.00" || withoutExtra.VATKZT != "1760.00" {
		t.Fatalf("without extra expenses = customs %s, duty %s, vat %s", withoutExtra.CustomsValueKZT, withoutExtra.DutyKZT, withoutExtra.VATKZT)
	}

	base.ExtraExpenses = "50"
	withExtra := calculateItem(base, decimal.NewFromInt(100), decimal.Zero, decimal.NewFromFloat(.3))
	if withExtra.CustomsValueKZT != "15000.00" || withExtra.DutyKZT != "1500.00" || withExtra.VATKZT != "2640.00" {
		t.Fatalf("with extra expenses = customs %s, duty %s, vat %s", withExtra.CustomsValueKZT, withExtra.DutyKZT, withExtra.VATKZT)
	}
}

func TestDeliveryExcludedFromCustomsRemainsASeparateExpense(t *testing.T) {
	item := calculateItem(CalculationItemInput{
		StandardName: "Перчатки", GrossKG: "100", GrossWeightUnit: "kg", NetKG: "100", NetWeightUnit: "kg", NetSource: "excel",
		Pairs: "100", Coefficient: "1", DeliveryUSD: "30", DeliveryIncluded: false,
		DirectorySnapshot: map[string]any{
			"duty": map[string]any{"type": "percentage", "percent": "10"},
			"vat":  map[string]any{"type": "percentage", "percent": "16"},
		},
	}, decimal.NewFromInt(100), decimal.Zero, decimal.NewFromFloat(.3))
	if item.CustomsValueKZT != "10000.00" || item.DutyKZT != "1000.00" || item.VATKZT != "1760.00" || item.DeliveryKZT != "3000.00" {
		t.Fatalf("excluded delivery = customs %s, duty %s, vat %s, delivery %s", item.CustomsValueKZT, item.DutyKZT, item.VATKZT, item.DeliveryKZT)
	}
	payments := mustDecimal(item.DutyKZT).Add(mustDecimal(item.VATKZT))
	expenses := payments.Add(mustDecimal(item.DeliveryKZT)).Round(2)
	if expenses.StringFixed(2) != "5760.00" {
		t.Fatalf("excluded delivery was removed from expenses: %s", expenses)
	}
}

func TestDeliveryIsAnExpenseOnlyOnceAndAllocationIsRebuilt(t *testing.T) {
	snapshot := map[string]any{
		"duty": map[string]any{"type": "percentage", "percent": "10"},
		"vat":  map[string]any{"type": "percentage", "percent": "16"},
	}
	items := []CalculationItem{
		{CalculationItemInput: CalculationItemInput{StandardName: "A", GrossKG: "10", GrossWeightUnit: "kg", NetKG: "10", NetWeightUnit: "kg", NetSource: "excel", Pairs: "10", Coefficient: "1", DeliveryIncluded: true, DirectorySnapshot: snapshot}},
		{CalculationItemInput: CalculationItemInput{StandardName: "B", GrossKG: "20", GrossWeightUnit: "kg", NetKG: "20", NetWeightUnit: "kg", NetSource: "excel", Pairs: "20", Coefficient: "1", DeliveryIncluded: true, DirectorySnapshot: snapshot}},
	}
	usd := decimal.NewFromInt(100)
	firstDelivery := decimal.NewFromInt(30)
	allocateDelivery(items, &firstDelivery, usd)
	if items[0].DeliveryUSD != "10.00" || items[1].DeliveryUSD != "20.00" {
		t.Fatalf("initial delivery allocation = %s, %s", items[0].DeliveryUSD, items[1].DeliveryUSD)
	}

	// A changed party total rebuilds automatic row shares; the old values are
	// not treated as an additional delivery amount.
	secondDelivery := decimal.NewFromInt(60)
	allocateDelivery(items, &secondDelivery, usd)
	if items[0].DeliveryUSD != "20.00" || items[1].DeliveryUSD != "40.00" {
		t.Fatalf("rebuilt delivery allocation = %s, %s", items[0].DeliveryUSD, items[1].DeliveryUSD)
	}
	for i := range items {
		items[i].CustomsValueKZT, items[i].DutyKZT, items[i].VATKZT = "", "", ""
		warnings := []string{}
		net, pairs := mustDecimal(items[i].NetKG), mustDecimal(items[i].Pairs)
		calculateTariff(&items[i], &net, &pairs, nil, usd, decimal.Zero, &warnings)
	}
	fee := decimal.NewFromInt(300)
	allocateFee(items, &fee, true)
	for i := range items {
		updateVATWithFee(&items[i])
	}
	var deliveryKZT, duty, vat, feeTotal decimal.Decimal
	for _, item := range items {
		deliveryKZT = deliveryKZT.Add(mustDecimal(item.DeliveryKZT))
		duty = duty.Add(mustDecimal(item.DutyKZT))
		vat = vat.Add(mustDecimal(item.VATKZT))
		feeTotal = feeTotal.Add(mustDecimal(item.FeeKZT))
	}
	customsPayments := duty.Add(vat).Add(feeTotal).Round(2)
	expenses := customsPayments.Add(deliveryKZT).Round(2)
	if deliveryKZT.StringFixed(2) != "6000.00" || feeTotal.StringFixed(2) != "300.00" {
		t.Fatalf("allocated totals = delivery %s, fee %s", deliveryKZT, feeTotal)
	}
	if expenses.Sub(customsPayments).StringFixed(2) != "6000.00" {
		t.Fatalf("delivery was not added exactly once: payments=%s expenses=%s", customsPayments, expenses)
	}
}

func TestDeliveryDistributionWithoutPositiveGrossIsBlocking(t *testing.T) {
	errorsList, warnings := splitCalculationIssues([]string{"Распределение доставки: невозможно распределить положительную доставку без положительного брутто"})
	if len(errorsList) != 1 || len(warnings) != 0 {
		t.Fatalf("delivery distribution issue = errors %v, warnings %v", errorsList, warnings)
	}
}

func TestKazakhstanCustomsControlExample(t *testing.T) {
	snapshot := map[string]any{
		"duty": map[string]any{"type": "percentage", "percent": "12"},
		"vat":  map[string]any{"type": "percentage", "percent": "16"},
	}
	usd := decimal.NewFromFloat(444.88)
	inputs := []CalculationItemInput{
		{StandardName: "Товар 1", CostInput: "2764.80", CostCurrency: "USD", DirectorySnapshot: snapshot},
		{StandardName: "Товар 2", CostInput: "3124.80", CostCurrency: "USD", DirectorySnapshot: snapshot},
	}
	items := make([]CalculationItem, len(inputs))
	for i, input := range inputs {
		items[i] = calculateItem(input, usd, decimal.Zero, decimal.NewFromFloat(.3))
		if len(items[i].Errors) != 0 || len(items[i].Warnings) != 0 {
			t.Fatalf("control row %d issues: errors=%v warnings=%v", i+1, items[i].Errors, items[i].Warnings)
		}
	}
	fee := decimal.NewFromInt(25950)
	allocateFee(items, &fee, true)
	var dutyTotal, vatTotal, feeTotal, customsTotal decimal.Decimal
	for i := range items {
		updateVATWithFee(&items[i])
		dutyTotal = dutyTotal.Add(mustDecimal(items[i].DutyKZT))
		vatTotal = vatTotal.Add(mustDecimal(items[i].VATKZT))
		feeTotal = feeTotal.Add(mustDecimal(items[i].FeeKZT))
		customsTotal = customsTotal.Add(mustDecimal(items[i].CustomsValueKZT))
	}
	if items[0].CustomsValueKZT != "1230004.22" || items[1].CustomsValueKZT != "1390161.02" {
		t.Fatalf("control customs values = %s, %s", items[0].CustomsValueKZT, items[1].CustomsValueKZT)
	}
	if items[0].DutyKZT != "147600.51" || items[1].DutyKZT != "166819.32" {
		t.Fatalf("control duties = %s, %s", items[0].DutyKZT, items[1].DutyKZT)
	}
	if items[0].FeeKZT != "12975.00" || items[1].FeeKZT != "12975.00" {
		t.Fatalf("control fee shares = %s, %s", items[0].FeeKZT, items[1].FeeKZT)
	}
	if items[0].VATBaseKZT != "1390579.73" || items[1].VATBaseKZT != "1569955.34" {
		t.Fatalf("control VAT bases = %s, %s", items[0].VATBaseKZT, items[1].VATBaseKZT)
	}
	if items[0].VATKZT != "222492.76" || items[1].VATKZT != "251192.85" {
		t.Fatalf("control VAT = %s, %s", items[0].VATKZT, items[1].VATKZT)
	}
	if dutyTotal.StringFixed(2) != "314419.83" || vatTotal.StringFixed(2) != "473685.61" || feeTotal.StringFixed(2) != "25950.00" {
		t.Fatalf("control totals = duty %s, vat %s, fee %s", dutyTotal, vatTotal, feeTotal)
	}
	payments := dutyTotal.Add(vatTotal).Add(feeTotal).Round(2)
	if payments.StringFixed(2) != "814055.44" {
		t.Fatalf("control customs payments = %s", payments)
	}
	if customsTotal.StringFixed(2) != "2620165.24" {
		t.Fatalf("control customs total = %s", customsTotal)
	}
}

func TestCustomsFeeSplitsEquallyWithTiynRemainder(t *testing.T) {
	items := []CalculationItem{{}, {}, {}}
	fee := decimal.RequireFromString("1.00")
	allocateFee(items, &fee, true)
	if items[0].FeeKZT != "0.34" || items[1].FeeKZT != "0.33" || items[2].FeeKZT != "0.33" {
		t.Fatalf("equal fee split = %s, %s, %s", items[0].FeeKZT, items[1].FeeKZT, items[2].FeeKZT)
	}
	var total decimal.Decimal
	for _, item := range items {
		total = total.Add(mustDecimal(item.FeeKZT))
	}
	if !total.Equal(fee) {
		t.Fatalf("fee shares must sum to the entered fee: %s", total)
	}
}

func TestCustomsFeeFromMRPMultiplier(t *testing.T) {
	fee, err := resolveCustomsFee("", "3692", "7")
	if err != nil {
		t.Fatal(err)
	}
	if fee == nil || fee.StringFixed(2) != "25844.00" {
		t.Fatalf("MRP fee = %v", fee)
	}
	direct, err := resolveCustomsFee("25950", "3692", "7")
	if err != nil {
		t.Fatal(err)
	}
	if direct == nil || direct.StringFixed(2) != "25950.00" {
		t.Fatalf("direct fee should win over MRP input: %v", direct)
	}
}

func TestPriceAndQuantityDeriveCurrentCost(t *testing.T) {
	snapshot := map[string]any{
		"duty": map[string]any{"type": "percentage", "percent": "0"},
		"vat":  map[string]any{"type": "percentage", "percent": "0"},
	}
	item := calculateItem(CalculationItemInput{StandardName: "Перчатки", Pairs: "96000", PricePerPair: "0.0288", CostCurrency: "USD", DirectorySnapshot: snapshot}, decimal.NewFromInt(500), decimal.Zero, decimal.NewFromFloat(.3))
	if item.GoodsCost != "2764.80" {
		t.Fatalf("derived cost = %s, want 2764.80", item.GoodsCost)
	}
	updated := calculateItem(CalculationItemInput{StandardName: "Перчатки", Pairs: "96000", PricePerPair: "0.03", CostCurrency: "USD", DirectorySnapshot: snapshot}, decimal.NewFromInt(500), decimal.Zero, decimal.NewFromFloat(.3))
	if updated.GoodsCost != "2880.00" {
		t.Fatalf("cost after price change = %s, want 2880.00", updated.GoodsCost)
	}
}

func TestRoundedPairPriceDrivesCostForAllImportedLines(t *testing.T) {
	snapshot := map[string]any{
		"duty": map[string]any{"type": "percentage", "percent": "0"},
		"vat":  map[string]any{"type": "percentage", "percent": "0"},
	}
	cases := []struct {
		pairs string
		price string
		cost  string
	}{
		{"96000", "0.0288", "2764.80"},
		{"48000", "0.0651", "3124.80"},
		{"24000", "0.0594", "1425.60"},
		{"120000", "0.0594", "7128.00"},
		{"42000", "0.0979", "4111.80"},
		{"39900", "0.0979", "3906.21"},
		{"30000", "0.0370", "1110.00"},
		{"54600", "0.0186", "1015.56"},
		{"42000", "0.0312", "1310.40"},
		{"42000", "0.0370", "1554.00"},
	}
	for _, tc := range cases {
		item := calculateItem(CalculationItemInput{
			StandardName: "РџРµСЂС‡Р°С‚РєРё", Pairs: tc.pairs, PricePerPair: tc.price, CostCurrency: "USD", DirectorySnapshot: snapshot,
		}, decimal.NewFromInt(500), decimal.Zero, decimal.NewFromFloat(.3))
		if item.PairPriceUSD != tc.price || item.CostUSD != tc.cost {
			t.Errorf("pairs=%s price=%s: got pair=%s cost=%s", tc.pairs, tc.price, item.PairPriceUSD, item.CostUSD)
		}
	}
}

func TestPairPriceTakesPriorityOverStaleManualCost(t *testing.T) {
	snapshot := map[string]any{
		"duty": map[string]any{"type": "percentage", "percent": "0"},
		"vat":  map[string]any{"type": "percentage", "percent": "0"},
	}
	item := calculateItem(CalculationItemInput{StandardName: "Перчатки", Pairs: "96000", PricePerPair: "0.0288", CostInput: "2763", CostCurrency: "USD", DirectorySnapshot: snapshot}, decimal.NewFromInt(500), decimal.Zero, decimal.NewFromFloat(.3))
	if item.GoodsCost != "2764.80" {
		t.Fatalf("cost must be recalculated from pair price: %s", item.GoodsCost)
	}
	found := len(item.Warnings) > 0
	for _, warning := range item.Warnings {
		if strings.Contains(warning, "Стоимость пересчитана по количеству пар") {
			found = true
		}
	}
	if !found && len(item.Warnings) == 0 {
		t.Fatalf("expected mismatch warning, got %v", item.Warnings)
	}
}

func TestDeliveryShareByGrossEntersCustomsValueControlRow(t *testing.T) {
	usd := decimal.NewFromFloat(444.88)
	weights := []string{"3100", "3800", "3800", "3800", "3800", "3800", "3800", "3800", "3800", "3800", "3800", "3870"}
	snapshot := map[string]any{
		"duty": map[string]any{"type": "percentage", "percent": "12"},
		"vat":  map[string]any{"type": "percentage", "percent": "16"},
	}
	items := make([]CalculationItem, len(weights))
	items[0] = calculateItem(CalculationItemInput{
		StandardName: "Товар 1", Pairs: "96000", PricePerPair: "0.0288", CostCurrency: "USD",
		GrossKG: "3100", GrossWeightUnit: "kg", DirectorySnapshot: snapshot,
	}, usd, decimal.Zero, decimal.NewFromFloat(.3))
	for i := 1; i < len(weights); i++ {
		items[i] = CalculationItem{CalculationItemInput: CalculationItemInput{
			StandardName: fmt.Sprintf("Товар %d", i+1), GrossKG: weights[i], GrossWeightUnit: "kg",
			CostInput: "0", CostCurrency: "USD", DirectorySnapshot: snapshot,
		}}
	}

	total := decimal.NewFromInt(840)
	allocateDelivery(items, &total, usd)
	if items[0].deliveryShareExact == nil {
		t.Fatal("exact delivery share was not allocated")
	}
	if share := items[0].deliveryShareExact.String(); !strings.HasPrefix(share, "57.905270180") {
		t.Fatalf("exact delivery share = %s, want 840 x 3100 / 44970", share)
	}
	if items[0].DeliveryUSD != "57.91" {
		t.Fatalf("displayed delivery share = %s, want rounded 57.91", items[0].DeliveryUSD)
	}

	warnings := []string{}
	calculateTariff(&items[0], nil, nil, nil, usd, decimal.Zero, &warnings)
	if len(warnings) != 0 {
		t.Fatalf("control row warnings: %v", warnings)
	}
	fee := decimal.NewFromInt(25950)
	allocateFee(items, &fee, true)
	updateVATWithFee(&items[0])

	if items[0].CustomsValueKZT != "1255765.12" {
		t.Fatalf("customs value = %s", items[0].CustomsValueKZT)
	}
	if items[0].DutyKZT != "150691.81" {
		t.Fatalf("duty = %s", items[0].DutyKZT)
	}
	if items[0].FeeKZT != "2162.50" {
		t.Fatalf("fee share = %s", items[0].FeeKZT)
	}
	if items[0].VATBaseKZT != "1408619.43" {
		t.Fatalf("vat base = %s", items[0].VATBaseKZT)
	}
	if items[0].VATKZT != "225379.11" {
		t.Fatalf("vat = %s", items[0].VATKZT)
	}
	payments := mustDecimal(items[0].DutyKZT).Add(mustDecimal(items[0].VATKZT)).Add(mustDecimal(items[0].FeeKZT))
	if payments.StringFixed(2) != "378233.42" {
		t.Fatalf("item payments = %s", payments)
	}

	// A delivery change must rebuild the shares and the dependent customs value.
	changed := decimal.NewFromInt(1000)
	allocateDelivery(items, &changed, usd)
	if items[0].DeliveryUSD == "57.91" {
		t.Fatal("delivery share did not react to a changed party total")
	}
	calculateTariff(&items[0], nil, nil, nil, usd, decimal.Zero, &warnings)
	if items[0].CustomsValueKZT == "1255765.12" {
		t.Fatal("customs value did not react to a changed delivery total")
	}
}

func TestDeliveryAlreadyInCostSkipsDoubleInclusion(t *testing.T) {
	snapshot := map[string]any{
		"duty": map[string]any{"type": "percentage", "percent": "0"},
		"vat":  map[string]any{"type": "percentage", "percent": "0"},
	}
	items := []CalculationItem{calculateItem(CalculationItemInput{
		StandardName: "Товар", CostInput: "100", CostCurrency: "USD",
		GrossKG: "10", GrossWeightUnit: "kg", DeliveryAlreadyInCost: true, DirectorySnapshot: snapshot,
	}, decimal.NewFromInt(100), decimal.Zero, decimal.NewFromFloat(.3))}
	total := decimal.NewFromInt(50)
	allocateDelivery(items, &total, decimal.NewFromInt(100))
	warnings := []string{}
	calculateTariff(&items[0], nil, nil, nil, decimal.NewFromInt(100), decimal.Zero, &warnings)
	if items[0].CustomsValueKZT != "10000.00" {
		t.Fatalf("delivery was added twice to customs value: %s", items[0].CustomsValueKZT)
	}
}
