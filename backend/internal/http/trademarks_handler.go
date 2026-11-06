package httpapi

// ТРОИС: сводка, список, логотипы и проверка строк — всем; загрузка файла реестра и подпись логотипа —
// администратор. Регистрационный номер с «/», поэтому он в запросе или теле, а не в пути.

import (
	"net/http"
	"strconv"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/trademarks"
	"github.com/gin-gonic/gin"
)

func (s *Server) listTrademarks(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	items, total, err := s.trademarks.List(c.Request.Context(), c.Query("q"), c.Query("unnamed") == "1", page)
	if err != nil {
		internalError(c)
		return
	}
	summary, err := s.trademarks.Summary(c.Request.Context())
	if err != nil {
		internalError(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"summary": summary, "items": items, "total": total})
}

func (s *Server) trademarkLogo(c *gin.Context) {
	mime, data, err := s.trademarks.Logo(c.Request.Context(), c.Query("reg"))
	if err != nil {
		if trademarks.IsNotFound(err) {
			notFound(c)
			return
		}
		internalError(c)
		return
	}
	c.Header("Cache-Control", "private, max-age=86400")
	c.Data(http.StatusOK, mime, data)
}

func (s *Server) checkTrademarks(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4<<20)
	var input struct {
		Rows []trademarks.CheckRow `json:"rows"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, "Некорректный запрос", "rows")
		return
	}
	results, err := s.trademarks.Check(c.Request.Context(), input.Rows)
	if err != nil {
		if validation, ok := err.(*service.ValidationError); ok {
			badRequest(c, validation.Message, validation.Field)
			return
		}
		internalError(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"results": results})
}

func (s *Server) importTrademarks(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 81<<20)
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		badRequest(c, "Выберите файл реестра .xls или .xlsx размером до 80 МБ", "file")
		return
	}
	defer file.Close()
	result, err := s.trademarks.Import(c.Request.Context(), currentUser(c).ID, file, header.Size)
	if err != nil {
		if validation, ok := err.(*service.ValidationError); ok {
			badRequest(c, validation.Message, validation.Field)
			return
		}
		internalError(c)
		return
	}
	s.saveDataUpdate(c, "trademarks", result)
	c.JSON(http.StatusOK, result)
}

func (s *Server) setTrademarkLogoName(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8<<10)
	var input struct {
		RegNo string `json:"regNo"`
		Name  string `json:"name"`
	}
	if err := c.ShouldBindJSON(&input); err != nil || input.RegNo == "" {
		badRequest(c, "Некорректный запрос", "regNo")
		return
	}
	if err := s.trademarks.SetLogoName(c.Request.Context(), input.RegNo, input.Name); err != nil {
		if trademarks.IsNotFound(err) {
			notFound(c)
			return
		}
		internalError(c)
		return
	}
	c.Status(http.StatusNoContent)
}
