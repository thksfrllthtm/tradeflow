package httpapi

import (
	"net/http"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/gin-gonic/gin"
)

// listAntiDumping — действующие антидемпинговые меры и откуда они загружены.
func (s *Server) listAntiDumping(c *gin.Context) {
	measures, err := s.antiDumping.Measures(c.Request.Context())
	if err != nil {
		internalError(c)
		return
	}
	info, err := s.antiDumping.LatestImport(c.Request.Context())
	if err != nil {
		internalError(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"measures": measures, "import": info})
}

// refreshAntiDumping скачивает действующие меры с сайта реестра ЕЭК.
func (s *Server) refreshAntiDumping(c *gin.Context) {
	// Меры до обновления — чтобы сохранить, что изменилось.
	before, err := s.antiDumping.Measures(c.Request.Context())
	if err != nil {
		internalError(c)
		return
	}
	info, err := s.antiDumping.Refresh(c.Request.Context(), currentUser(c).ID)
	if err != nil {
		if validation, ok := err.(*service.ValidationError); ok {
			c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"message": validation.Message}})
			return
		}
		internalError(c)
		return
	}
	measures, err := s.antiDumping.Measures(c.Request.Context())
	if err != nil {
		internalError(c)
		return
	}
	s.saveDataUpdate(c, "antidumping", diffMeasures(before, measures))
	c.JSON(http.StatusOK, gin.H{"measures": measures, "import": info})
}
