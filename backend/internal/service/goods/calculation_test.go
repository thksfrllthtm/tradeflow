package goods

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/png"
	"strings"
	"testing"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/xuri/excelize/v2"
)

func TestGoodsParseXlsxReturnsHeadersAndSourceRows(t *testing.T) {
	book := excelize.NewFile()
	if err := book.SetCellValue("Sheet1", "A1", "Наименование"); err != nil {
		t.Fatal(err)
	}
	if err := book.SetCellValue("Sheet1", "B1", "Места"); err != nil {
		t.Fatal(err)
	}
	if err := book.SetCellValue("Sheet1", "A2", "Перчатки"); err != nil {
		t.Fatal(err)
	}
	if err := book.SetCellValue("Sheet1", "B2", 3); err != nil {
		t.Fatal(err)
	}
	buffer, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := NewGoodsService().ParseXlsx(context.Background(), bytes.NewReader(buffer.Bytes()), int64(buffer.Len()), "test.xlsx", "Sheet1", 1, 2)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(parsed.Headers) != 2 || parsed.Headers[0] != "Наименование" || len(parsed.Rows) != 1 || parsed.Rows[0].SourceRow != 2 {
		t.Fatalf("unexpected parse result: %+v", parsed)
	}
}

func TestGoodsParseXlsxExcludesTotalRows(t *testing.T) {
	book := excelize.NewFile()
	if err := book.SetCellValue("Sheet1", "A1", "Наименование"); err != nil {
		t.Fatal(err)
	}
	if err := book.SetCellValue("Sheet1", "A2", "Перчатки"); err != nil {
		t.Fatal(err)
	}
	if err := book.SetCellValue("Sheet1", "A3", "Итого"); err != nil {
		t.Fatal(err)
	}
	buffer, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := NewGoodsService().ParseXlsx(context.Background(), bytes.NewReader(buffer.Bytes()), int64(buffer.Len()), "test.xlsx", "Sheet1", 1, 2)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(parsed.Rows) != 2 || !parsed.Rows[0].Included || parsed.Rows[1].Included {
		t.Fatalf("expected product and excluded total row, got %+v", parsed.Rows)
	}
	if len(parsed.Rows[1].Warnings) == 0 || len(parsed.Warnings) == 0 {
		t.Fatalf("expected explicit total-row warnings, got row=%v result=%v", parsed.Rows[1].Warnings, parsed.Warnings)
	}
}

func TestGoodsParseXlsxKeepsBlankAndDuplicateHeaders(t *testing.T) {
	book := excelize.NewFile()
	for cell, value := range map[string]string{"A1": "Наименование", "B1": "", "C1": "Наименование", "A2": "Перчатки", "B2": "10", "C2": "другое"} {
		if err := book.SetCellValue("Sheet1", cell, value); err != nil {
			t.Fatal(err)
		}
	}
	buffer, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := NewGoodsService().ParseXlsx(context.Background(), bytes.NewReader(buffer.Bytes()), int64(buffer.Len()), "test.xlsx", "Sheet1", 1, 2)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(parsed.Headers) != 3 || parsed.Headers[1] != "Колонка 2" || parsed.Headers[2] != "Наименование (2)" {
		t.Fatalf("unexpected canonical headers: %+v", parsed.Headers)
	}
	if parsed.Rows[0].Values["Колонка 2"] != "10" || parsed.Rows[0].Values["Наименование (2)"] != "другое" {
		t.Fatalf("values from blank/duplicate columns were lost: %+v", parsed.Rows[0].Values)
	}
}

func TestGoodsParseXlsxRejectsCorruptWorkbook(t *testing.T) {
	_, err := NewGoodsService().ParseXlsx(context.Background(), bytes.NewReader([]byte("not an xlsx")), int64(len("not an xlsx")), "bad.xlsx", "", 1, 2)
	if err == nil || !strings.Contains(err.Error(), "Файл повреждён") {
		t.Fatalf("expected clear corrupt-file error, got %v", err)
	}
}

func TestValidateGoodsPhotoData(t *testing.T) {
	valid := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("png"))
	if err := validateGoodsPhotoData(valid); err != nil {
		t.Fatalf("valid photo rejected: %v", err)
	}
	if err := validateGoodsPhotoData("data:image/gif;base64," + base64.StdEncoding.EncodeToString([]byte("gif"))); err == nil {
		t.Fatal("unsupported image type should be rejected")
	}
	if err := validateGoodsPhotoData("data:image/png;base64,not-base64"); err == nil {
		t.Fatal("invalid base64 should be rejected")
	}
}

