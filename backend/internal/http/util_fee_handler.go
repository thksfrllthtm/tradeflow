package httpapi

import (
	"errors"
	"net/http"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/utilfee"
	"github.com/gin-gonic/gin"
)

// listUtilFee — весь перечень утильсбора, чтобы смотреть без ввода кода.
func (s *Server) listUtilFee(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"edition": utilfee.Edition, "sections": utilfee.List()})
}

// calculateUtilFee — калькулятор утильсбора: позиции перечня для кода и суммы по заполненным полям.
func (s *Server) calculateUtilFee(c *gin.Context) {
	var input utilfee.Input
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, "Некорректный запрос", "")
		return
	}
	result, err := utilfee.Calculate(input)
	if err != nil {
		var validation *service.ValidationError
		if errors.As(err, &validation) {
			badRequest(c, validation.Message, validation.Field)
			return
		}
		internalError(c)
		return
	}
	c.JSON(http.StatusOK, result)
}
