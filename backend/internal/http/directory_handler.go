package httpapi

// Operator directories: TN VED entries and product cards (list, create, update, archive, restore, copy).

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/shopspring/decimal"
)

func (s *Server) listTNVEDDirectory(c *gin.Context) {
	entries, err := s.tnvedDirectory.List(c.Request.Context(), service.TNVEDDirectoryFilter{
		Query: c.Query("q"), Category: c.Query("category"), Status: c.Query("status"), IncludeArchived: c.Query("includeArchived") == "true",
	})
	if !s.handleDirectoryError(c, err) {
		return
	}
	items := make([]gin.H, 0, len(entries))
	for _, entry := range entries {
		items = append(items, tnvedDirectoryResponse(entry))
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (s *Server) createTNVEDDirectory(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	var input service.TNVEDDirectoryInput
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, "Проверьте поля записи справочника", "entry")
		return
	}
	entry, err := s.tnvedDirectory.Create(c.Request.Context(), currentUser(c).ID, input)
	if !s.handleDirectoryError(c, err) {
		return
	}
	c.JSON(http.StatusCreated, gin.H{"item": tnvedDirectoryResponse(entry)})
}

func (s *Server) updateTNVEDDirectory(c *gin.Context) {
	id, ok := directoryEntryID(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	var input service.TNVEDDirectoryInput
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, "Проверьте поля записи справочника", "entry")
		return
	}
	entry, err := s.tnvedDirectory.Update(c.Request.Context(), id, currentUser(c).ID, input)
	if !s.handleDirectoryError(c, err) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"item": tnvedDirectoryResponse(entry)})
}

func (s *Server) archiveTNVEDDirectory(c *gin.Context) {
	id, ok := directoryEntryID(c)
	if !ok {
		return
	}
	if err := s.tnvedDirectory.Archive(c.Request.Context(), id, currentUser(c).ID); !s.handleDirectoryError(c, err) {
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) restoreTNVEDDirectory(c *gin.Context) {
	id, ok := directoryEntryID(c)
	if !ok {
		return
	}
	if err := s.tnvedDirectory.Restore(c.Request.Context(), id, currentUser(c).ID); !s.handleDirectoryError(c, err) {
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) copyTNVEDDirectory(c *gin.Context) {
	id, ok := directoryEntryID(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8<<10)
	var input struct {
		Name string `json:"name"`
	}
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&input); err != nil {
			badRequest(c, "Проверьте название копии", "name")
			return
		}
	}
	entry, err := s.tnvedDirectory.Copy(c.Request.Context(), id, currentUser(c).ID, input.Name)
	if !s.handleDirectoryError(c, err) {
		return
	}
	c.JSON(http.StatusCreated, gin.H{"item": tnvedDirectoryResponse(entry)})
}

