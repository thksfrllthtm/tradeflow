package httpapi

// Ceramics module: base of known marks, lookup by mark, AI-assisted analysis of unknown
// marks and the price of a type and size. Separate from estimates and the customs calculation.

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/ceramics"
	"github.com/gin-gonic/gin"
)

// handleCeramicsError answers validation, not-found and internal errors; true means no error.
func handleCeramicsError(c *gin.Context, err error, notFoundMessage string) bool {
	if err == nil {
		return true
	}
	var validation *service.ValidationError
	switch {
	case errors.As(err, &validation):
		badRequest(c, validation.Message, validation.Field)
	case service.IsNotFound(err):
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"message": notFoundMessage}})
	default:
		_ = c.Error(err)
		internalError(c)
	}
	return false
}

func ceramicsID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		badRequest(c, "Некорректный идентификатор", "id")
		return 0, false
	}
	return id, true
}

func (s *Server) ceramicsStatus(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"ai": s.ceramics.AIStatus()})
}

func (s *Server) listCeramicsTypes(c *gin.Context) {
	types, err := s.ceramics.Types(c.Request.Context())
	if !handleCeramicsError(c, err, "") {
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": types})
}

func (s *Server) listCeramicsProducts(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	items, total, err := s.ceramics.List(c.Request.Context(), c.Query("q"), c.Query("type"), limit, offset)
	if !handleCeramicsError(c, err, "") {
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": total})
}

func (s *Server) createCeramicsProduct(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	var input ceramics.CeramicsProductInput
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, "Проверьте поля записи", "mark")
		return
	}
	item, err := s.ceramics.Create(c.Request.Context(), currentUser(c).ID, input)
	if !handleCeramicsError(c, err, "") {
		return
	}
	c.JSON(http.StatusCreated, gin.H{"item": item})
}

func (s *Server) updateCeramicsProduct(c *gin.Context) {
	id, ok := ceramicsID(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	var input ceramics.CeramicsProductInput
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, "Проверьте поля записи", "mark")
		return
	}
	item, err := s.ceramics.Update(c.Request.Context(), id, currentUser(c).ID, input)
	if !handleCeramicsError(c, err, "Запись не найдена") {
		return
	}
	c.JSON(http.StatusOK, gin.H{"item": item})
}

func (s *Server) deleteCeramicsProduct(c *gin.Context) {
	id, ok := ceramicsID(c)
	if !ok {
		return
	}
	if !handleCeramicsError(c, s.ceramics.Delete(c.Request.Context(), id), "Запись не найдена") {
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) importCeramicsProducts(c *gin.Context) {
	// Up to 5000 rows of name, mark and price.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8<<20)
	var input struct {
		Items     []ceramics.CeramicsProductInput `json:"items"`
		Overwrite bool                            `json:"overwrite"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, "Проверьте строки для загрузки", "items")
		return
	}
	result, err := s.ceramics.Import(c.Request.Context(), currentUser(c).ID, input.Items, input.Overwrite)
	if !handleCeramicsError(c, err, "") {
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) lookupCeramicsMarks(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
	var input struct {
		Marks []string `json:"marks"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, "Некорректный список марок", "marks")
		return
	}
	items, err := s.ceramics.Lookup(c.Request.Context(), input.Marks)
	if !handleCeramicsError(c, err, "") {
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (s *Server) analyzeCeramics(c *gin.Context) {
	// A few products with photos compressed for the model.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4<<20)
	var input struct {
		Items []ceramics.CeramicsInputItem `json:"items"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, "Некорректные товары для подбора", "items")
		return
	}
	items, err := s.ceramics.Analyze(c.Request.Context(), input.Items)
	if !handleCeramicsError(c, err, "") {
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (s *Server) ceramicsPrice(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	var input struct {
		ProductType string `json:"productType"`
		SizeValue   string `json:"sizeValue"`
		SizeUnit    string `json:"sizeUnit"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, "Укажите тип и размер", "productType")
		return
	}
	price, err := s.ceramics.Price(c.Request.Context(), input.ProductType, input.SizeValue, input.SizeUnit)
	if !handleCeramicsError(c, err, "") {
		return
	}
	c.JSON(http.StatusOK, gin.H{"price": price})
}
