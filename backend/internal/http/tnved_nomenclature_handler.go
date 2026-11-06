package httpapi

// Full TN VED EAEU nomenclature: every employee searches it, the administrator replaces it
// with a fresh TWS.BY Excel export.

import (
	"net/http"
	"strconv"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/gin-gonic/gin"
)

func (s *Server) listTNVEDNomenclature(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	items, total, err := s.tnvedNomenclature.List(c.Request.Context(), c.Query("q"), limit, offset)
	if err != nil {
		internalError(c)
		return
	}
	info, err := s.tnvedNomenclature.LatestImport(c.Request.Context())
	if err != nil {
		internalError(c)
		return
	}
	kzInfo, err := s.kzRates.LatestImport(c.Request.Context())
	if err != nil {
		internalError(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": total, "import": info, "kzImport": kzInfo})
}

// lookupTNVEDNomenclature finds a pasted column of codes: every employee checks codes before
// an estimate. Codes the nomenclature does not have are absent from items.
func (s *Server) lookupTNVEDNomenclature(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 256<<10)
	var input struct {
		Codes []string `json:"codes"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, "Некорректный список кодов", "codes")
		return
	}
	items, err := s.tnvedNomenclature.Lookup(c.Request.Context(), input.Codes)
	if err != nil {
		if validation, ok := err.(*service.ValidationError); ok {
			badRequest(c, validation.Message, validation.Field)
			return
		}
		internalError(c)
		return
	}
	info, err := s.tnvedNomenclature.LatestImport(c.Request.Context())
	if err != nil {
		internalError(c)
		return
	}
	kzInfo, err := s.kzRates.LatestImport(c.Request.Context())
	if err != nil {
		internalError(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "import": info, "kzImport": kzInfo})
}

// importTNVEDNomenclature answers 200 with the import summary, or 400 with the rows to fix
// (nothing saved).
func (s *Server) importTNVEDNomenclature(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 21<<20)
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		badRequest(c, "Выберите XLSX-файл размером до 20 МБ", "file")
		return
	}
	defer file.Close()
	result, err := s.tnvedNomenclature.Import(c.Request.Context(), currentUser(c).ID, header.Filename, file)
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
	s.saveDataUpdate(c, "nomenclature", result)
	c.JSON(http.StatusOK, result)
}
