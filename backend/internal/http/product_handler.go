package httpapi

// Imported products: editing, images, AI classification dialogue and applying TN VED codes.

import (
	"errors"
	"mime"
	"net/http"
	"strconv"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/gin-gonic/gin"
)

func (s *Server) listProducts(c *gin.Context) {
	estimateID, ok := estimateID(c)
	if !ok {
		return
	}
	products, err := s.imports.ListProducts(c.Request.Context(), estimateID, currentUser(c).ID)
	if !s.handleImportError(c, err) {
		return
	}
	response := make([]gin.H, 0, len(products))
	for _, product := range products {
		response = append(response, productResponse(product))
	}
	c.JSON(http.StatusOK, gin.H{"products": response})
}

func (s *Server) updateProduct(c *gin.Context) {
	productID, ok := productID(c)
	if !ok {
		return
	}
	var input service.ProductInput
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, "Проверьте данные позиции", "")
		return
	}
	product, err := s.imports.UpdateProduct(c.Request.Context(), productID, currentUser(c).ID, input)
	if !s.handleImportError(c, err) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"product": productResponse(product)})
}

func (s *Server) analyzeProduct(c *gin.Context) {
	productID, ok := productID(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 36<<10)
	var input struct {
		Extra string `json:"extra"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		var sizeErr *http.MaxBytesError
		if errors.As(err, &sizeErr) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": gin.H{"message": "Характеристики товара слишком большие", "code": "ai_request_too_large"}})
			return
		}
		badRequest(c, "Укажите дополнительные характеристики в JSON", "extra")
		return
	}
	result, err := s.ai.AnalyzeProduct(c.Request.Context(), productID, currentUser(c).ID, input.Extra)
	if !s.handleAIError(c, err) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"analysis": result})
}

func (s *Server) latestProductAIAnalysis(c *gin.Context) {
	productID, ok := productID(c)
	if !ok {
		return
	}
	result, err := s.ai.Latest(c.Request.Context(), productID, currentUser(c).ID)
	if service.IsNotFound(err) {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"message": "Для товара ещё нет анализа ИИ", "code": "analysis_not_found"}})
		return
	}
	if !s.handleAIError(c, err) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"analysis": result})
}

func (s *Server) getProductAIDialogue(c *gin.Context) {
	productID, ok := productID(c)
	if !ok {
		return
	}
	dialogue, err := s.ai.ProductAIDialogue(c.Request.Context(), productID, currentUser(c).ID)
	if !s.handleAIError(c, err) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"dialogue": dialogue})
}

func (s *Server) sendProductAIMessage(c *gin.Context) {
	productID, ok := productID(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16<<10)
	var input struct {
		Message            string  `json:"message"`
		ClientRequestID    string  `json:"clientRequestId"`
		AttachmentAssetIDs []int64 `json:"attachmentAssetIds"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		var sizeErr *http.MaxBytesError
		if errors.As(err, &sizeErr) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": gin.H{"message": "Сообщение слишком большое", "code": "ai_request_too_large"}})
			return
		}
		badRequest(c, "Укажите сообщение в JSON", "message")
		return
	}
	dialogue, err := s.ai.SendProductAIMessage(c.Request.Context(), productID, currentUser(c).ID, input.Message, input.ClientRequestID, input.AttachmentAssetIDs)
	if !s.handleAIError(c, err) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"dialogue": dialogue})
}

func (s *Server) applyAIChanges(c *gin.Context) {
	productID, ok := productID(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 24<<10)
	var input struct {
		Changes []service.AIProposedChange `json:"changes"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		var sizeErr *http.MaxBytesError
		if errors.As(err, &sizeErr) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": gin.H{"message": "Запрос слишком большой", "code": "ai_request_too_large"}})
			return
		}
		badRequest(c, "Укажите изменения в JSON", "changes")
		return
	}
	product, err := s.ai.ApplyChatChanges(c.Request.Context(), productID, currentUser(c).ID, input.Changes)
	if !s.handleAIError(c, err) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"product": productResponse(product)})
}

func (s *Server) applyProductTNVED(c *gin.Context) {
	productID, ok := productID(c)
	if !ok {
		return
	}
	var input struct {
		Code string `json:"code"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, "Укажите код ТН ВЭД строкой", "code")
		return
	}
	product, err := s.ai.ApplyCode(c.Request.Context(), productID, currentUser(c).ID, input.Code)
	if !s.handleAIError(c, err) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"product": productResponse(product)})
}

func (s *Server) applyProductTNVEDDirectory(c *gin.Context) {
	productID, ok := productID(c)
	if !ok {
		return
	}
	var input struct {
		EntryID int64 `json:"entryId"`
	}
	if err := c.ShouldBindJSON(&input); err != nil || input.EntryID < 1 {
		badRequest(c, "Укажите запись справочника", "entryId")
		return
	}
	product, err := s.tnvedDirectory.ApplyToProduct(c.Request.Context(), productID, currentUser(c).ID, input.EntryID)
	if !s.handleDirectoryError(c, err) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"product": productResponse(product)})
}

