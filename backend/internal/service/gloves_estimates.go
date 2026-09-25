package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
)

type GlovesService struct{ repository *repository.Repository }

// NewGlovesService accepts the repository optionally to keep the service easy
// to construct in isolated tests. In the running server it is always supplied
// so directory selections are re-read and verified on the server.
func NewGlovesService(repositories ...*repository.Repository) *GlovesService {
	var repo *repository.Repository
	if len(repositories) > 0 {
		repo = repositories[0]
	}
	return &GlovesService{repository: repo}
}

func (s *GlovesService) ListEstimates(ctx context.Context, userID int64) ([]GlovesEstimateSummary, error) {
	if s.repository == nil {
		return []GlovesEstimateSummary{}, nil
	}
	rows, err := s.repository.ListGlovesEstimates(ctx, userID)
	if err != nil {
		return nil, err
	}
	owners, err := estimateOwnerNames(ctx, s.repository, userID)
	if err != nil {
		return nil, err
	}
	result := make([]GlovesEstimateSummary, 0, len(rows))
	for _, row := range rows {
		record, decodeErr := decodeGlovesEstimate(row)
		if decodeErr != nil {
			return nil, decodeErr
		}
		record.OwnerName = owners[row.UserID]
		result = append(result, record.GlovesEstimateSummary)
	}
	return result, nil
}

