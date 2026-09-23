package httpapi

// XLSX imports of an estimate and the import workspace (preview, confirm, workspace editing and images).

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/gin-gonic/gin"
)

func (s *Server) uploadImport(c *gin.Context) {
	estimateID, ok := estimateID(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, service.MaxXLSXBytes+1024*1024)
	file, err := c.FormFile("file")
	if err != nil {
		badRequest(c, "Выберите XLSX-файл размером до 50 МБ", "file")
		return
	}
	result, err := s.imports.Upload(c.Request.Context(), estimateID, currentUser(c).ID, file)
	if errors.Is(err, service.ErrDuplicateImport) {
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{"message": "Такой файл уже импортировался в эту смету. Выберите замену или отмените импорт.", "code": "duplicate_import"}, "duplicateImport": importResponse(result.Import), "sheets": result.Sheets})
		return
	}
	if !s.handleImportError(c, err) {
		return
	}
	c.JSON(http.StatusCreated, gin.H{"import": importResponse(result.Import), "sheets": result.Sheets})
}

func (s *Server) latestImport(c *gin.Context) {
	estimateID, ok := estimateID(c)
	if !ok {
		return
	}
	item, err := s.imports.LatestImport(c.Request.Context(), estimateID, currentUser(c).ID)
	if service.IsNotFound(err) {
		notFound(c)
		return
	}
	if !s.handleImportError(c, err) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"import": importResponse(item)})
}

func (s *Server) previewImport(c *gin.Context) {
	importID, ok := importID(c)
	if !ok {
		return
	}
	headerRow, err := strconv.Atoi(c.DefaultQuery("headerRow", "1"))
	if err != nil {
		badRequest(c, "Некорректная строка заголовков", "headerRow")
		return
	}
	preview, err := s.imports.Preview(c.Request.Context(), importID, currentUser(c).ID, c.Query("sheet"), headerRow)
	if !s.handleImportError(c, err) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"preview": preview})
}

func (s *Server) confirmImport(c *gin.Context) {
	importID, ok := importID(c)
	if !ok {
		return
	}
	var input service.ConfirmInput
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, "Проверьте настройки импорта", "")
		return
	}
	products, err := s.imports.Confirm(c.Request.Context(), importID, currentUser(c).ID, input)
	if errors.Is(err, service.ErrReplacementRequired) {
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{"message": "В смете уже есть импортированные товары. Подтвердите явную замену.", "code": "replacement_required"}})
		return
	}
	if !s.handleImportError(c, err) {
		return
	}
	response := make([]gin.H, 0, len(products))
	for _, product := range products {
		response = append(response, productResponse(product))
	}
	c.JSON(http.StatusOK, gin.H{"products": response})
}

func (s *Server) getWorkspace(c *gin.Context) {
	importID, ok := importID(c)
	if !ok {
		return
	}
	headerRow, err := strconv.Atoi(c.DefaultQuery("headerRow", "1"))
	if err != nil {
		badRequest(c, "Некорректная строка заголовков", "headerRow")
		return
	}
	dataStartRow, err := strconv.Atoi(c.DefaultQuery("dataStartRow", "0"))
	if err != nil {
		badRequest(c, "Некорректное начало данных", "dataStartRow")
		return
	}
	workspace, err := s.imports.GetWorkspace(c.Request.Context(), importID, currentUser(c).ID, c.Query("sheet"), headerRow, dataStartRow)
	if !s.handleImportError(c, err) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"workspace": workspace})
}

func (s *Server) saveWorkspace(c *gin.Context) {
	importID, ok := importID(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32<<20)
	var workspace service.Workspace
	if err := c.ShouldBindJSON(&workspace); err != nil {
		badRequest(c, "Проверьте рабочую таблицу", "workspace")
		return
	}
	saved, err := s.imports.SaveWorkspace(c.Request.Context(), importID, currentUser(c).ID, workspace)
	if !s.handleImportError(c, err) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"workspace": saved})
}

func (s *Server) applyWorkspace(c *gin.Context) {
	importID, ok := importID(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 32<<20)
	var workspace service.Workspace
	if err := c.ShouldBindJSON(&workspace); err != nil {
		badRequest(c, "Проверьте рабочую таблицу", "workspace")
		return
	}
	products, saved, err := s.imports.ApplyWorkspace(c.Request.Context(), importID, currentUser(c).ID, workspace)
	if errors.Is(err, service.ErrReplacementRequired) {
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{"message": "В смете уже есть импортированные товары. Подтвердите явную замену.", "code": "replacement_required"}})
		return
	}
	if !s.handleImportError(c, err) {
		return
	}
	response := make([]gin.H, 0, len(products))
	for _, product := range products {
		response = append(response, productResponse(product))
	}
	c.JSON(http.StatusOK, gin.H{"workspace": saved, "products": response})
}

func (s *Server) uploadWorkspaceImage(c *gin.Context) {
	importID, ok := importID(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, service.MaxImageBytes+1024*1024)
	file, err := c.FormFile("image")
	if err != nil {
		badRequest(c, "Выберите JPG или PNG размером до 5 МБ", "image")
		return
	}
	asset, err := s.imports.UploadWorkspaceImage(c.Request.Context(), importID, currentUser(c).ID, c.PostForm("rowId"), c.PostForm("replace") == "true", file)
	if !s.handleImportError(c, err) {
		return
	}
	c.JSON(http.StatusCreated, gin.H{"image": asset})
}

func (s *Server) getWorkspaceImage(c *gin.Context) {
	importID, ok := importID(c)
	if !ok {
		return
	}
	assetID, err := strconv.ParseInt(c.Param("assetID"), 10, 64)
	if err != nil || assetID < 1 {
		notFound(c)
		return
	}
	file, asset, err := s.imports.OpenWorkspaceAsset(c.Request.Context(), importID, assetID, currentUser(c).ID)
	if service.IsNotFound(err) {
		notFound(c)
		return
	}
	if err != nil {
		internalError(c)
		return
	}
	defer file.Close()
	contentType := asset.MIMEType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	c.DataFromReader(http.StatusOK, asset.FileSize, contentType, file, nil)
}

func importID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		badRequest(c, "Некорректный идентификатор импорта", "")
		return 0, false
	}
	return id, true
}

func importResponse(item repository.EstimateImport) gin.H {
	return gin.H{"id": item.ID, "originalFilename": item.OriginalFilename, "fileSize": item.FileSize, "status": item.Status, "createdAt": item.CreatedAt.UTC().Format(time.RFC3339)}
}

func assetResponse(asset repository.ImportAsset) gin.H {
	return gin.H{"id": asset.ID, "mimeType": asset.MIMEType, "fileSize": asset.FileSize, "kind": asset.Kind}
}

func (s *Server) handleImportError(c *gin.Context, err error) bool {
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
