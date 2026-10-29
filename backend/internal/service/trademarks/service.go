package trademarks

// Таможенный реестр объектов интеллектуальной собственности КГД РК (ТРОИС). Администратор загружает файл
// реестра; программа ищет знаки реестра и похожие написания в наименованиях строк сметы и в подборе ИИ.
// Знаки только изображением подписывает ИИ (фоновый обработчик агента) или администратор вручную.

import (
	"context"
	"encoding/base64"
	"errors"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
)

const (
	listLimit    = 50
	maxCheckRows = 1000
	maxCheckText = 2000
)

type Service struct {
	repository *repository.Repository
	// index — записи реестра для поиска; nil — загрузить из базы при следующем поиске.
	// ponytail: кеш в памяти одного экземпляра backend; при нескольких экземплярах — сбрасывать по времени.
	mu    sync.Mutex
	index *Index
}

func NewService(r *repository.Repository) *Service { return &Service{repository: r} }

// Summary — состояние реестра для страницы номенклатуры.
type Summary struct {
	Total          int64  `json:"total"`
	Logos          int64  `json:"logos"`
	LogosRead      int64  `json:"logosRead"`
	ImportedAt     string `json:"importedAt"`
	ImportedByName string `json:"importedByName"`
}

// ImportResult — что загружено: сводка, пропущенные исключённые записи, логотипы, которые не удалось
// прочитать (векторный рисунок), и пропущенные строки файла.
type ImportResult struct {
	Summary  Summary  `json:"summary"`
	Excluded int      `json:"excluded"`
	BadLogos []string `json:"badLogos"`
	Skipped  []string `json:"skipped"`
}

// Item — запись реестра для списка.
type Item struct {
	RegNo           string   `json:"regNo"`
	Name            string   `json:"name"`
	HasLogo         bool     `json:"hasLogo"`
	LogoName        string   `json:"logoName"`
	LogoNameSource  string   `json:"logoNameSource"`
	LogoPicture     string   `json:"logoPicture"`
	LogoRead        bool     `json:"logoRead"`
	Goods           string   `json:"goods"`
	Codes           []string `json:"codes"`
	Holder          string   `json:"holder"`
	Document        string   `json:"document"`
	ProtectedUntil  string   `json:"protectedUntil"`
	Representatives string   `json:"representatives"`
}

// CheckRow — строка сметы для проверки: наименования и код ТН ВЭД.
type CheckRow struct {
	Texts []string `json:"texts"`
	Code  string   `json:"code"`
}

func (s *Service) Import(ctx context.Context, userID int64, file io.Reader, size int64) (ImportResult, error) {
	parsed, err := Parse(file, size)
	if err != nil {
		return ImportResult{}, err
	}
	rows := make([]repository.Trademark, 0, len(parsed.Entries))
	for _, entry := range parsed.Entries {
		row := repository.Trademark{
			RegNo: entry.RegNo, Name: entry.Name, Goods: entry.Goods, Codes: strings.Join(entry.Codes, " "), Holder: entry.Holder,
			Document: entry.Document, ProtectedUntil: entry.ProtectedUntil, Representatives: entry.Representatives,
		}
		if entry.Logo != "" {
			logo := entry.Logo
			row.Logo = &logo
		}
		rows = append(rows, row)
	}
	if err := s.repository.ReplaceTrademarks(ctx, rows, userID); err != nil {
		return ImportResult{}, err
	}
	s.reset()
	summary, err := s.Summary(ctx)
	return ImportResult{Summary: summary, Excluded: parsed.Excluded, BadLogos: nonNil(parsed.BadLogos), Skipped: nonNil(parsed.Skipped)}, err
}

func (s *Service) Summary(ctx context.Context) (Summary, error) {
	row, err := s.repository.TrademarkSummary(ctx)
	if err != nil {
		return Summary{}, err
	}
	summary := Summary{Total: row.Total, Logos: row.Logos, LogosRead: row.LogosRead, ImportedByName: row.ImportedByName}
	if row.ImportedAt != nil {
		summary.ImportedAt = row.ImportedAt.UTC().Format(time.RFC3339)
	}
	return summary, nil
}

// List — записи по запросу, по 50; onlyUnnamed — знаки-изображения без надписи.
func (s *Service) List(ctx context.Context, query string, onlyUnnamed bool, page int) ([]Item, int64, error) {
	rows, total, err := s.repository.SearchTrademarks(ctx, service.TruncateRunes(query, 100), onlyUnnamed, listLimit, max(page, 0)*listLimit)
	if err != nil {
		return nil, 0, err
	}
	items := make([]Item, 0, len(rows))
	for _, row := range rows {
		item := Item{
			RegNo: row.RegNo, Name: row.Name, HasLogo: row.HasLogo, LogoName: row.LogoName, LogoNameSource: row.LogoNameSource, LogoPicture: row.LogoPicture,
			LogoRead: row.LogoReadAt != nil, Goods: row.Goods, Codes: strings.Fields(row.Codes), Holder: row.Holder,
			Document: row.Document, Representatives: row.Representatives,
		}
		if row.ProtectedUntil != nil {
			item.ProtectedUntil = row.ProtectedUntil.Format("2006-01-02")
		}
		items = append(items, item)
	}
	return items, total, nil
}