func (s *GlovesService) CreateEstimate(ctx context.Context, userID int64, input GlovesEstimateInput) (GlovesEstimateSummary, error) {
	if s.repository == nil {
		return GlovesEstimateSummary{}, repository.ErrNotFound
	}
	name, client, date, err := normalizeGlovesEstimateInput(input)
	if err != nil {
		return GlovesEstimateSummary{}, err
	}
	now := time.Now().UTC()
	row, err := s.repository.CreateGlovesEstimate(ctx, repository.GlovesEstimate{
		UserID: userID, Name: name, Client: client, AssessmentDate: date,
		InputSnapshot: repository.JSON([]byte("{}")), ResultSnapshot: repository.JSON([]byte("{}")),
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return GlovesEstimateSummary{}, err
	}
	record, err := decodeGlovesEstimate(row)
	if err != nil {
		return GlovesEstimateSummary{}, err
	}
	return record.GlovesEstimateSummary, nil
}

func (s *GlovesService) GetEstimate(ctx context.Context, id, userID int64) (GlovesEstimateRecord, error) {
	if s.repository == nil {
		return GlovesEstimateRecord{}, repository.ErrNotFound
	}
	row, err := s.repository.GlovesEstimateByIDForUser(ctx, id, userID)
	if err != nil {
		return GlovesEstimateRecord{}, err
	}
	return decodeGlovesEstimate(row)
}

func (s *GlovesService) UpdateEstimate(ctx context.Context, id, userID int64, input GlovesEstimateInput) (GlovesEstimateSummary, error) {
	if s.repository == nil {
		return GlovesEstimateSummary{}, repository.ErrNotFound
	}
	name, client, date, err := normalizeGlovesEstimateInput(input)
	if err != nil {
		return GlovesEstimateSummary{}, err
	}
	row, err := s.repository.UpdateGlovesEstimate(ctx, repository.GlovesEstimate{ID: id, UserID: userID, Name: name, Client: client, AssessmentDate: date, UpdatedAt: time.Now().UTC()})
	if err != nil {
		return GlovesEstimateSummary{}, err
	}
	record, err := decodeGlovesEstimate(row)
	if err != nil {
		return GlovesEstimateSummary{}, err
	}
	return record.GlovesEstimateSummary, nil
}

func (s *GlovesService) DeleteEstimate(ctx context.Context, id, userID int64) error {
	if s.repository == nil {
		return repository.ErrNotFound
	}
	return s.repository.DeleteGlovesEstimate(ctx, id, userID)
}

func (s *GlovesService) SaveEstimateDraft(ctx context.Context, id, userID int64, draft GlovesDraft) (GlovesEstimateRecord, error) {
	if s.repository == nil {
		return GlovesEstimateRecord{}, repository.ErrNotFound
	}
	if userID < 1 && userID != repository.AllUsers {
		return GlovesEstimateRecord{}, &ValidationError{Field: "draft", Message: "Пользователь не определён"}
	}
	if err := validateGlovesDraftItems(draft.Input.Items); err != nil {
		return GlovesEstimateRecord{}, err
	}
	date, err := parseGlovesEstimateDate(draft.Input.AssessmentDate)
	if err != nil {
		return GlovesEstimateRecord{}, err
	}
	inputData, err := json.Marshal(draft.Input)
	if err != nil {
		return GlovesEstimateRecord{}, fmt.Errorf("не удалось сохранить черновик")
	}
	resultData := []byte("{}")
	if draft.Result != nil {
		resultData, err = json.Marshal(draft.Result)
		if err != nil {
			return GlovesEstimateRecord{}, fmt.Errorf("не удалось сохранить результат черновика")
		}
	}
	row, err := s.repository.SaveGlovesEstimateDraft(ctx, id, userID, repository.JSON(inputData), repository.JSON(resultData), date, time.Now().UTC())
	if err != nil {
		return GlovesEstimateRecord{}, err
	}
	return decodeGlovesEstimate(row)
}

func decodeGlovesEstimate(row repository.GlovesEstimate) (GlovesEstimateRecord, error) {
	var input GlovesCalculationInput
	if len(row.InputSnapshot) > 0 && string(row.InputSnapshot) != "{}" && string(row.InputSnapshot) != "null" {
		if err := json.Unmarshal(row.InputSnapshot, &input); err != nil {
			return GlovesEstimateRecord{}, fmt.Errorf("сохранённая смета перчаток повреждена")
		}
	}
	var result *GlovesCalculationResult
	if len(row.ResultSnapshot) > 0 && string(row.ResultSnapshot) != "{}" && string(row.ResultSnapshot) != "null" {
		var decoded GlovesCalculationResult
		if err := json.Unmarshal(row.ResultSnapshot, &decoded); err != nil {
			return GlovesEstimateRecord{}, fmt.Errorf("сохраненный результат сметы перчаток повреждён")
		}
		result = &decoded
	}
	status := "Черновик"
	if result != nil {
		status = "Рассчитано"
	}
	return GlovesEstimateRecord{
		GlovesEstimateSummary: GlovesEstimateSummary{
			ID: row.ID, Name: row.Name, Client: row.Client, AssessmentDate: row.AssessmentDate.Format("2006-01-02"),
			Status: status, CreatedAt: row.CreatedAt.UTC().Format(time.RFC3339), UpdatedAt: row.UpdatedAt.UTC().Format(time.RFC3339),
		},
		Input: input, Result: result,
	}, nil
}

func normalizeGlovesEstimateInput(input GlovesEstimateInput) (string, string, time.Time, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return "", "", time.Time{}, &ValidationError{Field: "name", Message: "Название сметы обязательно"}
	}
	if len([]rune(name)) > 200 {
		return "", "", time.Time{}, &ValidationError{Field: "name", Message: "Название не должно быть длиннее 200 символов"}
	}
	client := strings.TrimSpace(input.Client)
	if len([]rune(client)) > 200 {
		return "", "", time.Time{}, &ValidationError{Field: "client", Message: "Клиент не должен быть длиннее 200 символов"}
	}
	date, err := parseGlovesEstimateDate(input.AssessmentDate)
	if err != nil {
		return "", "", time.Time{}, err
	}
	return name, client, date, nil
}

func parseGlovesEstimateDate(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Now().UTC().Truncate(24 * time.Hour), nil
	}
	date, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, &ValidationError{Field: "assessmentDate", Message: "Укажите дату в формате ГГГГ-ММ-ДД"}
	}
	return date, nil
}

func validateGlovesDraftItems(items []GlovesItemInput) error {
	if len(items) > MaxRowsPerSheet {
		return &ValidationError{Field: "items", Message: fmt.Sprintf("В черновике не может быть больше %d строк", MaxRowsPerSheet)}
	}
	for index, item := range items {
		photos := append([]string{}, item.PhotoURLs...)
		if item.PhotoURL != "" {
			photos = append(photos, item.PhotoURL)
		}
		for _, photo := range photos {
			if err := validateGlovesPhotoData(photo); err != nil {
				return &ValidationError{Field: "items", Message: fmt.Sprintf("Строка %d: %s", index+1, err.Error())}
			}
		}
	}
	return nil
}

func validateGlovesPhotoData(value string) error {
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
