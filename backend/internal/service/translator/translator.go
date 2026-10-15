package translator

// Раздел «Переводчик»: перевод Word-документов (.docx) по сегментам с правкой на сайте.
// Документы видят и правят все сотрудники; удаляет документ тот, кто загрузил, или администратор.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
)

const (
	MaxFileBytes = 20 << 20
	maxSegments  = 3000
)

type language struct{ name, genitive string }

// Languages — языки перевода: код → название для промпта («с китайского на русский»).
// Новый язык — строка здесь и во frontend.
var Languages = map[string]language{
	"zh": {"китайский", "китайского"},
	"ru": {"русский", "русского"},
	"en": {"английский", "английского"},
	"kk": {"казахский", "казахского"},
}

var ErrForbidden = errors.New("translator: forbidden")

type Service struct {
	repository *repository.Repository
	model      Model
	mu         sync.Mutex
	running    map[int64]bool // документы, перевод которых идёт в этом процессе
}

func NewService(r *repository.Repository, model Model) *Service {
	return &Service{repository: r, model: model, running: map[int64]bool{}}
}

func (s *Service) List(ctx context.Context, query string) ([]repository.TranslatorDocumentSummary, error) {
	return s.repository.ListTranslatorDocuments(ctx, query)
}

// Upload разбирает .docx на сегменты и сохраняет документ; возвращает его id.
func (s *Service) Upload(ctx context.Context, userID int64, fileName string, data []byte, sourceLang, targetLang string) (int64, error) {
	if _, ok := Languages[sourceLang]; !ok {
		return 0, &service.ValidationError{Field: "sourceLang", Message: "Выберите язык оригинала"}
	}
	if _, ok := Languages[targetLang]; !ok {
		return 0, &service.ValidationError{Field: "targetLang", Message: "Выберите язык перевода"}
	}
	if sourceLang == targetLang {
		return 0, &service.ValidationError{Field: "targetLang", Message: "Язык перевода должен отличаться от языка оригинала"}
	}
	if err := checkFile(fileName, len(data)); err != nil {
		return 0, err
	}
	parsed, skipped, err := ParseSegments(data)
	switch {
	case errors.Is(err, errTrackedChanges):
		return 0, &service.ValidationError{Field: "file", Message: "В документе есть исправления в режиме рецензирования. Примите или отклоните их в Word и загрузите файл снова"}
	case err != nil:
		return 0, &service.ValidationError{Field: "file", Message: "Не удалось прочитать файл: это не документ Word (.docx) или он повреждён"}
	case len(parsed) == 0:
		return 0, &service.ValidationError{Field: "file", Message: "В документе нет текста для перевода"}
	case len(parsed) > maxSegments:
		return 0, &service.ValidationError{Field: "file", Message: fmt.Sprintf("Абзацев с текстом в документе: %d, а можно не больше 3 000. Разделите его на части и загрузите по очереди", len(parsed))}
	}
	skippedJSON, err := json.Marshal(skipped)
	if err != nil {
		return 0, err
	}
	segments := make([]repository.TranslatorSegment, len(parsed))
	for i, item := range parsed {
		tags, err := json.Marshal(item.Tags)
		if err != nil {
			return 0, err
		}
		segments[i] = repository.TranslatorSegment{Kind: item.Kind, SourceText: item.Text, Tags: tags, Status: "machine", QA: repository.JSON("[]")}
	}
	document := repository.TranslatorDocument{
		Name: documentName(fileName), SourceLang: sourceLang, TargetLang: targetLang, OriginalFile: data,
		Status: "parsed", Skipped: skippedJSON, CreatedBy: &userID,
	}
	if err := s.repository.CreateTranslatorDocument(ctx, &document, segments); err != nil {
		return 0, err
	}
	return document.ID, nil
}

func checkFile(fileName string, size int) error {
	switch strings.ToLower(filepath.Ext(fileName)) {
	case ".docx":
	case ".doc":
		return &service.ValidationError{Field: "file", Message: "Файлы .doc не поддерживаются: откройте документ в Word и сохраните его как .docx"}
	default:
		return &service.ValidationError{Field: "file", Message: "Выберите документ Word в формате .docx"}
	}
	if size > MaxFileBytes {
		return &service.ValidationError{Field: "file", Message: "Файл больше 20 МБ. Уменьшите его (например, сожмите картинки в Word) или разделите на части"}
	}
	return nil
}

// documentName — имя файла без «.docx»; из него же имя выгрузки.
func documentName(fileName string) string {
	name := strings.TrimSpace(strings.TrimSuffix(filepath.Base(fileName), filepath.Ext(fileName)))
	for utf8.RuneCountInString(name) > 255 {
		_, size := utf8.DecodeLastRuneInString(name)
		name = name[:len(name)-size]
	}
	if name == "" {
		return "Документ"
	}
	return name
}

