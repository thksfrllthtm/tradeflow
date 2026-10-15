package translator

// Книги Excel (.xlsx). Почти весь текст книги хранится один раз в xl/sharedStrings.xml, ячейки
// ссылаются на строку по номеру — сегмент и есть такая строка (<si>): одинаковый текст в сотне
// ячеек переводится один раз. Числа, даты и формулы не трогаются; строки без букв (артикулы
// цифрами, «1250,5») — не сегменты.
//
// Как и в .docx, меняется только содержимое <si>, остальной архив копируется байт в байт.
// Форматирование части текста (<r><rPr>) — теги <gN>, перенос строки в ячейке — <xN/>.
// Названия листов не переводятся: на них ссылаются формулы.

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"regexp"
	"strings"
	"unicode"
)

var errNoWorkbook = errors.New("xlsx: no xl/workbook.xml")

// sharedString — строка <si>: start и end — границы её содержимого в sharedStrings.xml.
type sharedString struct {
	paragraph
	rich        bool   // строка из кусков <r> со своим форматированием
	lead, trail string // пробелы по краям простой строки (заголовки «по центру» пробелами)
}

type parsedXlsx struct {
	sharedStrings []byte
	strings       []sharedString // только строки-сегменты, по порядку
	skipped       map[string]int
}

// Имена листов по умолчанию переводить незачем — предупреждение только о «своих» названиях.
var defaultSheetName = regexp.MustCompile(`(?i)^(sheet|лист)\d*$`)

func parseXlsx(data []byte) (parsedXlsx, error) {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return parsedXlsx{}, err
	}
	result := parsedXlsx{skipped: map[string]int{}}
	found := false
	for _, file := range archive.File {
		name := file.Name
		switch {
		case name == "xl/workbook.xml":
			found = true
			workbook, err := readPart(file)
			if err != nil {
				return parsedXlsx{}, err
			}
			result.skipped["sheets"] = countSheetNames(workbook)
		case name == "xl/sharedStrings.xml":
			if result.sharedStrings, err = readPart(file); err != nil {
				return parsedXlsx{}, err
			}
		case strings.HasPrefix(name, "xl/comments") || strings.HasPrefix(name, "xl/threadedComments/"):
			err = countIfText(file, result.skipped, "comments")
		case strings.HasPrefix(name, "xl/drawings/drawing"):
			err = countIfText(file, result.skipped, "textboxes")
		case strings.HasPrefix(name, "xl/charts/chart"):
			err = countIfText(file, result.skipped, "charts")
		case strings.HasPrefix(name, "xl/worksheets/sheet"):
			err = countInlineStrings(file, result.skipped)
		}
		if err != nil {
			return parsedXlsx{}, err
		}
	}
	if !found {
		return parsedXlsx{}, errNoWorkbook
	}
	if result.strings, err = parseSharedStrings(result.sharedStrings); err != nil {
		return parsedXlsx{}, err
	}
	for kind, count := range result.skipped {
		if count == 0 {
			delete(result.skipped, kind)
		}
	}
	return result, nil
}

var sheetName = regexp.MustCompile(`<(?:\w+:)?sheet\s[^>]*?name="([^"]*)"`)

func countSheetNames(workbook []byte) int {
	count := 0
	for _, match := range sheetName.FindAllSubmatch(workbook, -1) {
		name := string(match[1])
		if hasLetter(name) && !defaultSheetName.MatchString(name) {
			count++
		}
	}
	return count
}

