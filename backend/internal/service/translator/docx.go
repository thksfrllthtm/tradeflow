package translator

// Разбор .docx на сегменты. Документ не пересобирается целиком: encoding/xml при записи портит
// префиксы w:, поэтому разбор только находит байтовые границы абзацев, а выгрузка заменяет
// содержимое абзацев-сегментов и копирует остальное байт в байт.
//
// Сегмент — абзац w:p с текстом (в таблице — абзац ячейки). Соседние куски текста с одинаковым
// форматированием склеиваются (Word дробит текст на w:r из-за правок и проверки орфографии).
// Текст сегмента — XML-фрагмент: основное форматирование абзаца не размечается (g0), фрагменты
// с другим — <gN>…</gN>, неразбираемые элементы (табуляция, перенос, картинка, поле) — <xN/>.

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

var (
	errTrackedChanges = errors.New("docx: tracked changes")
	errNoDocument     = errors.New("docx: no word/document.xml")
)

// maxPartBytes — предел распакованной части .docx (защита от zip-бомбы).
const maxPartBytes = 100 << 20

// piece — кусок содержимого абзаца: текст с форматированием или неразбираемый элемент.
type piece struct {
	text string // текст w:t (для raw — пусто)
	key  string // форматирование для склейки: обёртка + rPr без несущественных отличий
	rPr  string // исходный <w:rPr>…</w:rPr>, "" — без форматирования
	wrap string // открывающий тег обёртки (<w:hyperlink …>) или ""
	raw  string // неразбираемый элемент целиком, отдельным w:r (для текста — пусто)
	kind string // для raw: tab, br, image, field, formula, other
}

type paragraph struct {
	kind    string // paragraph | table_cell
	start   int    // начало содержимого w:p после w:pPr
	end     int    // начало </w:p>
	close   int    // конец </w:p>
	pPr     string // исходный <w:pPr>…</w:pPr> ("" — нет)
	anchors string // закладки и метки комментариев: при сборке встают в начало абзаца
	pieces  []piece
}

type parsedDocx struct {
	document   []byte
	paragraphs []paragraph // только абзацы-сегменты, по порядку
	skipped    map[string]int
}

// ParsedSegment — сегмент для записи в базу.
type ParsedSegment struct {
	Kind string
	Text string
	Tags map[string]string // g0, gN → <w:rPr>; xN → вид элемента (tab, br, image, …)
}

func parseDocx(data []byte) (parsedDocx, error) {
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return parsedDocx{}, err
	}
	result := parsedDocx{skipped: map[string]int{}}
	found := false
	for _, file := range archive.File {
		name := file.Name
		switch {
		case name == "word/document.xml":
			found = true
			if result.document, err = readPart(file); err != nil {
				return parsedDocx{}, err
			}
		case strings.HasPrefix(name, "word/header") || strings.HasPrefix(name, "word/footer"):
			err = countIfText(file, result.skipped, "headers")
		case name == "word/footnotes.xml" || name == "word/endnotes.xml":
			err = countIfText(file, result.skipped, "footnotes")
		case strings.HasPrefix(name, "word/charts/chart") || strings.HasPrefix(name, "word/diagrams/data"):
			err = countIfText(file, result.skipped, "charts")
		}
		if err != nil {
			return parsedDocx{}, err
		}
	}
	if !found {
		return parsedDocx{}, errNoDocument
	}
	s := &scanner{data: result.document, decoder: xml.NewDecoder(bytes.NewReader(result.document)), skipped: result.skipped}
	if err := s.body(); err != nil {
		return parsedDocx{}, err
	}
	result.paragraphs = s.paragraphs
	for kind, count := range result.skipped {
		if count == 0 {
			delete(result.skipped, kind)
		}
	}
	return result, nil
}

func readPart(file *zip.File) ([]byte, error) {
	if file.UncompressedSize64 > maxPartBytes {
		return nil, fmt.Errorf("docx: %s is too large", file.Name)
	}
	reader, err := file.Open()
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(io.LimitReader(reader, maxPartBytes))
}

// countIfText считает часть документа (колонтитул, сноски, диаграмму), если в ней есть буквы:
// колонтитул с одним номером страницы переводить не нужно.
func countIfText(file *zip.File, skipped map[string]int, kind string) error {
	data, err := readPart(file)
	if err != nil {
		return err
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		token, err := decoder.RawToken()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if text, ok := token.(xml.CharData); ok && hasLetter(string(text)) {
			skipped[kind]++
			return nil
		}
	}
}

func hasLetter(text string) bool { return strings.IndexFunc(text, unicode.IsLetter) >= 0 }

type scanner struct {
	data       []byte
	decoder    *xml.Decoder
	skipped    map[string]int
	paragraphs []paragraph
	tables     int
}

// next возвращает следующий токен и его байтовый диапазон в document.xml.
func (s *scanner) next() (xml.Token, int, int, error) {
	start := int(s.decoder.InputOffset())
	token, err := s.decoder.RawToken()
	if err != nil {
		return nil, 0, 0, err
	}
	return xml.CopyToken(token), start, int(s.decoder.InputOffset()), nil
}