// TagView — как показать тег сегмента: фрагмент текста со своим форматированием (kind = text)
// или неразбираемый элемент (tab, br, image, field, formula, other).
type TagView struct {
	Kind      string `json:"kind"`
	Bold      bool   `json:"bold,omitempty"`
	Italic    bool   `json:"italic,omitempty"`
	Underline bool   `json:"underline,omitempty"`
}

type SegmentView struct {
	ID         int64              `json:"id"`
	Kind       string             `json:"kind"`
	SourceText string             `json:"sourceText"`
	MTText     string             `json:"mtText"`
	TargetText string             `json:"targetText"`
	Status     string             `json:"status"`
	Locked     bool               `json:"locked"`
	QA         repository.JSON    `json:"qa"`
	Tags       map[string]TagView `json:"tags"`
}

type DocumentView struct {
	Document repository.TranslatorDocumentSummary `json:"document"`
	Segments []SegmentView                        `json:"segments"`
	// Now — время ответа: страница передаёт его в следующий опрос (since) и получает только изменения.
	Now time.Time `json:"now"`
}

// pollOverlap — запас для since: сегмент, записанный чуть раньше ответа, но закоммиченный позже, не теряется.
const pollOverlap = 10 * time.Second

// Get — документ с сегментами; с since — только сегменты, изменённые после него. Если перевод
// не закончен, а задачи в этом процессе нет (сервер перезапускался), перевод продолжается.
func (s *Service) Get(ctx context.Context, id int64, since time.Time) (DocumentView, error) {
	now := time.Now()
	document, err := s.repository.TranslatorDocumentByID(ctx, id)
	if err != nil {
		return DocumentView{}, err
	}
	if document.Status == "translating" {
		s.start(document)
	}
	var segments []repository.TranslatorSegment
	if since.IsZero() {
		segments, err = s.repository.TranslatorSegments(ctx, id)
	} else {
		segments, err = s.repository.TranslatorSegmentsSince(ctx, id, since.Add(-pollOverlap))
	}
	if err != nil {
		return DocumentView{}, err
	}
	view := DocumentView{Document: document, Segments: make([]SegmentView, len(segments)), Now: now}
	for i, item := range segments {
		if view.Segments[i], err = segmentView(item); err != nil {
			return DocumentView{}, err
		}
	}
	return view, nil
}

func segmentView(item repository.TranslatorSegment) (SegmentView, error) {
	var tags map[string]string
	if err := json.Unmarshal(item.Tags, &tags); err != nil {
		return SegmentView{}, err
	}
	return SegmentView{
		ID: item.ID, Kind: item.Kind, SourceText: item.SourceText, MTText: item.MTText, TargetText: item.TargetText,
		Status: item.Status, Locked: item.Locked, QA: item.QA,
		Tags: tagViews(tags),
	}, nil
}

var (
	boldPattern      = regexp.MustCompile(`<w:b(?: w:val="(\w*)")?\s*/>`)
	italicPattern    = regexp.MustCompile(`<w:i(?: w:val="(\w*)")?\s*/>`)
	underlinePattern = regexp.MustCompile(`<w:u(?: [^>]*?w:val="(\w*)")?[^>]*/>`)
)

// switchedOn: у w:b / w:i значение «0», «false», «off» выключает форматирование, у w:u — «none».
func switchedOn(pattern *regexp.Regexp, rPr string) bool {
	match := pattern.FindStringSubmatch(rPr)
	if match == nil {
		return false
	}
	switch match[1] {
	case "0", "false", "off", "none":
		return false
	}
	return true
}

func tagViews(tags map[string]string) map[string]TagView {
	views := make(map[string]TagView, len(tags))
	for id, value := range tags {
		if strings.HasPrefix(id, "x") {
			views[id] = TagView{Kind: value}
			continue
		}
		views[id] = TagView{Kind: "text", Bold: switchedOn(boldPattern, value), Italic: switchedOn(italicPattern, value), Underline: switchedOn(underlinePattern, value)}
	}
	return views
}

// Delete удаляет документ: может тот, кто его загрузил, и администратор.
func (s *Service) Delete(ctx context.Context, user repository.User, id int64) error {
	document, err := s.repository.TranslatorDocumentByID(ctx, id)
	if err != nil {
		return err
	}
	if !user.IsAdmin() && (document.CreatedBy == nil || *document.CreatedBy != user.ID) {
		return ErrForbidden
	}
	return s.repository.DeleteTranslatorDocument(ctx, id)
}
