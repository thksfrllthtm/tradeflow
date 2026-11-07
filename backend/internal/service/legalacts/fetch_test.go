package legalacts

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

var actHTML = "<article><p>Коллегия Евразийской экономической комиссии решила: утвердить перечень.</p>" +
	strings.Repeat("<p>6. Машины для животноводства из 8418 69 000 8 сертификат соответствия</p>", 10) + "</article>"

func TestFetchSources(t *testing.T) {
	api, zakon := "ok", "ok"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/by-ngr/H18EK000006":
			switch api {
			case "ok":
				_, _ = w.Write([]byte(`{"document":{"id":"7f","group_number":"H18EK000006","status":"upd","end_date":"2021-12-30T18:00:00.000Z",` +
					`"official_name":"Решение Коллегии Евразийской экономической комиссии от 16 января 2018 года № 6.","title_ru":"Об утверждении перечня","text_content":` +
					quote(actHTML) + `}}`))
			case "changed":
				_, _ = w.Write([]byte(`{"data":{"id":"7f"}}`))
			case "missing":
				http.NotFound(w, r)
			default:
				http.Error(w, "down", http.StatusBadGateway)
			}
		case "/api/7f/versions":
			_, _ = w.Write([]byte(`{"versions":[{"is_current":false,"revision_date":"2019-01-01T18:00:00.000Z"},{"is_current":true,"revision_date":"2021-12-30T18:00:00.000Z"}]}`))
		case "/zakon/H18EK000006":
			switch zakon {
			case "ok":
				_, _ = w.Write([]byte(`<html><title>Об утверждении перечня</title><div>Реклама</div><span>изменениями на: 01.01.2022</span>` + actHTML + `<footer>подвал</footer></html>`))
			case "missing":
				http.NotFound(w, r)
			default:
				_, _ = w.Write([]byte(`<html><title>Технические работы</title><article>ИДУТ ТЕХНИЧЕСКИЕ РАБОТЫ</article></html>`))
			}
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	source := Source{Client: server.Client(), AdiletAPI: server.URL + "/api/", Zakon: server.URL + "/zakon/"}
	ctx := context.Background()

	revision, err := source.Fetch(ctx, "H18EK000006")
	if err != nil {
		t.Fatal(err)
	}
	if revision.Source != "adilet" || revision.SourceURL != "https://adilet.zan.kz/rus/docs/H18EK000006" || revision.ActStatus != "upd" || revision.APIProblem != "" {
		t.Errorf("adilet revision: %+v", revision)
	}
	// Текущая версия — 31.12.2021 (полночь UTC+6), а не первая в списке.
	if got := revision.RevisionDate.Format("2006-01-02"); got != "2021-12-31" {
		t.Errorf("revision date %s", got)
	}
	if string(revision.Content) != actHTML || len(revision.SHA256) != 64 {
		t.Errorf("content/sha: %d %s", len(revision.Content), revision.SHA256)
	}

	// API сменил формат или упал — текст с zakon.uchet.kz, причина для администратора; sha — только от <article>.
	for _, mode := range []string{"changed", "down"} {
		api = mode
		revision, err = source.Fetch(ctx, "H18EK000006")
		if err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
		if revision.Source != "zakon" || revision.APIProblem == "" || revision.RevisionDate.Format("02.01.2006") != "01.01.2022" {
			t.Errorf("%s: %+v", mode, revision)
		}
		if string(revision.Content) != actHTML || revision.SHA256 != digest([]byte(actHTML)) {
			t.Errorf("%s: zakon content must be the article only", mode)
		}
	}

	// Заглушка вместо акта — ошибка, а не редакция.
	api, zakon = "down", "maintenance"
	if _, err := source.Fetch(ctx, "H18EK000006"); err == nil || errors.Is(err, ErrNotFound) {
		t.Errorf("maintenance: %v", err)
	}
	// Кода нет нигде — not found.
	api, zakon = "missing", "missing"
	if _, err := source.Fetch(ctx, "H18EK000006"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing: %v", err)
	}
}

func quote(value string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value) + `"`
}

func TestAdiletDate(t *testing.T) {
	for value, want := range map[string]string{
		"2026-01-17T18:00:00.000Z": "2026-01-18",
		"2025-07-07T18:00:00.000Z": "2025-07-08",
		"2024-05-05T18:00:00Z":     "2024-05-06",
	} {
		if got := adiletDate(value).Format("2006-01-02"); got != want {
			t.Errorf("%s: %s, want %s", value, got, want)
		}
	}
	if !adiletDate("").IsZero() || !adiletDate("вчера").Equal(time.Time{}) {
		t.Error("not a date must be zero")
	}
}

func TestCheckTitle(t *testing.T) {
	for _, item := range []struct {
		registry, official string
		date, number       bool
	}{
		{"ТР ТС 010/2011 машины и оборудование — 16.01.2018 № 6 (ред. 2019, 2021)", "Решение Коллегии Евразийской экономической комиссии от 16 января 2018 года № 6.", true, true},
		{"ТР ТС 004/2011 низковольтное оборудование — 24.04.2013 № 91 (ред. 14.03.2023 № 29)", "Решение Коллегии Евразийской экономической комиссии от 24 апреля 2013 года № 91.", true, true},
		// В реестре дата 05.09.2023, у adilet — 5 октября 2023.
		{"ТР ТС 031/2012 тракторы и прицепы — 05.09.2023 № 136", "Решение Коллегии Евразийской экономической комиссии от 5 октября 2023 года № 136.", false, true},
		{"Закон РК от 10.07.2002 № 339 «О ветеринарии»", "Закон Республики Казахстан от 10 июля 2002 года N 339.", true, true},
		{"Приказ МСХ № 16-04/647 от 09.12.2014 — ветеринарное разрешение", "Приказ Министра сельского хозяйства Республики Казахстан от 9 декабря 2014 года № 16-04/647. Зарегистрирован", true, true},
		{"Решение КТС от 28.05.2010 № 299 — санитарный надзор", "Решение Комиссии таможенного союза от 28 мая 2010 года № 299.", true, true},
		{"Приказ МЗ № ҚР ДСМ-336/2020 от 30.12.2020 — санэпидзаключения и СГР", "Приказ Министра здравоохранения Республики Казахстан от 30 декабря 2020 года № ҚР ДСМ-336/2020. Зарегистрирован", true, true},
		{"Приказ МЗ № ҚР ДСМ-43/2020 от 30.04.2020", "Приказ Министра здравоохранения Республики Казахстан от 30 апреля 2020 года № ҚР ДСМ-177/2020.", true, false},
		{"ТР ТС 005/2011 упаковка — 19.03.2013 № 47", "Решение Коллегии Евразийской экономической комиссии от 19 марта 2013 года № 44", true, false},
	} {
		check := CheckTitle(item.registry, item.official)
		if check.Date != item.date || check.Number != item.number {
			t.Errorf("%s: %+v, want date %v number %v", item.registry, check, item.date, item.number)
		}
	}
}
