// Package legalacts — локальная база нормативных актов для «Документов для ввоза»: скачивание сводных
// редакций, проверка кодов, разбор перечней в строки. Источник — API adilet.zan.kz (без входа, неофициальное),
// при сбое — zakon.uchet.kz (копия adilet с теми же кодами).
package legalacts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	AdiletAPI = "https://adilet.zan.kz/api/documents/"
	ZakonDocs = "https://zakon.uchet.kz/rus/docs/"
	AdiletDoc = "https://adilet.zan.kz/rus/docs/"
	// maxActBytes — Кодекс о таможенном регулировании в API весит десятки мегабайт.
	maxActBytes = 96 << 20
	// minActRunes — короче не бывает даже решение в два абзаца; заглушки и пустые оболочки короче.
	minActRunes = 300
)

var (
	// ErrNotFound — акта с таким кодом нет ни в одном источнике.
	ErrNotFound = errors.New("act not found")
	// adiletZone — даты редакций adilet хранит как полночь по UTC+6 («2026-01-17T18:00:00Z» — редакция от 18.01.2026).
	adiletZone       = time.FixedZone("UTC+6", 6*60*60)
	zakonRevision    = regexp.MustCompile(`изменениями на:\s*(\d{2}\.\d{2}\.\d{4})`)
	zakonTitle       = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	tags             = regexp.MustCompile(`<[^>]+>`)
	maintenanceWords = []string{"идут технические работы", "сайт скоро возобновит"}
)

// Source — откуда скачивать (пусто — настоящие сайты; в тестах — свой сервер).
type Source struct {
	Client    *http.Client
	AdiletAPI string
	Zakon     string
}

// Revision — скачанная редакция акта.
type Revision struct {
	Code string
	// Source — "adilet" или "zakon"; SourceURL — страница акта для сотрудника.
	Source, SourceURL   string
	OfficialName, Title string
	// ActStatus — статус adilet: upd/new — действует, yts — утратил силу, err — ошибка в базе adilet; у zakon — "".
	ActStatus string
	// RevisionDate — дата сводной редакции (нулевая — источник её не указал).
	RevisionDate time.Time
	// Content — HTML текста акта (у zakon — только <article>, без шапки сайта: иначе sha256 менялся бы от рекламы).
	Content []byte
	SHA256  string
	// APIProblem — API adilet не ответил или сменил формат (текст взят с zakon): для администратора.
	APIProblem string
}

// Fetch — сводная редакция акта по коду adilet.
func (s Source) Fetch(ctx context.Context, code string) (Revision, error) {
	revision, err := s.fetchAdilet(ctx, code)
	if err == nil {
		return revision, nil
	}
	notFound := errors.Is(err, ErrNotFound)
	revision, zakonErr := s.fetchZakon(ctx, code)
	if zakonErr == nil {
		if !notFound {
			revision.APIProblem = err.Error()
		}
		return revision, nil
	}
	if notFound && errors.Is(zakonErr, ErrNotFound) {
		return Revision{}, ErrNotFound
	}
	return Revision{}, fmt.Errorf("adilet: %v; zakon.uchet.kz: %v", err, zakonErr)
}

type adiletDocument struct {
	Document struct {
		ID           string `json:"id"`
		GroupNumber  string `json:"group_number"`
		OfficialName string `json:"official_name"`
		TitleRU      string `json:"title_ru"`
		Status       string `json:"status"`
		EndDate      string `json:"end_date"`
		TextContent  string `json:"text_content"`
	} `json:"document"`
}

type adiletVersions struct {
	Versions []struct {
		IsCurrent    bool   `json:"is_current"`
		RevisionDate string `json:"revision_date"`
	} `json:"versions"`
}

