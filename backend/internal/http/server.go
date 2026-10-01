package httpapi

import (
	"context"
	"net/http"
	"sync"

	"github.com/east-trans-logistic/import-calculator/backend/internal/config"
	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/gin-gonic/gin"
)

const userContextKey = "authenticated_user"

type Server struct {
	config            config.Config
	auth              *service.AuthService
	tnvedDirectory    *service.TNVEDDirectoryService
	tnvedNomenclature *service.TNVEDNomenclatureService
	kzRates           *service.TNVEDKZRateService
	productDirectory  *service.ProductDirectoryService
	gloves            *service.GlovesService
	goodsEstimates    *service.GoodsEstimateService
	employees         *service.EmployeeService
	pricing           *service.PricingService
	ceramics          *service.CeramicsService
	decisions         *service.CodeDecisionService
	agent             *service.TNVEDAgentService
	authMu            sync.Mutex
	authRates         map[string]authRate
}

func NewServer(database *repository.Repository, cfg config.Config) *Server {
	decisions := service.NewCodeDecisionService(database)
	return &Server{
		config:            cfg,
		auth:              service.NewAuthService(database, cfg.SessionTTL),
		tnvedDirectory:    service.NewTNVEDDirectoryService(database),
		tnvedNomenclature: service.NewTNVEDNomenclatureService(database),
		kzRates:           service.NewTNVEDKZRateService(database),
		productDirectory:  service.NewProductDirectoryService(database),
		gloves:            service.NewGlovesService(database),
		goodsEstimates:    service.NewGoodsEstimateService(database),
		employees:         service.NewEmployeeService(database),
		pricing:           service.NewPricingService(database, service.NewDeepSeekClient(cfg.DeepSeekAPIKey, cfg.DeepSeekModel)),
		ceramics: service.NewCeramicsService(database, service.NewCeramicsAIClient(service.CeramicsAIConfig{
			APIKey: cfg.CeramicsAIAPIKey, BaseURL: cfg.CeramicsAIBaseURL, Model: cfg.CeramicsAIModel, Vision: cfg.CeramicsAIVision,
		})),
		decisions: decisions,
		agent: service.NewTNVEDAgentService(database, service.NewAgentModelClient(service.AgentModelConfig{
			APIKey: cfg.TNVEDAIAPIKey, BaseURL: cfg.TNVEDAIBaseURL, Model: cfg.TNVEDAIModel, Vision: cfg.TNVEDAIVision,
			PriceInput: cfg.TNVEDAIPriceInput, PriceCached: cfg.TNVEDAIPriceCached, PriceOutput: cfg.TNVEDAIPriceOutput,
		}), decisions, service.AgentSettings{DailyLimit: cfg.TNVEDAIDailyLimit, NamingRule: cfg.TNVEDAINamingRule}),
	}
}

