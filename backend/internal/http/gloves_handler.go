package httpapi

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/gin-gonic/gin"
)

func (s *Server) parseGlovesXlsx(c *gin.Context) {
	// Multipart headers and boundaries add overhead on top of the documented
	// file limit; leave a small fixed envelope without making the endpoint
	// unbounded.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, service.MaxGlovesXLSXBytes+(1<<20))
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		badRequest(c, fmt.Sprintf("Выберите XLSX-файл размером до %d МБ", service.MaxGlovesXLSXBytes>>20), "file")
		return
	}
	defer file.Close()

	if header.Size > service.MaxGlovesXLSXBytes {
		badRequest(c, fmt.Sprintf("XLSX-файл больше %d МБ не поддерживается", service.MaxGlovesXLSXBytes>>20), "file")
		return
	}
	if !strings.HasSuffix(strings.ToLower(header.Filename), ".xlsx") {
		badRequest(c, "Поддерживаются только обычные файлы .xlsx", "file")
		return
	}

	sheet := c.Query("sheet")
	headerRow, _ := strconv.Atoi(c.DefaultQuery("headerRow", "1"))
	dataStartRow, _ := strconv.Atoi(c.DefaultQuery("dataStartRow", "2"))

	var result service.GlovesParseResult
	if c.Query("shareImages") == "true" {
		result, err = s.gloves.ParseXlsxWithSharedImages(c.Request.Context(), file, header.Size, header.Filename, sheet, headerRow, dataStartRow)
	} else {
		result, err = s.gloves.ParseXlsx(c.Request.Context(), file, header.Size, header.Filename, sheet, headerRow, dataStartRow)
	}
	if err != nil {
		var valErr *service.ValidationError
		if errors.As(err, &valErr) {
			badRequest(c, valErr.Message, valErr.Field)
		} else {
			badRequest(c, err.Error(), "file")
		}
		return
	}

	// Photos are streamed into the response instead of c.JSON building the
	// whole (possibly 100+ MB) body in memory.
	c.Header("Content-Type", "application/json; charset=utf-8")
	c.Status(http.StatusOK)
	if err := result.WriteJSON(c.Writer); err != nil {
		_ = c.Error(err)
	}
}

func (s *Server) listGlovesEstimates(c *gin.Context) {
	estimates, err := s.gloves.ListEstimates(c.Request.Context(), estimateScope(c))
	if err != nil {
		internalError(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"estimates": estimates})
}

func (s *Server) createGlovesEstimate(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	var input service.GlovesEstimateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, "Проверьте данные сметы перчаток", "estimate")
		return
	}
	estimate, err := s.gloves.CreateEstimate(c.Request.Context(), currentUser(c).ID, input)
	if !handleGlovesEstimateError(c, err) {
		return
	}
	c.JSON(http.StatusCreated, gin.H{"estimate": estimate})
}

func (s *Server) getGlovesEstimate(c *gin.Context) {
	id, ok := glovesEstimateID(c)
	if !ok {
		return
	}
	estimate, err := s.gloves.GetEstimate(c.Request.Context(), id, estimateScope(c))
	if !handleGlovesEstimateError(c, err) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"estimate": estimate})
}

func (s *Server) updateGlovesEstimate(c *gin.Context) {
	id, ok := glovesEstimateID(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	var input service.GlovesEstimateInput
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, "Проверьте данные сметы перчаток", "estimate")
		return
	}
	estimate, err := s.gloves.UpdateEstimate(c.Request.Context(), id, estimateScope(c), input)
	if !handleGlovesEstimateError(c, err) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"estimate": estimate})
}

func (s *Server) deleteGlovesEstimate(c *gin.Context) {
	id, ok := glovesEstimateID(c)
	if !ok {
		return
	}
	if err := s.gloves.DeleteEstimate(c.Request.Context(), id, estimateScope(c)); !handleGlovesEstimateError(c, err) {
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) saveGlovesEstimateDraft(c *gin.Context) {
	id, ok := glovesEstimateID(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<20)
	var draft service.GlovesDraft
	if err := c.ShouldBindJSON(&draft); err != nil {
		badRequest(c, "Проверьте данные черновика перчаток", "draft")
		return
	}
	saved, err := s.gloves.SaveEstimateDraft(c.Request.Context(), id, estimateScope(c), draft)
	if !handleGlovesEstimateError(c, err) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"estimate": saved})
}

func glovesEstimateID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		badRequest(c, "Некорректный идентификатор сметы перчаток", "id")
		return 0, false
	}
	return id, true
}

func handleGlovesEstimateError(c *gin.Context, err error) bool {
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

func (s *Server) calculateGloves(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<20)
	var input service.GlovesCalculationInput
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, "Проверьте данные расчёта", "")
		return
	}

	result, err := s.gloves.Calculate(c.Request.Context(), input)
	if err != nil {
		var valErr *service.ValidationError
		if errors.As(err, &valErr) {
			badRequest(c, valErr.Message, valErr.Field)
		} else {
			internalError(c)
		}
		return
	}

	c.JSON(http.StatusOK, result)
}

func (s *Server) exportGloves(c *gin.Context) {
	// Rows carry their first photo (compressed JPG/PNG) for the XLSX file.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<20)
	var input service.GlovesCalculationInput
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, "Проверьте данные расчёта", "")
		return
	}

	buffer, err := s.gloves.Export(c.Request.Context(), input)
	if err != nil {
		var valErr *service.ValidationError
		if errors.As(err, &valErr) {
			badRequest(c, valErr.Message, valErr.Field)
		} else {
			internalError(c)
		}
		return
	}

	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Header("Content-Disposition", `attachment; filename="gloves-calculator.xlsx"`)
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", buffer.Bytes())
}