// Logo — картинка логотипа (MIME и байты); ErrNotFound — нет.
func (s *Service) Logo(ctx context.Context, regNo string) (string, []byte, error) {
	logo, err := s.repository.TrademarkLogoImage(ctx, regNo)
	if err != nil {
		return "", nil, err
	}
	header, data, ok := strings.Cut(logo, ",")
	mime := strings.TrimSuffix(strings.TrimPrefix(header, "data:"), ";base64")
	raw, decodeErr := base64.StdEncoding.DecodeString(data)
	if !ok || decodeErr != nil || !strings.HasPrefix(mime, "image/") {
		return "", nil, repository.ErrNotFound
	}
	return mime, raw, nil
}

// SetLogoName — подпись логотипа администратором.
func (s *Service) SetLogoName(ctx context.Context, regNo, name string) error {
	if err := s.repository.SetTrademarkLogoName(ctx, regNo, service.TruncateRunes(cleanText(name), 300)); err != nil {
		return err
	}
	s.reset()
	return nil
}

// Match — знаки реестра в наименованиях одного товара.
func (s *Service) Match(ctx context.Context, texts []string, code string) ([]Hit, error) {
	index, err := s.loadIndex(ctx)
	if err != nil {
		return nil, err
	}
	return index.Match(texts, code), nil
}

// Check — знаки реестра для строк сметы (по строке — список, пустой — ничего не найдено).
func (s *Service) Check(ctx context.Context, rows []CheckRow) ([][]Hit, error) {
	if len(rows) > maxCheckRows {
		return nil, &service.ValidationError{Field: "rows", Message: "Слишком много строк"}
	}
	index, err := s.loadIndex(ctx)
	if err != nil {
		return nil, err
	}
	result := make([][]Hit, len(rows))
	for number, row := range rows {
		texts := make([]string, 0, len(row.Texts))
		for _, text := range row.Texts {
			if text = service.TruncateRunes(strings.TrimSpace(text), maxCheckText); text != "" && len(texts) < 6 {
				texts = append(texts, text)
			}
		}
		result[number] = index.Match(texts, row.Code)
	}
	return result, nil
}

// LogosToRead — логотипы, которые ИИ ещё не смотрел (большие — уменьшенной копией, см. AILogo).
func (s *Service) LogosToRead(ctx context.Context, limit int) ([]repository.TrademarkLogo, error) {
	logos, err := s.repository.TrademarkLogosToRead(ctx, limit)
	for index := range logos {
		logos[index].Logo = AILogo(logos[index].Logo)
	}
	return logos, err
}

// SaveLogoReadings — прочитанное ИИ на логотипах по номеру: надпись ("" — не прочитал) и что изображено.
func (s *Service) SaveLogoReadings(ctx context.Context, readings map[string]repository.LogoReading) error {
	for regNo, reading := range readings {
		readings[regNo] = repository.LogoReading{
			Name: service.TruncateRunes(cleanText(reading.Name), 300), Picture: service.TruncateRunes(cleanText(reading.Picture), 300),
		}
	}
	if err := s.repository.SaveTrademarkLogoReadings(ctx, readings); err != nil {
		return err
	}
	s.reset()
	return nil
}

// PictureCandidates — логотипы реестра для сверки с фото товара, см. Index.PictureCandidates.
func (s *Service) PictureCandidates(ctx context.Context, pictures, texts []string, code string, exclude map[string]bool, limit int) ([]Hit, error) {
	index, err := s.loadIndex(ctx)
	if err != nil {
		return nil, err
	}
	return index.PictureCandidates(pictures, texts, code, exclude, limit), nil
}

// AILogos — логотипы записей по номеру копиями для ИИ (см. AILogo).
func (s *Service) AILogos(ctx context.Context, regNos []string) (map[string]string, error) {
	rows, err := s.repository.TrademarkLogos(ctx, regNos)
	if err != nil {
		return nil, err
	}
	logos := map[string]string{}
	for _, row := range rows {
		logos[row.RegNo] = AILogo(row.Logo)
	}
	return logos, nil
}

func (s *Service) loadIndex(ctx context.Context) (*Index, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.index != nil {
		return s.index, nil
	}
	rows, err := s.repository.TrademarkRows(ctx)
	if err != nil {
		return nil, err
	}
	records := make([]Record, 0, len(rows))
	for _, row := range rows {
		records = append(records, Record{
			RegNo: row.RegNo, Name: row.Name, LogoName: row.LogoName, LogoPicture: row.LogoPicture, HasLogo: row.HasLogo, Holder: row.Holder,
			Goods: row.Goods, Codes: strings.Fields(row.Codes), ProtectedUntil: row.ProtectedUntil,
		})
	}
	s.index = NewIndex(records)
	return s.index, nil
}

func (s *Service) reset() {
	s.mu.Lock()
	s.index = nil
	s.mu.Unlock()
}

// IsNotFound — записи или логотипа нет.
func IsNotFound(err error) bool { return errors.Is(err, repository.ErrNotFound) }

func nonNil(values []string) []string {
	if values == nil {
		return []string{}
	}
	return values
}
