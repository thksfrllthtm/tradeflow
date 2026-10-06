package httpapi

// Product directory: product cards with their TN VED code (list, create, update, archive, restore, copy).

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/directory"
	"github.com/gin-gonic/gin"
)

func (s *Server) listProductDirectory(c *gin.Context) {
	entries, err := s.productDirectory.List(c.Request.Context(), directory.ProductDirectoryFilter{
		Query: c.Query("q"), Category: c.Query("category"), Status: c.Query("status"), IncludeArchived: c.Query("includeArchived") == "true",
	})
	if !s.handleDirectoryError(c, err) {
		return
	}
	items := make([]gin.H, 0, len(entries))
	for _, entry := range entries {
		items = append(items, productDirectoryResponse(entry))
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (s *Server) createProductDirectory(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	var input directory.ProductDirectoryInput
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, "Проверьте поля карточки товара", "entry")
		return
	}
	entry, err := s.productDirectory.Create(c.Request.Context(), currentUser(c).ID, input)
	if !s.handleProductDirectoryError(c, err) {
		return
	}
	c.JSON(http.StatusCreated, gin.H{"item": productDirectoryResponse(entry)})
}

func (s *Server) updateProductDirectory(c *gin.Context) {
	id, ok := directoryEntryID(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	var input directory.ProductDirectoryInput
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, "Проверьте поля карточки товара", "entry")
		return
	}
	entry, err := s.productDirectory.Update(c.Request.Context(), id, currentUser(c).ID, input)
	if !s.handleProductDirectoryError(c, err) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"item": productDirectoryResponse(entry)})
}

func (s *Server) archiveProductDirectory(c *gin.Context) {
	id, ok := directoryEntryID(c)
	if !ok {
		return
	}
	if err := s.productDirectory.Archive(c.Request.Context(), id, currentUser(c).ID); !s.handleProductDirectoryError(c, err) {
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) restoreProductDirectory(c *gin.Context) {
	id, ok := directoryEntryID(c)
	if !ok {
		return
	}
	if err := s.productDirectory.Restore(c.Request.Context(), id, currentUser(c).ID); !s.handleProductDirectoryError(c, err) {
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) copyProductDirectory(c *gin.Context) {
	id, ok := directoryEntryID(c)
	if !ok {
		return
	}
	entry, err := s.productDirectory.Copy(c.Request.Context(), id, currentUser(c).ID)
	if !s.handleProductDirectoryError(c, err) {
		return
	}
	c.JSON(http.StatusCreated, gin.H{"item": productDirectoryResponse(entry)})
}

func productDirectoryResponse(entry repository.ProductDirectoryEntry) gin.H {
	return gin.H{
		"id": entry.ID, "category": entry.Category, "name": entry.Name, "nameZh": entry.NameZH, "unit": entry.Unit,
		"companyCoefficient": decimalString(entry.CompanyCoefficient),
		"synonymsRu":         directoryStrings(entry.SynonymsRU), "synonymsZh": directoryStrings(entry.SynonymsZH), "synonymsEn": directoryStrings(entry.SynonymsEN),
		"code": entry.Code, "verificationStatus": entry.VerificationStatus,
		"archivedAt": dateTimePtrString(entry.ArchivedAt), "archivedBy": entry.ArchivedBy, "createdBy": entry.CreatedBy, "updatedBy": entry.UpdatedBy,
		"createdAt": entry.CreatedAt.UTC().Format(time.RFC3339), "updatedAt": entry.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func directoryEntryID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		badRequest(c, "Некорректный идентификатор записи", "id")
		return 0, false
	}
	return id, true
}

func directoryStrings(raw repository.JSON) []string {
	var values []string
	if len(raw) > 0 && string(raw) != "null" {
		_ = json.Unmarshal(raw, &values)
	}
	return values
}

func dateTimePtrString(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.UTC().Format(time.RFC3339)
}

func (s *Server) handleDirectoryError(c *gin.Context, err error) bool {
	if err == nil {
		return true
	}
	if service.IsNotFound(err) {
		notFound(c)
		return false
	}
	if validation, ok := err.(*service.ValidationError); ok {
		badRequest(c, validation.Message, validation.Field)
		return false
	}
	internalError(c)
	return false
}

func (s *Server) handleProductDirectoryError(c *gin.Context, err error) bool {
	if err == nil {
		return true
	}
	if service.IsNotFound(err) {
		notFound(c)
		return false
	}
	if errors.Is(err, directory.ErrProductDirectoryConflict) {
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{"message": "Такая карточка товара уже существует. Измените наименование или откройте существующую запись", "code": "product_directory_duplicate"}})
		return false
	}
	if validation, ok := err.(*service.ValidationError); ok {
		badRequest(c, validation.Message, validation.Field)
		return false
	}
	internalError(c)
	return false
}
