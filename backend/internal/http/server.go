package httpapi

import (
	"net/http"
	"sync"

	"github.com/east-trans-logistic/import-calculator/backend/internal/config"
	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/gin-gonic/gin"
)

const userContextKey = "authenticated_user"

type Server struct {
	config           config.Config
	auth             *service.AuthService
	estimates        *service.EstimateService
	imports          *service.ImportService
	catalog          *service.CatalogService
	tnvedDirectory   *service.TNVEDDirectoryService
	productDirectory *service.ProductDirectoryService
	calculation      *service.CalculationService
	ai               *service.AIService
	gloves           *service.GlovesService
	goodsEstimates   *service.GoodsEstimateService
	authMu           sync.Mutex
	authRates        map[string]authRate
}

func NewServer(database *repository.Repository, cfg config.Config, files ...repository.FileStore) *Server {
	var store repository.FileStore
	if len(files) > 0 {
		store = files[0]
	}
	return &Server{
		config:           cfg,
		auth:             service.NewAuthService(database, cfg.SessionTTL),
		estimates:        service.NewEstimateService(database, store),
		imports:          service.NewImportService(database, store),
		catalog:          service.NewCatalogService(database),
		tnvedDirectory:   service.NewTNVEDDirectoryService(database),
		productDirectory: service.NewProductDirectoryService(database),
		calculation:      service.NewCalculationService(database),
		ai:               service.NewAIServiceWithProvider(database, cfg.AIProvider, cfg.GeminiAPIKey, cfg.GeminiModel, cfg.AnthropicAPIKey, cfg.AnthropicModel),
		gloves:           service.NewGlovesService(database),
		goodsEstimates:   service.NewGoodsEstimateService(database),
	}
}

func (s *Server) Router() *gin.Engine {
	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())
	router.GET("/api/health", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	auth := router.Group("/api/auth")
	auth.POST("/login", s.login)
	auth.POST("/register", s.register)
	auth.POST("/logout", s.logout)
	auth.GET("/me", s.requireUser, s.me)

	estimates := router.Group("/api/estimates")
	estimates.Use(s.requireUser)
	estimates.GET("", s.listEstimates)
	estimates.POST("", s.createEstimate)
	estimates.GET("/:id", s.getEstimate)
	estimates.PATCH("/:id", s.updateEstimate)
	estimates.DELETE("/:id", s.deleteEstimate)
	estimates.POST("/:id/imports", s.uploadImport)
	estimates.GET("/:id/imports/latest", s.latestImport)
	estimates.GET("/:id/products", s.listProducts)
	estimates.GET("/:id/calculation", s.getCalculation)
	estimates.PUT("/:id/calculation", s.saveCalculation)
	estimates.GET("/:id/calculation/export", s.exportCalculation)

	imports := router.Group("/api/imports")
	imports.Use(s.requireUser)
	imports.GET("/:id/preview", s.previewImport)
	imports.POST("/:id/confirm", s.confirmImport)
	imports.GET("/:id/workspace", s.getWorkspace)
	imports.PUT("/:id/workspace", s.saveWorkspace)
	imports.POST("/:id/workspace/apply", s.applyWorkspace)
	imports.POST("/:id/workspace/images", s.uploadWorkspaceImage)
	imports.GET("/:id/workspace/images/:assetID", s.getWorkspaceImage)
	imports.POST("/:id/apply", s.applyWorkspace)

	products := router.Group("/api/products")
	products.Use(s.requireUser)
	products.PATCH("/:id", s.updateProduct)
	products.POST("/:id/ai-analyze", s.analyzeProduct)
	products.GET("/:id/ai-analysis", s.latestProductAIAnalysis)
	products.GET("/:id/ai-dialog", s.getProductAIDialogue)
	products.POST("/:id/ai-dialog/messages", s.sendProductAIMessage)
	products.POST("/:id/ai-dialog/apply", s.applyAIChanges)
	products.PATCH("/:id/tnved", s.applyProductTNVED)
	products.POST("/:id/tnved-directory", s.applyProductTNVEDDirectory)
	products.POST("/:id/images", s.uploadProductImage)
	products.GET("/:id/images/:assetID", s.getProductImage)

	// Existing versioned classifier API remains available for integrations;
	// the operator directory below is a separate editable overlay.
	catalog := router.Group("/api/catalog")
	catalog.Use(s.requireUser)
	catalog.GET("/search", s.searchCatalog)
	catalog.GET("/codes/:code", s.catalogDetails)
	catalog.POST("/rules/applicable", s.applicableRules)
	catalog.POST("/import/preview", s.catalogImportPreview)
	catalog.POST("/import/apply", s.catalogImportApply)

	tnvedDirectory := router.Group("/api/tnved-directory")
	tnvedDirectory.Use(s.requireUser)
	tnvedDirectory.GET("", s.listTNVEDDirectory)
	tnvedDirectory.POST("", s.createTNVEDDirectory)
	tnvedDirectory.PATCH("/:id", s.updateTNVEDDirectory)
	tnvedDirectory.POST("/:id/archive", s.archiveTNVEDDirectory)
	tnvedDirectory.POST("/:id/restore", s.restoreTNVEDDirectory)
	tnvedDirectory.POST("/:id/copy", s.copyTNVEDDirectory)

	productsDirectory := router.Group("/api/product-directory")
	productsDirectory.Use(s.requireUser)
	productsDirectory.GET("", s.listProductDirectory)
	productsDirectory.POST("", s.createProductDirectory)
	productsDirectory.PATCH("/:id", s.updateProductDirectory)
	productsDirectory.POST("/:id/archive", s.archiveProductDirectory)
	productsDirectory.POST("/:id/restore", s.restoreProductDirectory)
	productsDirectory.POST("/:id/copy", s.copyProductDirectory)

	gloves := router.Group("/api/gloves")
	gloves.Use(s.requireUser)
	gloves.GET("/estimates", s.listGlovesEstimates)
	gloves.POST("/estimates", s.createGlovesEstimate)
	gloves.GET("/estimates/:id", s.getGlovesEstimate)
	gloves.PATCH("/estimates/:id", s.updateGlovesEstimate)
	gloves.DELETE("/estimates/:id", s.deleteGlovesEstimate)
	gloves.PUT("/estimates/:id/draft", s.saveGlovesEstimateDraft)
	gloves.GET("/draft", s.getGlovesDraft)
	gloves.PUT("/draft", s.saveGlovesDraft)
	gloves.POST("/parse-xlsx", s.parseGlovesXlsx)
	gloves.POST("/calculate", s.calculateGloves)
	gloves.POST("/export", s.exportGloves)

	// The copied gloves calculator uses the same parsing, calculation and
	// export rules, but keeps estimates and drafts in separate storage.
	goodsEstimates := router.Group("/api/gloves-copy")
	goodsEstimates.Use(s.requireUser)
	goodsEstimates.GET("/usd-rate", s.getGoodsEstimateUSDRate)
	goodsEstimates.GET("/estimates", s.listGoodsEstimates)
	goodsEstimates.POST("/estimates", s.createGoodsEstimate)
	goodsEstimates.GET("/estimates/:id", s.getGoodsEstimate)
	goodsEstimates.PATCH("/estimates/:id", s.updateGoodsEstimate)
	goodsEstimates.DELETE("/estimates/:id", s.deleteGoodsEstimate)
	goodsEstimates.PUT("/estimates/:id/draft", s.saveGoodsEstimateDraft)

	s.serveFrontend(router)
	return router
}
