package repository

import (
	"context"
	"strings"
	"time"
)

// База документов для администратора: сохранённые ответы «Документы для ввоза» по кодам (со всеми версиями),
// справки tnved.info и тексты актов, которые читали агенты (кэш web_pages).

// goodsHelpPrefix — ключ справки tnved.info в кэше страниц (см. directory.goodsHelpKey).
const goodsHelpPrefix = "https://tnved.info/Goodshelp/?Code="

// declarationHosts — страницы поиска деклараций и решений (ifcg, tks, сборник ЕАЭС): не акты и не справки.
var declarationHosts = []string{"%://www.ifcg.ru/%", "%://ifcg.ru/%", "%://www.tks.ru/%", "%://tks.ru/%", "%://nsi.eaeunion.org/%"}

// DocumentsBaseRow — последний ответ по коду: Versions — сколько всего ответов сохранено по коду.
type DocumentsBaseRow struct {
	Code      string
	Product   string
	SavedAt   time.Time
	Author    string
	Versions  int
	Documents JSON
}

// DocumentsBaseRows — коды с сохранёнными ответами, последний ответ на код; query — начало кода или слова
// товара ("" — все). Последние сохранённые — первыми.
func (r *Repository) DocumentsBaseRows(ctx context.Context, query string, limit int) ([]DocumentsBaseRow, error) {
	rows := []DocumentsBaseRow{}
	query = strings.TrimSpace(query)
	digits := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, query)
	inner := r.db.WithContext(ctx).Table("tnved_code_documents AS d").
		Select("DISTINCT ON (d.code) d.code, d.product, d.created_at AS saved_at, COALESCE(u.name, '') AS author, d.documents, COUNT(*) OVER (PARTITION BY d.code) AS versions").
		Joins("LEFT JOIN users u ON u.id = d.user_id").Order("d.code, d.created_at DESC, d.id DESC")
	if query != "" {
		if digits != "" && len(digits) == len(strings.ReplaceAll(query, " ", "")) {
			inner = inner.Where("d.code LIKE ?", digits+"%")
		} else {
			inner = inner.Where("d.product ILIKE ?", "%"+query+"%")
		}
	}
	err := r.db.WithContext(ctx).Table("(?) AS latest", inner).Order("saved_at DESC").Limit(limit).Scan(&rows).Error
	return rows, err
}

// CodeDocumentsVersion — один сохранённый ответ по коду.
type CodeDocumentsVersion struct {
	ID          int64
	Code        string
	Product     string
	Description string
	Documents   JSON
	SessionID   *int64
	Author      string
	CreatedAt   time.Time
}

// CodeDocumentsHistory — все ответы по коду, последний первым.
func (r *Repository) CodeDocumentsHistory(ctx context.Context, code string) ([]CodeDocumentsVersion, error) {
	rows := []CodeDocumentsVersion{}
	err := r.db.WithContext(ctx).Table("tnved_code_documents AS d").
		Select("d.id, d.code, d.product, d.description, d.documents, d.session_id, COALESCE(u.name, '') AS author, d.created_at").
		Joins("LEFT JOIN users u ON u.id = d.user_id").Where("d.code = ?", code).Order("d.created_at DESC, d.id DESC").Scan(&rows).Error
	return rows, err
}

// CachedPageInfo — страница в кэше без текста: адрес, заголовок, когда получена, длина текста.
type CachedPageInfo struct {
	URL       string
	Title     string
	FetchedAt time.Time
	Size      int
}

// CachedPagesByURL — сведения о страницах кэша по адресам (есть не все — те, что агенты открывали).
func (r *Repository) CachedPagesByURL(ctx context.Context, urls []string) ([]CachedPageInfo, error) {
	pages := []CachedPageInfo{}
	if len(urls) == 0 {
		return pages, nil
	}
	err := r.db.WithContext(ctx).Table("web_pages").Select("url, title, fetched_at, length(text) AS size").Where("url IN ?", urls).Scan(&pages).Error
	return pages, err
}

// CachedGoodsHelp — справка tnved.info по коду из кэша; found — есть ли.
func (r *Repository) CachedGoodsHelp(ctx context.Context, code string) (WebPageCache, bool, error) {
	var pages []WebPageCache
	err := r.db.WithContext(ctx).Where("url LIKE ?", goodsHelpPrefix+code+"&%").Order("fetched_at DESC").Limit(1).Find(&pages).Error
	if err != nil || len(pages) == 0 {
		return WebPageCache{}, false, err
	}
	return pages[0], true, nil
}

// CachedActs — акты и другие документы в кэше (без справок tnved.info и поиска деклараций); query — слова
// заголовка или адреса. Последние полученные — первыми.
func (r *Repository) CachedActs(ctx context.Context, query string, limit int) ([]CachedPageInfo, error) {
	pages := []CachedPageInfo{}
	scope := r.db.WithContext(ctx).Table("web_pages").Select("url, title, fetched_at, length(text) AS size").Where("url NOT LIKE ?", goodsHelpPrefix+"%")
	for _, host := range declarationHosts {
		scope = scope.Where("url NOT LIKE ?", host)
	}
	if query = strings.TrimSpace(query); query != "" {
		scope = scope.Where("(title ILIKE ? OR url ILIKE ?)", "%"+query+"%", "%"+query+"%")
	}
	err := scope.Order("fetched_at DESC").Limit(limit).Scan(&pages).Error
	return pages, err
}

// DocumentsBaseSummary — сколько всего: ответов, кодов, справок и актов в кэше.
type DocumentsBaseSummary struct {
	Answers int64
	Codes   int64
	Helps   int64
	Acts    int64
}

func (r *Repository) DocumentsBaseSummary(ctx context.Context) (DocumentsBaseSummary, error) {
	var summary DocumentsBaseSummary
	db := r.db.WithContext(ctx)
	if err := db.Table("tnved_code_documents").Select("COUNT(*) AS answers, COUNT(DISTINCT code) AS codes").Scan(&summary).Error; err != nil {
		return summary, err
	}
	if err := db.Table("web_pages").Where("url LIKE ?", goodsHelpPrefix+"%").Count(&summary.Helps).Error; err != nil {
		return summary, err
	}
	acts := db.Table("web_pages").Where("url NOT LIKE ?", goodsHelpPrefix+"%")
	for _, host := range declarationHosts {
		acts = acts.Where("url NOT LIKE ?", host)
	}
	err := acts.Count(&summary.Acts).Error
	return summary, err
}