func qname(name xml.Name) string {
	if name.Space == "" {
		return name.Local
	}
	return name.Space + ":" + name.Local
}

// body проходит весь документ: абзацы на любом уровне (в таблицах, блоках w:sdt), кроме надписей —
// те внутри w:r и уходят в неразбираемые куски вместе с картинкой.
func (s *scanner) body() error {
	for {
		token, _, end, err := s.next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		switch t := token.(type) {
		case xml.StartElement:
			switch qname(t.Name) {
			case "w:tbl":
				s.tables++
			case "w:p":
				kind := "paragraph"
				if s.tables > 0 {
					kind = "table_cell"
				}
				p := paragraph{kind: kind, start: end}
				if err := s.inline(&p, "", true); err != nil {
					return err
				}
				if isSegment(p) {
					s.paragraphs = append(s.paragraphs, p)
				}
			case "w:ins", "w:del", "w:moveFrom", "w:moveTo":
				return errTrackedChanges
			}
		case xml.EndElement:
			if qname(t.Name) == "w:tbl" {
				s.tables--
			}
		}
	}
}

// inline разбирает содержимое абзаца или обёртки внутри него (гиперссылка, smartTag) до её конца.
func (s *scanner) inline(p *paragraph, wrap string, top bool) error {
	for {
		token, start, end, err := s.next()
		if err != nil {
			return err
		}
		switch t := token.(type) {
		case xml.EndElement:
			if top {
				p.end, p.close = start, end
			}
			return nil
		case xml.StartElement:
			switch name := qname(t.Name); name {
			case "w:pPr":
				if p.start, err = s.skip(); err != nil {
					return err
				}
				p.pPr = string(s.data[start:p.start])
			case "w:r":
				err = s.run(p, wrap)
			case "w:hyperlink":
				err = s.inline(p, string(s.data[start:end]), false)
			case "w:smartTag", "w:customXml", "w:dir", "w:bdo":
				err = s.inline(p, wrap, false)
			case "w:smartTagPr", "w:customXmlPr", "w:proofErr":
				_, err = s.skip()
			case "w:bookmarkStart", "w:bookmarkEnd", "w:commentRangeStart", "w:commentRangeEnd", "w:permStart", "w:permEnd":
				var stop int
				if stop, err = s.skip(); err == nil {
					p.anchors += string(s.data[start:stop])
				}
			case "w:ins", "w:del", "w:moveFrom", "w:moveTo":
				return errTrackedChanges
			default:
				kind := "other"
				switch name {
				case "m:oMath", "m:oMathPara":
					kind = "formula"
					s.skipped["formulas"]++
				case "w:sdt":
					kind = "field"
					s.skipped["fields"]++
				case "w:fldSimple":
					kind = "field"
				}
				var stop int
				if stop, err = s.skip(); err == nil {
					p.pieces = append(p.pieces, piece{raw: string(s.data[start:stop]), kind: kind})
				}
			}
			if err != nil {
				return err
			}
		}
	}
}

// run разбирает w:r: текст w:t становится текстовыми кусками, остальное (табуляция, картинка,
// поле) — отдельными w:r с тем же rPr.
func (s *scanner) run(p *paragraph, wrap string) error {
	rPr := ""
	for {
		token, start, end, err := s.next()
		if err != nil {
			return err
		}
		switch t := token.(type) {
		case xml.EndElement:
			return nil
		case xml.StartElement:
			name := qname(t.Name)
			stop := end
			switch name {
			case "w:t":
				text, err := s.text()
				if err != nil {
					return err
				}
				if text != "" {
					p.pieces = append(p.pieces, piece{text: text, key: wrap + "|" + formatKey(rPr), rPr: rPr, wrap: wrap})
				}
				continue
			case "w:rPr":
				if stop, err = s.skip(); err != nil {
					return err
				}
				rPr = string(s.data[start:stop])
				continue
			case "w:lastRenderedPageBreak":
				_, err = s.skip()
				if err != nil {
					return err
				}
				continue
			}
			if stop, err = s.skip(); err != nil {
				return err
			}
			kind := "other"
			switch name {
			case "w:tab", "w:ptab":
				kind = "tab"
			case "w:br", "w:cr":
				kind = "br"
			case "w:drawing", "w:pict", "w:object", "mc:AlternateContent":
				kind = "image"
			case "w:fldChar", "w:instrText":
				kind = "field"
			}
			p.pieces = append(p.pieces, piece{raw: "<w:r>" + rPr + string(s.data[start:stop]) + "</w:r>", kind: kind, wrap: wrap})
		}
	}
}

// text читает содержимое w:t до закрывающего тега.
func (s *scanner) text() (string, error) {
	var text strings.Builder
	for {
		token, _, _, err := s.next()
		if err != nil {
			return "", err
		}
		switch t := token.(type) {
		case xml.CharData:
			text.Write(t)
		case xml.EndElement:
			return text.String(), nil
		}
	}
}