func testGoodsSnapshot() map[string]any {
	return map[string]any{
		"entryId": float64(1), "name": "Резиновые перчатки", "code": "4015190000", "companyCoefficient": "0.65",
		"duty": map[string]any{"type": "percentage", "percent": "0"},
		"vat":  map[string]any{"type": "percentage", "percent": "0"},
	}
}

func TestGoodsCalculationUsesPairsWithoutDividingByTwo(t *testing.T) {
	svc := NewGoodsService()
	result, err := svc.Calculate(context.Background(), GoodsCalculationInput{
		AssessmentDate: "2026-09-17", UsdKzt: "500", DeliveryMode: "total", DeliveryTotalUsd: "100", PackageWeightKg: "0.3",
		Items: []GoodsItemInput{{Name: "supplier", StandardName: "Резиновые перчатки", DirectorySnapshot: testGoodsSnapshot(), Places: "100", PairsPerPlace: "10", GrossKg: "100", Coefficient: "0.65", DeliveryIncluded: true}},
	})
	if err != nil {
		t.Fatalf("calculate: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("items: got %d", len(result.Items))
	}
	item := result.Items[0]
	if item.Pairs != "1000" || item.NetKG != "70" {
		t.Fatalf("unexpected quantities: pairs=%s net=%s", item.Pairs, item.NetKG)
	}
	if item.CostUSD != "45.50" {
		t.Fatalf("cost must be coefficient*net (0.65*70), got %s", item.CostUSD)
	}
	if item.PairPriceUSD != "0.0455" {
		t.Fatalf("pair price must use all pairs, got %s", item.PairPriceUSD)
	}
	if result.Totals.TotalDeliveryUSD != "100.00" || result.Totals.DeliveryKZT != "50000.00" {
		t.Fatalf("delivery must be a party total: %s USD / %s KZT", result.Totals.TotalDeliveryUSD, result.Totals.DeliveryKZT)
	}
	if item.Status != "calculated" {
		t.Fatalf("expected calculated item, got %s errors=%v warnings=%v", item.Status, item.Errors, item.Warnings)
	}
}

func TestGoodsCalculationAllowsPerRowCoefficientOverride(t *testing.T) {
	result, err := NewGoodsService().Calculate(context.Background(), GoodsCalculationInput{
		AssessmentDate: "2026-09-17", UsdKzt: "500", DeliveryMode: "total", DeliveryTotalUsd: "0",
		Items: []GoodsItemInput{{Name: "supplier", StandardName: "Резиновые перчатки", Places: "1", PairsPerPlace: "10", GrossKg: "10", NetKg: "10", NetSource: "excel", GrossWeightUnit: "kg", NetWeightUnit: "kg", Coefficient: "0.80", DirectorySnapshot: testGoodsSnapshot()}},
	})
	if err != nil {
		t.Fatalf("calculate: %v", err)
	}
	if got := result.Items[0].Coefficient; got != "0.80" {
		t.Fatalf("expected row override to remain, got %s", got)
	}
	if got := result.Items[0].CostUSD; got != "8.00" {
		t.Fatalf("expected overridden cost, got %s", got)
	}
}

func TestGoodsCalculationRequiresDirectoryAndValidDate(t *testing.T) {
	svc := NewGoodsService()
	if _, err := svc.Calculate(context.Background(), GoodsCalculationInput{AssessmentDate: "17.09.2026", UsdKzt: "500"}); err == nil {
		t.Fatal("invalid assessment date should be rejected")
	}
	result, err := svc.Calculate(context.Background(), GoodsCalculationInput{
		AssessmentDate: "2026-09-17", UsdKzt: "500", DeliveryMode: "total", DeliveryTotalUsd: "0", PackageWeightKg: "0.3",
		Items: []GoodsItemInput{{Name: "supplier", Places: "1", PairsPerPlace: "1", GrossKg: "1", Coefficient: "0.65", DeliveryIncluded: true}},
	})
	if err != nil {
		t.Fatalf("calculate incomplete item: %v", err)
	}
	if result.Items[0].Status != "needs_review" || len(result.Items[0].Errors) == 0 {
		t.Fatalf("missing directory should be a blocking review: %+v", result.Items[0])
	}
}

func TestGoodsCalculationRejectsNegativeInputs(t *testing.T) {
	svc := NewGoodsService()
	_, err := svc.Calculate(context.Background(), GoodsCalculationInput{AssessmentDate: "2026-09-17", UsdKzt: "-1"})
	if err == nil {
		t.Fatal("negative USD rate should be rejected")
	}
	if _, ok := err.(*service.ValidationError); !ok {
		t.Fatalf("expected validation error, got %T", err)
	}
}

func TestGoodsCalculationRejectsFractionalInferredPlaces(t *testing.T) {
	svc := NewGoodsService()
	result, err := svc.Calculate(context.Background(), GoodsCalculationInput{
		AssessmentDate: "2026-09-17", UsdKzt: "500", DeliveryMode: "total", DeliveryTotalUsd: "0",
		Items: []GoodsItemInput{{Name: "supplier", StandardName: "Перчатки", Places: "", Pairs: "10", PairsPerPlace: "3", GrossKg: "10", NetKg: "10", NetSource: "excel", Coefficient: "1", GrossWeightUnit: "kg", NetWeightUnit: "kg", DirectorySnapshot: testGoodsSnapshot()}},
	})
	if err != nil {
		t.Fatalf("calculate: %v", err)
	}
	if result.Items[0].Status != "needs_review" {
		t.Fatalf("fractional inferred places must require review: %+v", result.Items[0])
	}
	found := false
	for _, issue := range result.Items[0].Errors {
		if strings.Contains(issue, "Количество мест должно быть целым") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected fractional-place error, got %v", result.Items[0].Errors)
	}
}

func TestGoodsParseXlsxSendsSharedPictureOnce(t *testing.T) {
	book := excelize.NewFile()
	for cell, value := range map[string]string{"A1": "Наименование", "A2": "Перчатки S", "A3": "Перчатки M"} {
		if err := book.SetCellValue("Sheet1", cell, value); err != nil {
			t.Fatal(err)
		}
	}
	// A tall picture anchored at B2 covers rows 2 and 3 (default row height is 20 px).
	var picture bytes.Buffer
	if err := png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 10, 35))); err != nil {
		t.Fatal(err)
	}
	if err := book.AddPictureFromBytes("Sheet1", "B2", &excelize.Picture{Extension: ".png", File: picture.Bytes()}); err != nil {
		t.Fatal(err)
	}
	buffer, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := NewGoodsService().ParseXlsxWithSharedImages(context.Background(), bytes.NewReader(buffer.Bytes()), int64(buffer.Len()), "test.xlsx", "Sheet1", 1, 2)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var written bytes.Buffer
	if err := parsed.WriteJSON(&written); err != nil {
		t.Fatal(err)
	}
	var response struct {
		Rows      []GoodsParseRow   `json:"rows"`
		Images    []GoodsParseImage `json:"images"`
		ImageData []string          `json:"imageData"`
	}
	if err := json.Unmarshal(written.Bytes(), &response); err != nil {
		t.Fatalf("response must be valid JSON: %v", err)
	}
	if len(response.Rows) != 2 || len(response.ImageData) != 1 || len(response.Images) != 2 {
		t.Fatalf("one picture over two rows must be sent once and referenced twice, got %d data, %d links", len(response.ImageData), len(response.Images))
	}
	for index, link := range response.Images {
		if link.SourceRow != index+2 || link.DataIndex != 0 || link.MimeType != "image/png" {
			t.Fatalf("unexpected image link %+v", link)
		}
	}
	if decoded, _ := base64.StdEncoding.DecodeString(response.ImageData[0]); !bytes.Equal(decoded, picture.Bytes()) {
		t.Fatal("image data must be the original picture")
	}
}

