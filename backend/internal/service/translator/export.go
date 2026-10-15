package translator

// Выгрузка перевода в .docx. Исходный файл разбирается заново, и меняется только содержимое
// абзацев-сегментов: стили, таблицы, картинки, нумерация, колонтитулы и остальные части архива
// копируются как есть.
//
// Теги перевода превращаются обратно в w:r: <gN> — с исходным rPr фрагмента, текст без тега — с
// основным форматированием абзаца, <xN/> — исходный элемент (картинка, табуляция, поле). Сломанные
// теги не ломают выгрузку: лишние отбрасываются, а потерянные элементы (картинки, части полей)
// дописываются в конец абзаца, чтобы не пропасть из документа.

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"regexp"
	"strconv"
	"strings"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
)

// errSegmentsChanged — сегменты в базе не совпадают с разбором файла (разбор менялся после загрузки).
var errSegmentsChanged = errors.New("docx: segments do not match the original file")

type exportSegment struct {
	source      string // текст оригинала из базы — сверка с разбором файла
	translation string // "" — перевода нет, абзац не меняется
}

// buildDocx собирает .docx с переводом; bilingual — абзац перевода серым курсивом под оригиналом.
func buildDocx(original []byte, segments []exportSegment, bilingual bool) ([]byte, error) {
	parsed, err := parseDocx(original)
	if err != nil {
		return nil, err
	}
	if len(parsed.paragraphs) != len(segments) {
		return nil, errSegmentsChanged
	}
	var document bytes.Buffer
	last := 0
	for i, p := range parsed.paragraphs {
		source, fragments := p.tagged()
		if source.Text != segments[i].source {
			return nil, errSegmentsChanged
		}
		translation := segments[i].translation
		if translation == "" {
			continue
		}
		runs := renderRuns(normalizeTagged(translation), fragments, bilingual)
		if bilingual {
			document.Write(parsed.document[last:p.close])
			document.WriteString("<w:p>" + copiedPPr(p.pPr) + runs + "</w:p>")
			last = p.close
		} else {
			document.Write(parsed.document[last:p.start])
			document.WriteString(p.anchors + runs)
			last = p.end
		}
	}
	document.Write(parsed.document[last:])
	return replacePart(original, "word/document.xml", document.Bytes())
}

var openingName = regexp.MustCompile(`^<([\w:]+)`)

// renderRuns превращает текст с тегами в w:r. В двуязычной выгрузке картинки и поля не
// повторяются (дубликат картинки или половина поля испортили бы документ) — только табуляция и перенос.
func renderRuns(text string, fragments map[string]fragment, bilingual bool) string {
	var result strings.Builder
	wrap := ""
	setWrap := func(next string) {
		if next == wrap {
			return
		}
		if wrap != "" {
			result.WriteString("</" + openingName.FindStringSubmatch(wrap)[1] + ">")
		}
		result.WriteString(next)
		wrap = next
	}
	writeText := func(id, part string) {
		if part == "" {
			return
		}
		item, ok := fragments[id]
		if !ok {
			item = fragments["g0"] // неизвестный тег — основное форматирование абзаца
		}
		rPr := item.rPr
		if bilingual {
			rPr = grayItalic(rPr)
		}
		setWrap(item.wrap)
		result.WriteString(`<w:r>` + rPr + `<w:t xml:space="preserve">` + part + `</w:t></w:r>`)
	}
	used := map[string]bool{}
	writeRaw := func(id string) {
		item, ok := fragments[id]
		if !ok || used[id] || bilingual && item.kind != "tab" && item.kind != "br" {
			return
		}
		used[id] = true
		setWrap(item.wrap)
		result.WriteString(item.raw)
	}
	walkTagged(text, writeText, writeRaw)
	if !bilingual {
		for id := 1; id <= len(fragments); id++ {
			writeRaw("x" + strconv.Itoa(id))
		}
	}
	setWrap("")
	return result.String()
}