func (s Source) fetchAdilet(ctx context.Context, code string) (Revision, error) {
	base := s.AdiletAPI
	if base == "" {
		base = AdiletAPI
	}
	var data adiletDocument
	if err := s.getJSON(ctx, base+"by-ngr/"+url.PathEscape(code), &data); err != nil {
		return Revision{}, err
	}
	document := data.Document
	if !strings.EqualFold(document.GroupNumber, code) || document.ID == "" || document.OfficialName == "" {
		return Revision{}, fmt.Errorf("формат ответа by-ngr изменился: код %q, id %q, название %q", document.GroupNumber, document.ID, document.OfficialName)
	}
	if problem := actProblem(document.TextContent); problem != "" {
		return Revision{}, fmt.Errorf("текст акта в API: %s", problem)
	}
	revision := Revision{
		Code: code, Source: "adilet", SourceURL: AdiletDoc + code,
		OfficialName: clean(document.OfficialName), Title: clean(document.TitleRU), ActStatus: document.Status,
		Content: []byte(document.TextContent),
	}
	// Дата редакции — у текущей версии; история не открылась — дата окончания редакции из карточки.
	var versions adiletVersions
	if err := s.getJSON(ctx, base+url.PathEscape(document.ID)+"/versions?page=1&page_size=50", &versions); err == nil {
		for _, version := range versions.Versions {
			if version.IsCurrent {
				revision.RevisionDate = adiletDate(version.RevisionDate)
			}
		}
	}
	if revision.RevisionDate.IsZero() {
		revision.RevisionDate = adiletDate(document.EndDate)
	}
	revision.SHA256 = digest(revision.Content)
	return revision, nil
}

func (s Source) fetchZakon(ctx context.Context, code string) (Revision, error) {
	base := s.Zakon
	if base == "" {
		base = ZakonDocs
	}
	body, err := s.get(ctx, base+url.PathEscape(code), "text/html")
	if err != nil {
		return Revision{}, err
	}
	page := string(body)
	start, end := strings.Index(page, "<article"), strings.LastIndex(page, "</article>")
	if start < 0 || end <= start {
		return Revision{}, errors.New("на странице нет текста акта (<article>)")
	}
	article := page[start : end+len("</article>")]
	if problem := actProblem(article); problem != "" {
		return Revision{}, errors.New(problem)
	}
	revision := Revision{Code: code, Source: "zakon", SourceURL: base + code, Content: []byte(article)}
	if match := zakonTitle.FindStringSubmatch(page); match != nil {
		revision.Title = clean(match[1])
	}
	if match := zakonRevision.FindStringSubmatch(page); match != nil {
		revision.RevisionDate, _ = time.ParseInLocation("02.01.2006", match[1], adiletZone)
	}
	revision.SHA256 = digest(revision.Content)
	return revision, nil
}

// actProblem — почему HTML не текст акта ("" — похож на текст акта).
func actProblem(content string) string {
	text := clean(content)
	if utf8.RuneCountInString(text) < minActRunes {
		return fmt.Sprintf("текст короче %d знаков", minActRunes)
	}
	head := strings.ToLower(text)
	if len(head) > 4000 {
		head = head[:4000]
	}
	for _, word := range maintenanceWords {
		if strings.Contains(head, word) {
			return "заглушка «технические работы»"
		}
	}
	return ""
}

func (s Source) getJSON(ctx context.Context, address string, target any) error {
	body, err := s.get(ctx, address, "application/json")
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, target); err != nil {
		return fmt.Errorf("ответ не JSON: %w", err)
	}
	return nil
}

func (s Source) get(ctx context.Context, address, accept string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", "Mozilla/5.0 (compatible; TradeFlow/1.0; +internal customs estimate tool)")
	request.Header.Set("Accept", accept)
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Minute}
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("запрос %s: %w", address, err)
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		return nil, ErrNotFound
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: HTTP %d", address, response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxActBytes+1))
	if err != nil {
		return nil, fmt.Errorf("чтение %s: %w", address, err)
	}
	if len(body) > maxActBytes {
		return nil, fmt.Errorf("%s: больше %d МБ", address, maxActBytes>>20)
	}
	return body, nil
}

// adiletDate — «2026-01-17T18:00:00.000Z» → 18.01.2026 (полночь UTC+6); не дата — нулевое время.
func adiletDate(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Time{}
	}
	local := parsed.In(adiletZone)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, adiletZone)
}

func clean(value string) string {
	return strings.Join(strings.Fields(html.UnescapeString(tags.ReplaceAllString(value, " "))), " ")
}

func digest(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}
