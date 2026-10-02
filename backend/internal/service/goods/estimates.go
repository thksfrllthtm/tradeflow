package goods

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
)

// GoodsEstimateService manages «Сметы товаров»: estimates and drafts for any goods
// (table goods_estimates, API /api/goods-estimates; row photos in goods_estimate_photos).
// XLSX parsing and the calculation itself live in GoodsService (/api/goods).
type GoodsEstimateService struct{ repository *repository.Repository }

func NewGoodsEstimateService(repo *repository.Repository) *GoodsEstimateService {
	return &GoodsEstimateService{repository: repo}
}

func (s *GoodsEstimateService) CreateEstimate(ctx context.Context, userID int64, input GoodsEstimateInput) (GoodsEstimateSummary, error) {
	if s.repository == nil {
		return GoodsEstimateSummary{}, repository.ErrNotFound
	}
	name, client, date, err := normalizeGoodsEstimateInput(input)
	if err != nil {
		return GoodsEstimateSummary{}, err
	}
	now := time.Now().UTC()
	row, err := s.repository.CreateGoodsEstimate(ctx, repository.GoodsEstimate{
		UserID: userID, Name: name, Client: client, AssessmentDate: date,
		InputSnapshot: repository.JSON([]byte("{}")), ResultSnapshot: repository.JSON([]byte("{}")),
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return GoodsEstimateSummary{}, err
	}
	record, err := decodeGoodsEstimate(row)
	if err != nil {
		return GoodsEstimateSummary{}, err
	}
	return record.GoodsEstimateSummary, nil
}

func (s *GoodsEstimateService) UpdateEstimate(ctx context.Context, id, userID int64, input GoodsEstimateInput) (GoodsEstimateSummary, error) {
	if s.repository == nil {
		return GoodsEstimateSummary{}, repository.ErrNotFound
	}
	name, client, date, err := normalizeGoodsEstimateInput(input)
	if err != nil {
		return GoodsEstimateSummary{}, err
	}
	row, err := s.repository.UpdateGoodsEstimate(ctx, repository.GoodsEstimate{
		ID: id, UserID: userID, Name: name, Client: client, AssessmentDate: date, UpdatedAt: time.Now().UTC(),
	})
	if err != nil {
		return GoodsEstimateSummary{}, err
	}
	record, err := decodeGoodsEstimate(row)
	if err != nil {
		return GoodsEstimateSummary{}, err
	}
	return record.GoodsEstimateSummary, nil
}

func (s *GoodsEstimateService) DeleteEstimate(ctx context.Context, id, userID int64) error {
	if s.repository == nil {
		return repository.ErrNotFound
	}
	return s.repository.DeleteGoodsEstimate(ctx, id, userID)
}

func decodeGoodsEstimate(row repository.GoodsEstimate) (GoodsEstimateRecord, error) {
	var input GoodsCalculationInput
	if len(row.InputSnapshot) > 0 && string(row.InputSnapshot) != "{}" && string(row.InputSnapshot) != "null" {
		if err := json.Unmarshal(row.InputSnapshot, &input); err != nil {
			return GoodsEstimateRecord{}, fmt.Errorf("сохранённая смета повреждена")
		}
	}
	var result *GoodsCalculationResult
	if len(row.ResultSnapshot) > 0 && string(row.ResultSnapshot) != "{}" && string(row.ResultSnapshot) != "null" {
		var decoded GoodsCalculationResult
		if err := json.Unmarshal(row.ResultSnapshot, &decoded); err != nil {
			return GoodsEstimateRecord{}, fmt.Errorf("сохранённый результат сметы повреждён")
		}
		result = &decoded
	}
	status := "Черновик"
	if result != nil {
		status = "Рассчитано"
	}
	return GoodsEstimateRecord{
		GoodsEstimateSummary: GoodsEstimateSummary{
			ID: row.ID, Name: row.Name, Client: row.Client, AssessmentDate: row.AssessmentDate.Format("2006-01-02"),
			Status: status, CreatedAt: row.CreatedAt.UTC().Format(time.RFC3339), UpdatedAt: row.UpdatedAt.UTC().Format(time.RFC3339),
		},
		Input: input, Result: result,
	}, nil
}

func normalizeGoodsEstimateInput(input GoodsEstimateInput) (string, string, time.Time, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return "", "", time.Time{}, &service.ValidationError{Field: "name", Message: "Название сметы обязательно"}
	}
	if len([]rune(name)) > 200 {
		return "", "", time.Time{}, &service.ValidationError{Field: "name", Message: "Название не должно быть длиннее 200 символов"}
	}
	client := strings.TrimSpace(input.Client)
	if len([]rune(client)) > 200 {
		return "", "", time.Time{}, &service.ValidationError{Field: "client", Message: "Клиент не должен быть длиннее 200 символов"}
	}
	date, err := parseGoodsEstimateDate(input.AssessmentDate)
	if err != nil {
		return "", "", time.Time{}, err
	}
	return name, client, date, nil
}

func parseGoodsEstimateDate(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Now().UTC().Truncate(24 * time.Hour), nil
	}
	date, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, &service.ValidationError{Field: "assessmentDate", Message: "Укажите дату в формате ГГГГ-ММ-ДД"}
	}
	return date, nil
}

func validateGoodsPhotoData(value string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	comma := strings.IndexByte(value, ',')
	if !strings.HasPrefix(strings.ToLower(value), "data:image/") || comma < 0 {
		return fmt.Errorf("фото должно быть встроенным JPG или PNG")
	}
	header := strings.ToLower(value[:comma])
	if !(strings.HasPrefix(header, "data:image/jpeg;base64") || strings.HasPrefix(header, "data:image/png;base64")) {
		return fmt.Errorf("поддерживаются только JPG и PNG")
	}
	decoded, err := base64.StdEncoding.DecodeString(value[comma+1:])
	if err != nil {
		return fmt.Errorf("фото повреждено")
	}
	if len(decoded) > MaxImageBytes {
		return fmt.Errorf("фото больше %d МБ", MaxImageBytes/(1024*1024))
	}
	return nil
}

// estimateOwnerNames returns employee names for the administrator's list of
// all estimates; for an ordinary employee's own list it returns nothing.
func estimateOwnerNames(ctx context.Context, repo *repository.Repository, userID int64) (map[int64]string, error) {
	if userID != repository.AllUsers {
		return nil, nil
	}
	users, err := repo.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	names := make(map[int64]string, len(users))
	for _, user := range users {
		names[user.ID] = user.Name
		if names[user.ID] == "" {
			names[user.ID] = user.Email
		}
	}
	return names, nil
}
