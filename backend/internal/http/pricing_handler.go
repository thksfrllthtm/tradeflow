package httpapi

// Hints for distributing a budget over estimate prices: DeepSeek unit price
// proportions and the unit price history of saved calculations.

import (
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
		Kind       string   `json:"kind"`
		EstimateID int64    `json:"estimateId"`
		Codes      []string `json:"codes"`
	}
	if err := c.ShouldBindJSON(&request); err != nil {
		badRequest(c, "Некорректный запрос истории цен", "codes")
		return
	}
	prices, err := s.pricing.PriceHistory(c.Request.Context(), estimateScope(c), request.Kind, request.EstimateID, request.Codes)
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
