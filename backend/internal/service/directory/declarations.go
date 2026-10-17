package directory

// Примеры декларирования для агента ТН ВЭД: под какими кодами уже заявляли такой товар — обезличенные
// описания из деклараций (графа 31) на двух сайтах:
//   - ifcg.ru — по словам: товарные позиции с числом записей и описания; внутри позиции — коды, внутри
//     кода — до 40 описаний;
//   - tks.ru «Примеры декларирования» — по словам (все слова сразу): коды с числом записей и описания.
//
// alta.ru «Товары и коды» не подключён: бесплатно — 3 запроса в день с одного IP, дальше капча.
// tks.ru ищет только по словам, поэтому внутри позиции или кода — только ifcg.ru. Данные
// неофициальные: довод и подсказка, а не основание. Загрузка — тем же безопасным клиентом, что
// страницы для агента (без адресов внутренней сети), результат каждого сайта хранится в базе (кеш
// страниц) как JSON. Один сайт не ответил — остальные всё равно отдаются.

import (
	"context"
	"encoding/json"
	"errors"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"golang.org/x/text/encoding/charmap"
)

const (
	declarationsSearchURL = "https://www.ifcg.ru/kb/tnved/search/?s=stat&q="
	tksGoodsURL           = "https://www.tks.ru/db/tnved/goods/?searchstr="
	declarationRunes      = 400
)

// ErrDeclarationsUnavailable — ни один сайт с примерами декларирования не ответил.
var ErrDeclarationsUnavailable = errors.New("declaration examples unavailable")

var (
	declarationGroup   = regexp.MustCompile(`<h3 id="result--stat-g-(\d+)"`)
	declarationName    = regexp.MustCompile(`(?s)<div class="fsi">(.*?)</div>`)
	declarationCount   = regexp.MustCompile(`Показать все записи \((\d+)\)`)
	declarationExample = regexp.MustCompile(`(?s)<a href="/kb/tnved/(\d{10})/" class="h4"[^>]*>\d+</a>.*?<span class="font-xs">(.*?)</span>`)
	// Пустые хвосты описаний ifcg.ru: «; (ФИРМА) ; (TM) ».
	declarationEmptyTail = regexp.MustCompile(`[;\s]*\(ФИРМА\)\s*;\s*\(TM\)\s*$`)

	tksRow   = regexp.MustCompile(`(?s)<li>\s*<span style="font-size:14px;color:#f26b5b;">(\d{10})</span> - (.*?)<span style="white-space:nowrap">`)
	tksTotal = regexp.MustCompile(`^\(всего: (\d+)\) - (.*)$`)
)

// DeclarationExample — описание товара из декларации и заявленный код.
type DeclarationExample struct {
	Code string `json:"code"`
	Text string `json:"text"`
}

// DeclarationGroup — товарная позиция (4 цифры) или код (10 цифр): сколько записей деклараций нашлось
// и примеры описаний.
type DeclarationGroup struct {
	Code     string               `json:"code"`
	Name     string               `json:"name"`
	Count    int                  `json:"count"`
	Examples []DeclarationExample `json:"examples"`
}

// DeclarationSource — ответ одного сайта; Error — сайт не ответил.
type DeclarationSource struct {
	Site   string             `json:"site"`
	URL    string             `json:"url"`
	Groups []DeclarationGroup `json:"groups"`
	Error  string             `json:"error,omitempty"`
}

// DeclarationsURL — адрес поиска ifcg.ru по словам или коду; within — внутри позиции (4 цифры) или кода (10 цифр).
func DeclarationsURL(query, within string) string {
	address := declarationsSearchURL + url.QueryEscape(query)
	if within != "" {
		address += "&g=" + within
	}
	return address
}

// tksURL — поиск tks.ru: запрос в windows-1251 (в UTF-8 сайт ничего не находит); grouped — по кодам с
// числом записей. ok=false — в запросе символы не из windows-1251 (например, китайские).
func tksURL(query string, grouped bool) (string, bool) {
	encoded, err := charmap.Windows1251.NewEncoder().String(query)
	if err != nil {
		return "", false
	}
	address := tksGoodsURL + url.QueryEscape(encoded)
	if grouped {
		address += "&g=on"
	}
	return address, true
}

