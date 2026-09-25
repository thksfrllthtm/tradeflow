package httpapi

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/gin-gonic/gin"
)

func (s *Server) getGoodsEstimateUSDRate(c *gin.Context) {
	rate, err := s.gloves.FetchNBKUSD(c.Request.Context(), c.Query("date"))
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

func (s *Server) listGoodsEstimates(c *gin.Context) {
	estimates, err := s.goodsEstimates.ListEstimates(c.Request.Context(), estimateScope(c))
	if err != nil {
		internalError(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"estimates": estimates})
}

func (s *Server) createGoodsEstimate(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	var input service.GlovesEstimateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, "Проверьте данные сметы", "estimate")
		return
	}
	estimate, err := s.goodsEstimates.CreateEstimate(c.Request.Context(), currentUser(c).ID, input)
	if !handleGoodsEstimateError(c, err) {
		return
	}
	c.JSON(http.StatusCreated, gin.H{"estimate": estimate})
}

func (s *Server) getGoodsEstimate(c *gin.Context) {
	id, ok := goodsEstimateID(c)
	if !ok {
		return
	}
	estimate, err := s.goodsEstimates.GetEstimate(c.Request.Context(), id, estimateScope(c))
	if !handleGoodsEstimateError(c, err) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"estimate": estimate})
}

func (s *Server) updateGoodsEstimate(c *gin.Context) {
	id, ok := goodsEstimateID(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	var input service.GlovesEstimateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, "Проверьте данные сметы", "estimate")
		return
	}
	estimate, err := s.goodsEstimates.UpdateEstimate(c.Request.Context(), id, estimateScope(c), input)
	if !handleGoodsEstimateError(c, err) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"estimate": estimate})
}

func (s *Server) deleteGoodsEstimate(c *gin.Context) {
	id, ok := goodsEstimateID(c)
	if !ok {
		return
	}
	if err := s.goodsEstimates.DeleteEstimate(c.Request.Context(), id, estimateScope(c)); !handleGoodsEstimateError(c, err) {
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) saveGoodsEstimateDraft(c *gin.Context) {
	id, ok := goodsEstimateID(c)
	if !ok {
		return
	}
	// Автосохранение присылает только изменённые части; фото — порциями по несколько МБ.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32<<20)
	var patch service.GoodsDraftPatch
	if err := c.ShouldBindJSON(&patch); err != nil {
		badRequest(c, "Проверьте данные черновика сметы", "draft")
		return
	}
	saved, err := s.goodsEstimates.PatchEstimateDraft(c.Request.Context(), id, estimateScope(c), patch)
	if !handleGoodsEstimateError(c, err) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"estimate": saved})
}

func goodsEstimateID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		badRequest(c, "Некорректный идентификатор копии сметы", "id")
		return 0, false
	}
	return id, true
}

func handleGoodsEstimateError(c *gin.Context, err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, repository.ErrNotFound) || service.IsNotFound(err) {
		notFound(c)
		return false
	}
	var valErr *service.ValidationError
	if errors.As(err, &valErr) {
		badRequest(c, valErr.Message, valErr.Field)
		return false
	}
	internalError(c)
	return false
}
