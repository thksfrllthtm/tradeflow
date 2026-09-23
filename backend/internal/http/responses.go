package httpapi

// Shared HTTP helpers: current user from context, decimal formatting and JSON error responses.

import (
	"net/http"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

func currentUser(c *gin.Context) repository.User {
	user, _ := c.Get(userContextKey)
	return user.(repository.User)
}

func decimalString(value *decimal.Decimal) string {
	if value == nil {
		return ""
	}
	return value.String()
}

func badRequest(c *gin.Context, message, field string) {
	c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": message, "field": field}})
}

func unauthorized(c *gin.Context) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": gin.H{"message": "Требуется вход в систему"}})
}

func notFound(c *gin.Context) {
	c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"message": "Смета не найдена"}})
}

func internalError(c *gin.Context) {
	c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": "Внутренняя ошибка сервера"}})
}
