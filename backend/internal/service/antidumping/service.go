package antidumping

// Хранение мер в базе: начальная загрузка из файла программы (реестр ЕЭК на дату выпуска) и
// обновление с сайта ЕЭК по кнопке администратора.

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/shopspring/decimal"
)

//go:embed eec_registry_seed.json
var seedRegistry []byte

// RegistryURL — открытый реестр мер ЕЭК (тот же запрос делает страница remedies.eaeunion.org).
const (
	RegistryURL     = "https://remedies.eaeunion.org/spd2/find?collection=zvr.v_actionsregistry&limit=500&skip=0"
	RegistryPageURL = "https://remedies.eaeunion.org/dimd/ru/security/actions"
	maxRegistrySize = 20 << 20
)

type Service struct {
	repository *repository.Repository
	client     *http.Client
}

func NewService(r *repository.Repository) *Service {
	return &Service{repository: r, client: &http.Client{Timeout: 60 * time.Second}}
}

// ImportInfo — откуда и когда загружены меры.
type ImportInfo struct {
	Source         string `json:"source"`
	MeasureCount   int    `json:"measureCount"`
	ImportedAt     string `json:"importedAt"`
	ImportedByName string `json:"importedByName"`
	RegistryURL    string `json:"registryUrl"`
}

// SeedIfEmpty загружает меры из файла программы, если их ещё ни разу не загружали.
func (s *Service) SeedIfEmpty(ctx context.Context) error {
	if _, err := s.repository.LatestAntiDumpingImport(ctx); err == nil || !service.IsNotFound(err) {
		return err
	}
	measures, err := ParseRegistry(seedRegistry)
	if err != nil {
		return err
	}
	return s.save(ctx, measures, "seed", nil)
}

// Refresh скачивает действующие меры с сайта ЕЭК и заменяет ими сохранённые.
func (s *Service) Refresh(ctx context.Context, userID int64) (ImportInfo, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, RegistryURL, strings.NewReader("{}"))
	if err != nil {
		return ImportInfo{}, err
	}
	// Сервис реестра принимает тело запроса только как text/plain.
	request.Header.Set("Content-Type", "text/plain")
	request.Header.Set("Accept", "application/json")
	response, err := s.client.Do(request)
	if err != nil {
		return ImportInfo{}, &service.ValidationError{Field: "registry", Message: "Сайт реестра ЕЭК недоступен — попробуйте позже"}
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, maxRegistrySize))
	if err != nil || response.StatusCode != http.StatusOK || len(bytes.TrimSpace(body)) == 0 {
		return ImportInfo{}, &service.ValidationError{Field: "registry", Message: fmt.Sprintf("Реестр ЕЭК не ответил (код %d) — попробуйте позже", response.StatusCode)}
	}
	measures, err := ParseRegistry(body)
	if err != nil {
		return ImportInfo{}, &service.ValidationError{Field: "registry", Message: err.Error()}
	}
	if err := s.save(ctx, measures, "eec", &userID); err != nil {
		return ImportInfo{}, err
	}
	info, err := s.LatestImport(ctx)
	if err != nil || info == nil {
		return ImportInfo{}, err
	}
	return *info, nil
}

func (s *Service) save(ctx context.Context, measures []Measure, source string, userID *int64) error {
	now := time.Now().UTC()
	rows := make([]repository.TNVEDAntiDumpingMeasure, 0, len(measures))
	for _, measure := range measures {
		codes, _ := json.Marshal(measure.Codes)
		countries, _ := json.Marshal(measure.Countries)
		rates, _ := json.Marshal(measure.Rates)
		rows = append(rows, repository.TNVEDAntiDumpingMeasure{
			Number: measure.Number, MeasureType: measure.Type, ShortName: measure.ShortName, Description: measure.Description,
			Codes: repository.JSON(codes), Countries: repository.JSON(countries), Rates: repository.JSON(rates), RateText: measure.RateText,
			MinPercent: optionalDecimal(measure.MinPercent), MaxPercent: optionalDecimal(measure.MaxPercent),
			StartDate: optionalDate(measure.StartDate), EndDate: optionalDate(measure.EndDate), Suspended: measure.Suspended,
			NeedsCheck: measure.NeedsCheck, Foundation: measure.Foundation, DocumentURL: measure.DocumentURL, Note: measure.Note, UpdatedAt: now,
		})
	}
	return s.repository.ReplaceAntiDumpingMeasures(ctx, rows, repository.TNVEDAntiDumpingImport{Source: source, MeasureCount: len(rows), ImportedBy: userID, ImportedAt: now})
}

func optionalDecimal(value string) *decimal.Decimal {
	parsed, err := decimal.NewFromString(value)
	if err != nil {
		return nil
	}
	return &parsed
}

func optionalDate(value string) *time.Time {
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return nil
	}
	return &parsed
}

// Measures — сохранённые меры (для расчёта и номенклатуры).
func (s *Service) Measures(ctx context.Context) ([]Measure, error) {
	rows, err := s.repository.AntiDumpingMeasures(ctx)
	if err != nil {
		return nil, err
	}
	measures := make([]Measure, 0, len(rows))
	for _, row := range rows {
		measure := Measure{
			Number: row.Number, Type: row.MeasureType, ShortName: row.ShortName, Description: row.Description, RateText: row.RateText,
			Suspended: row.Suspended, NeedsCheck: row.NeedsCheck, Foundation: row.Foundation, DocumentURL: row.DocumentURL, Note: row.Note,
			MinPercent: service.DecimalText(row.MinPercent), MaxPercent: service.DecimalText(row.MaxPercent),
		}
		_ = json.Unmarshal(row.Codes, &measure.Codes)
		_ = json.Unmarshal(row.Countries, &measure.Countries)
		_ = json.Unmarshal(row.Rates, &measure.Rates)
		if row.StartDate != nil {
			measure.StartDate = row.StartDate.Format("2006-01-02")
		}
		if row.EndDate != nil {
			measure.EndDate = row.EndDate.Format("2006-01-02")
		}
		measures = append(measures, measure)
	}
	// По номеру меры: AD-1, AD-3 … AD-41.
	sort.SliceStable(measures, func(i, j int) bool { return measureOrder(measures[i].Number) < measureOrder(measures[j].Number) })
	return measures, nil
}

func measureOrder(number string) int {
	value, _ := strconv.Atoi(digits(number))
	return value
}

// LatestImport — nil, если меры не загружались.
func (s *Service) LatestImport(ctx context.Context) (*ImportInfo, error) {
	record, err := s.repository.LatestAntiDumpingImport(ctx)
	if service.IsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &ImportInfo{Source: record.Source, MeasureCount: record.MeasureCount, ImportedAt: record.ImportedAt.Format(time.RFC3339), ImportedByName: record.ImportedByName, RegistryURL: RegistryPageURL}, nil
}
