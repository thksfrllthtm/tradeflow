package decisions

// База решений: проверенные примеры «товар из инвойса → код ТН ВЭД». Запись создаёт любой
// сотрудник (из подбора агентом или вручную) со статусом «Не проверено»; проверяет, исправляет,
// отклоняет и импортирует старые инвойсы только администратор. Исправление кода агента
// отмечается — по нему считается точность агента.

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/directory"
)

const (
	maxDecisionPhotos     = 3
	maxDecisionPhotoBytes = 2 << 20
	maxDecisionThumbBytes = 120 << 10
	maxDecisionImportRows = 200
	maxDecisionTextRunes  = 2000
	maxDecisionNameRunes  = 500
)

var tnvedCodePattern = regexp.MustCompile(`^\d{10}$`)

type CodeDecisionService struct {
	repository *repository.Repository
	// embedder — модель эмбеддингов для поиска по смыслу; nil — только по словам.
	embedder Embedder
}

func NewCodeDecisionService(r *repository.Repository) *CodeDecisionService {
	return &CodeDecisionService{repository: r}
}

// SetEmbedder включает поиск по смыслу (похожие по фото и поиск текстом в базе решений).
func (s *CodeDecisionService) SetEmbedder(embedder Embedder) { s.embedder = embedder }

type RejectedCode struct {
	Code string `json:"code"`
	Why  string `json:"why"`
}

type CodeDecisionView struct {
	ID                 int64                       `json:"id"`
	Status             string                      `json:"status"`
	Source             string                      `json:"source"`
	InvoiceName        string                      `json:"invoiceName"`
	SupplierName       string                      `json:"supplierName"`
	FinalName          string                      `json:"finalName"`
	Code               string                      `json:"code"`
	Reason             string                      `json:"reason"`
	Quote              string                      `json:"quote"`
	SourceURL          string                      `json:"sourceUrl"`
	RejectedCodes      []RejectedCode              `json:"rejectedCodes"`
	UnitNetKg          string                      `json:"unitNetKg"`
	UnitPriceUSD       string                      `json:"unitPriceUsd"`
	Photos             []string                    `json:"photos"`
	Thumb              string                      `json:"thumb"`
	AgentCode          string                      `json:"agentCode"`
	AgentCorrect       *bool                       `json:"agentCorrect"`
	EstimateID         *int64                      `json:"estimateId,omitempty"`
	ItemID             string                      `json:"itemId,omitempty"`
	SessionID          *int64                      `json:"sessionId,omitempty"`
	ProductDirectoryID *int64                      `json:"productDirectoryId,omitempty"`
	Note               string                      `json:"note"`
	CreatedByName      string                      `json:"createdByName"`
	VerifiedByName     string                      `json:"verifiedByName"`
	VerifiedAt         *time.Time                  `json:"verifiedAt"`
	CreatedAt          time.Time                   `json:"createdAt"`
	UpdatedAt          time.Time                   `json:"updatedAt"`
	Nomenclature       *directory.NomenclatureCode `json:"nomenclature"`
	// Описание фото от ИИ одной строкой; пусто — ещё не составлено.
	PhotoDescription string `json:"photoDescription"`
	// То же по полям — для карточки записи; nil — описания нет.
	PhotoFeatures *PhotoFeatures `json:"photoFeatures,omitempty"`
	// Duplicate — этот товар сметы уже был в базе: возвращена прежняя запись, новая не создана.
	Duplicate bool `json:"duplicate,omitempty"`
}

type CodeDecisionInput struct {
	InvoiceName        string         `json:"invoiceName"`
	SupplierName       string         `json:"supplierName"`
	FinalName          string         `json:"finalName"`
	Code               string         `json:"code"`
	Reason             string         `json:"reason"`
	Quote              string         `json:"quote"`
	SourceURL          string         `json:"sourceUrl"`
	RejectedCodes      []RejectedCode `json:"rejectedCodes"`
	UnitNetKg          string         `json:"unitNetKg"`
	UnitPriceUSD       string         `json:"unitPriceUsd"`
	Photos             []string       `json:"photos"`
	Thumb              string         `json:"thumb"`
	AgentCode          string         `json:"agentCode"`
	EstimateID         *int64         `json:"estimateId"`
	ItemID             string         `json:"itemId"`
	SessionID          *int64         `json:"sessionId"`
	ProductDirectoryID *int64         `json:"productDirectoryId"`
	Note               string         `json:"note"`
}