func TestGoodsParseResultWriteJSONWithoutImages(t *testing.T) {
	var written bytes.Buffer
	if err := (GoodsParseResult{Headers: []string{"A"}}).WriteJSON(&written); err != nil {
		t.Fatal(err)
	}
	var response map[string]any
	if err := json.Unmarshal(written.Bytes(), &response); err != nil {
		t.Fatalf("response must be valid JSON: %v (%s)", err, written.String())
	}
	if data, ok := response["imageData"].([]any); !ok || len(data) != 0 {
		t.Fatalf("imageData must be an empty array, got %s", written.String())
	}
}

func TestGoodsCalculationWithoutDelivery(t *testing.T) {
	result, err := NewGoodsService().Calculate(context.Background(), GoodsCalculationInput{
		AssessmentDate: "2026-09-17", UsdKzt: "500", DeliveryMode: "none", Wagons: "", PackageWeightKg: "0.3",
		Items: []GoodsItemInput{{Name: "supplier", StandardName: "Резиновые перчатки", DirectorySnapshot: testGoodsSnapshot(), Places: "100", PairsPerPlace: "10", GrossKg: "100", Coefficient: "0.65", DeliveryIncluded: true}},
	})
	if err != nil {
		t.Fatalf("calculation without delivery must be allowed: %v", err)
	}
	item := result.Items[0]
	if item.Status != "calculated" {
		t.Fatalf("expected calculated item, got %s errors=%v warnings=%v", item.Status, item.Errors, item.Warnings)
	}
	if result.Totals.TotalDeliveryUSD != "" && result.Totals.TotalDeliveryUSD != "0" && result.Totals.TotalDeliveryUSD != "0.00" {
		t.Fatalf("delivery must be zero without a delivery mode, got %s", result.Totals.TotalDeliveryUSD)
	}
}

