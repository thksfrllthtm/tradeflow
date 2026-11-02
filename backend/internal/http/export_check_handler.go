package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/exportcheck"
	"github.com/gin-gonic/gin"
)

// exportCheck — сверка инвойса с экспортной декларацией: multipart с файлом инвойса Excel
// («invoice») и JSON деклараций («declarations»), разобранных браузером из PDF.
func (s *Server) exportCheck(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, exportcheck.MaxInvoiceBytes+(8<<20))
	file, header, err := c.Request.FormFile("invoice")
	if err != nil {
		badRequest(c, "Выберите файл инвойса Excel", "invoice")
		return
	}
	defer file.Close()
	if name := strings.ToLower(header.Filename); !strings.HasSuffix(name, ".xlsx") && !strings.HasSuffix(name, ".xls") {
		badRequest(c, "Инвойс должен быть файлом Excel .xlsx или .xls", "invoice")
		return
	}
	var declarations []exportcheck.Declaration
	if err := json.Unmarshal([]byte(c.PostForm("declarations")), &declarations); err != nil || len(declarations) == 0 {
		badRequest(c, "Загрузите хотя бы одну экспортную PDF", "declarations")
		return
	}
	invoice, err := exportcheck.ParseInvoice(file, header.Size, header.Filename)
	if err != nil {
		var valErr *service.ValidationError
		if errors.As(err, &valErr) {
			badRequest(c, valErr.Message, valErr.Field)
		} else {
			badRequest(c, "Не удалось прочитать инвойс: "+err.Error(), "invoice")
		}
		return
	}
	c.JSON(http.StatusOK, exportcheck.Compare(invoice, declarations))
}
