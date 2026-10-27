package directory

// «Справка по товару» tnved.info (значок ℹ на https://tnved.info/search/<код>/): меры
// нетарифного регулирования при ввозе в ЕАЭС — акт, ссылка на него, мера и пояснение, к каким
// товарам она относится. Страница справки — SPA; данные отдаёт API, которое вызывает сам сайт.
// Без входа у API маленький дневной лимит, поэтому справка хранится в базе неделю.

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	goodsHelpAPI      = "https://api.tnved.info/api/Goodshelp/Search"
	goodsHelpMaxAge   = 7 * 24 * time.Hour
	goodsHelpMaxBytes = 2 << 20
	// direction 1 — ввоз в ЕАЭС (2 — вывоз).
	goodsHelpImport = 1
)

// ErrGoodsHelpLimit — tnved.info ответил «Превышен лимит запросов».
var ErrGoodsHelpLimit = errors.New("tnved.info goods help limit")

type goodsHelpAct struct {
	ActName string `json:"actName"`
	ActURL  string `json:"actUrl"`
	Meras   []struct {
		MeraName     string   `json:"meraName"`
		CodeComments []string `json:"codeComments"`
	} `json:"meras"`
	ChildAct *goodsHelpAct `json:"childAct"`
}

type goodsHelpResponse struct {
	ResponseState int    `json:"responseState"`
	ErrorMessage  string `json:"errorMessage"`
	Success       bool   `json:"success"`
	TnvedName     string `json:"tnvedName"`
	MnrsInfo      struct {
		Restrictions []goodsHelpAct `json:"restrictions"`
		Benefits     []goodsHelpAct `json:"benefits"`
		Notes        []goodsHelpAct `json:"notes"`
		Others       []goodsHelpAct `json:"others"`
	} `json:"mnrsInfo"`
}

// GoodsHelpURL — страница справки для сотрудника (её открывает значок ℹ «Справка по коду»).
func GoodsHelpURL(code string, date time.Time) string {
	return fmt.Sprintf("https://tnved.info/Goodshelp/?Code=%s&date=%s&dir=%d", code, date.Format("02.01.2006"), goodsHelpImport)
}

// goodsHelpKey — ключ справки в кэше страниц (без даты: справка одна на код).
func goodsHelpKey(code string) string {
	return fmt.Sprintf("https://tnved.info/Goodshelp/?Code=%s&dir=%d", code, goodsHelpImport)
}

// GoodsHelp — справка по коду: из базы, если свежая, иначе с tnved.info. Лимит или недоступность
// сайта — старая копия, если есть.
func (f *WebPageFetcher) GoodsHelp(ctx context.Context, code string) (WebPage, error) {
	now := time.Now().In(time.FixedZone("UTC+5", 5*60*60))
	display := GoodsHelpURL(code, now)
	cached, found, err := f.cached(ctx, goodsHelpKey(code))
	if err != nil {
		return WebPage{}, err
	}
	cached.page.URL = display
	if found && time.Since(cached.fetchedAt) < goodsHelpMaxAge {
		return cached.page, nil
	}
	page, err := f.fetchGoodsHelp(ctx, code, now)
	if err == nil {
		page.URL = display
		f.saveAs(ctx, goodsHelpKey(code), page)
		return page, nil
	}
	if found {
		return cached.page, nil
	}
	return WebPage{URL: display}, err
}

func (f *WebPageFetcher) fetchGoodsHelp(ctx context.Context, code string, date time.Time) (WebPage, error) {
	payload, _ := json.Marshal(map[string]any{"code": code, "date": date.Format("02.01.2006"), "direction": goodsHelpImport, "sessionId": newSessionID()})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, goodsHelpAPI, bytes.NewReader(payload))
	if err != nil {
		return WebPage{}, err
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", webUserAgent)
	request.Header.Set("Origin", "https://tnved.info")
	request.Header.Set("Referer", "https://tnved.info/")
	response, err := f.client.Do(request)
	if err != nil {
		return WebPage{}, ErrWebPageUnavailable
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, goodsHelpMaxBytes))
	if err != nil {
		return WebPage{}, ErrWebPageUnavailable
	}
	if response.StatusCode == http.StatusTooManyRequests {
		return WebPage{}, ErrGoodsHelpLimit
	}
	if response.StatusCode != http.StatusOK {
		return WebPage{}, ErrWebPageUnavailable
	}
	page, err := parseGoodsHelp(code, body)
	if err == nil {
		// Перечни СС/ДС из справки — в общую базу.
		f.saveGeneralActs(ctx, body)
	}
	return page, err
}

