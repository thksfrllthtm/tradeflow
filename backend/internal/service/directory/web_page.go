package directory

// Веб-страницы для агента («Документы для ввоза»): акты по ссылкам из справки по товару
// tnved.info — текст без меню, шапки, подвала и скриптов плюс ссылки с подписями. Загрузка
// безопасная: только https, без адресов внутренней сети (проверка при соединении), с лимитами
// размера, времени и редиректов. PDF и Word не разбираются. Страницы хранятся в базе.

import (
	"context"
	"encoding/json"
	"errors"
	"html"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"regexp"
	"strings"
	"syscall"
	"time"
	"unicode/utf8"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
)

const (
	webPageMaxAge     = 30 * 24 * time.Hour
	webFetchTimeout   = 75 * time.Second
	webDialTimeout    = 10 * time.Second
	webMaxBytes       = 12 << 20
	webMaxRedirects   = 5
	webMaxLinks       = 300
	webLinkTextRunes  = 160
	webScriptMinRunes = 200
	webUserAgent      = "Mozilla/5.0 (compatible; TradeFlow/1.0; +internal customs estimate tool)"
)

var (
	ErrWebPageUnavailable = errors.New("web page unavailable")
	// ErrWebPageFile — по ссылке файл (PDF, Word, архив): программа его не читает.
	ErrWebPageFile = errors.New("web page is a file")
	// ErrWebPageScript — текст страницы подгружается скриптом, без браузера его не прочитать.
	ErrWebPageScript = errors.New("web page needs a browser")
	ErrWebPageURL    = errors.New("web page url not allowed")
	errBlockedAddr   = errors.New("address of internal network")

	webFileExtensions = map[string]bool{".pdf": true, ".doc": true, ".docx": true, ".rtf": true, ".zip": true, ".rar": true, ".7z": true, ".xls": true, ".xlsx": true, ".odt": true}
	// Адреса, которых нет в IsPrivate/IsLoopback: CGNAT, «этот» сеть, служебные и тестовые диапазоны.
	webBlockedPrefixes = []netip.Prefix{
		netip.MustParsePrefix("0.0.0.0/8"), netip.MustParsePrefix("100.64.0.0/10"), netip.MustParsePrefix("192.0.0.0/24"),
		netip.MustParsePrefix("198.18.0.0/15"), netip.MustParsePrefix("240.0.0.0/4"), netip.MustParsePrefix("64:ff9b::/96"),
	}

	webTitle      = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	webLinkTag    = regexp.MustCompile(`(?is)<a\s[^>]*?href\s*=\s*(?:"([^"]*)"|'([^']*)')[^>]*>(.*?)</a>`)
	webDropBlocks = []*regexp.Regexp{
		regexp.MustCompile(`(?is)<header\b.*?</header>`), regexp.MustCompile(`(?is)<nav\b.*?</nav>`),
		regexp.MustCompile(`(?is)<footer\b.*?</footer>`), regexp.MustCompile(`(?is)<aside\b.*?</aside>`),
		regexp.MustCompile(`(?is)<form\b.*?</form>`), regexp.MustCompile(`(?is)<button\b.*?</button>`),
	}
	webMain = regexp.MustCompile(`(?is)<main\b[^>]*>(.*?)</main>`)
)

// WebLink — ссылка со страницы: абсолютный адрес и подпись.
type WebLink struct {
	URL  string `json:"url"`
	Text string `json:"text"`
}

// WebPage — страница для агента. URL — адрес, по которому её открыл агент (для сотрудника).
type WebPage struct {
	URL       string
	Title     string
	Text      string
	Links     []WebLink
	FromCache bool
}

type WebPageFetcher struct {
	repository *repository.Repository
	client     *http.Client
}

func NewWebPageFetcher(r *repository.Repository) *WebPageFetcher {
	return &WebPageFetcher{repository: r, client: newWebClient()}
}

// newWebClient — клиент без прокси: адрес проверяется при соединении (после DNS), а не по имени
// хоста, иначе имя, указывающее на 127.0.0.1 или 10.x, прошло бы проверку.
func newWebClient() *http.Client {
	dialer := &net.Dialer{Timeout: webDialTimeout, Control: webDialControl}
	transport := &http.Transport{
		Proxy: nil, DialContext: dialer.DialContext, ForceAttemptHTTP2: true,
		TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 45 * time.Second, MaxIdleConns: 10, IdleConnTimeout: time.Minute,
	}
	return &http.Client{Timeout: webFetchTimeout, Transport: transport, CheckRedirect: checkWebRedirect}
}

func webDialControl(_, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip, err := netip.ParseAddr(host)
	if err != nil || blockedWebAddr(ip) {
		return errBlockedAddr
	}
	return nil
}

