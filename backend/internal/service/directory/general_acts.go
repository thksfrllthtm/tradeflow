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

// generalActsFrom — перечни к регламентам, которые ещё не вступили в силу: с какой даты они участвуют в
// поиске (и из основного списка, и из справок). ТР ЕАЭС 048/2019 вступает в силу «не ранее 1 сентября
// 2028 г.» — Решение Совета ЕЭК №114 от 08.08.2019 в редакции 2025 г.
var generalActsFrom = map[string]time.Time{
	"https://adilet.zan.kz/rus/docs/H22EK000023": time.Date(2028, time.September, 1, 0, 0, 0, 0, time.UTC),
}

// generalActInForce — перечень участвует в поиске на дату.
func generalActInForce(address string, date time.Time) bool {
	from, ok := generalActsFrom[address]
	return !ok || !date.Before(from)
}

var (
	// eecDecision — «Решение Коллегии ЕЭК №6 от 16.01.2018», «№30 (2015) от 21.04.2015», «Решение Совета ЕЭК №11
	// от 22.01.2025»: орган, номер и год.
	eecDecision = regexp.MustCompile(`Решение (Коллегии|Совета) ЕЭК\s*№\s*(\d+)(?:\s*\(\d{4}\))?\s+от\s+\d{2}\.\d{2}\.(\d{4})`)
	// Карточка решения на docs.eaeunion.org: короткий заголовок и дата принятия.
	eecCardTitle = regexp.MustCompile(`Короткий заголовок документа\s*\n\s*(Решение (?:Коллегии|Совета) ЕЭК\s*№\s*\d+)`)
	eecCardDate  = regexp.MustCompile(`Дата принятия документа\s*\n\s*(\d{2}\.\d{2}\.\d{4})`)
)

// EECCardAdiletURL — карточка решения Коллегии или Совета на docs.eaeunion.org (сам текст там в PDF, который не
// читается) → сводная редакция на adilet; не карточка решения ЕЭК — "".
func EECCardAdiletURL(page WebPage) string {
	title, date := eecCardTitle.FindStringSubmatch(page.Text), eecCardDate.FindStringSubmatch(page.Text)
	if title == nil || date == nil {
		return ""
	}
	return AdiletURL(title[1] + " от " + date[1])
}

// WithAdiletLinks — в справке ссылки на решения Коллегии и Совета ЕЭК (docs.eaeunion.org, PDF) заменены на
// сводные редакции adilet; прежние ссылки остаются среди разрешённых.
func WithAdiletLinks(page WebPage) WebPage {
	links := append([]WebLink{}, page.Links...)
	for _, link := range page.Links {
		if address := AdiletURL(link.Text); address != "" && address != link.URL {
			page.Text = strings.ReplaceAll(page.Text, link.URL, address)
			links = append(links, WebLink{URL: address, Text: link.Text})
		}
	}
	page.Links = links
	return page
}

// AdiletURL — сводная редакция решения Коллегии или Совета ЕЭК на adilet.zan.kz по названию из справки:
// H<год>EK<номер из 6 цифр> у Коллегии, H<год>EV… у Совета; не решение ЕЭК — "".
func AdiletURL(title string) string {
	match := eecDecision.FindStringSubmatch(title)
	if match == nil {
		return ""
	}
	body := "EK"
	if match[1] == "Совета" {
		body = "EV"
	}
	number, _ := strconv.Atoi(match[2])
	return fmt.Sprintf("https://adilet.zan.kz/rus/docs/H%s%s%06d", match[3][2:], body, number)
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

// GeneralActs — общая база на сегодня: основной список и перечни из справок (без повторов адресов и
// перечней к регламентам, которые ещё не вступили в силу).
func (f *WebPageFetcher) GeneralActs(ctx context.Context) ([]WebLink, error) {
	var rows []repository.GeneralAct
	var err error
	if f.repository != nil {
		rows, err = f.repository.GeneralActs(ctx)
	}
	return generalActsOn(rows, time.Now()), err
}

func generalActsOn(rows []repository.GeneralAct, date time.Time) []WebLink {
	acts := append([]WebLink{}, generalActs...)
	for _, row := range rows {
		acts = append(acts, WebLink{URL: row.URL, Text: row.Title})
	}
	inForce := []WebLink{}
	seen := map[string]bool{}
	for _, act := range acts {
		if !seen[act.URL] && generalActInForce(act.URL, date) {
			inForce = append(inForce, act)
		}
		seen[act.URL] = true
	}
	return inForce
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
