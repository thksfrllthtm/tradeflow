package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/shopspring/decimal"
)

type EstimateInput struct {
	Name           string `json:"name"`
	Client         string `json:"client"`
	AssessmentDate string `json:"assessmentDate"`
	Currency       string `json:"currency"`
	ExchangeRate   string `json:"exchangeRate"`
	Origin         string `json:"origin"`
	DeliveryTerms  string `json:"deliveryTerms"`
	Note           string `json:"note"`
}

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

type EstimateService struct {
	repository *repository.Repository
	files      *repository.FileStore
}

func NewEstimateService(repo *repository.Repository, files ...*repository.FileStore) *EstimateService {
	var store *repository.FileStore
	if len(files) > 0 {
		store = files[0]
	}
	return &EstimateService{repository: repo, files: store}
}

func (s *EstimateService) List(ctx context.Context, ownerID int64) ([]repository.Estimate, error) {
	return s.repository.ListEstimates(ctx, ownerID)
}

func (s *EstimateService) Get(ctx context.Context, id, ownerID int64) (repository.Estimate, error) {
	return s.repository.EstimateByIDForOwner(ctx, id, ownerID)
}

func (s *EstimateService) Create(ctx context.Context, ownerID int64, input EstimateInput) (repository.Estimate, error) {
	estimate, err := estimateFromInput(input)
	if err != nil {
		return repository.Estimate{}, err
	}
	estimate.OwnerID = ownerID
	estimate.CreatedBy = ownerID
	estimate.UpdatedBy = ownerID
	estimate.ImportCountry = "KZ"
	estimate.Status = "draft"
	return s.repository.CreateEstimate(ctx, estimate)
}

func (s *EstimateService) Update(ctx context.Context, id, ownerID int64, input EstimateInput) (repository.Estimate, error) {
	estimate, err := estimateFromInput(input)
	if err != nil {
		return repository.Estimate{}, err
	}
	estimate.ID = id
	estimate.OwnerID = ownerID
	estimate.UpdatedBy = ownerID
	return s.repository.UpdateEstimate(ctx, estimate)
}

func (s *EstimateService) Delete(ctx context.Context, id, ownerID int64) error {
	paths, err := s.repository.DeleteEstimateForOwner(ctx, id, ownerID)
	if err != nil {
		return err
	}
	if s.files != nil {
		for _, path := range paths {
			// Database deletion is already committed. A stale private file is
			// harmless if cleanup fails, while the estimate must stay deleted.
			_ = s.files.Remove(path)
		}
	}
	return nil
}

func estimateFromInput(input EstimateInput) (repository.Estimate, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return repository.Estimate{}, &ValidationError{Field: "name", Message: "Название сметы обязательно"}
	}
	if len([]rune(name)) > 200 {
		return repository.Estimate{}, &ValidationError{Field: "name", Message: "Название не должно быть длиннее 200 символов"}
	}

	date, err := time.Parse("2006-01-02", input.AssessmentDate)
	if err != nil {
		return repository.Estimate{}, &ValidationError{Field: "assessmentDate", Message: "Укажите дату оценки в формате ГГГГ-ММ-ДД"}
	}

	currency := strings.ToUpper(strings.TrimSpace(input.Currency))
	if len(currency) != 3 || !isLatinUppercase(currency) {
		return repository.Estimate{}, &ValidationError{Field: "currency", Message: "Валюта должна быть трёхбуквенным кодом, например USD или KZT"}
	}

	rate, err := decimal.NewFromString(strings.TrimSpace(input.ExchangeRate))
	if err != nil || !rate.IsPositive() {
		return repository.Estimate{}, &ValidationError{Field: "exchangeRate", Message: "Курс должен быть положительным числом"}
	}
	if currency == "KZT" && !rate.Equal(decimal.NewFromInt(1)) {
		return repository.Estimate{}, &ValidationError{Field: "exchangeRate", Message: "Для KZT курс должен быть равен 1"}
	}

	client, err := optionalText("client", input.Client, 200)
	if err != nil {
		return repository.Estimate{}, err
	}
	origin, err := optionalText("origin", input.Origin, 200)
	if err != nil {
		return repository.Estimate{}, err
	}
	deliveryTerms, err := optionalText("deliveryTerms", input.DeliveryTerms, 100)
	if err != nil {
		return repository.Estimate{}, err
	}
	note := strings.TrimSpace(input.Note)

	return repository.Estimate{
		Name:           name,
		Client:         client,
		AssessmentDate: date,
		Currency:       currency,
		ExchangeRate:   rate,
		Origin:         origin,
		DeliveryTerms:  deliveryTerms,
		Note:           note,
	}, nil
}

func optionalText(field, value string, limit int) (string, error) {
	trimmed := strings.TrimSpace(value)
	if len([]rune(trimmed)) > limit {
		return "", &ValidationError{Field: field, Message: fmt.Sprintf("Поле не должно быть длиннее %d символов", limit)}
	}
	return trimmed, nil
}

func isLatinUppercase(value string) bool {
	for _, char := range value {
		if char < 'A' || char > 'Z' {
			return false
		}
	}
	return true
}

func IsNotFound(err error) bool {
	return errors.Is(err, repository.ErrNotFound)
}
