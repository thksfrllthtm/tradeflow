package directory

// Общая база «Документов для ввоза»: перечни продукции, при ввозе которой нужен сертификат или
// декларация о соответствии (СС/ДС), и раздел II Решения КТС №299 (СГР). Нужна, когда справки
// tnved.info по коду нет (лимит) или исчерпан дневной лимит подборов: код ищется прямо в текстах
// перечней. Основной список — ниже; перечни из каждой открытой справки сохраняются в general_acts,
// и база растёт сама. Тексты актов — сводные редакции adilet.zan.kz (со всеми изменениями) в web_pages.

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
)

// SGRActURL — Решение КТС №299: в общей базе из него берётся только раздел II (государственная регистрация).
const SGRActURL = "https://adilet.zan.kz/rus/docs/H10T0000299"

// generalActs — основной список: №299 и перечни к ТР ТС/ЕАЭС, которые чаще всего встречаются в справках.
var generalActs = []WebLink{
	{URL: SGRActURL, Text: "Решение Комиссии Таможенного союза №299 от 28.05.2010 (раздел II — СГР)"},
	{URL: "https://adilet.zan.kz/rus/docs/H13EK000091", Text: "Решение Коллегии ЕЭК №91 от 24.04.2013 (ТР ТС 004/2011)"},
	{URL: "https://adilet.zan.kz/rus/docs/H14EK000002", Text: "Решение Коллегии ЕЭК №2 от 16.01.2014 (ТР ТС 020/2011)"},
	{URL: "https://adilet.zan.kz/rus/docs/H18EK000006", Text: "Решение Коллегии ЕЭК №6 от 16.01.2018 (ТР ТС 010/2011)"},
	{URL: "https://adilet.zan.kz/rus/docs/H18EK000167", Text: "Решение Коллегии ЕЭК №167 от 16.10.2018 (ТР ЕАЭС 037/2016)"},
	{URL: "https://adilet.zan.kz/rus/docs/H19EK000170", Text: "Решение Коллегии ЕЭК №170 от 08.10.2019 (ТР ЕАЭС 043/2017)"},
	{URL: "https://adilet.zan.kz/rus/docs/H24EK000029", Text: "Решение Коллегии ЕЭК №29 от 26.03.2024 (ТР ЕАЭС 050/2021)"},
	{URL: "https://adilet.zan.kz/rus/docs/H12EK000079", Text: "Решение Коллегии ЕЭК №79 от 13.06.2012 (ТР ТС 019/2011)"},
	{URL: "https://adilet.zan.kz/rus/docs/H13EK000028", Text: "Решение Коллегии ЕЭК №28 от 05.03.2013 (ТР ТС 007/2011)"},
	{URL: "https://adilet.zan.kz/rus/docs/H13EK000228", Text: "Решение Коллегии ЕЭК №228 от 15.10.2013 (ТР ТС 017/2011)"},
	{URL: "https://adilet.zan.kz/rus/docs/H15EK000077", Text: "Решение Коллегии ЕЭК №77 от 14.07.2015 (ТР ТС 018/2011)"},
	{URL: "https://adilet.zan.kz/rus/docs/H19EK000012", Text: "Решение Коллегии ЕЭК №12 от 22.01.2019 (ТР ТС 032/2013)"},
	{URL: "https://adilet.zan.kz/rus/docs/H13EK000047", Text: "Решение Коллегии ЕЭК №47 от 19.03.2013 (ТР ТС 005/2011)"},
	{URL: "https://adilet.zan.kz/rus/docs/H22EK000023", Text: "Решение Коллегии ЕЭК №23 от 08.02.2022 (ТР ЕАЭС 048/2019)"},
}

// collegeDecision — «Решение Коллегии ЕЭК №6 от 16.01.2018»: номер и год.
var collegeDecision = regexp.MustCompile(`Решение Коллегии ЕЭК\s*№\s*(\d+)\s+от\s+\d{2}\.\d{2}\.(\d{4})`)

// AdiletURL — сводная редакция решения Коллегии ЕЭК на adilet.zan.kz по названию из справки
// (H<год>EK<номер из 6 цифр>, как у всех решений основного списка); не решение Коллегии — "".
func AdiletURL(title string) string {
	match := collegeDecision.FindStringSubmatch(title)
	if match == nil {
		return ""
	}
	number, _ := strconv.Atoi(match[1])
	return fmt.Sprintf("https://adilet.zan.kz/rus/docs/H%sEK%06d", match[2][2:], number)
}

// conformityActs — перечни СС/ДС из ответа справки: мера «подача ДТ … сопровождается … сертификатом или
// декларацией о соответствии». Решения Коллегии — по адресу сводной редакции adilet.
func conformityActs(body []byte) []WebLink {
	var data goodsHelpResponse
	if json.Unmarshal(body, &data) != nil {
		return nil
	}
	acts := []WebLink{}
	for _, act := range data.MnrsInfo.Restrictions {
		conformity := false
		for _, measure := range act.Meras {
			text := strings.ToLower(measure.MeraName)
			conformity = conformity || (strings.Contains(text, "соответств") && (strings.Contains(text, "сертификат") || strings.Contains(text, "деклараци")))
		}
		title := strings.Join(strings.Fields(act.ActName), " ")
		address := AdiletURL(title)
		if address == "" {
			address, _ = CanonicalWebURL(act.ActURL)
		}
		if conformity && address != "" {
			acts = append(acts, WebLink{URL: address, Text: title})
		}
	}
	return acts
}

func (f *WebPageFetcher) saveGeneralActs(ctx context.Context, body []byte) {
	if f.repository == nil {
		return
	}
	rows := []repository.GeneralAct{}
	for _, act := range conformityActs(body) {
		rows = append(rows, repository.GeneralAct{URL: act.URL, Title: act.Text, AddedAt: time.Now().UTC()})
	}
	_ = f.repository.SaveGeneralActs(ctx, rows)
}

// GeneralActs — общая база: основной список и перечни из справок (без повторов адресов).
func (f *WebPageFetcher) GeneralActs(ctx context.Context) ([]WebLink, error) {
	acts := append([]WebLink{}, generalActs...)
	if f.repository == nil {
		return acts, nil
	}
	rows, err := f.repository.GeneralActs(ctx)
	if err != nil {
		return acts, err
	}
	seen := map[string]bool{}
	for _, act := range acts {
		seen[act.URL] = true
	}
	for _, row := range rows {
		if !seen[row.URL] {
			seen[row.URL] = true
			acts = append(acts, WebLink{URL: row.URL, Text: row.Title})
		}
	}
	return acts, nil
}

// WarmGeneralActs — тексты перечней в базу заранее, в фоне при запуске сервера: adilet отвечает до
// минуты на страницу, а поиск по общей базе должен идти из базы. Свежие копии не скачиваются заново.
func (f *WebPageFetcher) WarmGeneralActs(ctx context.Context) {
	acts, _ := f.GeneralActs(ctx)
	for _, act := range acts {
		if ctx.Err() != nil {
			return
		}
		_, _ = f.Page(ctx, act.URL)
	}
}
