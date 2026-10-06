package httpapi

// Официальный курс Национального банка РК на дату.

import (
	"errors"
	"net/http"
	"strings"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/rates"
	"github.com/gin-gonic/gin"
)

// getNBKRate — курс НБРК на дату: USD по умолчанию, ?currency=EUR — евро
// (справочно в шапке и на входе — через /api/rates/today; расчёт идёт в долларах).
func (s *Server) getNBKRate(c *gin.Context) {
	currency := strings.ToUpper(strings.TrimSpace(c.DefaultQuery("currency", "USD")))
	if currency != "USD" && currency != "EUR" {
		badRequest(c, "Поддерживаются только USD и EUR", "currency")
		return
	}
	rate, err := rates.FetchNBKRate(c.Request.Context(), c.Query("date"), currency)
	if err != nil {
		var validationErr *service.ValidationError
		if errors.As(err, &validationErr) {
			badRequest(c, validationErr.Message, validationErr.Field)
			return
		}
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"message": err.Error(), "field": "usdKzt"}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"rate": rate})
}

// getTodayRates — курсы НБРК на сегодня для страниц входа и регистрации: открыт без входа,
// дату выбирает сервер, ответ запоминается (rates.TodayRates).
func (s *Server) getTodayRates(c *gin.Context) {
	list, err := rates.TodayRates(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"message": err.Error()}})
		return
	}
	c.JSON(http.StatusOK, gin.H{"rates": list})
}