func TestGoodsCalculationZeroFeeMultiplierDropsFee(t *testing.T) {
	row := GoodsItemInput{Name: "supplier", StandardName: "Резиновые перчатки", DirectorySnapshot: testGoodsSnapshot(), Places: "100", PairsPerPlace: "10", GrossKg: "100", Coefficient: "0.65"}
	input := GoodsCalculationInput{AssessmentDate: "2026-09-17", UsdKzt: "500", DeliveryMode: "none", PackageWeightKg: "0.3", Mrp: "4325", FeeMultiplier: "0", Items: []GoodsItemInput{row, row}}
	result, err := NewGoodsService().Calculate(context.Background(), input)
	if err != nil {
		t.Fatalf("multiplier 0 must be allowed: %v", err)
	}
	if result.Totals.FeeKZT != "0.00" {
		t.Fatalf("fee must be zero with multiplier 0, got %s", result.Totals.FeeKZT)
	}
	withFee := input
	withFee.FeeMultiplier = "6"
	expected, err := NewGoodsService().Calculate(context.Background(), withFee)
	if err != nil {
		t.Fatalf("calculate with fee: %v", err)
	}
	// Сбор входит в базу НДС: без сбора растаможка меньше на сбор и на НДС с него.
	if result.Totals.CustomsPaymentsKZT == expected.Totals.CustomsPaymentsKZT {
		t.Fatalf("zero fee must lower customs payments, both %s", result.Totals.CustomsPaymentsKZT)
	}
	if expected.Totals.FeeKZT != "25950.00" {
		t.Fatalf("fee with multiplier 6 must be 4325 × 6 = 25950.00, got %s", expected.Totals.FeeKZT)
	}
}

func TestGoodsCalculationUsesExcelPriceOnlyWhereGiven(t *testing.T) {
	withPrice := GoodsItemInput{Name: "with price", StandardName: "Резиновые перчатки", DirectorySnapshot: testGoodsSnapshot(), Places: "100", PairsPerPlace: "10", GrossKg: "100", Coefficient: "0.65", PricePerPair: "1.5", SourcePricePerPair: "1.5"}
	withoutPrice := GoodsItemInput{Name: "without price", StandardName: "Резиновые перчатки", DirectorySnapshot: testGoodsSnapshot(), Places: "100", PairsPerPlace: "10", GrossKg: "100", Coefficient: "0.65"}
	input := GoodsCalculationInput{AssessmentDate: "2026-09-17", UsdKzt: "500", DeliveryMode: "none", PackageWeightKg: "0.3", Items: []GoodsItemInput{withPrice, withoutPrice}}
	result, err := NewGoodsService().Calculate(context.Background(), input)
	if err != nil {
		t.Fatalf("calculate: %v", err)
	}
	if got := result.Items[0].CostUSD; got != "1500.00" {
		t.Fatalf("row with an Excel price must cost price × pairs (1.5 × 1000), got %s", got)
	}
	if got := result.Items[1].CostUSD; got != "45.50" {
		t.Fatalf("row without a price must keep the usual ИТС × net cost (0.65 × 70), got %s", got)
	}

	encoded, _ := json.Marshal(input)
	var restored GoodsCalculationInput
	if err := json.Unmarshal(encoded, &restored); err != nil || restored.Items[0].SourcePricePerPair != "1.5" {
		t.Fatalf("the Excel price must survive a saved draft, got %+v (%v)", restored.Items[0].SourcePricePerPair, err)
	}
}

