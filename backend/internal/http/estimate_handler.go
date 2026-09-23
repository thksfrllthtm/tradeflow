package httpapi

// Estimates of the import calculator: CRUD endpoints under /api/estimates.

import (
	"net/http"
	"strconv"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/gin-gonic/gin"
)

func (s *Server) listEstimates(c *gin.Context) {
	list, err := s.estimates.List(c.Request.Context(), currentUser(c).ID)
	if err != nil {
		internalError(c)
		return
	}
	response := make([]gin.H, 0, len(list))
	for _, estimate := range list {
		response = append(response, estimateResponse(estimate))
	}
	c.JSON(http.StatusOK, gin.H{"estimates": response})
}

func (s *Server) createEstimate(c *gin.Context) {
	input, ok := readEstimateInput(c)
	if !ok {
		return
	}
	estimate, err := s.estimates.Create(c.Request.Context(), currentUser(c).ID, input)
	if !s.handleEstimateError(c, err) {
		return
	}
	c.JSON(http.StatusCreated, gin.H{"estimate": estimateResponse(estimate)})
}

func (s *Server) getEstimate(c *gin.Context) {
	id, ok := estimateID(c)
	if !ok {
		return
	}
	estimate, err := s.estimates.Get(c.Request.Context(), id, currentUser(c).ID)
	if service.IsNotFound(err) {
		notFound(c)
		return
	}
	if err != nil {
		internalError(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"estimate": estimateResponse(estimate)})
}

func (s *Server) updateEstimate(c *gin.Context) {
	id, ok := estimateID(c)
	if !ok {
		return
	}
	input, ok := readEstimateInput(c)
	if !ok {
		return
	}
	estimate, err := s.estimates.Update(c.Request.Context(), id, currentUser(c).ID, input)
	if service.IsNotFound(err) {
		notFound(c)
		return
	}
	if !s.handleEstimateError(c, err) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"estimate": estimateResponse(estimate)})
}

func (s *Server) deleteEstimate(c *gin.Context) {
	id, ok := estimateID(c)
	if !ok {
		return
	}
	if err := s.estimates.Delete(c.Request.Context(), id, currentUser(c).ID); service.IsNotFound(err) {
		notFound(c)
		return
	} else if err != nil {
		internalError(c)
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) handleEstimateError(c *gin.Context, err error) bool {
	if err == nil {
		return true
	}
	if validation, ok := err.(*service.ValidationError); ok {
		badRequest(c, validation.Message, validation.Field)
		return false
	}
	internalError(c)
	return false
}

func readEstimateInput(c *gin.Context) (service.EstimateInput, bool) {
	var input service.EstimateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, "Проверьте заполнение формы", "")
		return service.EstimateInput{}, false
	}
	return input, true
}

func estimateID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		badRequest(c, "Некорректный идентификатор сметы", "")
		return 0, false
	}
	return id, true
}

func estimateResponse(estimate repository.Estimate) gin.H {
	return gin.H{
		"id":             estimate.ID,
		"name":           estimate.Name,
		"client":         estimate.Client,
		"assessmentDate": estimate.AssessmentDate.Format("2006-01-02"),
		"importCountry":  "Казахстан",
		"currency":       estimate.Currency,
		"exchangeRate":   estimate.ExchangeRate.String(),
		"origin":         estimate.Origin,
		"deliveryTerms":  estimate.DeliveryTerms,
		"note":           estimate.Note,
		"status":         "Черновик",
		"statusCode":     estimate.Status,
		"createdAt":      estimate.CreatedAt.UTC().Format(time.RFC3339),
		"updatedAt":      estimate.UpdatedAt.UTC().Format(time.RFC3339),
		"createdBy":      estimate.CreatedBy,
		"updatedBy":      estimate.UpdatedBy,
	}
}
