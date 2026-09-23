package httpapi

// Customs calculation of an estimate: read, save and XLSX export.

import (
	"errors"
	"net/http"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/gin-gonic/gin"
)

func (s *Server) getCalculation(c *gin.Context) {
	id, ok := estimateID(c)
	if !ok {
		return
	}
	result, err := s.calculation.Get(c.Request.Context(), id, currentUser(c).ID)
	if !s.handleCalculationError(c, err) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"calculation": result})
}

func (s *Server) saveCalculation(c *gin.Context) {
	id, ok := estimateID(c)
	if !ok {
		return
	}
	// A calculation can contain the editable snapshot for every imported row;
	// keep a bounded but practical request size for the documented 5,000-row
	// workbook limit.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<20)
	var input service.CalculationInput
	if err := c.ShouldBindJSON(&input); err != nil {
		var sizeErr *http.MaxBytesError
		if errors.As(err, &sizeErr) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": gin.H{"message": "Данные расчёта слишком большие", "code": "calculation_too_large"}})
			return
		}
		badRequest(c, "Проверьте данные расчёта", "")
		return
	}
	result, err := s.calculation.Save(c.Request.Context(), id, currentUser(c).ID, input)
	if !s.handleCalculationError(c, err) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"calculation": result})
}

func (s *Server) exportCalculation(c *gin.Context) {
	id, ok := estimateID(c)
	if !ok {
		return
	}
	buffer, err := s.calculation.Export(c.Request.Context(), id, currentUser(c).ID)
	if !s.handleCalculationError(c, err) {
		return
	}
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("Content-Disposition", `attachment; filename="customs-calculation-preliminary.xlsx"`)
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", buffer.Bytes())
}

func (s *Server) handleCalculationError(c *gin.Context, err error) bool {
	if err == nil {
		return true
	}
	if service.IsNotFound(err) {
		notFound(c)
		return false
	}
	if validation, ok := err.(*service.ValidationError); ok {
		badRequest(c, validation.Message, validation.Field)
		return false
	}
	internalError(c)
	return false
}