func TestGoodsCalculationFillsMissingQuantity(t *testing.T) {
	for name, item := range map[string]GoodsItemInput{
		"per place from quantity and places": {Pairs: "800", Places: "20", GrossKg: "1220"},
		"places from quantity and per place": {Pairs: "800", PairsPerPlace: "40", GrossKg: "1220"},
		"quantity from places and per place": {Places: "20", PairsPerPlace: "40", GrossKg: "1220"},
	} {
		item.Name, item.StandardName, item.DirectorySnapshot, item.Coefficient = "row", "Резиновые перчатки", testGoodsSnapshot(), "0.65"
		result, err := NewGoodsService().Calculate(context.Background(), GoodsCalculationInput{AssessmentDate: "2026-09-17", UsdKzt: "500", DeliveryMode: "none", PackageWeightKg: "0.3", Items: []GoodsItemInput{item}})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		got := result.Items[0]
		if got.Pairs != "800" || got.Places != "20" || got.PairsPerPlace != "40" || got.NetKG != "1214" {
			t.Fatalf("%s: got pairs=%s places=%s perPlace=%s net=%s", name, got.Pairs, got.Places, got.PairsPerPlace, got.NetKG)
		}
	}
}

func TestGoodsExportKeepsExcelNameUnlessProductChosen(t *testing.T) {
	codeOnly := testGoodsSnapshot()
	codeOnly["name"] = "Цистерны, бочки, барабаны"
	product := testGoodsSnapshot()
	product["name"], product["productDirectoryId"] = "Перчатки нитриловые", float64(5)
	row := func(name string, snapshot map[string]any) GoodsItemInput {
		return GoodsItemInput{Name: name, SupplierName: name, StandardName: snapshot["name"].(string), DirectorySnapshot: snapshot, Pairs: "10", Places: "1", GrossKg: "5", Coefficient: "1"}
	}
	buffer, err := NewGoodsService().Export(context.Background(), GoodsCalculationInput{
		AssessmentDate: "2026-09-17", UsdKzt: "500", DeliveryMode: "none", GenericItems: true,
		Items: []GoodsItemInput{row("Товар из Excel", codeOnly), row("Перчатки из Excel", product)},
	})
	if err != nil {
		t.Fatal(err)
	}
	book, err := excelize.OpenReader(buffer)
	if err != nil {
		t.Fatal(err)
	}
	sheet := book.GetSheetName(0)
	codeRow, _ := book.GetCellValue(sheet, "C2")
	productRow, _ := book.GetCellValue(sheet, "C3")
	if codeRow != "Товар из Excel" {
		t.Fatalf("a TN ВЭД code must not replace the Excel name, got %q", codeRow)
	}
	if productRow != "Перчатки нитриловые" {
		t.Fatalf("a chosen product card must give its name, got %q", productRow)
	}
}

