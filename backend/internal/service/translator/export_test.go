package translator

import (
	"archive/zip"
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// mockTranslate — «перевод» для тестов: каждый кусок текста между тегами в «ёлочках», теги на месте.
func mockTranslate(text string) string {
	var result strings.Builder
	last := 0
	part := func(value string) {
		if value != "" {
			result.WriteString("«" + value + "»")
		}
	}
	for _, bounds := range tagToken.FindAllStringIndex(text, -1) {
		part(text[last:bounds[0]])
		result.WriteString(text[bounds[0]:bounds[1]])
		last = bounds[1]
	}
	part(text[last:])
	return result.String()
}

func exportItems(t *testing.T, data []byte, translate func(string) string) ([]ParsedSegment, []exportSegment) {
	t.Helper()
	segments, _, err := ParseSegments(data)
	if err != nil {
		t.Fatal(err)
	}
	items := make([]exportSegment, len(segments))
	for i, segment := range segments {
		items[i] = exportSegment{source: segment.Text, translation: translate(segment.Text)}
	}
	return segments, items
}

func zipParts(t *testing.T, data []byte) map[string]string {
	t.Helper()
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	parts := map[string]string{}
	for _, file := range archive.File {
		reader, _ := file.Open()
		content, _ := io.ReadAll(reader)
		reader.Close()
		parts[file.Name] = string(content)
	}
	return parts
}

func sampleFiles(t *testing.T) map[string][]byte {
	files, _ := filepath.Glob(filepath.Join("testdata", "*.docx"))
	samples := map[string][]byte{}
	for _, name := range files {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		samples[filepath.Base(name)] = data
	}
	samples["inline.docx"] = testDocx(t, `<w:p><w:pPr><w:numPr><w:ilvl w:val="0"/><w:numId w:val="1"/></w:numPr></w:pPr>`+
		`<w:bookmarkStart w:id="0" w:name="_Top"/><w:r><w:t xml:space="preserve">Цена </w:t></w:r><w:r>`+bold+`<w:t>100 USD</w:t></w:r>`+
		`<w:r><w:tab/><w:t>A &amp; B</w:t></w:r><w:hyperlink r:id="rId5"><w:r><w:t>сайт</w:t></w:r></w:hyperlink><w:bookmarkEnd w:id="0"/></w:p>`+
		`<w:tbl><w:tr><w:tc><w:p><w:r><w:t>Ячейка</w:t></w:r></w:p></w:tc></w:tr></w:tbl>`, nil)
	return samples
}

// Разобрать → перевести → собрать → разобрать снова: те же сегменты с теми же тегами и форматированием,
// остальные части архива не меняются.
func TestExportKeepsStructureAndFormatting(t *testing.T) {
	for name, data := range sampleFiles(t) {
		for _, translate := range []func(string) string{func(text string) string { return text }, mockTranslate} {
			segments, items := exportItems(t, data, translate)
			out, err := buildDocx(data, items, false)
			if err != nil {
				t.Fatalf("%s: %v", name, err)
			}
			again, _, err := ParseSegments(out)
			if err != nil {
				t.Fatalf("%s: exported file does not parse: %v", name, err)
			}
			if len(again) != len(segments) {
				t.Fatalf("%s: %d segments after export, want %d", name, len(again), len(segments))
			}
			for i := range segments {
				if again[i].Text != translate(segments[i].Text) || !reflect.DeepEqual(again[i].Tags, segments[i].Tags) || again[i].Kind != segments[i].Kind {
					t.Fatalf("%s segment %d:\n got %#v\nwant %q with tags %v", name, i, again[i], translate(segments[i].Text), segments[i].Tags)
				}
			}
			before, after := zipParts(t, data), zipParts(t, out)
			for part, content := range before {
				if part != "word/document.xml" && after[part] != content {
					t.Fatalf("%s: part %s changed", name, part)
				}
			}
			if len(after) != len(before) {
				t.Fatalf("%s: parts %d → %d", name, len(before), len(after))
			}
		}
	}
}

func TestExportBilingualAddsGrayParagraphUnderOriginal(t *testing.T) {
	data := sampleFiles(t)["inline.docx"]
	segments, items := exportItems(t, data, mockTranslate)
	out, err := buildDocx(data, items, true)
	if err != nil {
		t.Fatal(err)
	}
	again, _, err := ParseSegments(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(again) != 2*len(segments) {
		t.Fatalf("got %d segments", len(again))
	}
	for i, segment := range segments {
		original, translation := again[2*i], again[2*i+1]
		if original.Text != segment.Text || translation.Kind != segment.Kind {
			t.Fatalf("pair %d: %#v / %#v", i, original, translation)
		}
		if !strings.Contains(translation.Tags["g0"], `<w:i/><w:color w:val="808080"/>`) {
			t.Fatalf("pair %d: translation is not gray italic: %v", i, translation.Tags)
		}
	}
	// Абзац перевода без нумерации и без закладок; табуляция повторена, жирный сохранён.
	document := zipParts(t, out)["word/document.xml"]
	if strings.Count(document, "<w:numPr>") != 1 || strings.Count(document, "bookmarkStart") != 1 || strings.Count(document, "<w:tab/>") != 2 {
		t.Fatalf("document %s", document)
	}
	if again[1].Text != "«Цена »<g1>«100 USD»</g1><x2/>«A &amp; B»<g3>«сайт»</g3>" {
		t.Fatalf("translation %q", again[1].Text)
	}
}

func TestExportSurvivesBrokenTags(t *testing.T) {
	data := testDocx(t, `<w:p><w:r><w:t>Маркировка </w:t></w:r><w:r><w:drawing><wp:inline><a:graphic/></wp:inline></w:drawing></w:r><w:r>`+bold+`<w:t>на коробке</w:t></w:r></w:p>`, nil)
	segments, _, _ := ParseSegments(data)
	if segments[0].Text != "Маркировка <x1/><g2>на коробке</g2>" {
		t.Fatalf("source %q", segments[0].Text)
	}
	// Картинка потеряна, тег g2 не закрыт, лишний g9 — картинка всё равно в документе.
	out, err := buildDocx(data, []exportSegment{{source: segments[0].Text, translation: "Marking <g9>on <g2>the box"}}, false)
	if err != nil {
		t.Fatal(err)
	}
	document := zipParts(t, out)["word/document.xml"]
	if !strings.Contains(document, "<w:drawing>") || strings.Contains(document, "g9") || !strings.Contains(document, ">the box<") {
		t.Fatalf("document %s", document)
	}
	if _, _, err := ParseSegments(out); err != nil {
		t.Fatal(err)
	}
}

func TestExportRejectsChangedSegments(t *testing.T) {
	data := testDocx(t, `<w:p><w:r><w:t>Один</w:t></w:r></w:p>`, nil)
	for _, items := range [][]exportSegment{{}, {{source: "Другой", translation: "x"}}} {
		if _, err := buildDocx(data, items, false); !errors.Is(err, errSegmentsChanged) {
			t.Fatalf("expected errSegmentsChanged, got %v", err)
		}
	}
}
