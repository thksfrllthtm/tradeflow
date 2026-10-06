package httpapi

import (
	"context"
	"log"
	"net/http"
	"sync"

	"github.com/east-trans-logistic/import-calculator/backend/internal/config"
	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/ai"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/antidumping"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/auth"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/ceramics"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/decisions"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/directory"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/employees"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/goods"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/riskindicators"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/tnvedagent"
	"github.com/gin-gonic/gin"
)

const userContextKey = "authenticated_user"

type Server struct {
	config            config.Config
	auth              *auth.AuthService
	tnvedDirectory    *directory.TNVEDDirectoryService
	tnvedNomenclature *directory.TNVEDNomenclatureService
	kzRates           *directory.TNVEDKZRateService
	antiDumping       *antidumping.Service
	riskIndicators    *riskindicators.Service
	productDirectory  *directory.ProductDirectoryService
	goods             *goods.GoodsService
	goodsEstimates    *goods.GoodsEstimateService
	employees         *employees.EmployeeService
	pricing           *goods.PricingService
	ceramics          *ceramics.CeramicsService
	decisions         *decisions.CodeDecisionService
	agent             *tnvedagent.TNVEDAgentService
	authMu            sync.Mutex
	authRates         map[string]authRate
}

func NewServer(database *repository.Repository, cfg config.Config) *Server {
	decisions := decisions.NewCodeDecisionService(database)
	return &Server{
		config:            cfg,
		auth:              auth.NewAuthService(database, cfg.SessionTTL),
		tnvedDirectory:    directory.NewTNVEDDirectoryService(database),
		tnvedNomenclature: directory.NewTNVEDNomenclatureService(database),
		kzRates:           directory.NewTNVEDKZRateService(database),
		antiDumping:       antidumping.NewService(database),
		riskIndicators:    riskindicators.NewService(database),
		productDirectory:  directory.NewProductDirectoryService(database),
		goods:             goods.NewGoodsService(database),
		goodsEstimates:    goods.NewGoodsEstimateService(database),
		employees:         employees.NewEmployeeService(database),
		pricing:           goods.NewPricingService(database, ai.NewDeepSeekClient(cfg.DeepSeekAPIKey, cfg.DeepSeekModel)),
		ceramics: ceramics.NewCeramicsService(database, ceramics.NewCeramicsAIClient(ceramics.CeramicsAIConfig{
			APIKey: cfg.CeramicsAIAPIKey, BaseURL: cfg.CeramicsAIBaseURL, Model: cfg.CeramicsAIModel, Vision: cfg.CeramicsAIVision,
		})),
		decisions: decisions,
		agent: tnvedagent.NewTNVEDAgentService(database, tnvedagent.NewAgentModelClient(tnvedagent.AgentModelConfig{
			APIKey: cfg.TNVEDAIAPIKey, BaseURL: cfg.TNVEDAIBaseURL, Model: cfg.TNVEDAIModel, Vision: cfg.TNVEDAIVision,
			PriceInput: cfg.TNVEDAIPriceInput, PriceCached: cfg.TNVEDAIPriceCached, PriceOutput: cfg.TNVEDAIPriceOutput,
		}), decisions, tnvedagent.AgentSettings{DailyLimit: cfg.TNVEDAIDailyLimit, NamingRule: cfg.TNVEDAINamingRule}),
	}
}

// StartBackground запускает фоновые задачи: антидемпинговые меры из файла программы (если их ещё
// не загружали), миниатюры записей базы решений без неё и описания фото для поиска похожих.
func (s *Server) StartBackground(ctx context.Context) {
	if err := s.antiDumping.SeedIfEmpty(ctx); err != nil {
		log.Printf("anti-dumping measures: initial load failed: %v", err)
	}
	go s.decisions.FillMissingThumbs(ctx)
	s.agent.StartPhotoWorker(ctx)
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

	// Антидемпинговые меры ЕАЭС (реестр ЕЭК): список — всем, обновление с сайта ЕЭК — администратор.
	antiDumping := router.Group("/api/tnved-antidumping")
	antiDumping.Use(s.requireUser)
	antiDumping.GET("", s.listAntiDumping)
	antiDumping.POST("/refresh", s.requireAdmin, s.refreshAntiDumping)

	// Стоимостные индикаторы риска КГД (профили риска): список — всем, загрузка и удаление — администратор.
	riskIndicators := router.Group("/api/risk-indicators")
	riskIndicators.Use(s.requireUser)
	riskIndicators.GET("", s.listRiskIndicators)
	riskIndicators.POST("/import", s.requireAdmin, s.importRiskIndicators)
	riskIndicators.DELETE("/:id", s.requireAdmin, s.deleteRiskIndicators)

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

	// Разбор Excel, расчёт растаможки и экспорт смет товаров.
	goods := router.Group("/api/goods")
	goods.Use(s.requireUser)
	goods.POST("/parse-xlsx", s.parseGoodsXlsx)
	goods.POST("/calculate", s.calculateGoods)
	goods.POST("/export", s.exportGoods)

	// Сметы товаров: список, карточка и сохранённый черновик с результатом.
	goodsEstimates := router.Group("/api/goods-estimates")
	goodsEstimates.Use(s.requireUser)
	goodsEstimates.GET("", s.listGoodsEstimates)
	goodsEstimates.POST("", s.createGoodsEstimate)
	goodsEstimates.GET("/:id", s.getGoodsEstimate)
	goodsEstimates.PATCH("/:id", s.updateGoodsEstimate)
	goodsEstimates.DELETE("/:id", s.deleteGoodsEstimate)
	goodsEstimates.PATCH("/:id/draft", s.saveGoodsEstimateDraft)

	// Официальный курс НБРК на дату (?date=ГГГГ-ММ-ДД, ?currency=USD|EUR).
	rates := router.Group("/api/rates")
	rates.Use(s.requireUser)
	rates.GET("/nbk", s.getNBKRate)

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
	agent.POST("/sessions/:id/begin", s.beginAgentSession)
	agent.POST("/sessions/:id/messages", s.messageAgentSession)
	agent.POST("/sessions/:id/rerun", s.rerunAgentSession)
	agent.POST("/sessions/:id/documents", s.documentsAgentSession)
	agent.POST("/sessions/:id/cancel", s.cancelAgentSession)
	agent.POST("/sessions/:id/apply", s.applyAgentSession)
	agent.POST("/sessions/:id/name", s.applyAgentName)
	agent.GET("/reference", s.referenceHistory)
	agent.POST("/reference", s.askReference)
	agent.DELETE("/reference/:id", s.deleteReference)

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

	// Сервер отдаёт только API (сайт — на Vercel): любой неизвестный адрес — JSON 404.
	router.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"message": "Не найдено", "code": "not_found"}})
	})
	return router
}
