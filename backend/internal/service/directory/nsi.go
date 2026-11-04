package directory

// Сборник предварительных решений таможенных органов государств ЕАЭС по классификации товаров —
// справочник 1999 подсистемы НСИ ЕЭК (nsi.eaeunion.org): официальные решения с кодом, описанием товара,
// страной и обоснованием (ОПИ). Для агента — третий источник в search_declarations, старше деклараций:
// решение по такому же товару — доказательство (decisionQuote). Поиск — тот же открытый API, что у
// страницы портала (полнотекстовый, все слова запроса сразу), без ключа; ответ хранится в кеше страниц,
// как у других сайтов.

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
)

const (
	// NSIDecisionsSite — имя источника в ответе search_declarations.
	NSIDecisionsSite  = "nsi.eaeunion.org"
	nsiDecisionsAPI   = "https://nsi.eaeunion.org/portal/api/dictionaries/1999/get-list-data"
	nsiDecisionsPage  = "https://nsi.eaeunion.org/portal/1999?searchText="
	nsiDecisionsLimit = 20
	nsiMaxBytes       = 4 << 20
)

var nsiNonDigits = regexp.MustCompile(`\D`)

// NSIDecisionsURL — страница сборника с этим запросом: ссылка для сотрудника и ключ кеша.
func NSIDecisionsURL(query string) string { return nsiDecisionsPage + url.QueryEscape(query) }

// NSIDecision — одно предварительное решение.
type NSIDecision struct {
	Code, Country, Description, Justification string
}

// parseNSIDecisions — ответ API; код без пробелов (в сборнике коды бывают и с обычными, и с неразрывными).
func parseNSIDecisions(body []byte) ([]NSIDecision, error) {
	var rows []struct {
		Data struct {
			Code          string `json:"CodeTNVED"`
			Description   string `json:"Description"`
			Justification string `json:"Justification"`
			Country       struct {
				Name string `json:"name"`
			} `json:"Country"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &rows); err != nil {
		return nil, err
	}
	decisions := make([]NSIDecision, 0, len(rows))
	for _, row := range rows {
		code := nsiNonDigits.ReplaceAllString(row.Data.Code, "")
		if len(code) != 10 {
			continue
		}
		decisions = append(decisions, NSIDecision{Code: code, Country: strings.TrimSpace(row.Data.Country.Name),
			Description: strings.TrimSpace(row.Data.Description), Justification: strings.TrimSpace(row.Data.Justification)})
	}
	return decisions, nil
}

// nsiDecisions — поиск в сборнике по словам: до nsiDecisionsLimit решений.
func (f *WebPageFetcher) nsiDecisions(ctx context.Context, query string) ([]NSIDecision, error) {
	payload, _ := json.Marshal(map[string]any{
		"date": time.Now().Format("2006-01-02"), "offset": 0, "limit": nsiDecisionsLimit,
		"filter": []map[string]string{{"code": "searchText", "value": query, "conditionType": "like"}}, "sort": []any{},
	})
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, nsiDecisionsAPI, bytes.NewReader(payload))
	if err != nil {
		return nil, ErrWebPageUnavailable
	}
	request.Header.Set("User-Agent", webUserAgent)
	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	request.Header.Set("Accept", "application/json")
	response, err := f.client.Do(request)
	if err != nil {
		return nil, ErrWebPageUnavailable
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, ErrWebPageUnavailable
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, nsiMaxBytes))
	if err != nil {
		return nil, ErrWebPageUnavailable
	}
	decisions, err := parseNSIDecisions(body)
	if err != nil {
		return nil, ErrWebPageUnavailable
	}
	return decisions, nil
}

// nsiGroups — решения по кодам: сколько решений под кодом в выборке и сами решения (страна, описание,
// обоснование) — коды по числу решений. Source примера — подпись для decisionSource.
func nsiGroups(decisions []NSIDecision) []DeclarationGroup {
	byCode := map[string]*DeclarationGroup{}
	order := []string{}
	for _, decision := range decisions {
		group, ok := byCode[decision.Code]
		if !ok {
			group = &DeclarationGroup{Code: decision.Code, Name: "предварительное решение", Examples: []DeclarationExample{}}
			byCode[decision.Code] = group
			order = append(order, decision.Code)
		}
		group.Count++
		text := service.TruncateRunes(decision.Description, declarationRunes)
		if decision.Justification != "" {
			text += " Обоснование: " + service.TruncateRunes(decision.Justification, 200)
		}
		group.Examples = append(group.Examples, DeclarationExample{Code: decision.Code, Text: text, Source: "предварительное решение · " + decision.Country + " (НСИ ЕАЭС)"})
	}
	groups := make([]DeclarationGroup, 0, len(order))
	for _, code := range order {
		groups = append(groups, *byCode[code])
	}
	sort.SliceStable(groups, func(a, b int) bool { return groups[a].Count > groups[b].Count })
	return groups
}

// groupsWithin — только коды с этим началом (позиция — 4 цифры, код — 10); "" — все.
func groupsWithin(groups []DeclarationGroup, within string) []DeclarationGroup {
	if within == "" {
		return groups
	}
	kept := []DeclarationGroup{}
	for _, group := range groups {
		if strings.HasPrefix(group.Code, within) {
			kept = append(kept, group)
		}
	}
	return kept
}
