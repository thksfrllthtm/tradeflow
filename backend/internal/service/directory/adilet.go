package directory

// Акты adilet.zan.kz. Сайт — SPA, а old.adilet.zan.kz, через который текст брался раньше, отдаёт заглушку
// «Технические работы» (и она сохранялась в web_pages вместо акта). Теперь текст сводной редакции берётся
// из API самого сайта (без входа): by-ngr/<код> отдаёт название и текст одним ответом. API неофициальное:
// не ответил или формат изменился — тот же акт с zakon.uchet.kz (копия adilet, адреса те же). Страница,
// которая не похожа на текст акта, не сохраняется: в кэше остаётся прежняя копия.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

const (
	adiletAPI  = "https://adilet.zan.kz/api/documents/by-ngr/"
	zakonUchet = "https://zakon.uchet.kz/rus/docs/"
	// actMinRunes — короче не бывает даже решение в два абзаца; заглушки и пустые оболочки короче.
	actMinRunes = 300
)

var (
	adiletDocPath = regexp.MustCompile(`^/(?:rus|kaz|eng)/docs/([A-Za-z0-9_]+)/?$`)
	// errNotAct — страница по адресу акта не текст акта (заглушка, пустая оболочка, нет названия).
	errNotAct = errors.New("page is not an act text")
	// maintenanceMarkers — заглушки сайтов на время работ.
	maintenanceMarkers = []string{"идут технические работы", "сайт скоро возобновит"}
)

// adiletCode — код акта из адреса https://adilet.zan.kz/rus/docs/<код>; "" — не акт adilet.
func adiletCode(pageURL string) string {
	parsed, err := url.Parse(pageURL)
	if err != nil || (parsed.Hostname() != "adilet.zan.kz" && parsed.Hostname() != "www.adilet.zan.kz") {
		return ""
	}
	if match := adiletDocPath.FindStringSubmatch(parsed.Path); match != nil {
		return match[1]
	}
	return ""
}

// adiletDocument — нужная часть ответа by-ngr.
type adiletDocument struct {
	Document struct {
		ID           string `json:"id"`
		GroupNumber  string `json:"group_number"`
		TitleRU      string `json:"title_ru"`
		OfficialName string `json:"official_name"`
		TextContent  string `json:"text_content"`
	} `json:"document"`
}

// adiletPage — акт adilet: API, при сбое — zakon.uchet.kz. Обе ошибки — ошибка первого источника.
func (f *WebPageFetcher) adiletPage(ctx context.Context, pageURL, code string) (WebPage, error) {
	page, err := f.adiletAPIPage(ctx, pageURL, code)
	if err == nil {
		return page, nil
	}
	log.Printf("adilet API %s: %v — читаю zakon.uchet.kz", code, err)
	page, zakonErr := f.zakonPage(ctx, pageURL, code)
	if zakonErr == nil {
		return page, nil
	}
	log.Printf("zakon.uchet.kz %s: %v", code, zakonErr)
	return WebPage{URL: pageURL}, err
}

func (f *WebPageFetcher) adiletAPIPage(ctx context.Context, pageURL, code string) (WebPage, error) {
	base := f.adiletAPI
	if base == "" {
		base = adiletAPI
	}
	response, err := f.open(ctx, base+url.PathEscape(code), "application/json")
	if err != nil {
		return WebPage{}, err
	}
	defer response.Body.Close()
	var data adiletDocument
	if err := json.NewDecoder(io.LimitReader(response.Body, webMaxBytes)).Decode(&data); err != nil {
		return WebPage{}, fmt.Errorf("формат ответа: %w", err)
	}
	document := data.Document
	if !strings.EqualFold(document.GroupNumber, code) || document.TextContent == "" {
		return WebPage{}, fmt.Errorf("формат ответа: код %q, текст %d байт", document.GroupNumber, len(document.TextContent))
	}
	page := parseWebPage(pageURL, document.TextContent)
	page.Title = strings.Join(strings.Fields(document.TitleRU), " ")
	// Шапка акта («Решение Коллегии … от … № …») в API — отдельным полем, а в тексте её нет.
	if name := strings.Join(strings.Fields(document.OfficialName), " "); name != "" && !strings.Contains(page.Text, name) {
		page.Text = name + "\n" + page.Text
	}
	return page, checkActPage(page)
}

func (f *WebPageFetcher) zakonPage(ctx context.Context, pageURL, code string) (WebPage, error) {
	base := f.zakonBase
	if base == "" {
		base = zakonUchet
	}
	body, _, err := f.download(ctx, base+url.PathEscape(code))
	if err != nil {
		return WebPage{}, err
	}
	// Ссылки на другие акты — относительные, те же пути, что у adilet: от адреса adilet.
	page := parseWebPage(pageURL, body)
	return page, checkActPage(page)
}

// checkActPage — страница похожа на текст акта: есть название, текст не короче actMinRunes и не заглушка.
func checkActPage(page WebPage) error {
	if strings.TrimSpace(page.Title) == "" || utf8.RuneCountInString(page.Text) < actMinRunes || maintenancePage(page) {
		return errNotAct
	}
	return nil
}

// maintenancePage — заглушка «идут технические работы» (смотрится только начало: в длинном акте фраза
// может встретиться по делу).
func maintenancePage(page WebPage) bool {
	head := strings.ToLower(page.Title + " " + firstRunes(page.Text, 2000))
	for _, marker := range maintenanceMarkers {
		if strings.Contains(head, marker) {
			return true
		}
	}
	return false
}

func firstRunes(text string, limit int) string {
	for index := range text {
		if limit == 0 {
			return text[:index]
		}
		limit--
	}
	return text
}

// open — GET по https с проверкой адреса и статуса 200; тело закрывает вызывающий.
func (f *WebPageFetcher) open(ctx context.Context, address, accept string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil || request.URL.Scheme != "https" {
		return nil, ErrWebPageURL
	}
	request.Header.Set("User-Agent", webUserAgent)
	request.Header.Set("Accept", accept)
	request.Header.Set("Accept-Language", "ru")
	response, err := f.client.Do(request)
	if err != nil {
		if errors.Is(err, errBlockedAddr) {
			return nil, errBlockedAddr
		}
		return nil, ErrWebPageUnavailable
	}
	if response.StatusCode != http.StatusOK {
		response.Body.Close()
		return nil, ErrWebPageUnavailable
	}
	return response, nil
}
