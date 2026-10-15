package translator

// Теги сегмента в переводе: <gN>…</gN> (фрагмент с форматированием) и <xN/> (элемент Word).

import (
	"regexp"
	"slices"
	"strings"
)

var (
	tagToken     = regexp.MustCompile(`</?g\d+>|<x\d+/>`)
	textEntity   = regexp.MustCompile(`&(amp|lt|gt);`)
	textEntities = map[string]string{"&amp;": "&", "&lt;": "<", "&gt;": ">"}
)

// tagSet — открывающие теги и <xN/> по порядку сортировки; ok = false, если теги сломаны:
// закрывающий без открывающего, вложенные или незакрытые <gN>.
func tagSet(text string) ([]string, bool) {
	var ids []string
	open := ""
	for _, token := range tagToken.FindAllString(text, -1) {
		switch {
		case strings.HasPrefix(token, "<x"):
			ids = append(ids, token)
		case strings.HasPrefix(token, "</"):
			if open == "" || token != "</"+open[1:] {
				return nil, false
			}
			open = ""
		default:
			if open != "" {
				return nil, false
			}
			open = token
			ids = append(ids, token)
		}
	}
	slices.Sort(ids)
	return ids, open == ""
}

// tagsMatch: в переводе те же теги, что в оригинале, по одному разу и не сломаны.
func tagsMatch(source, translation string) bool {
	want, _ := tagSet(source)
	got, ok := tagSet(translation)
	return ok && slices.Equal(want, got)
}

// normalizeTagged экранирует текст между тегами: модель или сотрудник может написать «<» и «&»
// без экранирования, а в document.xml текст должен остаться корректным XML.
func normalizeTagged(text string) string {
	var result strings.Builder
	last := 0
	escape := func(part string) {
		result.WriteString(textEscaper.Replace(textEntity.ReplaceAllStringFunc(part, func(entity string) string { return textEntities[entity] })))
	}
	for _, bounds := range tagToken.FindAllStringIndex(text, -1) {
		escape(text[last:bounds[0]])
		result.WriteString(text[bounds[0]:bounds[1]])
		last = bounds[1]
	}
	escape(text[last:])
	return result.String()
}