var (
	rFontsElement = regexp.MustCompile(`<w:rFonts [^>]*/>`)
	sizeElement   = regexp.MustCompile(`<w:sz w:val="\d+"\s*/>`)
)

// grayItalic — форматирование перевода в двуязычной выгрузке: серый курсив, жирный, подчёркивание,
// шрифт и размер оригинала. rPr собирается заново, потому что Word требует порядок элементов.
func grayItalic(rPr string) string {
	result := "<w:rPr>" + rFontsElement.FindString(rPr)
	if switchedOn(boldPattern, rPr) {
		result += "<w:b/>"
	}
	result += `<w:i/><w:color w:val="808080"/>` + sizeElement.FindString(rPr)
	if switchedOn(underlinePattern, rPr) {
		result += `<w:u w:val="single"/>`
	}
	return result + "</w:rPr>"
}

// Свойства абзаца, которые нельзя повторять в абзаце перевода: нумерация списка (дала бы лишний
// номер), разрыв раздела, рамка и история правок.
var uncopiedPPr = regexp.MustCompile(`<w:numPr>[\s\S]*?</w:numPr>|<w:numPr/>|<w:sectPr[\s\S]*?</w:sectPr>|<w:framePr [^>]*/>|<w:pPrChange[\s\S]*?</w:pPrChange>`)

func copiedPPr(pPr string) string { return uncopiedPPr.ReplaceAllString(pPr, "") }

// replacePart копирует архив, заменяя одну часть; остальные части переносятся без перепаковки.
func replacePart(original []byte, name string, data []byte) ([]byte, error) {
	archive, err := zip.NewReader(bytes.NewReader(original), int64(len(original)))
	if err != nil {
		return nil, err
	}
	var result bytes.Buffer
	writer := zip.NewWriter(&result)
	for _, file := range archive.File {
		if file.Name == name {
			header := file.FileHeader
			header.Method = zip.Deflate
			part, err := writer.CreateHeader(&header)
			if err != nil {
				return nil, err
			}
			if _, err := part.Write(data); err != nil {
				return nil, err
			}
			continue
		}
		part, err := writer.CreateRaw(&file.FileHeader)
		if err != nil {
			return nil, err
		}
		raw, err := file.OpenRaw()
		if err != nil {
			return nil, err
		}
		if _, err := io.Copy(part, raw); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return result.Bytes(), nil
}

// Export собирает файл документа: перевод вместо оригинала или (только Word) двуязычный.
// Перевод сегмента — правка сотрудника или машинный; без перевода текст остаётся как есть.
func (s *Service) Export(ctx context.Context, id int64, bilingual bool) (string, []byte, error) {
	document, err := s.repository.TranslatorDocumentByID(ctx, id)
	if err != nil {
		return "", nil, err
	}
	if bilingual && document.Format == "xlsx" {
		return "", nil, &service.ValidationError{Field: "mode", Message: "Двуязычная выгрузка есть только для документов Word"}
	}
	original, err := s.repository.TranslatorDocumentFile(ctx, id)
	if err != nil {
		return "", nil, err
	}
	segments, err := s.repository.TranslatorSegments(ctx, id)
	if err != nil {
		return "", nil, err
	}
	items := make([]exportSegment, len(segments))
	for i, segment := range segments {
		translation := segment.TargetText
		if translation == "" {
			translation = segment.MTText
		}
		items[i] = exportSegment{source: segment.SourceText, translation: translation}
	}
	var data []byte
	if document.Format == "xlsx" {
		data, err = buildXlsx(original, items)
	} else {
		data, err = buildDocx(original, items, bilingual)
	}
	if errors.Is(err, errSegmentsChanged) {
		return "", nil, &service.ValidationError{Field: "file", Message: "Документ разобран старой версией программы и не совпадает с файлом. Загрузите файл заново"}
	}
	if err != nil {
		return "", nil, err
	}
	suffix := document.TargetLang
	if bilingual {
		suffix = document.SourceLang + "-" + document.TargetLang
	}
	return document.Name + "_" + suffix + "." + document.Format, data, nil
}
