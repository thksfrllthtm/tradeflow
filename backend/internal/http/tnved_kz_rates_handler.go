package httpapi

// «Перечень РК» (administrator): Excel from the rows the browser read out of the EEC PDF,
// and loading the list from those rows or from an Excel file of the same shape.

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/directory"
	"github.com/gin-gonic/gin"
)

// About 4 000 rows with names and footnote texts.
const maxKZExtractBody = 8 << 20

func bindKZExtract(c *gin.Context) (directory.KZListExtract, bool) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxKZExtractBody)
	var extract directory.KZListExtract
	if err := c.ShouldBindJSON(&extract); err != nil {
		badRequest(c, "Не удалось прочитать строки перечня", "file")
		return extract, false
	}
	return extract, true
}

func kzRowsError(c *gin.Context, errors []directory.DirectoryImportError) {
	c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "В перечне есть строки с ошибками — ничего не сохранено", "field": "file"}, "result": gin.H{"errors": errors}})
}

func handleKZError(c *gin.Context, err error) bool {
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

func (s *Server) kzRatesExcel(c *gin.Context) {
	extract, ok := bindKZExtract(c)
	if !ok {
		return
	}
	data, rowErrors, err := s.kzRates.Excel(c.Request.Context(), extract)
	if !handleKZError(c, err) {
		return
	}
	if len(rowErrors) > 0 {
		kzRowsError(c, rowErrors)
		return
	}
	name := strings.TrimSuffix(extract.FileName, ".pdf")
	if strings.TrimSpace(name) == "" {
		name = "Перечень РК " + time.Now().Format("2006-01-02")
	}
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="kz-list.xlsx"; filename*=UTF-8''%s`, url.PathEscape(name+".xlsx")))
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", data)
}

func (s *Server) importKZRatesPDF(c *gin.Context) {
	extract, ok := bindKZExtract(c)
	if !ok {
		return
	}
	result, err := s.kzRates.ImportExtract(c.Request.Context(), currentUser(c).ID, extract)
	if !handleKZError(c, err) {
		return
	}
	if len(result.Errors) > 0 {
		kzRowsError(c, result.Errors)
		return
	}
	s.saveDataUpdate(c, "kz", result)
	c.JSON(http.StatusOK, result)
}

func (s *Server) importKZRatesExcel(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 21<<20)
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		badRequest(c, "Выберите XLSX-файл размером до 20 МБ", "file")
		return
	}
	defer file.Close()
	result, err := s.kzRates.ImportExcel(c.Request.Context(), currentUser(c).ID, header.Filename, file, header.Size)
	if !handleKZError(c, err) {
		return
	}
	if len(result.Errors) > 0 {
		kzRowsError(c, result.Errors)
		return
	}
	s.saveDataUpdate(c, "kz", result)
	c.JSON(http.StatusOK, result)
}
