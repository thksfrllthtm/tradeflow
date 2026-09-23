package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
)

const (
	maxChatMessageBytes = 4000
	maxChatMessages     = 50
	maxChatAttachments  = 4
)

type AIChatMessageResponse struct {
	ID                 int64           `json:"id"`
	Author             string          `json:"author"`
	Content            string          `json:"content"`
	AttachmentAssetIDs []int64         `json:"attachmentAssetIds"`
	Response           *AIChatResponse `json:"response,omitempty"`
	Model              string          `json:"model"`
	Status             string          `json:"status"`
	AnalysisID         *int64          `json:"analysisId,omitempty"`
	ProductRevision    int             `json:"productRevision"`
	Stale              bool            `json:"stale"`
	CreatedAt          time.Time       `json:"createdAt"`
}

type AIDialogueResponse struct {
	ProductID       int64                   `json:"productId"`
	ProductRevision int                     `json:"productRevision"`
	Messages        []AIChatMessageResponse `json:"messages"`
	LatestAnalysis  *AIAnalysisResponse     `json:"latestAnalysis,omitempty"`
}

func (s *AIService) chatLock(productID int64) *sync.Mutex {
	s.chatMu.Lock()
	defer s.chatMu.Unlock()
	if s.chatLocks == nil {
		s.chatLocks = make(map[int64]*sync.Mutex)
	}
	lock := s.chatLocks[productID]
	if lock == nil {
		lock = &sync.Mutex{}
		s.chatLocks[productID] = lock
	}
	return lock
}

func (s *AIService) ProductAIDialogue(ctx context.Context, productID, userID int64) (AIDialogueResponse, error) {
	product, err := s.repository.ProductByIDForOwner(ctx, productID, userID)
	if err != nil {
		return AIDialogueResponse{}, err
	}
	messages, err := s.repository.ListProductAIMessagesForOwner(ctx, productID, userID, maxChatMessages)
	if err != nil {
		return AIDialogueResponse{}, err
	}
	response := AIDialogueResponse{ProductID: productID, ProductRevision: product.AIDataRevision, Messages: make([]AIChatMessageResponse, 0, len(messages))}
	for _, message := range messages {
		response.Messages = append(response.Messages, chatMessageResponse(message, product.AIDataRevision))
	}
	if analysis, analysisErr := s.Latest(ctx, productID, userID); analysisErr == nil {
		response.LatestAnalysis = &analysis
	} else if !errors.Is(analysisErr, repository.ErrNotFound) {
		return AIDialogueResponse{}, analysisErr
	}
	return response, nil
}

