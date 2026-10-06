package httpapi

// Стоимостные индикаторы риска (СИР): список профилей — всем, загрузка файла и удаление — администратор.

import (
	"net/http"
	"strconv"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/riskindicators"
	"github.com/gin-gonic/gin"
)

func (s *Server) listRiskIndicators(c *gin.Context) {
	profiles, err := s.riskIndicators.Profiles(c.Request.Context())
	if err != nil {
		internalError(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"profiles": profiles})
}

func (s *Server) importRiskIndicators(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 21<<20)
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		badRequest(c, "Выберите XLSX-файл размером до 20 МБ", "file")
		return
	}
	defer file.Close()
	result, err := s.riskIndicators.Import(c.Request.Context(), currentUser(c).ID, header.Filename, file, header.Size)
	if err != nil {
		if validation, ok := err.(*service.ValidationError); ok {
			badRequest(c, validation.Message, validation.Field)
			return
		}
		internalError(c)
		return
	}
	if len(result.Errors) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "В файле есть строки с ошибками — ничего не сохранено", "field": "file"}, "result": gin.H{"errors": result.Errors}})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) deleteRiskIndicators(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		notFound(c)
		return
	}
	if err := s.riskIndicators.Delete(c.Request.Context(), id); err != nil {
		if riskindicators.IsNotFound(err) {
			notFound(c)
			return
		}
		internalError(c)
		return
	}
	c.Status(http.StatusNoContent)
}