// Declarations — примеры декларирования со всех сайтов, которые умеют такой поиск. fromCache — все ответы
// из базы. Ошибка — только если не ответил ни один сайт.
func (f *WebPageFetcher) Declarations(ctx context.Context, query, within string) (sources []DeclarationSource, fromCache bool, err error) {
	fromCache = true
	add := func(site, address string, load func() ([]DeclarationGroup, error)) {
		groups, cached, err := f.cachedGroups(ctx, address, load)
		source := DeclarationSource{Site: site, URL: address, Groups: groups}
		if err != nil {
			source.Groups, source.Error = []DeclarationGroup{}, "сайт не ответил"
		}
		fromCache = fromCache && cached
		sources = append(sources, source)
	}
	ifcg := DeclarationsURL(query, within)
	add("ifcg.ru", ifcg, func() ([]DeclarationGroup, error) {
		body, _, err := f.download(ctx, ifcg)
		return parseDeclarations(body), err
	})
	if within == "" {
		if grouped, ok := tksURL(query, true); ok {
			plain, _ := tksURL(query, false)
			add("tks.ru", grouped, func() ([]DeclarationGroup, error) {
				codes, _, err := f.download(ctx, grouped)
				if err != nil {
					return nil, err
				}
				examples, _, err := f.download(ctx, plain)
				if err != nil {
					return nil, err
				}
				return parseTKS(decodeWindows1251(codes), decodeWindows1251(examples)), nil
			})
		}
	}
	for _, source := range sources {
		if source.Error == "" {
			return sources, fromCache, nil
		}
	}
	return sources, false, ErrDeclarationsUnavailable
}

// cachedGroups — ответ сайта из базы, если свежий, иначе load; сайт недоступен — старая копия. Пустой
// результат тоже хранится («[]» — непустой текст): повторный поиск не ходит на сайт.
func (f *WebPageFetcher) cachedGroups(ctx context.Context, address string, load func() ([]DeclarationGroup, error)) ([]DeclarationGroup, bool, error) {
	var groups []DeclarationGroup
	cached, found, err := f.cached(ctx, address)
	if err != nil {
		return nil, false, err
	}
	if found && time.Since(cached.fetchedAt) < webPageMaxAge && json.Unmarshal([]byte(cached.page.Text), &groups) == nil {
		return groups, true, nil
	}
	groups, err = load()
	if err != nil {
		if found && json.Unmarshal([]byte(cached.page.Text), &groups) == nil {
			return groups, true, nil
		}
		return nil, false, ErrDeclarationsUnavailable
	}
	data, _ := json.Marshal(groups)
	f.saveAs(ctx, address, WebPage{URL: address, Title: "Примеры декларирования", Text: string(data)})
	return groups, false, nil
}

func decodeWindows1251(body string) string {
	text, err := charmap.Windows1251.NewDecoder().String(body)
	if err != nil {
		return body
	}
	return text
}

// parseDeclarations — группы раздела «Статистика декларирования» страницы поиска ifcg.ru (в порядке
// сайта — по убыванию числа записей). Нет раздела — ничего не нашлось.
func parseDeclarations(body string) []DeclarationGroup {
	groups := []DeclarationGroup{}
	start := strings.Index(body, `id="result--stat"`)
	if start < 0 {
		return groups
	}
	body = body[start:]
	bounds := declarationGroup.FindAllStringSubmatchIndex(body, -1)
	for index, bound := range bounds {
		end := len(body)
		if index+1 < len(bounds) {
			end = bounds[index+1][0]
		}
		block := body[bound[0]:end]
		group := DeclarationGroup{Code: body[bound[2]:bound[3]], Examples: []DeclarationExample{}}
		if match := declarationName.FindStringSubmatch(block); match != nil {
			group.Name = declarationText(match[1])
		}
		for _, match := range declarationExample.FindAllStringSubmatch(block, -1) {
			if text := declarationEmptyTail.ReplaceAllString(declarationText(match[2]), ""); text != "" {
				group.Examples = append(group.Examples, DeclarationExample{Code: match[1], Text: service.TruncateRunes(text, declarationRunes)})
			}
		}
		group.Count = len(group.Examples)
		if match := declarationCount.FindStringSubmatch(block); match != nil {
			group.Count, _ = strconv.Atoi(match[1])
		}
		groups = append(groups, group)
	}
	return groups
}

// parseTKS — коды с числом записей (сгруппированный поиск tks.ru, по убыванию) и описания первой страницы
// обычного поиска — примерами к своим кодам. Описание с кодом не из первых групп — отдельной группой.
func parseTKS(grouped, plain string) []DeclarationGroup {
	groups := []DeclarationGroup{}
	index := map[string]int{}
	for _, match := range tksRow.FindAllStringSubmatch(grouped, -1) {
		total := tksTotal.FindStringSubmatch(declarationText(match[2]))
		if total == nil {
			continue
		}
		count, _ := strconv.Atoi(total[1])
		index[match[1]] = len(groups)
		groups = append(groups, DeclarationGroup{Code: match[1], Name: total[2], Count: count, Examples: []DeclarationExample{}})
	}
	for _, match := range tksRow.FindAllStringSubmatch(plain, -1) {
		position, ok := index[match[1]]
		if !ok {
			position = len(groups)
			index[match[1]] = position
			groups = append(groups, DeclarationGroup{Code: match[1], Count: 1, Examples: []DeclarationExample{}})
		}
		groups[position].Examples = append(groups[position].Examples, DeclarationExample{Code: match[1], Text: service.TruncateRunes(declarationText(match[2]), declarationRunes)})
	}
	return groups
}

func declarationText(fragment string) string {
	return strings.Join(strings.Fields(html.UnescapeString(htmlTags.ReplaceAllString(fragment, ""))), " ")
}