func TestExportContainsRowsPhotoTotalsAndSummary(t *testing.T) {
	var photo bytes.Buffer
	if err := png.Encode(&photo, image.NewRGBA(image.Rect(0, 0, 4, 4))); err != nil {
		t.Fatal(err)
	}
	snapshot := testGoodsSnapshot()
	snapshot["duty"] = map[string]any{"type": "percentage", "percent": "10"}
	snapshot["vat"] = map[string]any{"type": "percentage", "percent": "12"}
	row := GoodsItemInput{Name: "Бочка", SupplierName: "Бочка", StandardName: "Резиновые перчатки", DirectorySnapshot: snapshot, Places: "10", PairsPerPlace: "80", GrossKg: "1220",
		PricePerPair: "6.4792", PhotoURL: "data:image/png;base64," + base64.StdEncoding.EncodeToString(photo.Bytes())}
	buffer, err := NewGoodsService().Export(context.Background(), GoodsCalculationInput{
		AssessmentDate: "2026-09-17", UsdKzt: "500", DeliveryMode: "none", Mrp: "4325", FeeMultiplier: "6", GenericItems: true, Items: []GoodsItemInput{row},
	})
	if err != nil {
		t.Fatal(err)
	}
	book, err := excelize.OpenReader(buffer)
	if err != nil {
		t.Fatal(err)
	}
	if sheets := book.GetSheetList(); len(sheets) != 2 || sheets[0] != "Расчёт" || sheets[1] != "Итоги" {
		t.Fatalf("expected sheets Расчёт and Итоги, got %v", sheets)
	}
	cell := func(sheet, name string) string { value, _ := book.GetCellValue(sheet, name); return value }
	if cell("Расчёт", "C1") != "Наименование" || cell("Расчёт", "F2") != "10%" || cell("Расчёт", "G2") != "12%" || cell("Расчёт", "K2") != "800" {
		t.Fatalf("unexpected row: name header %q duty %q vat %q quantity %q", cell("Расчёт", "C1"), cell("Расчёт", "F2"), cell("Расчёт", "G2"), cell("Расчёт", "K2"))
	}
	if price, _ := book.GetCellValue("Расчёт", "O2", excelize.Options{RawCellValue: true}); price != "6.4792" {
		t.Fatalf("the price on screen must be exported, got %q", price)
	}
	if pictures, _ := book.GetPictures("Расчёт", "B2"); len(pictures) != 1 {
		t.Fatalf("the row photo must be embedded, got %d pictures", len(pictures))
	}
	if cell("Расчёт", "C3") != "Итого" || cell("Расчёт", "Z3") == "" {
		t.Fatalf("a total row with the customs payment is expected, got %q / %q", cell("Расчёт", "C3"), cell("Расчёт", "Z3"))
	}
	lineCustoms, _ := book.GetCellValue("Расчёт", "Z2", excelize.Options{RawCellValue: true})
	payment, _ := book.GetCellValue("Итоги", "B18", excelize.Options{RawCellValue: true})
	paymentUSD, _ := book.GetCellValue("Итоги", "C18", excelize.Options{RawCellValue: true})
	if cell("Итоги", "A18") != "Растаможка к оплате" || payment != lineCustoms || paymentUSD == "" {
		t.Fatalf("summary must show the customs payment in KZT and USD: %q %q %q (row %q)", cell("Итоги", "A18"), payment, paymentUSD, lineCustoms)
	}
	if cell("Итоги", "B9") != "МРП 4 325 × 6" {
		t.Fatalf("fee parameters expected, got %q", cell("Итоги", "B9"))
	}
}

// Картинка в объединённой ячейке (B2:B5) короче ячейки — фото получают все её строки.
func TestGoodsParseXlsxPictureInMergedCellCoversAllRows(t *testing.T) {
	book := excelize.NewFile()
	for cell, value := range map[string]string{"A1": "Наименование", "A2": "Цепь 3*2,3", "A3": "Цепь 4*2,3", "A4": "Цепь 5*2,3", "A5": "Цепь 10*2,3", "A6": "Брусок"} {
		if err := book.SetCellValue("Sheet1", cell, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := book.MergeCell("Sheet1", "B2", "B5"); err != nil {
		t.Fatal(err)
	}
	var picture bytes.Buffer
	if err := png.Encode(&picture, image.NewRGBA(image.Rect(0, 0, 10, 15))); err != nil {
		t.Fatal(err)
	}
	if err := book.AddPictureFromBytes("Sheet1", "B2", &excelize.Picture{Extension: ".png", File: picture.Bytes()}); err != nil {
		t.Fatal(err)
	}
	buffer, err := book.WriteToBuffer()
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := NewGoodsService().ParseXlsxWithSharedImages(context.Background(), bytes.NewReader(buffer.Bytes()), int64(buffer.Len()), "test.xlsx", "Sheet1", 1, 2)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	rows := map[int]bool{}
	for _, link := range parsed.Images {
		rows[link.SourceRow] = true
	}
	if !rows[2] || !rows[3] || !rows[4] || !rows[5] || rows[6] {
		t.Fatalf("фото должны получить строки 2–5 объединённой ячейки, получили %v", rows)
	}
}