// blockedWebAddr — адрес внутренней сети: localhost, 10.x, 172.16–31.x, 192.168.x, 169.254.x,
// 100.64.x, IPv6 loopback, link-local и уникальные локальные.
func blockedWebAddr(ip netip.Addr) bool {
	ip = ip.Unmap()
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsInterfaceLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	for _, prefix := range webBlockedPrefixes {
		if prefix.Contains(ip) {
			return true
		}
	}
	return false
}

func checkWebRedirect(request *http.Request, via []*http.Request) error {
	if len(via) >= webMaxRedirects {
		return errors.New("too many redirects")
	}
	if request.URL.Scheme != "https" {
		return errors.New("redirect to non-https")
	}
	return nil
}

// CanonicalWebURL — адрес для сравнения и загрузки: https (http поднимается до https), хост в нижнем
// регистре, без #якоря и порта 443. ok=false — не веб-адрес или другой протокол.
func CanonicalWebURL(raw string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Host == "" || parsed.User != nil {
		return "", false
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return "", false
	}
	parsed.Scheme = "https"
	parsed.Host = strings.TrimSuffix(strings.ToLower(parsed.Host), ":443")
	if strings.Contains(parsed.Host, ":") && !strings.HasPrefix(parsed.Host, "[") {
		// Другой порт — не обычный сайт.
		return "", false
	}
	parsed.Fragment, parsed.RawFragment = "", ""
	return parsed.String(), true
}

// TrustedWebHost — сайты, которые агент может открыть без ссылки с уже открытой страницы.
func TrustedWebHost(pageURL string) bool {
	parsed, err := url.Parse(pageURL)
	if err != nil {
		return false
	}
	switch parsed.Hostname() {
	case "tnved.info", "www.tnved.info", "tws.by", "www.tws.by":
		return true
	}
	return false
}

// IsWebFileURL — ссылка по расширению ведёт на файл (PDF, Word, архив).
func IsWebFileURL(pageURL string) bool {
	parsed, err := url.Parse(pageURL)
	return err == nil && webFileExtensions[strings.ToLower(path.Ext(parsed.Path))]
}

// fetchAddress — откуда скачивать: новая версия adilet.zan.kz — SPA (текст только через API с
// токеном), у старой версии полный текст акта прямо в HTML.
func fetchAddress(pageURL string) string {
	parsed, err := url.Parse(pageURL)
	if err != nil {
		return pageURL
	}
	if host := parsed.Hostname(); (host == "adilet.zan.kz" || host == "www.adilet.zan.kz") && strings.Contains(parsed.Path, "/docs/") {
		parsed.Host = "old.adilet.zan.kz"
	}
	return parsed.String()
}

// Page — страница по адресу: из базы, если свежая, иначе с сайта; сайт недоступен — старая копия.
func (f *WebPageFetcher) Page(ctx context.Context, rawURL string) (WebPage, error) {
	pageURL, ok := CanonicalWebURL(rawURL)
	if !ok {
		return WebPage{}, ErrWebPageURL
	}
	if IsWebFileURL(pageURL) {
		return WebPage{URL: pageURL}, ErrWebPageFile
	}
	cached, found, err := f.cached(ctx, pageURL)
	if err != nil {
		return WebPage{}, err
	}
	if found && time.Since(cached.fetchedAt) < webPageMaxAge {
		return cached.page, nil
	}
	body, finalURL, err := f.download(ctx, fetchAddress(pageURL))
	if err == nil {
		var page WebPage
		if page, err = pageFromBody(pageURL, finalURL, body); err == nil {
			f.save(ctx, page)
			return page, nil
		}
	}
	if found {
		return cached.page, nil
	}
	return WebPage{URL: pageURL}, err
}

type cachedWebPage struct {
	page      WebPage
	fetchedAt time.Time
}

func (f *WebPageFetcher) cached(ctx context.Context, key string) (cachedWebPage, bool, error) {
	if f.repository == nil {
		return cachedWebPage{}, false, nil
	}
	row, err := f.repository.WebPage(ctx, key)
	if errors.Is(err, repository.ErrNotFound) {
		return cachedWebPage{}, false, nil
	}
	if err != nil {
		return cachedWebPage{}, false, err
	}
	page := WebPage{URL: row.URL, Title: row.Title, Text: row.Text, FromCache: true}
	_ = json.Unmarshal(row.Links, &page.Links)
	return cachedWebPage{page: page, fetchedAt: row.FetchedAt}, row.Text != "", nil
}

func (f *WebPageFetcher) saveAs(ctx context.Context, key string, page WebPage) {
	if f.repository == nil {
		return
	}
	links, _ := json.Marshal(page.Links)
	if page.Links == nil {
		links = []byte("[]")
	}
	_ = f.repository.SaveWebPage(ctx, repository.WebPageCache{URL: key, Title: page.Title, Text: page.Text, Links: repository.JSON(links), FetchedAt: time.Now().UTC()})
}