// newSessionID — GUID для API (сайт создаёт его в браузере; без него запрос отклоняется).
func newSessionID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// parseGoodsHelp — текст справки «акт → мера → к каким товарам» и ссылки на акты.
func parseGoodsHelp(code string, body []byte) (WebPage, error) {
	var data goodsHelpResponse
	if err := json.Unmarshal(body, &data); err != nil {
		return WebPage{}, ErrWebPageUnavailable
	}
	if data.ResponseState == http.StatusForbidden || strings.Contains(strings.ToLower(data.ErrorMessage), "лимит") {
		return WebPage{}, ErrGoodsHelpLimit
	}
	if !data.Success || data.ResponseState != http.StatusOK {
		return WebPage{}, ErrWebPageUnavailable
	}
	page := WebPage{Title: "Справка по товару " + formatHelpCode(code) + " (tnved.info, ввоз в ЕАЭС)", Links: []WebLink{}}
	var text strings.Builder
	text.WriteString(page.Title + "\n")
	if name := strings.TrimSpace(data.TnvedName); name != "" {
		text.WriteString("Наименование кода: " + name + "\n")
	}
	seen := map[string]bool{}
	link := func(act goodsHelpAct) string {
		address, ok := CanonicalWebURL(act.ActURL)
		if !ok {
			return ""
		}
		if !seen[address] {
			seen[address] = true
			page.Links = append(page.Links, WebLink{URL: address, Text: strings.TrimSpace(act.ActName)})
		}
		return address
	}
	groups := []struct {
		title string
		acts  []goodsHelpAct
	}{
		{"Меры нетарифного регулирования (ограничения)", data.MnrsInfo.Restrictions},
		{"Льготы", data.MnrsInfo.Benefits},
		{"Примечания", data.MnrsInfo.Notes},
		{"Прочие сведения", data.MnrsInfo.Others},
	}
	total := 0
	for _, group := range groups {
		if len(group.acts) == 0 {
			continue
		}
		text.WriteString("\n" + group.title + ":\n")
		for index, act := range group.acts {
			total++
			text.WriteString(fmt.Sprintf("%d. %s", index+1, strings.TrimSpace(act.ActName)))
			if address := link(act); address != "" {
				text.WriteString(" — " + address)
			}
			text.WriteString("\n")
			for _, measure := range act.Meras {
				if name := strings.Join(strings.Fields(measure.MeraName), " "); name != "" {
					text.WriteString("   Мера: " + name + "\n")
				}
				for _, comment := range measure.CodeComments {
					if comment = strings.Join(strings.Fields(comment), " "); comment != "" {
						text.WriteString("   Относится к товарам: " + comment + "\n")
					}
				}
			}
			for child := act.ChildAct; child != nil; child = child.ChildAct {
				text.WriteString("   Изменения: " + strings.TrimSpace(child.ActName))
				if address := link(*child); address != "" {
					text.WriteString(" — " + address)
				}
				text.WriteString("\n")
			}
		}
	}
	if total == 0 {
		text.WriteString("\nМер нетарифного регулирования при ввозе для этого кода в справке нет.\n")
	}
	page.Text = strings.TrimSpace(text.String())
	return page, nil
}

func formatHelpCode(code string) string {
	if len(code) != 10 {
		return code
	}
	return code[:4] + " " + code[4:6] + " " + code[6:9] + " " + code[9:]
}
