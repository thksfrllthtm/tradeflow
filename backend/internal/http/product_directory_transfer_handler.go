package httpapi

// Export and import of the TN VED and product directories as XLSX (administrator only).

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/directory"
	"github.com/gin-gonic/gin"
)

func sendDirectoryXLSX(c *gin.Context, name string, data []byte, err error) {
	if err != nil {
		internalError(c)
		return
	}
	filename := fmt.Sprintf("%s %s.xlsx", name, time.Now().Format("2006-01-02"))
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="directory.xlsx"; filename*=UTF-8''%s`, url.PathEscape(filename)))
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", data)
}

func (s *Server) exportProductDirectory(c *gin.Context) {
	data, err := s.productDirectory.Export(c.Request.Context())
	sendDirectoryXLSX(c, "Справочник товаров", data, err)
}

type directoryImporter func(ctx context.Context, userID int64, file io.Reader, size int64, update bool) (directory.DirectoryImportResult, error)

// importDirectory answers 200 with the counts, or 400 with the rows to fix (nothing saved).
func (s *Server) importDirectory(c *gin.Context, run directoryImporter) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 21<<20)
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		badRequest(c, "Выберите XLSX-файл размером до 20 МБ", "file")
		return
	}
	defer file.Close()
	result, err := run(c.Request.Context(), currentUser(c).ID, file, header.Size, c.Query("update") != "false")
	if err != nil {
		if validation, ok := err.(*service.ValidationError); ok {
			badRequest(c, validation.Message, validation.Field)
			return
		}
		internalError(c)
		return
	}
	if len(result.Errors) > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": gin.H{"message": "Файл не загружен: исправьте строки и загрузите снова", "field": "file"}, "result": result})
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) importProductDirectory(c *gin.Context) {
	s.importDirectory(c, s.productDirectory.Import)
}
