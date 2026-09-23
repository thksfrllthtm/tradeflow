package service

// AIService: TN VED classification of imported products through the configured AI provider.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
)

type AIService struct {
	repository *repository.Repository
	client     aiClient
	chatMu     sync.Mutex
	chatLocks  map[int64]*sync.Mutex
}

// NewAIServiceWithProvider selects the configured server-side AI provider.
// Gemini remains available for backwards compatibility; Anthropic is the
// default for new installations.
func NewAIServiceWithProvider(r *repository.Repository, provider, geminiKey, geminiModel, anthropicKey, anthropicModel string) *AIService {
	provider = strings.ToLower(strings.TrimSpace(provider))
	if provider == "" || provider == "anthropic" || provider == "claude" {
		return &AIService{repository: r, client: NewAnthropicClient(anthropicKey, anthropicModel), chatLocks: make(map[int64]*sync.Mutex)}
	}
	return &AIService{repository: r, client: NewGeminiClient(geminiKey, geminiModel), chatLocks: make(map[int64]*sync.Mutex)}
}

func productAnalysisInput(product repository.ImportedProduct, extra string) ProductAnalysisInput {
	input := ProductAnalysisInput{Name: product.Name, Translation: product.Translation, SKU: product.SKU, Material: product.Material, Characteristics: product.Characteristics, PhotoURL: product.PhotoURL, Extra: strings.TrimSpace(extra)}
	if len(product.AdditionalFields) > 0 {
		_ = json.Unmarshal(product.AdditionalFields, &input.AdditionalFields)
	}
	return input
}

func (s *AIService) AnalyzeProduct(ctx context.Context, productID, userID int64, extra string) (AIAnalysisResponse, error) {
	if s == nil || s.repository == nil {
		return AIAnalysisResponse{}, ErrGeminiNotConfigured
	}
	if len([]byte(extra)) > maxAIInputBytes {
		return AIAnalysisResponse{}, ErrAIRequestTooLarge
	}
	product, err := s.repository.ProductByIDForOwner(ctx, productID, userID)
	if err != nil {
		return AIAnalysisResponse{}, err
	}
	input := productAnalysisInput(product, extra)
	result, err := s.client.Generate(ctx, userID, input)
	if err != nil {
		return AIAnalysisResponse{}, err
	}
	status := map[string]string{statusProposed: "proposed_code", statusQuestions: "needs_clarification", statusFailed: "failed"}[result.Status]
	resultJSON, _ := json.Marshal(result)
	inputJSON, _ := json.Marshal(input)
	record, err := s.repository.CreateProductAIAnalysis(ctx, repository.ProductAIAnalysis{ProductID: productID, UserID: userID, Model: s.client.Model(), Status: status, InputSnapshot: repository.JSON(inputJSON), Result: repository.JSON(resultJSON), CreatedAt: time.Now().UTC(), NeedsRecheck: false})
	if err != nil {
		return AIAnalysisResponse{}, err
	}
	return AIAnalysisResponse{ID: record.ID, ProductID: productID, Model: record.Model, Status: result.Status, Result: result, Input: input, CreatedAt: record.CreatedAt}, nil
}

func (s *AIService) Latest(ctx context.Context, productID, userID int64) (AIAnalysisResponse, error) {
	record, err := s.repository.LatestProductAIAnalysisForOwner(ctx, productID, userID)
	if err != nil {
		return AIAnalysisResponse{}, err
	}
	var result AIClassificationResult
	var input ProductAnalysisInput
	if json.Unmarshal(record.Result, &result) != nil || json.Unmarshal(record.InputSnapshot, &input) != nil {
		return AIAnalysisResponse{}, ErrGeminiBadResponse
	}
	if err := validateAIResult(result); err != nil {
		return AIAnalysisResponse{}, err
	}
	return AIAnalysisResponse{ID: record.ID, ProductID: record.ProductID, Model: record.Model, Status: result.Status, Result: result, Input: input, CreatedAt: record.CreatedAt, NeedsRecheck: record.NeedsRecheck}, nil
}

func (s *AIService) ApplyCode(ctx context.Context, productID, userID int64, code string) (repository.ImportedProduct, error) {
	code = strings.TrimSpace(code)
	if code != "" && !aiCodePattern.MatchString(code) {
		return repository.ImportedProduct{}, &ValidationError{Field: "tnVedCode", Message: "\u041a\u043e\u0434 \u0422\u041d \u0412\u042d\u0414 \u0434\u043e\u043b\u0436\u0435\u043d \u0441\u043e\u0434\u0435\u0440\u0436\u0430\u0442\u044c 10 \u0446\u0438\u0444\u0440 \u0438\u043b\u0438 \u0431\u044b\u0442\u044c \u043f\u0443\u0441\u0442\u044b\u043c"}
	}
	return s.repository.UpdateProductTNVEDCode(ctx, productID, userID, code, "manual")
}

func AIErrorMessage(err error) string {
	switch {
	case errors.Is(err, ErrGeminiNotConfigured):
		return "Ключ ИИ-провайдера не настроен на сервере"
	case errors.Is(err, ErrGeminiUnauthorized):
		return "ИИ-провайдер отклонил ключ API"
	case errors.Is(err, ErrGeminiRateLimited):
		return "ИИ-провайдер временно ограничил запросы. Повторите позже"
	case errors.Is(err, ErrGeminiTimeout):
		return "ИИ-провайдер не ответил вовремя. Повторите запрос"
	case errors.Is(err, ErrGeminiRequestRejected):
		return "ИИ-провайдер отклонил запрос. Проверьте модель и формат запроса"
	case errors.Is(err, ErrGeminiModelUnavailable):
		return "Выбранная модель ИИ-провайдера недоступна для этого ключа"
	case errors.Is(err, ErrAIRequestTooLarge):
		return "Характеристики товара слишком большие"
	case errors.Is(err, ErrGeminiBadResponse):
		return "ИИ-провайдер вернул ответ, который не прошёл проверку"
	case errors.Is(err, ErrAIUnavailable):
		return "ИИ-провайдер временно недоступен. Повторите запрос"
	default:
		return "Не удалось выполнить анализ товара"
	}
}
