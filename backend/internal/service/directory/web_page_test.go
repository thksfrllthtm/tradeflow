package directory

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"strings"
	"testing"
)

func readTestdata(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func hasLink(links []WebLink, address string) bool {
	for _, link := range links {
		if link.URL == address {
			return true
		}
	}
	return false
}

// Образец — ответ API справки tnved.info для 0207 11 300 1 (мясо кур), ввоз.
func TestParseGoodsHelpMeasuresCommentsAndLinks(t *testing.T) {
	page, err := parseGoodsHelp("0207113001", readTestdata(t, "goodshelp_0207113001.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"Справка по товару 0207 11 300 1",
		"1. Постановление Правительства РК №61 от 20.01.2003 — https://adilet.zan.kz/rus/docs/P030000061_",
		"Мера: Обязательно наличие подтверждения осуществления ветеринарного контроля.",
		"Решение Комиссии Таможенного Союза №299 от 28.05.2010",
		"Относится к товарам: Пищевые продукты (продукты в натуральном или переработанном виде",
		"Изменения: Решение Коллегии ЕЭК №83 от 14.07.2015",
		"Прочие сведения:",
	} {
		if !strings.Contains(page.Text, want) {
			t.Errorf("missing %q in:\n%s", want, page.Text)
		}
	}
	// http:// из справки поднимается до https, ссылки вложенных актов — тоже в списке.
	for _, want := range []string{"https://adilet.zan.kz/rus/docs/H10T0000299", "https://docs.eaeunion.org/documents/446/10220/"} {
		if !hasLink(page.Links, want) {
			t.Errorf("missing link %s in %+v", want, page.Links)
		}
	}
	if !strings.Contains(page.Links[0].Text, "Постановление Правительства РК №61") {
		t.Errorf("link text: %+v", page.Links[0])
	}
	other, err := parseGoodsHelp("8471300000", readTestdata(t, "goodshelp_8471300000.json"))
	if err != nil || !strings.Contains(other.Text, "Решение Межгоссовета ЕврАзЭС №190") {
		t.Fatalf("8471: %v\n%s", err, other.Text)
	}
}

func TestParseGoodsHelpLimit(t *testing.T) {
	if _, err := parseGoodsHelp("6402999800", readTestdata(t, "goodshelp_limit.json")); !errors.Is(err, ErrGoodsHelpLimit) {
		t.Fatalf("expected limit error, got %v", err)
	}
}

// Образец — Решение КТС №299 на old.adilet.zan.kz (документ обрезан): текст акта без меню и подвала.
func TestParseAdiletDocument(t *testing.T) {
	page, err := pageFromBody("https://adilet.zan.kz/rus/docs/H10T0000299", "https://old.adilet.zan.kz/rus/docs/H10T0000299", string(readTestdata(t, "adilet_old_H10T0000299.html")))
	if err != nil {
		t.Fatal(err)
	}
	if page.URL != "https://adilet.zan.kz/rus/docs/H10T0000299" || !strings.Contains(page.Title, "О применении санитарных мер") {
		t.Errorf("url/title: %s / %s", page.URL, page.Title)
	}
	for _, want := range []string{"Комиссия Евразийского экономического союза решила", "контактирующим с кожей человека"} {
		if !strings.Contains(page.Text, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, unwanted := range []string{"Состояние базы", "Бесплатный звонок", "Вернуться в мобильную версию", "function(", "{"} {
		if strings.Contains(page.Text, unwanted) {
			t.Errorf("unexpected %q", unwanted)
		}
	}
	if len(page.Links) == 0 {
		t.Fatal("no links")
	}
	for _, link := range page.Links {
		if !strings.HasPrefix(link.URL, "https://") || strings.Contains(link.URL, "#") {
			t.Errorf("bad link %+v", link)
		}
	}
}

// Образец — карточка акта на docs.eaeunion.org: текст только в PDF и Word, ссылки на них абсолютные.
func TestParseEAEUDocumentCard(t *testing.T) {
	page, err := pageFromBody("https://docs.eaeunion.org/documents/446/10220/", "https://docs.eaeunion.org/documents/446/10220/", string(readTestdata(t, "eaeunion_document_pdf.html")))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(page.Text, "Решение Коллегии ЕЭК № 89") || strings.Contains(page.Text, "Департамент финансов") {
		t.Errorf("text:\n%s", page.Text)
	}
	pdf := "https://docs.eaeunion.org/upload/iblock/b47/uvbyquwdzpw1rx6y67w4p2g5qt56zsyg/Reshenie-Kollegii-_-89-ot-7-oktyabrya-2025-g.pdf"
	if !hasLink(page.Links, pdf) || !IsWebFileURL(pdf) {
		t.Errorf("pdf link missing: %+v", page.Links)
	}
}

func TestSPAShellNeedsBrowser(t *testing.T) {
	if _, err := pageFromBody("https://adilet.zan.kz/rus/docs/X", "https://adilet.zan.kz/rus/docs/X", string(readTestdata(t, "adilet_spa_shell.html"))); !errors.Is(err, ErrWebPageScript) {
		t.Fatalf("expected script error, got %v", err)
	}
}

func TestCanonicalWebURLAndFetchAddress(t *testing.T) {
	for raw, want := range map[string]string{
		"http://adilet.zan.kz/rus/docs/H10T0000299":      "https://adilet.zan.kz/rus/docs/H10T0000299",
		"https://Docs.EAEUnion.org:443/documents/1/#top": "https://docs.eaeunion.org/documents/1/",
		"javascript:alert(1)":                            "",
		"ftp://example.com/file":                         "",
		"https://example.com:8443/":                      "",
		"https://user:pass@example.com/":                 "",
		"/rus/docs/X":                                    "",
	} {
		got, ok := CanonicalWebURL(raw)
		if got != want || ok != (want != "") {
			t.Errorf("%s: got %q %v, want %q", raw, got, ok, want)
		}
	}
	if got := fetchAddress("https://adilet.zan.kz/rus/docs/H10T0000299"); got != "https://old.adilet.zan.kz/rus/docs/H10T0000299" {
		t.Errorf("adilet: %s", got)
	}
	if got := fetchAddress("https://docs.eaeunion.org/documents/1/"); got != "https://docs.eaeunion.org/documents/1/" {
		t.Errorf("eaeunion: %s", got)
	}
}

func TestBlockedWebAddresses(t *testing.T) {
	for _, address := range []string{"127.0.0.1", "10.1.2.3", "172.16.0.1", "172.31.255.255", "192.168.1.1", "169.254.169.254", "100.64.0.1", "0.0.0.0", "::1", "fe80::1", "fd00::1", "::ffff:127.0.0.1", "::ffff:10.0.0.1"} {
		if !blockedWebAddr(netip.MustParseAddr(address)) {
			t.Errorf("%s should be blocked", address)
		}
	}
	for _, address := range []string{"8.8.8.8", "185.22.66.1", "172.32.0.1", "2a00:1450:4001::1"} {
		if blockedWebAddr(netip.MustParseAddr(address)) {
			t.Errorf("%s should be allowed", address)
		}
	}
}

// Настоящий клиент не соединяется с адресом внутренней сети, даже если сервер отвечает.
func TestWebFetcherRefusesInternalNetwork(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("<html>secret</html>")) }))
	defer server.Close()
	fetcher := &WebPageFetcher{client: newWebClient()}
	if _, _, err := fetcher.download(context.Background(), server.URL); !errors.Is(err, errBlockedAddr) {
		t.Fatalf("expected blocked address, got %v", err)
	}
	if _, err := fetcher.Page(context.Background(), "https://localhost/page"); err == nil {
		t.Fatal("localhost must not open")
	}
}