// RecoverInterrupted помечает ошибкой подборы ИИ, прерванные перезапуском сервера.
func (s *Server) RecoverInterrupted(ctx context.Context) error {
	return s.agent.RecoverInterrupted(ctx)
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
	employees.DELETE("/:id", s.deleteEmployee)

	// Подсказки для подбора цен под бюджет — общие для смет перчаток и товаров.
	pricing := router.Group("/api/estimates")
	pricing.Use(s.requireUser)
	pricing.POST("/price-weights", s.suggestPriceWeights)
	pricing.POST("/price-history", s.priceHistory)

	tnvedDirectory := router.Group("/api/tnved-directory")
	tnvedDirectory.Use(s.requireUser)
	tnvedDirectory.GET("", s.listTNVEDDirectory)
	tnvedDirectory.GET("/export", s.requireAdmin, s.exportTNVEDDirectory)
	tnvedDirectory.POST("/import", s.requireAdmin, s.importTNVEDDirectory)
	tnvedDirectory.POST("", s.requireTNVEDAuthor, s.createTNVEDDirectory)
	tnvedDirectory.PATCH("/:id", s.requireAdmin, s.updateTNVEDDirectory)
	tnvedDirectory.POST("/:id/archive", s.requireAdmin, s.archiveTNVEDDirectory)
	tnvedDirectory.POST("/:id/restore", s.requireAdmin, s.restoreTNVEDDirectory)
	tnvedDirectory.POST("/:id/copy", s.requireAdmin, s.copyTNVEDDirectory)

	tnvedNomenclature := router.Group("/api/tnved-nomenclature")
	tnvedNomenclature.Use(s.requireUser)
	tnvedNomenclature.GET("", s.listTNVEDNomenclature)
	tnvedNomenclature.POST("/lookup", s.lookupTNVEDNomenclature)
	tnvedNomenclature.POST("/import", s.requireAdmin, s.importTNVEDNomenclature)

	kzRates := router.Group("/api/tnved-kz-rates")
	kzRates.Use(s.requireUser, s.requireAdmin)
	kzRates.POST("/excel", s.kzRatesExcel)
	kzRates.POST("/import-pdf", s.importKZRatesPDF)
	kzRates.POST("/import", s.importKZRatesExcel)

	productsDirectory := router.Group("/api/product-directory")
	productsDirectory.Use(s.requireUser)
	productsDirectory.GET("", s.listProductDirectory)
	productsDirectory.GET("/export", s.requireAdmin, s.exportProductDirectory)
	productsDirectory.POST("/import", s.requireAdmin, s.importProductDirectory)
	productsDirectory.POST("", s.requireAdmin, s.createProductDirectory)
	productsDirectory.PATCH("/:id", s.requireAdmin, s.updateProductDirectory)
	productsDirectory.POST("/:id/archive", s.requireAdmin, s.archiveProductDirectory)
	productsDirectory.POST("/:id/restore", s.requireAdmin, s.restoreProductDirectory)
	productsDirectory.POST("/:id/copy", s.requireAdmin, s.copyProductDirectory)

	// Разбор Excel, расчёт растаможки и экспорт для смет товаров (адрес остался от первой
	// версии калькулятора — сметы перчаток).
	gloves := router.Group("/api/gloves")
	gloves.Use(s.requireUser)
	gloves.POST("/parse-xlsx", s.parseGlovesXlsx)
	gloves.POST("/calculate", s.calculateGloves)
	gloves.POST("/export", s.exportGloves)

	// Сметы товаров: список, карточка и сохранённый черновик с результатом.
	goodsEstimates := router.Group("/api/gloves-copy")
	goodsEstimates.Use(s.requireUser)
	goodsEstimates.GET("/usd-rate", s.getGoodsEstimateUSDRate)
	goodsEstimates.GET("/estimates", s.listGoodsEstimates)
	goodsEstimates.POST("/estimates", s.createGoodsEstimate)
	goodsEstimates.GET("/estimates/:id", s.getGoodsEstimate)
	goodsEstimates.PATCH("/estimates/:id", s.updateGoodsEstimate)
	goodsEstimates.DELETE("/estimates/:id", s.deleteGoodsEstimate)
	goodsEstimates.PATCH("/estimates/:id/draft", s.saveGoodsEstimateDraft)

	// База решений «товар из инвойса → код ТН ВЭД»: проверка, правка и импорт — администратор.
	decisions := router.Group("/api/code-decisions")
	decisions.Use(s.requireUser)
	decisions.GET("", s.listCodeDecisions)
	decisions.GET("/stats", s.codeDecisionStats)
	decisions.GET("/:id", s.getCodeDecision)
	decisions.POST("", s.createCodeDecision)
	decisions.POST("/import", s.requireAdmin, s.importCodeDecisions)
	decisions.PATCH("/:id", s.requireAdmin, s.updateCodeDecision)
	decisions.POST("/:id/verify", s.requireAdmin, s.verifyCodeDecision)
	decisions.POST("/:id/reject", s.requireAdmin, s.rejectCodeDecision)
	decisions.DELETE("/:id", s.requireAdmin, s.deleteCodeDecision)

	// ИИ-агент подбора кода ТН ВЭД для строки сметы.
	agent := router.Group("/api/tnved-agent")
	agent.Use(s.requireUser)
	agent.GET("/status", s.agentStatus)
	agent.GET("/usage", s.requireAdmin, s.agentUsage)
	agent.GET("/sessions", s.listAgentSessions)
	agent.POST("/sessions", s.startAgentSession)
	agent.GET("/sessions/:id", s.getAgentSession)
	agent.POST("/sessions/:id/messages", s.messageAgentSession)
	agent.POST("/sessions/:id/rerun", s.rerunAgentSession)
	agent.POST("/sessions/:id/cancel", s.cancelAgentSession)
	agent.POST("/sessions/:id/apply", s.applyAgentSession)

	// Подбор наименования и цены керамики — отдельный модуль, не связан со сметами и растаможкой.
	ceramics := router.Group("/api/ceramics")
	ceramics.Use(s.requireUser)
	ceramics.GET("/status", s.ceramicsStatus)
	ceramics.GET("/types", s.listCeramicsTypes)
	ceramics.GET("/products", s.listCeramicsProducts)
	ceramics.POST("/products", s.createCeramicsProduct)
	ceramics.PATCH("/products/:id", s.updateCeramicsProduct)
	ceramics.DELETE("/products/:id", s.requireAdmin, s.deleteCeramicsProduct)
	ceramics.POST("/products/import", s.requireAdmin, s.importCeramicsProducts)
	ceramics.POST("/lookup", s.lookupCeramicsMarks)
	ceramics.POST("/analyze", s.analyzeCeramics)
	ceramics.POST("/price", s.ceramicsPrice)

	s.serveFrontend(router)
	return router
}