func (s *AIService) SendProductAIMessage(ctx context.Context, productID, userID int64, content, requestID string, attachmentAssetIDs []int64) (AIDialogueResponse, error) {
	content = strings.TrimSpace(content)
	requestID = strings.TrimSpace(requestID)
	if content == "" {
		return AIDialogueResponse{}, &ValidationError{Field: "message", Message: "Напишите сообщение или вопрос для ИИ"}
	}
	if len([]byte(content)) > maxChatMessageBytes {
		return AIDialogueResponse{}, &ValidationError{Field: "message", Message: "Сообщение слишком длинное (максимум 4 000 байт)"}
	}
	if requestID == "" || len(requestID) > 128 {
		return AIDialogueResponse{}, &ValidationError{Field: "clientRequestId", Message: "Не удалось определить запрос. Повторите отправку"}
	}
	if len(attachmentAssetIDs) > maxChatAttachments {
		return AIDialogueResponse{}, &ValidationError{Field: "attachmentAssetIds", Message: "Можно прикрепить не более 4 фотографий"}
	}
	lock := s.chatLock(productID)
	lock.Lock()
	defer lock.Unlock()
	product, err := s.repository.ProductByIDForOwner(ctx, productID, userID)
	if err != nil {
		return AIDialogueResponse{}, err
	}
	if _, previousErr := s.repository.ProductAIMessageByRequest(ctx, productID, userID, requestID); previousErr == nil {
		return s.ProductAIDialogue(ctx, productID, userID)
	} else if !errors.Is(previousErr, repository.ErrNotFound) {
		return AIDialogueResponse{}, previousErr
	}
	for _, assetID := range attachmentAssetIDs {
		if assetID < 1 {
			return AIDialogueResponse{}, &ValidationError{Field: "attachmentAssetIds", Message: "Некорректное прикреплённое изображение"}
		}
		if _, err := s.repository.AssetByIDForProductOwner(ctx, assetID, productID, userID); err != nil {
			return AIDialogueResponse{}, &ValidationError{Field: "attachmentAssetIds", Message: "Фотография не принадлежит выбранному товару"}
		}
	}
	dialogue, err := s.repository.ProductAIDialogueForOwner(ctx, productID, userID)
	if err != nil {
		return AIDialogueResponse{}, err
	}
	historyRows, err := s.repository.ListProductAIMessagesForOwner(ctx, productID, userID, maxChatMessages)
	if err != nil {
		return AIDialogueResponse{}, err
	}
	history := make([]ProductChatHistoryMessage, 0, len(historyRows))
	for _, row := range historyRows {
		if row.Status != "completed" || strings.TrimSpace(row.Content) == "" {
			continue
		}
		role := row.Author
		if role == "assistant" {
			role = "assistant"
		}
		history = append(history, ProductChatHistoryMessage{Role: role, Content: row.Content})
	}
	var currentAnalysis *AIClassificationResult
	var currentAnalysisID *int64
	if latest, latestErr := s.Latest(ctx, productID, userID); latestErr == nil {
		currentAnalysis = &latest.Result
		currentAnalysisID = &latest.ID
	} else if !errors.Is(latestErr, repository.ErrNotFound) {
		return AIDialogueResponse{}, latestErr
	}
	assets, err := s.repository.ImageAssetIDsForProducts(ctx, []int64{productID})
	if err != nil {
		return AIDialogueResponse{}, err
	}
	input := productAnalysisInput(product, "")
	contextData := ProductChatContext{Product: input, CurrentCode: product.TNVEDCode, CurrentAnalysis: currentAnalysis, PhotoAssetIDs: assets[productID], Conversation: history}
	requestPtr := requestID
	attachmentJSON, _ := json.Marshal(attachmentAssetIDs)
	userMessage, err := s.repository.CreateProductAIMessage(ctx, repository.ProductAIMessage{DialogueID: dialogue.ID, UserID: &userID, Author: "user", Content: content, AttachmentAssetIDs: repository.JSON(attachmentJSON), Response: repository.JSON([]byte("{}")), Model: s.client.Model(), Status: "pending", ClientRequestID: &requestPtr, AnalysisID: currentAnalysisID, ProductRevision: product.AIDataRevision, CreatedAt: time.Now().UTC()})
	if err != nil {
		if duplicate, duplicateErr := s.repository.ProductAIMessageByRequest(ctx, productID, userID, requestID); duplicateErr == nil {
			_ = duplicate
			return s.ProductAIDialogue(ctx, productID, userID)
		}
		return AIDialogueResponse{}, err
	}
	chatProvider, ok := s.client.(aiChatClient)
	if !ok {
		_ = s.repository.UpdateProductAIMessage(ctx, userMessage.ID, "failed", repository.JSON([]byte("{}")))
		return AIDialogueResponse{}, ErrAIUnavailable
	}
	result, err := chatProvider.GenerateChat(ctx, userID, contextData, content)
	if err != nil {
		_ = s.repository.UpdateProductAIMessage(ctx, userMessage.ID, "failed", repository.JSON([]byte("{}")))
		return AIDialogueResponse{}, err
	}
	result = normalizeChatResponse(result, product)
	if err := s.enrichChatCandidates(ctx, &result); err != nil {
		_ = s.repository.UpdateProductAIMessage(ctx, userMessage.ID, "failed", repository.JSON([]byte("{}")))
		return AIDialogueResponse{}, err
	}
	resultJSON, _ := json.Marshal(result)
	if _, err := s.repository.CreateProductAIMessage(ctx, repository.ProductAIMessage{DialogueID: dialogue.ID, Author: "assistant", Content: result.Message, AttachmentAssetIDs: repository.JSON([]byte("[]")), Response: repository.JSON(resultJSON), Model: s.client.Model(), Status: "completed", AnalysisID: currentAnalysisID, ProductRevision: product.AIDataRevision, CreatedAt: time.Now().UTC()}); err != nil {
		_ = s.repository.UpdateProductAIMessage(ctx, userMessage.ID, "failed", repository.JSON([]byte("{}")))
		return AIDialogueResponse{}, err
	}
	_ = s.repository.UpdateProductAIMessage(ctx, userMessage.ID, "completed", repository.JSON([]byte("{}")))
	_ = s.repository.TouchProductAIDialogue(ctx, dialogue.ID)
	return s.ProductAIDialogue(ctx, productID, userID)
}

func normalizeChatResponse(result AIChatResponse, product repository.ImportedProduct) AIChatResponse {
	for index := range result.ProposedChanges {
		change := &result.ProposedChanges[index]
		change.Field = strings.TrimSpace(change.Field)
		change.Value = strings.TrimSpace(change.Value)
		if change.Label == "" {
			change.Label = change.Field
		}
		switch change.Field {
		case "material":
			change.PreviousValue = product.Material
		case "characteristics":
			change.PreviousValue = product.Characteristics
		case "name":
			change.PreviousValue = product.Name
		case "translation":
			change.PreviousValue = product.Translation
		default:
			change.Field = ""
		}
	}
	filtered := result.ProposedChanges[:0]
	for _, change := range result.ProposedChanges {
		if change.Field != "" && len([]byte(change.Value)) <= 2000 {
			filtered = append(filtered, change)
		}
	}
	result.ProposedChanges = filtered
	return result
}

