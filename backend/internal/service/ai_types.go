package service

// Provider-independent AI contract: limits, statuses, errors, client interfaces and request/response shapes.

import (
	"context"
	"errors"
	"time"
)

const (
	maxAIInputBytes = 32 << 10
	geminiTimeout   = 25 * time.Second
	// Keep the complete interaction bounded as well as the provider request.
	// This remains a safety net if the caller's context has no deadline.
	geminiOverallTimeout = 40 * time.Second
	statusProposed       = "\u043f\u0440\u0435\u0434\u043b\u043e\u0436\u0435\u043d \u043a\u043e\u0434"
	statusQuestions      = "\u043d\u0443\u0436\u043d\u043e \u0443\u0442\u043e\u0447\u043d\u0435\u043d\u0438\u0435"
	statusFailed         = "\u043d\u0435 \u0443\u0434\u0430\u043b\u043e\u0441\u044c \u043e\u043f\u0440\u0435\u0434\u0435\u043b\u0438\u0442\u044c"
)

var (
	ErrGeminiNotConfigured    = errors.New("gemini is not configured")
	ErrGeminiUnauthorized     = errors.New("gemini authorization failed")
	ErrGeminiRateLimited      = errors.New("gemini rate limit")
	ErrGeminiTimeout          = errors.New("gemini timeout")
	ErrGeminiRequestRejected  = errors.New("gemini request rejected")
	ErrGeminiModelUnavailable = errors.New("gemini model unavailable")
	ErrGeminiBadResponse      = errors.New("gemini returned invalid response")
	ErrAIRequestTooLarge      = errors.New("ai input is too large")
	ErrAIUnavailable          = errors.New("ai service unavailable")
)

type aiClient interface {
	Generate(context.Context, int64, ProductAnalysisInput) (AIClassificationResult, error)
	Model() string
}

type aiChatClient interface {
	GenerateChat(context.Context, int64, ProductChatContext, string) (AIChatResponse, error)
}

type ProductAnalysisInput struct {
	Name             string            `json:"name"`
	Translation      string            `json:"translation,omitempty"`
	SKU              string            `json:"sku,omitempty"`
	Material         string            `json:"material,omitempty"`
	Characteristics  string            `json:"characteristics,omitempty"`
	PhotoURL         string            `json:"photoUrl,omitempty"`
	AdditionalFields map[string]string `json:"additionalFields,omitempty"`
	Extra            string            `json:"extra,omitempty"`
}

type ProductChatHistoryMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ProductChatContext struct {
	Product         ProductAnalysisInput        `json:"product"`
	CurrentCode     string                      `json:"currentCode,omitempty"`
	CurrentAnalysis *AIClassificationResult     `json:"currentAnalysis,omitempty"`
	PhotoAssetIDs   []int64                     `json:"photoAssetIds,omitempty"`
	Conversation    []ProductChatHistoryMessage `json:"conversation,omitempty"`
}

type AIProposedChange struct {
	Field         string `json:"field"`
	Label         string `json:"label"`
	Value         string `json:"value"`
	PreviousValue string `json:"previousValue"`
	Evidence      string `json:"evidence,omitempty"`
}

type AIChatCandidate struct {
	Code               string   `json:"code"`
	PositionName       string   `json:"positionName"`
	Explanation        string   `json:"explanation"`
	CatalogStatus      string   `json:"catalogStatus,omitempty"`
	CatalogDescription string   `json:"catalogDescription,omitempty"`
	CatalogSources     []string `json:"catalogSources,omitempty"`
}

type AIChatResponse struct {
	Message         string             `json:"message"`
	Questions       []string           `json:"questions"`
	ProposedChanges []AIProposedChange `json:"proposedChanges"`
	Candidates      []AIChatCandidate  `json:"candidates"`
	NeedsRefresh    bool               `json:"needsRefresh"`
}

type AIAlternative struct {
	Code        string `json:"code"`
	Explanation string `json:"explanation"`
}

type AIClassificationResult struct {
	PrimaryCode            string          `json:"primaryCode"`
	PositionName           string          `json:"positionName"`
	Explanation            string          `json:"explanation"`
	Alternatives           []AIAlternative `json:"alternatives"`
	MissingCharacteristics []string        `json:"missingCharacteristics"`
	Questions              []string        `json:"questions"`
	Status                 string          `json:"status"`
}

type AIAnalysisResponse struct {
	ID           int64                  `json:"id"`
	ProductID    int64                  `json:"productId"`
	Model        string                 `json:"model"`
	Status       string                 `json:"status"`
	Result       AIClassificationResult `json:"result"`
	Input        ProductAnalysisInput   `json:"input"`
	CreatedAt    time.Time              `json:"createdAt"`
	NeedsRecheck bool                   `json:"needsRecheck"`
}