func testFetcher(server *httptest.Server) *WebPageFetcher {
	client := server.Client()
	client.CheckRedirect = checkWebRedirect
	return &WebPageFetcher{client: client}
}

func TestWebFetcherFilesAndRedirects(t *testing.T) {
	var server *httptest.Server
	server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/act":
			w.Header().Set("Content-Type", "application/pdf")
			_, _ = w.Write([]byte("%PDF-1.7"))
		case "/word":
			w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.wordprocessingml.document")
		case "/loop":
			http.Redirect(w, r, server.URL+"/loop", http.StatusFound)
		case "/plain":
			http.Redirect(w, r, "http://example.com/", http.StatusFound)
		default:
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte("<html><body><p>Текст</p></body></html>"))
		}
	}))
	defer server.Close()
	fetcher := testFetcher(server)
	ctx := context.Background()
	for _, path := range []string{"/act", "/word"} {
		if _, _, err := fetcher.download(ctx, server.URL+path); !errors.Is(err, ErrWebPageFile) {
			t.Errorf("%s: expected file, got %v", path, err)
		}
	}
	for _, path := range []string{"/loop", "/plain"} {
		if _, _, err := fetcher.download(ctx, server.URL+path); err == nil {
			t.Errorf("%s: redirect must fail", path)
		}
	}
	if body, _, err := fetcher.download(ctx, server.URL+"/page"); err != nil || !strings.Contains(body, "Текст") {
		t.Errorf("page: %v %q", err, body)
	}
	if _, _, err := fetcher.download(ctx, strings.Replace(server.URL, "https://", "http://", 1)+"/page"); err == nil {
		t.Error("http must not open")
	}
	// PDF по расширению — без скачивания.
	if _, err := fetcher.Page(ctx, "https://docs.eaeunion.org/upload/a.pdf"); !errors.Is(err, ErrWebPageFile) {
		t.Errorf("pdf by extension: %v", err)
	}
}