// enrichChatCandidates is deliberately conservative: model text is never
// treated as an official description. A candidate is marked verified only
// when a current non-DEMO catalog row exists on the server.
func (s *AIService) enrichChatCandidates(ctx context.Context, result *AIChatResponse) error {
	for index := range result.Candidates {
		candidate := &result.Candidates[index]
		candidate.CatalogStatus = "not_verified"
		candidate.CatalogSources = []string{}
		if candidate.Code == "" {
			candidate.CatalogStatus = "not_checked"
			continue
		}
		details, err := s.repository.CatalogDetails(ctx, candidate.Code, time.Now().UTC(), false)
		if err != nil {
			// A missing/unavailable catalog must not turn a dialogue into a
			// false confirmation. Keep the explicit not_verified status.
			continue
		}
		if len(details) == 0 {
			continue
		}
		if details[0].Code.IsGroup {
			candidate.CatalogStatus = "group_code"
			continue
		}
		candidate.CatalogStatus = "verified"
		candidate.CatalogDescription = details[0].Code.OfficialDescription
		if source := strings.TrimSpace(details[0].Code.Source); source != "" {
			candidate.CatalogSources = []string{source}
		}
	}
	return nil
}

func chatMessageResponse(message repository.ProductAIMessage, currentRevision int) AIChatMessageResponse {
	var assets []int64
	_ = json.Unmarshal(message.AttachmentAssetIDs, &assets)
	var response *AIChatResponse
	if message.Author == "assistant" && len(message.Response) > 0 && string(message.Response) != "{}" {
		var parsed AIChatResponse
		if json.Unmarshal(message.Response, &parsed) == nil {
			response = &parsed
		}
	}
	return AIChatMessageResponse{ID: message.ID, Author: message.Author, Content: message.Content, AttachmentAssetIDs: assets, Response: response, Model: message.Model, Status: message.Status, AnalysisID: message.AnalysisID, ProductRevision: message.ProductRevision, Stale: message.ProductRevision != currentRevision, CreatedAt: message.CreatedAt}
}

func (s *AIService) ApplyChatChanges(ctx context.Context, productID, userID int64, changes []AIProposedChange) (repository.ImportedProduct, error) {
	if len(changes) == 0 || len(changes) > 10 {
		return repository.ImportedProduct{}, &ValidationError{Field: "changes", Message: "Выберите изменения, которые нужно добавить к товару"}
	}
	product, err := s.repository.ProductByIDForOwner(ctx, productID, userID)
	if err != nil {
		return repository.ImportedProduct{}, err
	}
	updated := product
	seenFields := map[string]bool{}
	for _, change := range changes {
		value := strings.TrimSpace(change.Value)
		if value == "" || len([]byte(value)) > 2000 {
			return repository.ImportedProduct{}, &ValidationError{Field: "changes", Message: "Некорректное значение уточнения"}
		}
		if seenFields[change.Field] {
			return repository.ImportedProduct{}, &ValidationError{Field: "changes", Message: "Одно поле нельзя изменить несколькими предложениями одновременно"}
		}
		seenFields[change.Field] = true
		switch change.Field {
		case "material":
			if product.Material != change.PreviousValue {
				return repository.ImportedProduct{}, &ValidationError{Field: "changes", Message: "Материал уже изменён. Обновите карточку и выберите актуальное значение"}
			}
			updated.Material = value
		case "characteristics":
			if product.Characteristics != change.PreviousValue {
				return repository.ImportedProduct{}, &ValidationError{Field: "changes", Message: "Характеристики уже изменены. Обновите карточку и выберите актуальное значение"}
			}
			updated.Characteristics = value
		case "name":
			updated.Name = value
		case "translation":
			updated.Translation = value
		default:
			return repository.ImportedProduct{}, &ValidationError{Field: "changes", Message: "ИИ предложил неподдерживаемое поле"}
		}
	}
	updated.AIDataRevision = product.AIDataRevision + 1
	updated.UpdatedBy = userID
	if err := s.repository.MarkProductAIAnalysesRecheck(ctx, product.ID); err != nil {
		return repository.ImportedProduct{}, err
	}
	saved, err := s.repository.UpdateProduct(ctx, updated)
	if err != nil {
		return repository.ImportedProduct{}, err
	}
	assets, err := s.repository.ImageAssetIDsForProducts(ctx, []int64{productID})
	if err == nil {
		saved.ImageAssetIDs = assets[productID]
	}
	return saved, err
}
