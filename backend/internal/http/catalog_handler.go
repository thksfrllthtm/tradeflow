package httpapi

// TN VED classifier catalog: search, code details, applicable rules and catalog import.

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/gin-gonic/gin"
)

func (s *Server) searchCatalog(c *gin.Context) {
	rows, err := s.catalog.Search(c.Request.Context(), c.Query("q"), c.Query("date"), c.Query("includeDemo") == "true")
	if !s.handleImportError(c, err) {
		return
	}
	result := make([]gin.H, 0, len(rows))
	for _, row := range rows {
		result = append(result, catalogCodeResponse(row.Code, row.Version))
	}
	c.JSON(http.StatusOK, gin.H{"items": result})
}

func (s *Server) catalogDetails(c *gin.Context) {
	items, err := s.catalog.Details(c.Request.Context(), c.Param("code"), c.Query("date"), c.Query("includeDemo") == "true")
	if !s.handleImportError(c, err) {
		return
	}
	result := make([]gin.H, 0, len(items))
	for _, item := range items {
		h := catalogCodeResponse(item.Code, item.Version)
		h["documents"] = item.Documents
		h["rules"] = item.Rules
		result = append(result, h)
	}
	c.JSON(http.StatusOK, gin.H{"items": result})
}

func (s *Server) applicableRules(c *gin.Context) {
	var input service.ApplicableInput
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, "Проверьте код, дату и условия партии", "catalog")
		return
	}
	result, err := s.catalog.Applicable(c.Request.Context(), input)
	if !s.handleImportError(c, err) {
		return
	}
	c.JSON(http.StatusOK, result)
}

func (s *Server) catalogImportPreview(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<20)
	var input service.CatalogImportRequest
	if c.ContentType() == "text/csv" {
		parsed, err := service.ParseCatalogCSV(c.Request.Body)
		if err != nil {
			badRequest(c, err.Error(), "catalog")
			return
		}
		input = parsed
	} else if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, "Проверьте JSON-контракт каталога", "catalog")
		return
	}
	report, _ := s.catalog.Validate(input)
	c.JSON(http.StatusOK, report)
}

func (s *Server) catalogImportApply(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<20)
	var input service.CatalogImportRequest
	if c.ContentType() == "text/csv" {
		parsed, err := service.ParseCatalogCSV(c.Request.Body)
		if err != nil {
			badRequest(c, err.Error(), "catalog")
			return
		}
		input = parsed
	} else if err := json.NewDecoder(io.LimitReader(c.Request.Body, 16<<20)).Decode(&input); err != nil {
		badRequest(c, "Проверьте JSON-контракт каталога", "catalog")
		return
	}
	v, report, err := s.catalog.Apply(c.Request.Context(), input)
	if err != nil {
		if !s.handleImportError(c, err) {
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"version": v, "report": report})
}

func catalogCodeResponse(code repository.ClassifierCode, version repository.ClassifierVersion) gin.H {
	return gin.H{"code": code.Code, "hierarchyLevel": code.HierarchyLevel, "parentCode": code.ParentCode, "officialDescription": code.OfficialDescription, "isGroup": code.IsGroup, "kind": map[bool]string{true: "group", false: "final"}[code.IsGroup], "validFrom": code.ValidFrom.Format("2006-01-02"), "validTo": datePtrString(code.ValidTo), "source": code.Source, "versionKey": version.VersionKey, "datasetKind": version.DatasetKind, "versionSource": version.Source, "versionSourceDate": datePtrString(version.SourceDate), "loadedAt": version.LoadedAt.UTC().Format(time.RFC3339)}
}

func datePtrString(v *time.Time) string {
	if v == nil {
		return ""
	}
	return v.Format("2006-01-02")
}