// CodeDecisionEdit — правка администратора; nil-поля не меняются.
type CodeDecisionEdit struct {
	Code         *string `json:"code"`
	FinalName    *string `json:"finalName"`
	InvoiceName  *string `json:"invoiceName"`
	SupplierName *string `json:"supplierName"`
	Reason       *string `json:"reason"`
	Note         *string `json:"note"`
	// Цена за 1 шт, USD; пустая строка стирает цену.
	UnitPriceUSD       *string `json:"unitPriceUsd"`
	ProductDirectoryID *int64  `json:"productDirectoryId"`
}

type CodeDecisionStats struct {
	Pending  int64 `json:"pending"`
	Verified int64 `json:"verified"`
	Rejected int64 `json:"rejected"`
	// Точность агента по решениям, проверенным за 90 дней.
	AgentChecked int64 `json:"agentChecked"`
	AgentCorrect int64 `json:"agentCorrect"`
}

func (s *CodeDecisionService) Stats(ctx context.Context) (CodeDecisionStats, error) {
	stats, err := s.repository.CodeDecisionStats(ctx)
	return CodeDecisionStats(stats), err
}

func (s *CodeDecisionService) List(ctx context.Context, filter repository.CodeDecisionFilter) ([]CodeDecisionView, int64, error) {
	if filter.Limit <= 0 || filter.Limit > 200 {
		filter.Limit = 50
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	switch filter.Status {
	case "", "pending", "verified", "rejected":
	default:
		filter.Status = ""
	}
	// Текстовый запрос — ещё и по смыслу описания фото: «ключ для труб» найдёт «газовый ключ».
	if text := strings.TrimSpace(filter.Query); strings.ContainsFunc(text, unicode.IsLetter) {
		for id := range s.nearestByMeaning(ctx, text) {
			filter.AlsoIDs = append(filter.AlsoIDs, id)
		}
	}
	rows, total, err := s.repository.ListCodeDecisions(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	views, err := s.views(ctx, rows)
	return views, total, err
}

func (s *CodeDecisionService) Get(ctx context.Context, id int64) (CodeDecisionView, error) {
	row, err := s.repository.CodeDecisionByID(ctx, id)
	if err != nil {
		return CodeDecisionView{}, err
	}
	views, err := s.views(ctx, []repository.CodeDecision{row})
	if err != nil {
		return CodeDecisionView{}, err
	}
	return views[0], nil
}

// views — записи для ответа вместе с текущим состоянием кода в номенклатуре.
func (s *CodeDecisionService) views(ctx context.Context, rows []repository.CodeDecision) ([]CodeDecisionView, error) {
	codes := make([]string, 0, len(rows))
	for _, row := range rows {
		codes = append(codes, row.Code)
	}
	found, err := directory.NewTNVEDNomenclatureService(s.repository).Lookup(ctx, codes)
	if err != nil {
		return nil, err
	}
	byCode := make(map[string]*directory.NomenclatureCode, len(found))
	for index := range found {
		byCode[found[index].Code] = &found[index]
	}
	views := make([]CodeDecisionView, 0, len(rows))
	for _, row := range rows {
		view := CodeDecisionView{
			ID: row.ID, Status: row.Status, Source: row.Source, InvoiceName: row.InvoiceName, SupplierName: row.SupplierName,
			FinalName: row.FinalName, Code: row.Code, Reason: row.Reason, Quote: row.Quote, SourceURL: row.SourceURL,
			UnitNetKg: service.DecimalText(row.UnitNetKg), UnitPriceUSD: service.DecimalText(row.UnitPriceUSD), Thumb: row.Thumb,
			AgentCode: row.AgentCode, AgentCorrect: row.AgentCorrect, EstimateID: row.EstimateID, ItemID: row.ItemID,
			SessionID: row.SessionID, ProductDirectoryID: row.ProductDirectoryID, Note: row.Note,
			CreatedByName: row.CreatedByName, VerifiedByName: row.VerifiedByName, VerifiedAt: row.VerifiedAt,
			CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, Nomenclature: byCode[row.Code],
			RejectedCodes: []RejectedCode{}, Photos: []string{},
		}
		if features, ok := ParsePhotoFeatures(row.PhotoFeatures); ok {
			view.PhotoDescription = features.Summary()
			view.PhotoFeatures = &features
		}
		_ = json.Unmarshal(row.RejectedCodes, &view.RejectedCodes)
		_ = json.Unmarshal(row.Photos, &view.Photos)
		if view.RejectedCodes == nil {
			view.RejectedCodes = []RejectedCode{}
		}
		if view.Photos == nil {
			view.Photos = []string{}
		}
		views = append(views, view)
	}
	return views, nil
}
