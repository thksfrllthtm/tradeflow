package legalacts

import (
	"regexp"
	"strings"
)

var (
	numericDate = regexp.MustCompile(`\b(\d{2})\.(\d{2})\.(\d{4})\b`)
	wordDate    = regexp.MustCompile(`(\d{1,2})\s+(января|февраля|марта|апреля|мая|июня|июля|августа|сентября|октября|ноября|декабря)\s+(\d{4})`)
	// actNumber — «№ 6», «№ ҚР ДСМ-336/2020» (с пробелом после «ҚР»), «N 339», «№ 199-НҚ»: до пробела, запятой,
	// точки в конце или скобки.
	actNumber = regexp.MustCompile(`(?:№|\bN)\s*((?:[ҚқK][РрR]\s+)?[0-9A-Za-zА-Яа-яҚқӘәҒғҢңӨөҰұҮүІі/\-]+(?:\s*/\s*[0-9]+)?)`)
	months    = map[string]string{"января": "01", "февраля": "02", "марта": "03", "апреля": "04", "мая": "05", "июня": "06",
		"июля": "07", "августа": "08", "сентября": "09", "октября": "10", "ноября": "11", "декабря": "12"}
)

// TitleCheck — совпадают ли дата и номер акта из adilet (official_name) с названием в реестре.
type TitleCheck struct {
	Date, Number bool
	// Official — дата и номер из official_name («16.01.2018 № 6»).
	Official string
}

// OK — и дата, и номер нашлись в названии реестра.
func (c TitleCheck) OK() bool { return c.Date && c.Number }

// CheckTitle — дата и номер из official_name adilet («…от 16 января 2018 года № 6.») среди дат и номеров
// названия в реестре («ТР ТС 010/2011 … — 16.01.2018 № 6 (ред. 2019)»).
func CheckTitle(registryTitle, officialName string) TitleCheck {
	check := TitleCheck{}
	date := ""
	if match := wordDate.FindStringSubmatch(officialName); match != nil {
		day := match[1]
		if len(day) == 1 {
			day = "0" + day
		}
		date = day + "." + months[match[2]] + "." + match[3]
	}
	number := ""
	if match := actNumber.FindStringSubmatch(officialName); match != nil {
		number = normalizeNumber(match[1])
	}
	check.Official = strings.TrimSpace(date + " № " + number)
	for _, match := range numericDate.FindAllString(registryTitle, -1) {
		check.Date = check.Date || (date != "" && match == date)
	}
	for _, match := range actNumber.FindAllStringSubmatch(registryTitle, -1) {
		check.Number = check.Number || (number != "" && normalizeNumber(match[1]) == number)
	}
	return check
}

// normalizeNumber — «ҚР ДСМ-336/2020.» и «қр дсм-336/2020» одинаковы; хвостовые точки и дефисы не важны.
func normalizeNumber(value string) string {
	value = strings.ToLower(strings.Join(strings.Fields(value), ""))
	return strings.TrimRight(value, ".-")
}
