package directory

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAdiletCode(t *testing.T) {
	for address, want := range map[string]string{
		"https://adilet.zan.kz/rus/docs/H10T0000299":     "H10T0000299",
		"https://adilet.zan.kz/rus/docs/Z990000344_":     "Z990000344_",
		"https://adilet.zan.kz/kaz/docs/H18EK000006/":    "H18EK000006",
		"https://adilet.zan.kz/rus/docs/H10T0000299/x":   "",
		"https://adilet.zan.kz/rus/search/docs":          "",
		"https://zakon.uchet.kz/rus/docs/H10T0000299":    "",
		"https://docs.eaeunion.org/documents/446/10220/": "",
	} {
		if got := adiletCode(address); got != want {
			t.Errorf("%s: got %q, want %q", address, got, want)
		}
	}
}

// Образцы — Решение Коллегии ЕЭК №6 (перечень к ТР ТС 010/2011): ответ API adilet by-ngr и та же сводная
// редакция на zakon.uchet.kz; заглушка old.adilet.zan.kz «идут технические работы».
func TestAdiletPageSources(t *testing.T) {
	const actURL = "https://adilet.zan.kz/rus/docs/H18EK000006"
	api, zakon := "json", "act"
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var mode, fixture string
		switch r.URL.Path {
		case "/api/H18EK000006":
			mode, fixture = api, "adilet_api_H18EK000006.json"
		case "/zakon/H18EK000006":
			mode, fixture = zakon, "zakon_H18EK000006.html"
		}
		switch mode {
		case "down":
			http.Error(w, "fail", http.StatusBadGateway)
			return
		case "maintenance":
			fixture = "adilet_maintenance.html"
		case "other":
			_, _ = w.Write([]byte(`{"data":{"id":"x"}}`))
			return
		}
		if strings.HasSuffix(fixture, ".json") {
			w.Header().Set("Content-Type", "application/json")
		} else {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
		}
		_, _ = w.Write(readTestdata(t, fixture))
	}))
	defer server.Close()
	fetcher := testFetcher(server)
	fetcher.adiletAPI, fetcher.zakonBase = server.URL+"/api/", server.URL+"/zakon/"
	ctx := context.Background()

	check := func(name string, wantHeader bool) {
		t.Helper()
		page, err := fetcher.Page(ctx, actURL)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if page.URL != actURL || !strings.Contains(page.Title, "Об утверждении перечня продукции") {
			t.Errorf("%s: url/title %s / %s", name, page.URL, page.Title)
		}
		// Позиция перечня: пункт, коды с «из» и документ — то, что ищет findInAct.
		for _, want := range []string{"Машины для животноводства", "из 8418 69 000 8", "сертификат соответствия"} {
			if !strings.Contains(page.Text, want) {
				t.Errorf("%s: missing %q", name, want)
			}
		}
		if wantHeader && !strings.Contains(page.Text, "Решение Коллегии Евразийской экономической комиссии от 16 января 2018 года № 6") {
			t.Errorf("%s: no act header", name)
		}
		for _, unwanted := range []string{"function(", "zakon.uchet.kz", "Технические работы"} {
			if strings.Contains(page.Text, unwanted) {
				t.Errorf("%s: unexpected %q", name, unwanted)
			}
		}
		for _, link := range page.Links {
			if strings.Contains(link.URL, server.URL) {
				t.Errorf("%s: link to test server %s", name, link.URL)
			}
		}
	}
	check("api", true)
	// API не ответил, сменил формат или отдал заглушку — текст с zakon.uchet.kz.
	for _, mode := range []string{"down", "other", "maintenance"} {
		api = mode
		check("zakon after api "+mode, false)
	}
	// Оба источника отдали не акт — ошибка, а не заглушка в кэш.
	api, zakon = "maintenance", "maintenance"
	if _, err := fetcher.Page(ctx, actURL); err == nil {
		t.Error("maintenance pages must not become an act")
	}
}

func TestMaintenancePageIsNotAPage(t *testing.T) {
	body := string(readTestdata(t, "adilet_maintenance.html"))
	if _, err := pageFromBody("https://example.kz/", "https://example.kz/", body); !errors.Is(err, ErrWebPageUnavailable) {
		t.Fatalf("maintenance page: %v", err)
	}
	long := WebPage{Title: "Закон о связи", Text: strings.Repeat("Статья. Текст закона. ", 200) + "идут технические работы на линии связи"}
	if checkActPage(long) != nil {
		t.Error("phrase deep inside an act is not a stub")
	}
	if checkActPage(WebPage{Title: "Акт", Text: "коротко"}) == nil || checkActPage(WebPage{Text: long.Text}) == nil {
		t.Error("short text and missing title are not an act")
	}
}