// skip пропускает текущий элемент до его конца и возвращает конец в байтах. Попутно считает
// надписи с текстом (w:txbxContent); копия надписи для старых версий Word (mc:Fallback) не считается.
func (s *scanner) skip() (int, error) {
	depth, fallback, textbox := 1, 0, -1
	textboxText := false
	for depth > 0 {
		token, _, end, err := s.next()
		if err != nil {
			return 0, err
		}
		switch t := token.(type) {
		case xml.StartElement:
			depth++
			switch qname(t.Name) {
			case "mc:Fallback":
				fallback++
			case "w:txbxContent":
				if fallback == 0 && textbox < 0 {
					textbox, textboxText = depth, false
				}
			}
		case xml.EndElement:
			switch qname(t.Name) {
			case "mc:Fallback":
				fallback--
			case "w:txbxContent":
				if depth == textbox {
					if textboxText {
						s.skipped["textboxes"]++
					}
					textbox = -1
				}
			}
			depth--
			if depth == 0 {
				return end, nil
			}
		case xml.CharData:
			if textbox >= 0 && hasLetter(string(t)) {
				textboxText = true
			}
		}
	}
	return 0, nil
}

func isSegment(p paragraph) bool {
	for _, item := range p.pieces {
		if item.raw == "" && strings.TrimSpace(item.text) != "" {
			return true
		}
	}
	return false
}

// Отличия rPr, из-за которых Word дробит текст, но которые не меняют вид: язык проверки
// орфографии, «не проверять», подсказка шрифта для иероглифов, история правок форматирования.
var formatNoise = regexp.MustCompile(`<w:lang [^>]*/>|<w:noProof(?: [^>]*)?/>|\s+w:hint="[^"]*"|<w:rPrChange[\s\S]*?</w:rPrChange>`)

func formatKey(rPr string) string {
	return strings.ReplaceAll(formatNoise.ReplaceAllString(rPr, ""), "<w:rFonts/>", "")
}

var textEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

// fragment — склеенные соседние куски с одинаковым форматированием.
type fragment struct {
	piece
	id string
}

func (p paragraph) fragments() []fragment {
	var result []fragment
	for _, item := range p.pieces {
		last := len(result) - 1
		if item.raw == "" && last >= 0 && result[last].raw == "" && result[last].key == item.key {
			result[last].text += item.text
			continue
		}
		result = append(result, fragment{piece: item})
	}
	return result
}

// baseKey — форматирование большей части текста абзаца: оно не размечается тегом.
func baseKey(fragments []fragment) string {
	length := map[string]int{}
	var order []string
	for _, item := range fragments {
		if item.raw != "" {
			continue
		}
		if _, seen := length[item.key]; !seen {
			order = append(order, item.key)
		}
		length[item.key] += utf8.RuneCountInString(item.text)
	}
	best := ""
	for _, key := range order {
		if best == "" || length[key] > length[best] {
			best = key
		}
	}
	return best
}

// segment собирает текст сегмента с тегами и форматирование тегов.
func (p paragraph) segment() ParsedSegment {
	segment, _ := p.tagged()
	return segment
}

// tagged — сегмент и исходные фрагменты по тегам (g0 — первый фрагмент основного форматирования):
// по ним выгрузка превращает теги перевода обратно в w:r.
func (p paragraph) tagged() (ParsedSegment, map[string]fragment) {
	fragments := p.fragments()
	base := baseKey(fragments)
	tags := map[string]string{}
	byID := map[string]fragment{}
	var text strings.Builder
	n := 0
	for _, item := range fragments {
		switch {
		case item.raw != "":
			n++
			id := fmt.Sprintf("x%d", n)
			tags[id], byID[id] = item.kind, item
			text.WriteString("<" + id + "/>")
		case item.key == base:
			if item.rPr != "" {
				tags["g0"] = item.rPr
			}
			if _, ok := byID["g0"]; !ok {
				byID["g0"] = item
			}
			text.WriteString(textEscaper.Replace(item.text))
		default:
			n++
			id := fmt.Sprintf("g%d", n)
			tags[id], byID[id] = item.rPr, item
			text.WriteString("<" + id + ">" + textEscaper.Replace(item.text) + "</" + id + ">")
		}
	}
	return ParsedSegment{Kind: p.kind, Text: text.String(), Tags: tags}, byID
}

// ParseSegments разбирает .docx на сегменты и считает то, что остаётся без перевода.
func ParseSegments(data []byte) ([]ParsedSegment, map[string]int, error) {
	parsed, err := parseDocx(data)
	if err != nil {
		return nil, nil, err
	}
	segments := make([]ParsedSegment, len(parsed.paragraphs))
	for i, p := range parsed.paragraphs {
		segments[i] = p.segment()
	}
	return segments, parsed.skipped, nil
}