// Перечни СС/ДС из справки — в общую базу, решения Коллегии — сводной редакцией adilet; санэпиднадзор и
// экспортный контроль — не перечни СС/ДС.
func TestConformityActsFromGoodsHelp(t *testing.T) {
	acts := conformityActs(readTestdata(t, "goodshelp_8471300000.json"))
	urls := map[string]bool{}
	for _, act := range acts {
		urls[act.URL] = true
	}
	for _, want := range []string{"https://adilet.zan.kz/rus/docs/H13EK000091", "https://adilet.zan.kz/rus/docs/H14EK000002", "https://adilet.zan.kz/rus/docs/H18EK000167"} {
		if !urls[want] {
			t.Errorf("no %s in %+v", want, acts)
		}
	}
	if urls[SGRActURL] || len(acts) != 3 {
		t.Errorf("acts %+v", acts)
	}
	if got := AdiletURL("Решение Коллегии ЕЭК №28  от 05.03.2013"); got != "https://adilet.zan.kz/rus/docs/H13EK000028" {
		t.Errorf("adilet %s", got)
	}
	if got := AdiletURL("Решение Комиссии Таможенного Союза №299 от 28.05.2010"); got != "" {
		t.Errorf("not a college decision: %s", got)
	}
}

// Решения Коллегии ЕЭК — сводной редакцией adilet: по названию из справки (в том числе «№30 (2015)») и по
// карточке docs.eaeunion.org (сам текст там в PDF).
func TestAdiletLinks(t *testing.T) {
	if got := AdiletURL("Решение Коллегии ЕЭК №30 (2015) от 21.04.2015"); got != "https://adilet.zan.kz/rus/docs/H15EK000030" {
		t.Errorf("№30: %s", got)
	}
	card := WebPage{Text: "Информация о документе\nНомер документа\n137\nКороткий заголовок документа\nРешение Коллегии ЕЭК № 137\nВид документа\nАкты\nДата принятия документа\n11.10.2021\nДата опубликования\n12.10.2021"}
	if got := EECCardAdiletURL(card); got != "https://adilet.zan.kz/rus/docs/H21EK000137" {
		t.Errorf("card: %s", got)
	}
	if got := AdiletURL("Решение Совета ЕЭК №11  от 22.01.2025"); got != "https://adilet.zan.kz/rus/docs/H25EV000011" {
		t.Errorf("council: %s", got)
	}
	if got := EECCardAdiletURL(WebPage{Text: "Решение Совета ЕЭК № 46"}); got != "" {
		t.Errorf("no card fields: %s", got)
	}
	card6 := "https://docs.eaeunion.org/docs/ru-ru/01515803/clcd_19012018_6"
	help := WithAdiletLinks(WebPage{Text: "1. Решение Коллегии ЕЭК №6 от 16.01.2018 — " + card6, Links: []WebLink{{URL: card6, Text: "Решение Коллегии ЕЭК №6 от 16.01.2018"}}})
	if !strings.Contains(help.Text, "https://adilet.zan.kz/rus/docs/H18EK000006") || strings.Contains(help.Text, card6) || len(help.Links) != 2 {
		t.Errorf("help %+v", help)
	}
}