func (s *Server) uploadProductImage(c *gin.Context) {
	productID, ok := productID(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, service.MaxImageBytes+1024*1024)
	file, err := c.FormFile("image")
	if err != nil {
		badRequest(c, "Выберите JPG или PNG размером до 5 МБ", "image")
		return
	}
	asset, err := s.imports.UploadManualImage(c.Request.Context(), productID, currentUser(c).ID, file)
	if !s.handleImportError(c, err) {
		return
	}
	c.JSON(http.StatusCreated, gin.H{"image": assetResponse(asset)})
}

func (s *Server) getProductImage(c *gin.Context) {
	productID, ok := productID(c)
	if !ok {
		return
	}
	assetID, err := strconv.ParseInt(c.Param("assetID"), 10, 64)
	if err != nil || assetID < 1 {
		notFound(c)
		return
	}
	file, asset, err := s.imports.OpenProductAsset(c.Request.Context(), productID, assetID, currentUser(c).ID)
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
		contentType = mime.TypeByExtension(".bin")
	}
	c.DataFromReader(http.StatusOK, asset.FileSize, contentType, file, nil)
}

func productID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		badRequest(c, "Некорректный идентификатор позиции", "")
		return 0, false
	}
	return id, true
}

func productResponse(product repository.ImportedProduct) gin.H {
	return gin.H{
		"id": product.ID, "sourceImportId": product.SourceImportID, "sourceSheet": product.SourceSheet, "sourceRow": product.SourceRow, "sourceValues": product.SourceValues,
		"name": product.Name, "translation": product.Translation, "sku": product.SKU, "quantity": decimalString(product.Quantity), "quantityUnit": product.QuantityUnit, "pairs": decimalString(product.Pairs), "places": decimalString(product.Places), "pairsPerPlace": decimalString(product.PairsPerPlace), "unitPrice": decimalString(product.UnitPrice), "lineTotal": decimalString(product.LineTotal), "currency": product.Currency,
		"weight": decimalString(product.Weight), "grossWeight": decimalString(product.GrossWeight), "grossWeightUnit": product.GrossWeightUnit, "grossWeightBasis": product.GrossWeightBasis, "netWeight": decimalString(product.NetWeight), "netWeightUnit": product.NetWeightUnit, "netWeightBasis": product.NetWeightBasis, "material": product.Material, "characteristics": product.Characteristics, "photoUrl": product.PhotoURL, "tnVedCode": product.TNVEDCode, "tnVedCodeSource": product.TNVEDCodeSource, "tnVedDirectoryEntryId": product.TNVEDDirectoryEntryID, "tnVedDirectorySnapshot": product.TNVEDDirectorySnapshot, "additionalFields": product.AdditionalFields, "priceBasis": product.PriceBasis, "priceUnitLabel": product.PriceUnitLabel, "weightBasis": product.WeightBasis,
		"imageAssetIds": product.ImageAssetIDs, "importStatus": product.ImportStatus, "warnings": product.Warnings, "excluded": product.Excluded, "exclusionReason": product.ExclusionReason, "aiDataRevision": product.AIDataRevision, "updatedAt": product.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

func (s *Server) handleAIError(c *gin.Context, err error) bool {
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
	status := http.StatusBadGateway
	code := "ai_error"
	switch {
	case errors.Is(err, service.ErrGeminiNotConfigured):
		status, code = http.StatusServiceUnavailable, "ai_provider_not_configured"
	case errors.Is(err, service.ErrGeminiRateLimited):
		status, code = http.StatusTooManyRequests, "ai_rate_limited"
	case errors.Is(err, service.ErrGeminiTimeout):
		status, code = http.StatusGatewayTimeout, "ai_timeout"
	case errors.Is(err, service.ErrAIRequestTooLarge):
		status, code = http.StatusRequestEntityTooLarge, "ai_request_too_large"
	case errors.Is(err, service.ErrGeminiUnauthorized):
		status, code = http.StatusBadGateway, "ai_unauthorized"
	case errors.Is(err, service.ErrGeminiRequestRejected):
		status, code = http.StatusBadGateway, "ai_request_rejected"
	case errors.Is(err, service.ErrGeminiModelUnavailable):
		status, code = http.StatusBadGateway, "ai_model_unavailable"
	case errors.Is(err, service.ErrGeminiBadResponse):
		status, code = http.StatusBadGateway, "ai_bad_response"
	case errors.Is(err, service.ErrAIUnavailable):
		status, code = http.StatusBadGateway, "ai_unavailable"
	default:
		status, code = http.StatusInternalServerError, "ai_error"
	}
	c.JSON(status, gin.H{"error": gin.H{"message": service.AIErrorMessage(err), "code": code}})
	return false
}
