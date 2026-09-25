package service

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
)

// GoodsEstimateService manages «Сметы товаров»: estimates and drafts for any goods
// (table gloves_copy_estimates, API /api/gloves-copy; row photos in goods_estimate_photos).
// XLSX parsing and the calculation itself live in the gloves service (/api/gloves).
type GoodsEstimateService struct{ repository *repository.Repository }

func NewGoodsEstimateService(repo *repository.Repository) *GoodsEstimateService {
	return &GoodsEstimateService{repository: repo}
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
