package tnvedagent

import (
	"context"
	"encoding/json"
	"slices"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/directory"
)

// База документов (страница администратора): что программа уже собрала по «Документам для ввоза» — ответы по
// кодам со всеми версиями, справки tnved.info и тексты актов, которые читали агенты.

const documentsBaseLimit = 300

type DocumentsBaseSummaryView struct {
	Answers int64 `json:"answers"`
	Codes   int64 `json:"codes"`
	Helps   int64 `json:"helps"`
	Acts    int64 `json:"acts"`
}

// DocumentsBaseItem — код в списке: последний ответ, сколько версий, мер «нужно», «возможно», «не относится»
// и виды документов среди нужных и возможных (СС, ДС, СГР…).
type DocumentsBaseItem struct {
	Code     string    `json:"code"`
	Product  string    `json:"product"`
	SavedAt  time.Time `json:"savedAt"`
	Author   string    `json:"author"`
	Versions int       `json:"versions"`
	Yes      int       `json:"yes"`
	Maybe    int       `json:"maybe"`
	No       int       `json:"no"`
	Kinds    []string  `json:"kinds"`
}

type DocumentsBasePage struct {
	Summary DocumentsBaseSummaryView `json:"summary"`
	Items   []DocumentsBaseItem      `json:"items"`
}

// DocumentsBaseVersion — один сохранённый ответ по коду.
type DocumentsBaseVersion struct {
	ID          int64           `json:"id"`
	Product     string          `json:"product"`
	Description string          `json:"description"`
	Documents   []AgentDocument `json:"documents"`
	SessionID   *int64          `json:"sessionId"`
	Author      string          `json:"author"`
	SavedAt     time.Time       `json:"savedAt"`
}

// CachedPageView — страница в кэше: Cached — есть ли её текст у программы.
type CachedPageView struct {
	URL       string     `json:"url"`
	Title     string     `json:"title"`
	FetchedAt *time.Time `json:"fetchedAt"`
	Size      int        `json:"size"`
	Cached    bool       `json:"cached"`
}

type GoodsHelpView struct {
	URL       string    `json:"url"`
	Title     string    `json:"title"`
	Text      string    `json:"text"`
	FetchedAt time.Time `json:"fetchedAt"`
}

// DocumentsBaseEntry — карточка кода: наименование, все ответы, справка tnved.info и акты из ответов.
type DocumentsBaseEntry struct {
	Code     string                 `json:"code"`
	Name     string                 `json:"name"`
	Versions []DocumentsBaseVersion `json:"versions"`
	Help     *GoodsHelpView         `json:"help"`
	Acts     []CachedPageView       `json:"acts"`
}

func documentsOf(raw repository.JSON) []AgentDocument {
	documents := []AgentDocument{}
	_ = json.Unmarshal(raw, &documents)
	return documents
}

// DocumentsBase — коды с сохранёнными ответами и сводка по базе.
func (s *TNVEDAgentService) DocumentsBase(ctx context.Context, query string) (DocumentsBasePage, error) {
	summary, err := s.repository.DocumentsBaseSummary(ctx)
	if err != nil {
		return DocumentsBasePage{}, err
	}
	rows, err := s.repository.DocumentsBaseRows(ctx, query, documentsBaseLimit)
	if err != nil {
		return DocumentsBasePage{}, err
	}
	page := DocumentsBasePage{Summary: DocumentsBaseSummaryView(summary), Items: make([]DocumentsBaseItem, 0, len(rows))}
	for _, row := range rows {
		item := DocumentsBaseItem{Code: row.Code, Product: row.Product, SavedAt: row.SavedAt, Author: row.Author, Versions: row.Versions, Kinds: []string{}}
		for _, document := range documentsOf(row.Documents) {
			switch document.Applies {
			case "yes":
				item.Yes++
			case "maybe":
				item.Maybe++
			default:
				item.No++
			}
			if document.Applies != "no" && document.Kind != "" && document.Kind != "other" && !slices.Contains(item.Kinds, document.Kind) {
				item.Kinds = append(item.Kinds, document.Kind)
			}
		}
		page.Items = append(page.Items, item)
	}
	return page, nil
}

// DocumentsBaseCode — карточка кода для администратора.
func (s *TNVEDAgentService) DocumentsBaseCode(ctx context.Context, code string) (DocumentsBaseEntry, error) {
	code = nonDigits.ReplaceAllString(code, "")
	if len(code) != 10 {
		return DocumentsBaseEntry{}, &service.ValidationError{Field: "code", Message: "Код — 10 цифр"}
	}
	entry := DocumentsBaseEntry{Code: code, Versions: []DocumentsBaseVersion{}, Acts: []CachedPageView{}}
	if items, err := s.data.LookupCodes(ctx, []string{code}); err == nil && len(items) > 0 {
		entry.Name = items[0].Name
	}
	history, err := s.repository.CodeDocumentsHistory(ctx, code)
	if err != nil {
		return DocumentsBaseEntry{}, err
	}
	urls := []string{}
	for _, row := range history {
		version := DocumentsBaseVersion{ID: row.ID, Product: row.Product, Description: row.Description, Documents: documentsOf(row.Documents), SessionID: row.SessionID, Author: row.Author, SavedAt: row.CreatedAt}
		for _, document := range version.Documents {
			if address, ok := directory.CanonicalWebURL(document.URL); ok && !slices.Contains(urls, address) {
				urls = append(urls, address)
			}
		}
		entry.Versions = append(entry.Versions, version)
	}
	help, found, err := s.repository.CachedGoodsHelp(ctx, code)
	if err != nil {
		return DocumentsBaseEntry{}, err
	}
	if found {
		entry.Help = &GoodsHelpView{URL: directory.GoodsHelpURL(code, help.FetchedAt), Title: help.Title, Text: help.Text, FetchedAt: help.FetchedAt}
	}
	pages, err := s.repository.CachedPagesByURL(ctx, urls)
	if err != nil {
		return DocumentsBaseEntry{}, err
	}
	byURL := map[string]repository.CachedPageInfo{}
	for _, page := range pages {
		byURL[page.URL] = page
	}
	for _, address := range urls {
		view := CachedPageView{URL: address}
		if page, ok := byURL[address]; ok {
			fetched := page.FetchedAt
			view.Title, view.FetchedAt, view.Size, view.Cached = page.Title, &fetched, page.Size, true
		}
		entry.Acts = append(entry.Acts, view)
	}
	return entry, nil
}

// DocumentsSources — акты и документы в кэше программы (без справок и поиска деклараций).
func (s *TNVEDAgentService) DocumentsSources(ctx context.Context, query string) ([]CachedPageView, error) {
	pages, err := s.repository.CachedActs(ctx, query, documentsBaseLimit)
	if err != nil {
		return nil, err
	}
	views := make([]CachedPageView, 0, len(pages))
	for _, page := range pages {
		fetched := page.FetchedAt
		views = append(views, CachedPageView{URL: page.URL, Title: page.Title, FetchedAt: &fetched, Size: page.Size, Cached: true})
	}
	return views, nil
}
