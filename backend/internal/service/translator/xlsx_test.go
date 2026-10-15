package translator

import (
	"archive/zip"
	"bytes"
	"os"
	"reflect"
	"strings"
	"testing"
)

// testXlsx собирает минимальную книгу: shared — содержимое <sst>, parts — другие части архива.
func testXlsx(t *testing.T, shared string, parts map[string]string) []byte {
	t.Helper()
	files := map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"/>`,
		"xl/workbook.xml":     `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheets><sheet name="Sheet1" sheetId="1"/><sheet name="计划1" sheetId="2"/></sheets></workbook>`,
		"xl/sharedStrings.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<sst xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">` + shared + `</sst>`,
		"xl/worksheets/sheet1.xml": `<worksheet><sheetData><row r="1"><c r="A1" t="s"><v>0</v></c></row></sheetData></worksheet>`,
	}
	for name, content := range parts {
		files[name] = content
	}
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	for name, content := range files {
		writer, _ := archive.Create(name)
		writer.Write([]byte(content))
	}
	archive.Close()
	return buffer.Bytes()
}

const sampleShared = `<si><t xml:space="preserve">  出库单     </t></si>` +
	`<si><t>品名（英文/&#10;俄文）</t></si>` +
	`<si><t>6911100000</t></si>` +
	`<si><r><rPr><b/><sz val="11"/></rPr><t>型号</t></r><r><rPr><sz val="11"/></rPr><t xml:space="preserve"> A&amp;B</t></r><phoneticPr fontId="1"/></si>` +
	`<si><t>件数</t><rPh sb="0" eb="1"><t>けん</t></rPh></si>`

func TestParseXlsxSharedStrings(t *testing.T) {
	segments, skipped, err := ParseXlsxSegments(testXlsx(t, sampleShared, map[string]string{
		"xl/comments1.xml":         `<comments><commentList><comment ref="A1"><text><t>Проверить</t></text></comment></commentList></comments>`,
		"xl/worksheets/sheet2.xml": `<worksheet><sheetData><row r="1"><c r="A1" t="inlineStr"><is><t>текст</t></is></c></row></sheetData></worksheet>`,
	}))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, segment := range segments {
		got = append(got, segment.Text)
	}
	// Цифры без букв — не сегмент; пробелы по краям не уходят в перевод; перенос — тег.
	want := []string{"出库单", "品名（英文/<x1/>俄文）", "<g1>型号</g1> A&amp;B", "件数"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("segments %q", got)
	}
	if segments[0].Kind != "table_cell" || segments[1].Tags["x1"] != "br" || segments[2].Tags["g1"] != `<rPr><b/><sz val="11"/></rPr>` {
		t.Fatalf("tags %#v", segments)
	}
	if !reflect.DeepEqual(skipped, map[string]int{"sheets": 1, "comments": 1, "inline": 1}) {
		t.Fatalf("skipped %v", skipped)
	}
}

func TestExportXlsxReplacesOnlySharedStrings(t *testing.T) {
	data := testXlsx(t, sampleShared, nil)
	segments, _, _ := ParseXlsxSegments(data)
	items := make([]exportSegment, len(segments))
	for i, segment := range segments {
		items[i] = exportSegment{source: segment.Text, translation: mockTranslate(segment.Text)}
	}
	out, err := buildXlsx(data, items)
	if err != nil {
		t.Fatal(err)
	}
	shared := zipParts(t, out)["xl/sharedStrings.xml"]
	for _, want := range []string{
		`<si><t xml:space="preserve">  «出库单»     </t></si>`, // пробелы по краям на месте
		`«品名（英文/»&#10;«俄文）»`,
		`<si><t>6911100000</t></si>`, // не сегмент — без изменений
		`<r><rPr><b/><sz val="11"/></rPr><t xml:space="preserve">«型号»</t></r><r><rPr><sz val="11"/></rPr><t xml:space="preserve">« A&amp;B»</t></r>`,
	} {
		if !strings.Contains(shared, want) {
			t.Fatalf("no %s in %s", want, shared)
		}
	}
	if strings.Contains(shared, "けん") {
		t.Fatal("phonetic hint must be dropped from translated string")
	}
	again, _, err := ParseXlsxSegments(out)
	if err != nil || len(again) != len(segments) || again[2].Text != mockTranslate(segments[2].Text) {
		t.Fatalf("reparse %#v, %v", again, err)
	}
	before, after := zipParts(t, data), zipParts(t, out)
	for part, content := range before {
		if part != "xl/sharedStrings.xml" && after[part] != content {
			t.Fatalf("part %s changed", part)
		}
	}
}

// Настоящая книга поставщика: разбор → мок-перевод → сборка → разбор, остальные части не меняются.
func TestXlsxSampleRoundTrip(t *testing.T) {
	data, err := os.ReadFile("testdata/c106.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	segments, skipped, err := ParseXlsxSegments(data)
	if err != nil || len(segments) == 0 {
		t.Fatalf("segments %d, err %v", len(segments), err)
	}
	t.Logf("c106.xlsx: %d segments, skipped %v", len(segments), skipped)
	items := make([]exportSegment, len(segments))
	for i, segment := range segments {
		items[i] = exportSegment{source: segment.Text, translation: mockTranslate(segment.Text)}
	}
	out, err := buildXlsx(data, items)
	if err != nil {
		t.Fatal(err)
	}
	again, _, err := ParseXlsxSegments(out)
	if err != nil || len(again) != len(segments) {
		t.Fatalf("reparse: %d segments, %v", len(again), err)
	}
	for i := range segments {
		if again[i].Text != mockTranslate(segments[i].Text) || !reflect.DeepEqual(again[i].Tags, segments[i].Tags) {
			t.Fatalf("segment %d: %q, want %q", i, again[i].Text, mockTranslate(segments[i].Text))
		}
	}
	before, after := zipParts(t, data), zipParts(t, out)
	for part, content := range before {
		if part != "xl/sharedStrings.xml" && after[part] != content {
			t.Fatalf("part %s changed", part)
		}
	}
}