// countInlineStrings считает листы с текстом прямо в ячейках (t="inlineStr"), а не в общем списке
// строк: Excel так не пишет, но так делают некоторые выгрузки. Лист читается потоком — он бывает
// больше 100 МБ.
func countInlineStrings(file *zip.File, skipped map[string]int) error {
	reader, err := file.Open()
	if err != nil {
		return err
	}
	defer reader.Close()
	const marker = `t="inlineStr"`
	buffer := make([]byte, 64<<10)
	tail := []byte{}
	for {
		n, err := reader.Read(buffer)
		chunk := append(tail, buffer[:n]...)
		if bytes.Contains(chunk, []byte(marker)) {
			skipped["inline"]++
			return nil
		}
		if len(chunk) > len(marker) {
			tail = append([]byte{}, chunk[len(chunk)-len(marker):]...)
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

func parseSharedStrings(data []byte) ([]sharedString, error) {
	if len(data) == 0 {
		return nil, nil
	}
	s := &scanner{data: data, decoder: xml.NewDecoder(bytes.NewReader(data)), skipped: map[string]int{}}
	var result []sharedString
	for {
		token, _, end, err := s.next()
		if err == io.EOF {
			return result, nil
		}
		if err != nil {
			return nil, err
		}
		if start, ok := token.(xml.StartElement); ok && start.Name.Local == "si" {
			item := sharedString{paragraph: paragraph{kind: "table_cell", start: end}}
			if err := s.sharedString(&item); err != nil {
				return nil, err
			}
			if item.hasLetters() {
				item.trimEdges()
				result = append(result, item)
			}
		}
	}
}

// sharedString разбирает <si>: простой текст <t> или куски <r> с форматированием <rPr>.
// Фонетические подсказки (<rPh>, <phoneticPr>) к переводу не относятся и при сборке убираются.
func (s *scanner) sharedString(item *sharedString) error {
	rPr := ""
	for {
		token, start, _, err := s.next()
		if err != nil {
			return err
		}
		switch t := token.(type) {
		case xml.EndElement:
			switch t.Name.Local {
			case "si":
				item.end = start
				return nil
			case "r":
				rPr = ""
			}
		case xml.StartElement:
			switch t.Name.Local {
			case "r":
				item.rich = true
			case "rPr":
				stop, err := s.skip()
				if err != nil {
					return err
				}
				rPr = string(s.data[start:stop])
			case "t":
				text, err := s.text()
				if err != nil {
					return err
				}
				item.addText(text, rPr)
			case "rPh", "phoneticPr":
				if _, err := s.skip(); err != nil {
					return err
				}
			}
		}
	}
}

// addText — текст куска; перенос строки внутри ячейки становится отдельным элементом (<xN/>).
func (item *sharedString) addText(text, rPr string) {
	for i, line := range strings.Split(text, "\n") {
		if i > 0 {
			item.pieces = append(item.pieces, piece{raw: "&#10;", kind: "br"})
		}
		if line != "" {
			item.pieces = append(item.pieces, piece{text: line, key: "|" + formatKey(rPr), rPr: rPr})
		}
	}
}

func (item sharedString) hasLetters() bool {
	for _, p := range item.pieces {
		if strings.IndexFunc(p.text, unicode.IsLetter) >= 0 {
			return true
		}
	}
	return false
}

// trimEdges убирает пробелы по краям простой строки в lead/trail — при сборке они вернутся вокруг перевода.
func (item *sharedString) trimEdges() {
	if item.rich || len(item.pieces) == 0 {
		return
	}
	first, last := &item.pieces[0], &item.pieces[len(item.pieces)-1]
	if first.raw == "" {
		trimmed := strings.TrimLeftFunc(first.text, unicode.IsSpace)
		item.lead, first.text = first.text[:len(first.text)-len(trimmed)], trimmed
	}
	if last.raw == "" {
		trimmed := strings.TrimRightFunc(last.text, unicode.IsSpace)
		item.trail, last.text = last.text[len(trimmed):], trimmed
	}
}

// ParseXlsxSegments разбирает .xlsx на сегменты и считает то, что остаётся без перевода.
func ParseXlsxSegments(data []byte) ([]ParsedSegment, map[string]int, error) {
	parsed, err := parseXlsx(data)
	if err != nil {
		return nil, nil, err
	}
	segments := make([]ParsedSegment, len(parsed.strings))
	for i, item := range parsed.strings {
		segments[i] = item.segment()
	}
	return segments, parsed.skipped, nil
}

// buildXlsx собирает .xlsx с переводом вместо оригинала (двуязычной выгрузки у Excel нет).
func buildXlsx(original []byte, segments []exportSegment) ([]byte, error) {
	parsed, err := parseXlsx(original)
	if err != nil {
		return nil, err
	}
	if len(parsed.strings) != len(segments) {
		return nil, errSegmentsChanged
	}
	var shared bytes.Buffer
	last := 0
	for i, item := range parsed.strings {
		source, fragments := item.tagged()
		if source.Text != segments[i].source {
			return nil, errSegmentsChanged
		}
		translation := segments[i].translation
		if translation == "" {
			continue
		}
		shared.Write(parsed.sharedStrings[last:item.start])
		shared.WriteString(item.render(normalizeTagged(translation), fragments))
		last = item.end
	}
	shared.Write(parsed.sharedStrings[last:])
	return replacePart(original, "xl/sharedStrings.xml", shared.Bytes())
}

// render — содержимое <si> для перевода: простая строка — один <t>, строка с форматированием —
// куски <r> с исходным rPr (текст без тега и с неизвестным тегом — основное форматирование).
func (item sharedString) render(text string, fragments map[string]fragment) string {
	newline := func(id string) string {
		if fragments[id].kind == "br" {
			return "&#10;"
		}
		return ""
	}
	if !item.rich {
		var plain strings.Builder
		walkTagged(text, func(_, part string) { plain.WriteString(part) }, func(id string) { plain.WriteString(newline(id)) })
		return `<t xml:space="preserve">` + item.lead + plain.String() + item.trail + `</t>`
	}
	var runs strings.Builder
	run := func(tag, part string) {
		if part == "" {
			return
		}
		fragment, ok := fragments[tag]
		if !ok {
			fragment = fragments["g0"]
		}
		runs.WriteString(`<r>` + fragment.rPr + `<t xml:space="preserve">` + part + `</t></r>`)
	}
	walkTagged(text, run, func(id string) { run("", newline(id)) })
	return runs.String()
}
