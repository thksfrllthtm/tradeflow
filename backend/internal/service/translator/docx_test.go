package translator

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const testNamespaces = `xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" ` +
	`xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" ` +
	`xmlns:mc="http://schemas.openxmlformats.org/markup-compatibility/2006" ` +
	`xmlns:m="http://schemas.openxmlformats.org/officeDocument/2006/math" ` +
	`xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" ` +
	`xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" ` +
	`xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture" ` +
	`xmlns:wps="http://schemas.microsoft.com/office/word/2010/wordprocessingShape"`

// testDocx собирает минимальный .docx: body — содержимое w:body, parts — другие части архива.
func testDocx(t *testing.T, body string, parts map[string]string) []byte {
	t.Helper()
	files := map[string]string{
		"[Content_Types].xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">` +
			`<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>` +
			`<Default Extension="xml" ContentType="application/xml"/>` +
			`<Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`,
		"_rels/.rels": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">` +
			`<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`,
		"word/document.xml": `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
			`<w:document ` + testNamespaces + `><w:body>` + body + `<w:sectPr/></w:body></w:document>`,
	}
	for name, content := range parts {
		files[name] = content
	}
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	for name, content := range files {
		writer, err := archive.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func parseTest(t *testing.T, body string, parts map[string]string) ([]ParsedSegment, map[string]int) {
	t.Helper()
	segments, skipped, err := ParseSegments(testDocx(t, body, parts))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	return segments, skipped
}

const bold = `<w:rPr><w:b/></w:rPr>`

func TestParseMergesRunsSplitBySpellcheck(t *testing.T) {
	segments, _ := parseTest(t, `<w:p><w:pPr><w:jc w:val="center"/></w:pPr>`+
		`<w:r w:rsidR="00A1"><w:rPr><w:sz w:val="24"/><w:lang w:val="en-US"/></w:rPr><w:t>Hel</w:t></w:r>`+
		`<w:proofErr w:type="spellStart"/>`+
		`<w:r w:rsidR="00B2"><w:rPr><w:sz w:val="24"/><w:lang w:val="ru-RU"/></w:rPr><w:t xml:space="preserve">lo, </w:t></w:r>`+
		`<w:proofErr w:type="spellEnd"/>`+
		`<w:r><w:rPr><w:rFonts w:hint="eastAsia"/><w:sz w:val="24"/></w:rPr><w:t>мир</w:t></w:r></w:p>`, nil)
	want := []ParsedSegment{{Kind: "paragraph", Text: "Hello, мир", Tags: map[string]string{"g0": `<w:rPr><w:sz w:val="24"/><w:lang w:val="en-US"/></w:rPr>`}}}
	if !reflect.DeepEqual(segments, want) {
		t.Fatalf("got %#v", segments)
	}
}

func TestParseTagsFormattingAndInlineElements(t *testing.T) {
	segments, _ := parseTest(t, `<w:p>`+
		`<w:r><w:t xml:space="preserve">Цена </w:t></w:r>`+
		`<w:r>`+bold+`<w:t>100 USD</w:t></w:r>`+
		`<w:r><w:tab/><w:t>за шт.</w:t><w:br/></w:r>`+
		`<w:hyperlink r:id="rId5"><w:r><w:t>сайт</w:t></w:r></w:hyperlink>`+
		`</w:p>`, nil)
	want := ParsedSegment{
		Kind: "paragraph",
		Text: "Цена <g1>100 USD</g1><x2/>за шт.<x3/><g4>сайт</g4>",
		Tags: map[string]string{"g1": bold, "x2": "tab", "x3": "br", "g4": ""},
	}
	if len(segments) != 1 || !reflect.DeepEqual(segments[0], want) {
		t.Fatalf("got %#v", segments)
	}
}

func TestParseEscapesTextAndSkipsEmptyParagraphs(t *testing.T) {
	segments, _ := parseTest(t, `<w:p/><w:p><w:r><w:t>   </w:t></w:r></w:p>`+
		`<w:p><w:r><w:drawing><wp:inline><a:graphic/></wp:inline></w:drawing></w:r></w:p>`+
		`<w:p><w:r><w:t>A &amp; B &lt; C</w:t></w:r></w:p>`, nil)
	if len(segments) != 1 || segments[0].Text != "A &amp; B &lt; C" {
		t.Fatalf("got %#v", segments)
	}
}

func TestParseTableCells(t *testing.T) {
	segments, _ := parseTest(t, `<w:p><w:r><w:t>До таблицы</w:t></w:r></w:p>`+
		`<w:tbl><w:tr><w:tc><w:p><w:r><w:t>Код</w:t></w:r></w:p></w:tc><w:tc><w:p><w:r><w:t>Товар</w:t></w:r></w:p><w:p><w:r><w:t>строка 2</w:t></w:r></w:p></w:tc></w:tr></w:tbl>`+
		`<w:p><w:r><w:t>После</w:t></w:r></w:p>`, nil)
	var got []string
	for _, item := range segments {
		got = append(got, item.Kind+":"+item.Text)
	}
	want := []string{"paragraph:До таблицы", "table_cell:Код", "table_cell:Товар", "table_cell:строка 2", "paragraph:После"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestParseRejectsTrackedChanges(t *testing.T) {
	_, _, err := ParseSegments(testDocx(t, `<w:p><w:ins w:id="1" w:author="A"><w:r><w:t>новое</w:t></w:r></w:ins></w:p>`, nil))
	if !errors.Is(err, errTrackedChanges) {
		t.Fatalf("expected tracked changes error, got %v", err)
	}
}

func TestParseCountsSkippedParts(t *testing.T) {
	textbox := `<w:r><mc:AlternateContent><mc:Choice Requires="wps"><w:drawing><wps:txbx><w:txbxContent><w:p><w:r><w:t>Надпись</w:t></w:r></w:p></w:txbxContent></wps:txbx></w:drawing></mc:Choice>` +
		`<mc:Fallback><w:pict><w:txbxContent><w:p><w:r><w:t>Надпись</w:t></w:r></w:p></w:txbxContent></w:pict></mc:Fallback></mc:AlternateContent></w:r>`
	header := func(text string) string {
		return `<w:hdr ` + testNamespaces + `><w:p><w:r><w:t>` + text + `</w:t></w:r></w:p></w:hdr>`
	}
	segments, skipped := parseTest(t, `<w:p><w:r><w:t>Текст</w:t></w:r>`+textbox+`</w:p>`+
		`<w:p>`+textbox+`</w:p>`+
		`<w:p><w:r><w:t>Формула </w:t></w:r><m:oMath><m:r><m:t>x</m:t></m:r></m:oMath></w:p>`,
		map[string]string{"word/header1.xml": header("ООО «Ромашка»"), "word/footer1.xml": header("1")})
	if len(segments) != 2 || segments[0].Text != "Текст<x1/>" || segments[0].Tags["x1"] != "image" {
		t.Fatalf("segments %#v", segments)
	}
	want := map[string]int{"textboxes": 2, "formulas": 1, "headers": 1}
	if !reflect.DeepEqual(skipped, want) {
		t.Fatalf("skipped %v, want %v", skipped, want)
	}
}

func TestParseRecordsParagraphContentRange(t *testing.T) {
	data := testDocx(t, `<w:p><w:pPr><w:jc w:val="left"/></w:pPr><w:bookmarkStart w:id="0" w:name="_Top"/><w:r><w:t>Раз</w:t></w:r><w:bookmarkEnd w:id="0"/></w:p>`, nil)
	parsed, err := parseDocx(data)
	if err != nil {
		t.Fatal(err)
	}
	p := parsed.paragraphs[0]
	content := string(parsed.document[p.start:p.end])
	if content != `<w:bookmarkStart w:id="0" w:name="_Top"/><w:r><w:t>Раз</w:t></w:r><w:bookmarkEnd w:id="0"/>` {
		t.Fatalf("content %q", content)
	}
	if p.anchors != `<w:bookmarkStart w:id="0" w:name="_Top"/><w:bookmarkEnd w:id="0"/>` {
		t.Fatalf("anchors %q", p.anchors)
	}
}

func TestParseRejectsNonDocx(t *testing.T) {
	if _, _, err := ParseSegments([]byte("not a zip")); err == nil {
		t.Fatal("expected error")
	}
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	archive.Create("xl/workbook.xml")
	archive.Close()
	if _, _, err := ParseSegments(buffer.Bytes()); !errors.Is(err, errNoDocument) {
		t.Fatalf("expected errNoDocument, got %v", err)
	}
}

// Образцы в testdata (свои файлы сотрудников кладут туда же) разбираются без ошибок.
func TestParseSampleFiles(t *testing.T) {
	files, _ := filepath.Glob(filepath.Join("testdata", "*.docx"))
	for _, name := range files {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		segments, skipped, err := ParseSegments(data)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if len(segments) == 0 {
			t.Errorf("%s: no segments", name)
		}
		for _, item := range segments {
			if strings.TrimSpace(item.Text) == "" {
				t.Errorf("%s: empty segment", name)
			}
		}
		t.Logf("%s: %d segments, skipped %v", filepath.Base(name), len(segments), skipped)
	}
}

func TestTagViewsReadFormatting(t *testing.T) {
	got := tagViews(map[string]string{
		"g0": `<w:rPr><w:b/><w:bCs/><w:u w:val="single" w:color="FF0000"/></w:rPr>`,
		"g1": `<w:rPr><w:b w:val="0"/><w:i/><w:iCs/><w:u w:val="none"/></w:rPr>`,
		"x2": "tab",
	})
	want := map[string]TagView{"g0": {Kind: "text", Bold: true, Underline: true}, "g1": {Kind: "text", Italic: true}, "x2": {Kind: "tab"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v", got)
	}
}

// Проверки загрузки срабатывают до записи в базу (репозитория в тесте нет).
func TestUploadValidation(t *testing.T) {
	s := NewService(nil, nil)
	many := strings.Repeat(`<w:p><w:r><w:t>абзац</w:t></w:r></w:p>`, maxSegments+1)
	for _, test := range []struct {
		name, file, source, target string
		data                       []byte
		message                    string
	}{
		{"same language", "a.docx", "ru", "ru", nil, "Язык перевода должен отличаться от языка оригинала"},
		{"unknown language", "a.docx", "de", "ru", nil, "Выберите язык оригинала"},
		{"old word", "a.doc", "zh", "ru", nil, "Файлы .doc не поддерживаются: откройте документ в Word и сохраните его как .docx"},
		{"not docx", "a.docx", "zh", "ru", []byte("zip?"), "Не удалось прочитать файл: это не документ Word (.docx) или он повреждён"},
		{"empty", "a.docx", "zh", "ru", testDocx(t, `<w:p/>`, nil), "В файле нет текста для перевода"},
		{"too many", "a.docx", "zh", "ru", testDocx(t, many, nil), "Абзацев с текстом в документе: 3001, а можно не больше 3 000. Разделите файл на части и загрузите по очереди"},
		{"tracked", "a.docx", "zh", "ru", testDocx(t, `<w:p><w:del><w:r><w:delText>x</w:delText></w:r></w:del></w:p>`, nil), "В документе есть исправления в режиме рецензирования. Примите или отклоните их в Word и загрузите файл снова"},
	} {
		_, err := s.Upload(context.Background(), 1, test.file, test.data, test.source, test.target)
		if err == nil || err.Error() != test.message {
			t.Errorf("%s: got %v", test.name, err)
		}
	}
	for _, test := range []struct {
		name, format, message string
		size                  int
	}{
		{"big.DOCX", "", "больше 20 МБ", maxDocxBytes + 1},
		{"book.xlsx", "xlsx", "", maxDocxBytes + 1}, // Excel — до 100 МБ
		{"big.xlsx", "", "больше 100 МБ", MaxFileBytes + 1},
		{"old.xls", "", "сохраните её как .xlsx", 1},
		{"scan.pdf", "", "Выберите документ Word (.docx) или книгу Excel (.xlsx)", 1},
	} {
		format, err := checkFile(test.name, test.size)
		failed := err != nil
		if format != test.format || failed != (test.message != "") || failed && !strings.Contains(err.Error(), test.message) {
			t.Errorf("%s: format %q, err %v", test.name, format, err)
		}
	}
	if name := documentName("  Контракт №5.docx"); name != "Контракт №5" {
		t.Errorf("name %q", name)
	}
}
