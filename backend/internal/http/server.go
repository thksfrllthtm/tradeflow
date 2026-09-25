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
	tnvedDirectory   *service.TNVEDDirectoryService
	productDirectory *service.ProductDirectoryService
	gloves           *service.GlovesService
	goodsEstimates   *service.GoodsEstimateService
	employees        *service.EmployeeService
	pricing          *service.PricingService
	authMu           sync.Mutex
	authRates        map[string]authRate
}

func NewServer(database *repository.Repository, cfg config.Config) *Server {
	return &Server{
		config:           cfg,
		auth:             service.NewAuthService(database, cfg.SessionTTL),
		tnvedDirectory:   service.NewTNVEDDirectoryService(database),
		productDirectory: service.NewProductDirectoryService(database),
		gloves:           service.NewGlovesService(database),
		goodsEstimates:   service.NewGoodsEstimateService(database),
		employees:        service.NewEmployeeService(database),
		pricing:          service.NewPricingService(database, service.NewDeepSeekClient(cfg.DeepSeekAPIKey, cfg.DeepSeekModel)),
	}
}

func (s *Server) Router() *gin.Engine {
	router := gin.New()
	// Файлы больше 8 МБ из multipart-запросов gin сохраняет во временный файл, а не держит
	// в памяти: на бесплатном Render всего 512 МБ, а Excel со сметой бывает до 100 МБ.
	router.MaxMultipartMemory = 8 << 20
	router.Use(gin.Logger(), gin.Recovery())
	router.GET("/api/health", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	auth := router.Group("/api/auth")
	auth.POST("/login", s.login)
	auth.POST("/register", s.register)
	auth.POST("/logout", s.logout)
	auth.GET("/me", s.requireUser, s.me)

	employees := router.Group("/api/employees")
	employees.Use(s.requireUser, s.requireAdmin)
	employees.GET("", s.listEmployees)
	employees.POST("", s.createEmployee)
	employees.PATCH("/:id", s.updateEmployee)

	// Подсказки для подбора цен под бюджет — общие для смет перчаток и товаров.
	pricing := router.Group("/api/estimates")
	pricing.Use(s.requireUser)
	pricing.POST("/price-weights", s.suggestPriceWeights)
	pricing.POST("/price-history", s.priceHistory)

	tnvedDirectory := router.Group("/api/tnved-directory")
	tnvedDirectory.Use(s.requireUser)
	tnvedDirectory.GET("", s.listTNVEDDirectory)
	tnvedDirectory.POST("", s.requireAdmin, s.createTNVEDDirectory)
	tnvedDirectory.PATCH("/:id", s.requireAdmin, s.updateTNVEDDirectory)
	tnvedDirectory.POST("/:id/archive", s.requireAdmin, s.archiveTNVEDDirectory)
	tnvedDirectory.POST("/:id/restore", s.requireAdmin, s.restoreTNVEDDirectory)
	tnvedDirectory.POST("/:id/copy", s.requireAdmin, s.copyTNVEDDirectory)

	productsDirectory := router.Group("/api/product-directory")
	productsDirectory.Use(s.requireUser)
	productsDirectory.GET("", s.listProductDirectory)
	productsDirectory.POST("", s.requireAdmin, s.createProductDirectory)
	productsDirectory.PATCH("/:id", s.requireAdmin, s.updateProductDirectory)
	productsDirectory.POST("/:id/archive", s.requireAdmin, s.archiveProductDirectory)
	productsDirectory.POST("/:id/restore", s.requireAdmin, s.restoreProductDirectory)
	productsDirectory.POST("/:id/copy", s.requireAdmin, s.copyProductDirectory)

	gloves := router.Group("/api/gloves")
	gloves.Use(s.requireUser)
	gloves.GET("/estimates", s.listGlovesEstimates)
	gloves.POST("/estimates", s.createGlovesEstimate)
	gloves.GET("/estimates/:id", s.getGlovesEstimate)
	gloves.PATCH("/estimates/:id", s.updateGlovesEstimate)
	gloves.DELETE("/estimates/:id", s.deleteGlovesEstimate)
	gloves.PUT("/estimates/:id/draft", s.saveGlovesEstimateDraft)
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
