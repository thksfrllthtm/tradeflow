package service

// Общие помощники текста для всех сервисов.

import "unicode/utf8"

// truncateRunes обрезает строку до limit символов (не байт).
func TruncateRunes(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
}
