package directory

// Страница кода на TWS.BY («Подробнее» из номенклатуры), очищенная до полезного для подбора:
// дерево кода, дополнительная единица и признаки, пояснения и примечания к ТН ВЭД, меры
// нетарифного регулирования. Белорусские
// сборы и ставки отбрасываются. Страница скачивается один раз и хранится в базе.

import (
	"context"
	"errors"
	"html"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
)

const (
	detailsMaxAge       = 30 * 24 * time.Hour
	detailsFetchTimeout = 20 * time.Second
	detailsMaxBytes     = 4 << 20
	detailsMaxRunes     = 6000
	detailsPartRunes    = 1400
	// Пояснения и примечания к ТН ВЭД — лучшее подтверждение, им больше места.
	detailsNotesRunes   = 2600
	detailsMeasureRunes = 800
)

var (
	ErrDetailsUnavailable = errors.New("details page unavailable")

	// Go regexp без обратных ссылок: по шаблону на тег (иначе <header> закрылся бы чужим </head>).
	htmlDropBlocks = []*regexp.Regexp{
		regexp.MustCompile(`(?is)<script\b.*?</script>`), regexp.MustCompile(`(?is)<style\b.*?</style>`),
		regexp.MustCompile(`(?is)<noscript\b.*?</noscript>`), regexp.MustCompile(`(?is)<svg\b.*?</svg>`), regexp.MustCompile(`(?is)<head\b.*?</head>`),
	}
	htmlBlockTags = regexp.MustCompile(`(?i)</?(p|div|li|tr|td|th|h[1-6]|br|ul|ol|table|section)[^>]*>`)
	htmlTags      = regexp.MustCompile(`<[^>]+>`)
	spaces        = regexp.MustCompile(`[ \t\f\r\x{00a0}]+`)
	blankLines    = regexp.MustCompile(`\s*\n\s*`)
	// Шум дерева кодов: «всего 117 кодов, адвалорные ставки 0–15%, специфические ставки 2.02 EUR за шт».
	treeCounts   = regexp.MustCompile(`всего \d+ код[а-яё]*,?`)
	treeAdValor  = regexp.MustCompile(`адвалорные ставки [\d.,\s–\-]+%;?,?`)
	treeSpecific = regexp.MustCompile(`специфические ставки [^\n]*?(EUR|USD) за [^\s;,]+(;\s*[\d.,\s–\-]+(EUR|USD) за [^\s;,]+)*[;,]?`)
)

type DetailsFetcher struct {
	repository *repository.Repository
	client     *http.Client
}

func NewDetailsFetcher(r *repository.Repository) *DetailsFetcher {
	return &DetailsFetcher{repository: r, client: &http.Client{Timeout: detailsFetchTimeout}}
}

// Page — текст страницы кода: из базы, если он свежий, иначе с TWS. Если TWS недоступен,
// отдаётся старая копия. fromCache — текст взят из базы.
func (f *DetailsFetcher) Page(ctx context.Context, code, url string) (text string, fromCache bool, err error) {
	cached, cacheErr := f.repository.DetailsPage(ctx, code)
	if cacheErr == nil && time.Since(cached.FetchedAt) < detailsMaxAge && cached.Text != "" {
		return cached.Text, true, nil
	}
	if cacheErr != nil && !errors.Is(cacheErr, repository.ErrNotFound) {
		return "", false, cacheErr
	}
	fresh, fetchErr := f.fetch(ctx, url)
	if fetchErr != nil || fresh == "" {
		if cached.Text != "" {
			return cached.Text, true, nil
		}
		return "", false, ErrDetailsUnavailable
	}
	_ = f.repository.SaveDetailsPage(ctx, repository.TNVEDDetailsPage{Code: code, URL: url, Text: fresh, FetchedAt: time.Now().UTC()})
	return fresh, false, nil
}

