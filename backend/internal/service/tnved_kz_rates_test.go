package service

import (
	"bytes"
	"testing"
	"time"
)

func TestParseKZRateCoversListForms(t *testing.T) {
	for text, want := range map[string]NomenclatureDuty{
		"8":                  {Type: "percentage", Percent: "8"},
		"6,5":                {Type: "percentage", Percent: "6.5"},
		"0":                  {Type: "percentage", Percent: "0"},
		"0,13 евро за 1 л":   {Type: "specific", Amount: "0.13", Currency: "EUR", Unit: "л"},
		"0,6 евро за 1 пару": {Type: "specific", Amount: "0.6", Currency: "EUR", Unit: "пару"},
		"5, но не менее 0,03 евро за 1 кг":                   {Type: "combined_minimum", Percent: "5", Amount: "0.03", Currency: "EUR", Unit: "кг"},
		"15, но не менее 1,2 евро за 1 см³ объема двигателя": {Type: "combined_minimum", Percent: "15", Amount: "1.2", Currency: "EUR", Unit: "см³ объема двигателя"},
		"13 или 1,32 евро за 1 л в зависимости, что ниже":    {Type: "other"},
		"1,5 евро за 1 кг, но не менее 5, но не более 10":    {Type: "other"},
	} {
		if got := ParseKZRate(text); got != want {
			t.Errorf("%q: got %+v, want %+v", text, got, want)
		}
	}
}

func TestFootnoteValidityReadsBothKinds(t *testing.T) {
	date := func(value *time.Time) string {
		if value == nil {
			return ""
		}
		return value.Format("2006-01-02")
	}
	for text, want := range map[string][2]string{
		"Ставка ввозной таможенной пошлины применяется с 1 апреля 2023 г.":                                                                                                                             {"2023-04-01", ""},
		"Ставка ввозной таможенной пошлины Единого таможенного тарифа Евразийского экономического союза применяется с 1 января 2026 г.":                                                                {"", "2025-12-31"},
		"Ставка ввозной таможенной пошлины Единого таможенного тарифа Евразийского экономического союза применяется с 28 марта 2022 г. по 30 сентября 2022 г. включительно, а также с 1 марта 2023 г.": {"", "2023-02-28"},
		"Сноска утратила силу в соответствии с Решением Совета Евразийской экономической комиссии от 15.04.2022 № 63.":                                                                                 {"", ""},
	} {
		from, to := footnoteValidity(text)
		if got := [2]string{date(from), date(to)}; got != want {
			t.Errorf("%q: got %v, want %v", text, got, want)
		}
	}
}

func TestKZStatusOnDate(t *testing.T) {
	today := time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC)
	day := func(y int, m time.Month, d int) *time.Time {
		value := time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
		return &value
	}
	cases := []struct {
		from, to *time.Time
		active   bool
		status   string
	}{
		{nil, nil, true, "действует"},
		{day(2023, 4, 1), nil, true, "действует"},
		{day(2030, 1, 1), nil, false, "с 01.01.2030, до этого ЕТТ"},
		{nil, day(2025, 12, 31), false, "не действует с 01.01.2026, ставка ЕТТ"},
		{nil, day(2027, 2, 28), true, "действует до 28.02.2027"},
	}
	for _, item := range cases {
		if active, status := kzStatus(item.from, item.to, today); active != item.active || status != item.status {
			t.Errorf("%v–%v: got %v %q", item.from, item.to, active, status)
		}
	}
}

func TestKZExtractRoundTripsThroughExcel(t *testing.T) {
	extract := KZListExtract{
		FileName: "ПЕРЕЧЕНЬ РК на 07-02-2026.pdf",
		Rows: []KZListRow{
			{Code: "4015120009", Name: "– – – прочие", Rate: "8", Notes: []string{"3"}, Page: 139},
			{Code: "4015190000", Name: "– – прочие", Rate: "8", Page: 139},
			{Code: "0301919000", Name: "– – – прочая", Rate: "0", Notes: []string{"9"}, Page: 28},
		},
		Footnotes: map[string]string{
			"3": "Ставка ввозной таможенной пошлины применяется с 1 апреля 2023 г.",
			"9": "Ставка ввозной таможенной пошлины Единого таможенного тарифа Евразийского экономического союза применяется с 1 января 2026 г.",
		},
	}
	rates, errs := kzRatesFromExtract(extract)
	if len(errs) != 0 || len(rates) != 3 {
		t.Fatalf("unexpected: %v %v", rates, errs)
	}
	if rates[0].ValidFrom == nil || rates[0].ValidFrom.Format("2006-01-02") != "2023-04-01" || rates[0].NotesText != "3) Ставка ввозной таможенной пошлины применяется с 1 апреля 2023 г." {
		t.Fatalf("footnote 3 not applied: %+v", rates[0])
	}
	if rates[2].ValidTo == nil || rates[2].ValidTo.Format("2006-01-02") != "2025-12-31" {
		t.Fatalf("footnote 9 not applied: %+v", rates[2])
	}
	rows := [][]string{}
	for _, rate := range rates {
		rows = append(rows, []string{rate.Code, rate.Name, rate.RateText, dateCell(rate.ValidFrom), dateCell(rate.ValidTo), "", rate.Notes, rate.NotesText, "139"})
	}
	data, err := writeDirectoryXLSX("Перечень РК", kzColumns, rows)
	if err != nil {
		t.Fatal(err)
	}
	sheet, err := readDirectorySheet(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	if !sheet.has("Код ТН ВЭД") || !sheet.has("Ставка РК") || !sheet.has("Действует с") {
		t.Fatal("Excel must have the columns the import reads")
	}
	from, _ := sheet.at(sheet.raw, 0, "Действует с")
	if value, err := cellDate(from); err != nil || value != "2023-04-01" {
		t.Fatalf("date must read back: %q %v", value, err)
	}
}

func TestKZExtractReportsBadRows(t *testing.T) {
	_, errs := kzRatesFromExtract(KZListExtract{Rows: []KZListRow{
		{Code: "4015190000", Rate: "8"},
		{Code: "401519000", Rate: "8"},
		{Code: "4015190000", Rate: "8"},
		{Code: "6116102000", Rate: ""},
	}})
	if len(errs) != 3 || errs[0].Row != 2 || errs[1].Row != 3 || errs[2].Row != 4 {
		t.Fatalf("expected errors in rows 2, 3, 4, got %+v", errs)
	}
}
