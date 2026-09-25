package httpapi

// Hints for distributing a budget over estimate prices: DeepSeek unit price
// proportions and the unit price history of saved calculations.

import (
	"errors"
	"net/http"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/gin-gonic/gin"
)

func (s *Server) suggestPriceWeights(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	var request service.PriceWeightsRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		badRequest(c, "Некорректные группы товаров", "groups")
		return
	}
	prices, err := s.pricing.SuggestPrices(c.Request.Context(), currentUser(c).ID, request)
	if !s.handleAIError(c, err) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"prices": prices})
}

func (s *Server) priceHistory(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	var request struct {
		EstimateID int64    `json:"estimateId"`
		Codes      []string `json:"codes"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		badRequest(c, "Некорректный запрос истории цен", "codes")
		return
	}
	prices, err := s.pricing.PriceHistory(c.Request.Context(), estimateScope(c), request.EstimateID, request.Codes)
	if err != nil {
		if validation, ok := err.(*service.ValidationError); ok {
			badRequest(c, validation.Message, validation.Field)
			return
		}
		internalError(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"prices": prices})
}

// handleAIError answers with the AI error status and message; true means no error.
func (s *Server) handleAIError(c *gin.Context, err error) bool {
	if err == nil {
		return true
	}
	if validation, ok := err.(*service.ValidationError); ok {
		badRequest(c, validation.Message, validation.Field)
		return false
	}
	var status int
	var code string
	switch {
	case errors.Is(err, service.ErrAINotConfigured):
		status, code = http.StatusServiceUnavailable, "ai_provider_not_configured"
	case errors.Is(err, service.ErrAIRateLimited):
		status, code = http.StatusTooManyRequests, "ai_rate_limited"
	case errors.Is(err, service.ErrAITimeout):
		status, code = http.StatusGatewayTimeout, "ai_timeout"
	case errors.Is(err, service.ErrAIRequestTooLarge):
		status, code = http.StatusRequestEntityTooLarge, "ai_request_too_large"
	case errors.Is(err, service.ErrAIUnauthorized):
		status, code = http.StatusBadGateway, "ai_unauthorized"
	case errors.Is(err, service.ErrAIRequestRejected):
		status, code = http.StatusBadGateway, "ai_request_rejected"
	case errors.Is(err, service.ErrAIModelUnavailable):
		status, code = http.StatusBadGateway, "ai_model_unavailable"
	case errors.Is(err, service.ErrAIBadResponse):
		status, code = http.StatusBadGateway, "ai_bad_response"
	case errors.Is(err, service.ErrAIUnavailable):
		status, code = http.StatusBadGateway, "ai_unavailable"
	default:
		status, code = http.StatusInternalServerError, "ai_error"
	}
	c.JSON(status, gin.H{"error": gin.H{"message": service.AIErrorMessage(err), "code": code}})
	return false
}