func (f *DetailsFetcher) fetch(ctx context.Context, url string) (string, error) {
	if !strings.HasPrefix(url, "https://www.tws.by/") && !strings.HasPrefix(url, "https://tws.by/") {
		return "", ErrDetailsUnavailable
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	request.Header.Set("User-Agent", "Mozilla/5.0 (compatible; TradeFlow/1.0; +internal customs estimate tool)")
	request.Header.Set("Accept-Language", "ru")
	response, err := f.client.Do(request)
	if err != nil {
		return "", err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", ErrDetailsUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, detailsMaxBytes))
	if err != nil {
		return "", err
	}
	return cleanTWSPage(string(body)), nil
}

// htmlToText — текст страницы с переносами строк на месте блоков.
func htmlToText(page string) string {
	text := page
	for _, block := range htmlDropBlocks {
		text = block.ReplaceAllString(text, " ")
	}
	text = htmlBlockTags.ReplaceAllString(text, "\n")
	text = htmlTags.ReplaceAllString(text, " ")
	text = html.UnescapeString(text)
	text = spaces.ReplaceAllString(text, " ")
	return strings.TrimSpace(blankLines.ReplaceAllString(text, "\n"))
}

// section — текст от начала маркера start до ближайшего из маркеров end (или до конца).
func section(text, start string, ends ...string) string {
	index := strings.Index(text, start)
	if index < 0 {
		return ""
	}
	part := text[index:]
	cut := len(part)
	for _, end := range ends {
		if position := strings.Index(part[len(start):], end); position >= 0 && position+len(start) < cut {
			cut = position + len(start)
		}
	}
	return strings.TrimSpace(part[:cut])
}

func cleanTWSPage(page string) string {
	text := htmlToText(page)
	stops := []string{"Полное дерево кодов", "Дополнительная единица измерения", "Дополнительные признаки", "Актуальные ставки сборов", "Единый таможенный тариф", "Меры нетарифного регулирования"}
	tree := section(text, "актуальность информации", stops...)
	if tree == "" {
		return ""
	}
	// «актуальность информации : 28.09.2026» — дата не нужна модели.
	if _, rest, ok := strings.Cut(tree, "\n"); ok {
		tree = rest
	}
	tree = treeCounts.ReplaceAllString(tree, "")
	tree = treeAdValor.ReplaceAllString(tree, "")
	tree = treeSpecific.ReplaceAllString(tree, "")
	tree = strings.TrimSpace(blankLines.ReplaceAllString(spaces.ReplaceAllString(tree, " "), "\n"))

	parts := []string{"Дерево кода:\n" + tree}
	if unit := section(text, "Дополнительная единица измерения", "Дополнительные признаки", "Актуальные ставки сборов", "Единый таможенный тариф", "Меры нетарифного регулирования"); unit != "" {
		parts = append(parts, service.TruncateRunes(unit, 200))
	}
	// Признаки, утратившие силу, и белорусские сборы — не для Казахстана.
	if features := section(text, "Дополнительные признаки", "Дополнительные признаки, утратившие силу", "Актуальные ставки сборов", "Единый таможенный тариф", "Меры нетарифного регулирования"); features != "" {
		parts = append(parts, service.TruncateRunes(dropBelarus(features), detailsPartRunes))
	}
	if notes := section(text, "Дополнительная информация", "ОБЩИЕ ПОЛОЖЕНИЯ", "Footer menu"); notes != "" {
		parts = append(parts, service.TruncateRunes(dropBelarus(notes), detailsNotesRunes))
	}
	if measures := section(text, "Меры нетарифного регулирования", "Дополнительная информация", "ОБЩИЕ ПОЛОЖЕНИЯ", "Footer menu"); measures != "" {
		parts = append(parts, service.TruncateRunes(dropBelarus(measures), detailsMeasureRunes))
	}
	return service.TruncateRunes(strings.Join(parts, "\n\n"), detailsMaxRunes)
}

// dropBelarus убирает строки с белорусскими нормами (BYN, указы и постановления РБ).
func dropBelarus(text string) string {
	lines := strings.Split(text, "\n")
	kept := lines[:0]
	for _, line := range lines {
		lower := strings.ToLower(line)
		if strings.Contains(lower, "беларусь") || strings.Contains(line, "BYN") || strings.Contains(line, " РБ") {
			continue
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}