func (f *WebPageFetcher) save(ctx context.Context, page WebPage) { f.saveAs(ctx, page.URL, page) }

// download скачивает HTML; файл (PDF, Word, архив) — ErrWebPageFile без чтения тела.
func (f *WebPageFetcher) download(ctx context.Context, address string) (body, finalURL string, err error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil || request.URL.Scheme != "https" {
		return "", "", ErrWebPageURL
	}
	request.Header.Set("User-Agent", webUserAgent)
	request.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9,*/*;q=0.5")
	request.Header.Set("Accept-Language", "ru")
	response, err := f.client.Do(request)
	if err != nil {
		if errors.Is(err, errBlockedAddr) {
			return "", "", errBlockedAddr
		}
		return "", "", ErrWebPageUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", "", ErrWebPageUnavailable
	}
	if webFileResponse(response.Header.Get("Content-Type"), response.Request.URL.Path) {
		return "", "", ErrWebPageFile
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, webMaxBytes))
	if err != nil {
		return "", "", ErrWebPageUnavailable
	}
	return string(data), response.Request.URL.String(), nil
}

// webFileResponse — ответ не страница: PDF, Word, архив или другой не текстовый тип.
func webFileResponse(contentType, urlPath string) bool {
	if webFileExtensions[strings.ToLower(path.Ext(urlPath))] {
		return true
	}
	if contentType == "" {
		return false
	}
	media, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return true
	}
	return media != "text/html" && media != "application/xhtml+xml" && media != "text/plain"
}

// pageFromBody — страница по скачанному HTML; ссылки разрешаются от адреса после редиректов. Почти
// пустой текст при скриптах — SPA (новый adilet.zan.kz, gov.kz): ErrWebPageScript.
func pageFromBody(pageURL, finalURL, body string) (WebPage, error) {
	page := parseWebPage(finalURL, body)
	page.URL = pageURL
	if utf8.RuneCountInString(page.Text) < webScriptMinRunes && strings.Contains(strings.ToLower(body), "<script") {
		return WebPage{URL: pageURL}, ErrWebPageScript
	}
	return page, nil
}

// parseWebPage — заголовок, текст основного блока и ссылки из него (абсолютные, без повторов и якорей).
func parseWebPage(pageURL, body string) WebPage {
	page := WebPage{URL: pageURL}
	if match := webTitle.FindStringSubmatch(body); match != nil {
		page.Title = strings.Join(strings.Fields(html.UnescapeString(htmlTags.ReplaceAllString(match[1], " "))), " ")
	}
	content := webContent(body)
	for _, block := range htmlDropBlocks {
		content = block.ReplaceAllString(content, " ")
	}
	for _, block := range webDropBlocks {
		content = block.ReplaceAllString(content, " ")
	}
	page.Links = webLinks(pageURL, content)
	page.Text = htmlToText(content)
	return page
}

// webContent — основной блок страницы: текст акта на old.adilet.zan.kz, <main>, иначе <body>.
func webContent(body string) string {
	if start := strings.Index(body, `class="container_gamma text`); start >= 0 {
		part := body[start:]
		if end := strings.Index(part, `<div id="footer"`); end > 0 {
			part = part[:end]
		}
		// Начало тега до class="…" отрезано — убираем его хвост.
		if close := strings.Index(part, ">"); close >= 0 {
			part = part[close+1:]
		}
		return part
	}
	if match := webMain.FindStringSubmatch(body); match != nil {
		return match[1]
	}
	if start := strings.Index(strings.ToLower(body), "<body"); start >= 0 {
		return body[start:]
	}
	return body
}

func webLinks(pageURL, content string) []WebLink {
	base, err := url.Parse(pageURL)
	if err != nil {
		return nil
	}
	links := []WebLink{}
	seen := map[string]bool{}
	for _, match := range webLinkTag.FindAllStringSubmatch(content, -1) {
		href := strings.TrimSpace(html.UnescapeString(match[1] + match[2]))
		if href == "" || strings.HasPrefix(href, "#") {
			continue
		}
		reference, err := url.Parse(href)
		if err != nil {
			continue
		}
		absolute, ok := CanonicalWebURL(base.ResolveReference(reference).String())
		if !ok || seen[absolute] {
			continue
		}
		seen[absolute] = true
		text := strings.Join(strings.Fields(html.UnescapeString(htmlTags.ReplaceAllString(match[3], " "))), " ")
		links = append(links, WebLink{URL: absolute, Text: service.TruncateRunes(text, webLinkTextRunes)})
		if len(links) == webMaxLinks {
			break
		}
	}
	return links
}