func (s *Server) listProductDirectory(c *gin.Context) {
	entries, err := s.productDirectory.List(c.Request.Context(), service.ProductDirectoryFilter{
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
	var input service.ProductDirectoryInput
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
	var input service.ProductDirectoryInput
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
	item := gin.H{
		"id": entry.ID, "category": entry.Category, "name": entry.Name, "nameZh": entry.NameZH, "unit": entry.Unit,
		"companyCoefficient": decimalString(entry.CompanyCoefficient),
		"synonymsRu":         directoryStrings(entry.SynonymsRU), "synonymsZh": directoryStrings(entry.SynonymsZH), "synonymsEn": directoryStrings(entry.SynonymsEN),
		"tnvedDirectoryEntryId": entry.TNVEDDirectoryEntryID, "verificationStatus": entry.VerificationStatus,
		"archivedAt": dateTimePtrString(entry.ArchivedAt), "archivedBy": entry.ArchivedBy, "createdBy": entry.CreatedBy, "updatedBy": entry.UpdatedBy,
		"createdAt": entry.CreatedAt.UTC().Format(time.RFC3339), "updatedAt": entry.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if linked := entry.TNVEDDirectoryEntry; linked != nil {
		item["tnvedCode"] = linked.Code
		item["tnvedName"] = linked.Name
		item["tnvedNameZh"] = linked.NameZH
		item["antiDumpingPercent"] = decimalString(linked.AntiDumpingPercent)
		item["tnved"] = tnvedDirectoryResponse(*linked)
	} else {
		item["tnvedCode"] = ""
		item["tnvedName"] = ""
		item["tnvedNameZh"] = ""
		item["antiDumpingPercent"] = ""
	}
	return item
}

func directoryEntryID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		badRequest(c, "Некорректный идентификатор записи", "id")
		return 0, false
	}
	return id, true
}

func tnvedDirectoryResponse(entry repository.TNVEDDirectoryEntry) gin.H {
	return gin.H{
		"id": entry.ID, "category": entry.Category, "name": entry.Name, "nameZh": entry.NameZH, "code": entry.Code, "companyCoefficient": decimalString(entry.CompanyCoefficient), "antiDumpingPercent": decimalString(entry.AntiDumpingPercent),
		"entrySource": "operator",
		"synonymsRu":  directoryStrings(entry.SynonymsRU), "synonymsZh": directoryStrings(entry.SynonymsZH), "synonymsEn": directoryStrings(entry.SynonymsEN),
		"verificationStatus": entry.VerificationStatus,
		"duty":               directoryRateResponse(entry.DutyType, entry.DutyPercent, entry.DutyAmount, entry.DutyCurrency, entry.DutyUnit, entry.DutyValidFrom, entry.DutyValidTo, entry.DutyCheckedAt, entry.DutySource),
		"vat":                vatDirectoryResponse(entry),
		"archivedAt":         dateTimePtrString(entry.ArchivedAt), "archivedBy": entry.ArchivedBy, "createdBy": entry.CreatedBy, "updatedBy": entry.UpdatedBy, "createdAt": entry.CreatedAt.UTC().Format(time.RFC3339), "updatedAt": entry.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func directoryStrings(raw repository.JSON) []string {
	var values []string
	if len(raw) > 0 && string(raw) != "null" {
		_ = json.Unmarshal(raw, &values)
	}
	return values
}

func directoryRateResponse(kind string, percent, amount *decimal.Decimal, currency, unit string, from, to, checkedAt *time.Time, source string) gin.H {
	return gin.H{"type": kind, "percent": decimalString(percent), "amount": decimalString(amount), "currency": currency, "unit": unit, "validFrom": datePtrString(from), "validTo": datePtrString(to), "checkedAt": datePtrString(checkedAt), "source": source, "sourceKind": "operator"}
}

func vatDirectoryResponse(entry repository.TNVEDDirectoryEntry) gin.H {
	return gin.H{"type": entry.VATType, "percent": decimalString(entry.VATPercent), "basis": entry.VATBasis, "validFrom": datePtrString(entry.VATValidFrom), "validTo": datePtrString(entry.VATValidTo), "checkedAt": datePtrString(entry.VATCheckedAt), "source": entry.VATSource, "sourceKind": "operator"}
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
	if errors.Is(err, service.ErrTNVEDDirectoryConflict) {
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{"message": "Такая запись уже существует. Измените название или откройте существующую запись", "code": "directory_duplicate"}})
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
	if errors.Is(err, service.ErrProductDirectoryConflict) {
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{"message": "Такая карточка товара уже существует. Измените наименование или откройте существующую запись", "code": "product_directory_duplicate"}})
		return false
	}
	if errors.Is(err, service.ErrTNVEDDirectoryConflict) {
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{"message": "Такая запись уже существует", "code": "directory_duplicate"}})
		return false
	}
	if validation, ok := err.(*service.ValidationError); ok {
		badRequest(c, validation.Message, validation.Field)
		return false
	}
	internalError(c)
	return false
}

func datePtrString(v *time.Time) string {
	if v == nil {
		return ""
	}
	return v.Format("2006-01-02")
}
