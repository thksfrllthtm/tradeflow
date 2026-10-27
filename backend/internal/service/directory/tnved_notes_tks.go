package directory

// Резервные пояснения к ТН ВЭД с tks.ru — когда у кода нет страницы TWS.BY или она не открылась. Две
// страницы: к группе (2 знака: примечания к группе и общие положения) и к товарной позиции (4 знака:
// текст позиции с субпозициями и «В данную товарную позицию включаются…»). Дерева кода, ставок, мер и
// классификационных решений на tks.ru нет. Кодировка windows-1251; страницы хранятся в кеше страниц
// 30 дней, как остальные.

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"
)

const tksNotesBaseURL = "https://www.tks.ru/db/tnved/prim/c"

// ErrTKSNotesUnavailable — ни одна из страниц пояснений не открылась или пояснений к коду на сайте нет.
var ErrTKSNotesUnavailable = errors.New("tks.ru notes unavailable")

// ponytail: блок пояснений — до первого </div>; на страницах групп и позиций вложенных div нет.
var tksNotesContent = regexp.MustCompile(`(?s)<div id="prim_issue_content">(.*?)</div>`)

// TKSNotesURL — адрес пояснений к группе (2 цифры) или товарной позиции (4 цифры).
func TKSNotesURL(prefix string) string { return tksNotesBaseURL + prefix + "/" }

// TKSNotes — пояснения к группе и товарной позиции кода одним текстом; fromCache — обе страницы из
// базы. Не открылась одна страница — отдаётся другая.
func (f *WebPageFetcher) TKSNotes(ctx context.Context, code string) (text string, fromCache bool, err error) {
	if len(code) < 4 {
		return "", false, ErrTKSNotesUnavailable
	}
	fromCache = true
	parts := []string{}
	for _, part := range []struct{ prefix, title string }{{code[:2], "группа " + code[:2]}, {code[:4], "товарная позиция " + code[:4]}} {
		page, err := f.tksNotesPage(ctx, part.prefix)
		if err != nil {
			continue
		}
		fromCache = fromCache && page.FromCache
		parts = append(parts, "Пояснения к ТН ВЭД (tks.ru), "+part.title+":\n"+page.Text)
	}
	if len(parts) == 0 {
		return "", false, ErrTKSNotesUnavailable
	}
	return strings.Join(parts, "\n\n"), fromCache, nil
}

func (f *WebPageFetcher) tksNotesPage(ctx context.Context, prefix string) (WebPage, error) {
	address := TKSNotesURL(prefix)
	cached, found, err := f.cached(ctx, address)
	if err != nil {
		return WebPage{}, err
	}
	if found && time.Since(cached.fetchedAt) < webPageMaxAge {
		return cached.page, nil
	}
	body, _, err := f.download(ctx, address)
	if err == nil {
		if text := parseTKSNotes(decodeWindows1251(body)); text != "" {
			page := WebPage{URL: address, Title: "Пояснения к ТН ВЭД " + prefix, Text: text}
			f.save(ctx, page)
			return page, nil
		}
		err = ErrTKSNotesUnavailable
	}
	if found {
		return cached.page, nil
	}
	return WebPage{}, err
}

// parseTKSNotes — текст блока пояснений страницы tks.ru ("" — блока нет: пояснений к коду на сайте нет).
func parseTKSNotes(body string) string {
	match := tksNotesContent.FindStringSubmatch(body)
	if match == nil {
		return ""
	}
	return htmlToText(match[1])
}
