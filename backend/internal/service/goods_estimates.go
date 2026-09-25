package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
)

// GoodsEstimateService manages «Сметы товаров»: estimates and drafts for any goods,
// stored separately from gloves estimates (table gloves_copy_estimates, API /api/gloves-copy).
// XLSX parsing and the calculation itself are shared with the gloves calculator.
type GoodsEstimateService struct{ repository *repository.Repository }

func NewGoodsEstimateService(repo *repository.Repository) *GoodsEstimateService {
	return &GoodsEstimateService{repository: repo}
}

func (s *GoodsEstimateService) ListEstimates(ctx context.Context, userID int64) ([]GlovesEstimateSummary, error) {
	if s.repository == nil {
		return []GlovesEstimateSummary{}, nil
	}
	rows, err := s.repository.ListGoodsEstimates(ctx, userID)
	if err != nil {
		return nil, err
	}
	owners, err := estimateOwnerNames(ctx, s.repository, userID)
	if err != nil {
		return nil, err
	}
	result := make([]GlovesEstimateSummary, 0, len(rows))
	for _, row := range rows {
		record, decodeErr := decodeGoodsEstimate(row)
		if decodeErr != nil {
			return nil, decodeErr
		}
		record.OwnerName = owners[row.UserID]
		result = append(result, record.GlovesEstimateSummary)
	}
	return result, nil
}

func (s *GoodsEstimateService) CreateEstimate(ctx context.Context, userID int64, input GlovesEstimateInput) (GlovesEstimateSummary, error) {
	if s.repository == nil {
		return GlovesEstimateSummary{}, repository.ErrNotFound
	}
	name, client, date, err := normalizeGlovesEstimateInput(input)
	if err != nil {
		return GlovesEstimateSummary{}, err
	}
	now := time.Now().UTC()
	row, err := s.repository.CreateGoodsEstimate(ctx, repository.GoodsEstimate{
		UserID: userID, Name: name, Client: client, AssessmentDate: date,
		InputSnapshot: repository.JSON([]byte("{}")), ResultSnapshot: repository.JSON([]byte("{}")),
		CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return GlovesEstimateSummary{}, err
	}
	record, err := decodeGoodsEstimate(row)
	if err != nil {
		return GlovesEstimateSummary{}, err
	}
	return record.GlovesEstimateSummary, nil
}

func (s *GoodsEstimateService) GetEstimate(ctx context.Context, id, userID int64) (GlovesEstimateRecord, error) {
	if s.repository == nil {
		return GlovesEstimateRecord{}, repository.ErrNotFound
	}
	row, err := s.repository.GoodsEstimateByIDForUser(ctx, id, userID)
	if err != nil {
		return GlovesEstimateRecord{}, err
	}
	return decodeGoodsEstimate(row)
}

func (s *GoodsEstimateService) UpdateEstimate(ctx context.Context, id, userID int64, input GlovesEstimateInput) (GlovesEstimateSummary, error) {
	if s.repository == nil {
		return GlovesEstimateSummary{}, repository.ErrNotFound
	}
	name, client, date, err := normalizeGlovesEstimateInput(input)
	if err != nil {
		return GlovesEstimateSummary{}, err
	}
	row, err := s.repository.UpdateGoodsEstimate(ctx, repository.GoodsEstimate{
		ID: id, UserID: userID, Name: name, Client: client, AssessmentDate: date, UpdatedAt: time.Now().UTC(),
	})
	if err != nil {
		return GlovesEstimateSummary{}, err
	}
	record, err := decodeGoodsEstimate(row)
	if err != nil {
		return GlovesEstimateSummary{}, err
	}
	return record.GlovesEstimateSummary, nil
}

func (s *GoodsEstimateService) DeleteEstimate(ctx context.Context, id, userID int64) error {
	if s.repository == nil {
		return repository.ErrNotFound
	}
	return s.repository.DeleteGoodsEstimate(ctx, id, userID)
}

func (s *GoodsEstimateService) SaveEstimateDraft(ctx context.Context, id, userID int64, draft GlovesDraft) (GlovesEstimateRecord, error) {
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
	row, err := s.repository.SaveGoodsEstimateDraft(ctx, id, userID, repository.JSON(inputData), repository.JSON(resultData), date, time.Now().UTC())
	if err != nil {
		return GlovesEstimateRecord{}, err
	}
	return decodeGoodsEstimate(row)
}

func decodeGoodsEstimate(row repository.GoodsEstimate) (GlovesEstimateRecord, error) {
	var input GlovesCalculationInput
	if len(row.InputSnapshot) > 0 && string(row.InputSnapshot) != "{}" && string(row.InputSnapshot) != "null" {
		if err := json.Unmarshal(row.InputSnapshot, &input); err != nil {
			return GlovesEstimateRecord{}, fmt.Errorf("сохранённая смета повреждена")
		}
	}
	var result *GlovesCalculationResult
	if len(row.ResultSnapshot) > 0 && string(row.ResultSnapshot) != "{}" && string(row.ResultSnapshot) != "null" {
		var decoded GlovesCalculationResult
		if err := json.Unmarshal(row.ResultSnapshot, &decoded); err != nil {
			return GlovesEstimateRecord{}, fmt.Errorf("сохранённый результат сметы повреждён")
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
