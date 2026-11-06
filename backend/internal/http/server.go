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
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/trademarks"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/translator"
	"github.com/gin-gonic/gin"
)

const userContextKey = "authenticated_user"

type Server struct {
	config            config.Config
	auth              *auth.AuthService
	tnvedNomenclature *directory.TNVEDNomenclatureService
	kzRates           *directory.TNVEDKZRateService
	antiDumping       *antidumping.Service
	riskIndicators    *riskindicators.Service
	trademarks        *trademarks.Service
	productDirectory  *directory.ProductDirectoryService
	goods             *goods.GoodsService
	goodsEstimates    *goods.GoodsEstimateService
	employees         *employees.EmployeeService
	pricing           *goods.PricingService
	ceramics          *ceramics.CeramicsService
	decisions         *decisions.CodeDecisionService
	agent             *tnvedagent.TNVEDAgentService
	translator        *translator.Service
	authMu            sync.Mutex
	authRates         map[string]authRate
}

func NewServer(database *repository.Repository, cfg config.Config) *Server {
	decisions := decisions.NewCodeDecisionService(database)
	server := &Server{
		config:            cfg,
		auth:              auth.NewAuthService(database, cfg.SessionTTL),
		tnvedNomenclature: directory.NewTNVEDNomenclatureService(database),
		kzRates:           directory.NewTNVEDKZRateService(database),
		antiDumping:       antidumping.NewService(database),
		riskIndicators:    riskindicators.NewService(database),
		trademarks:        trademarks.NewService(database),
		productDirectory:  directory.NewProductDirectoryService(database),
		goods:             goods.NewGoodsService(database),
		goodsEstimates:    goods.NewGoodsEstimateService(database),
		employees:         employees.NewEmployeeService(database),
		pricing:           goods.NewPricingService(database, ai.NewDeepSeekClient(cfg.DeepSeekAPIKey, cfg.DeepSeekModel)),
		ceramics: ceramics.NewCeramicsService(database, ceramics.NewCeramicsAIClient(ceramics.CeramicsAIConfig{
			APIKey: cfg.CeramicsAIAPIKey, BaseURL: cfg.CeramicsAIBaseURL, Model: cfg.CeramicsAIModel, Vision: cfg.CeramicsAIVision,
		})),
		decisions:  decisions,
		translator: translator.NewService(database, translator.NewClient(cfg.TranslatorAIAPIKey, cfg.TranslatorAIBaseURL, cfg.TranslatorAIModel)),
		agent:      newAgentService(database, decisions, cfg),
	}
	server.agent.SetTrademarks(server.trademarks)
	return server
}

// newAgentService — агент ТН ВЭД: модель рассуждений TNVED_AI_MODEL и, если задана, отдельная модель для
// фото TNVED_AI_VISION_MODEL (описание фото, поиск похожих). Модель рассуждений без зрения (DeepSeek) фото не
// получает; Claude (TNVED_AI_PROVIDER=anthropic) смотрит фото сам и получает ещё и описание.
func newAgentService(database *repository.Repository, decisions *decisions.CodeDecisionService, cfg config.Config) *tnvedagent.TNVEDAgentService {
	model := tnvedagent.NewAgentModelClient(tnvedagent.AgentModelConfig{
		Provider: cfg.TNVEDAIProvider, APIKey: cfg.TNVEDAIAPIKey, BaseURL: cfg.TNVEDAIBaseURL, Model: cfg.TNVEDAIModel, Effort: cfg.TNVEDAIEffort,
		Vision:     cfg.TNVEDAIVision && (cfg.TNVEDAIVisionModel == "" || cfg.TNVEDAIProvider == "anthropic"),
		PriceInput: cfg.TNVEDAIPriceInput, PriceCached: cfg.TNVEDAIPriceCached, PriceOutput: cfg.TNVEDAIPriceOutput,
	})
	var vision *tnvedagent.AgentModelClient
	if cfg.TNVEDAIVisionModel != "" {
		vision = tnvedagent.NewAgentModelClient(tnvedagent.AgentModelConfig{
			APIKey: cfg.TNVEDAIVisionAPIKey, BaseURL: cfg.TNVEDAIVisionBaseURL, Model: cfg.TNVEDAIVisionModel, Vision: cfg.TNVEDAIVision,
			PriceInput: cfg.TNVEDAIVisionPriceInput, PriceCached: cfg.TNVEDAIVisionPriceCached, PriceOutput: cfg.TNVEDAIVisionPriceOutput,
			EmbedModel: cfg.TNVEDAIEmbedModel,
		})
	}
	agent := tnvedagent.NewTNVEDAgentService(database, model, vision, decisions, tnvedagent.AgentSettings{
		DailyLimit: cfg.TNVEDAIDailyLimit, NamingRule: cfg.TNVEDAINamingRule, WebSearchKey: cfg.TavilyAPIKey,
	})
	if cfg.TNVEDAIDocumentsEffort != "" {
		agent.SetDocumentsModel(model.WithEffort(cfg.TNVEDAIDocumentsEffort))
	}
	if cfg.TNVEDAIShadowModel != "" {
		agent.SetShadow(tnvedagent.NewAgentModelClient(tnvedagent.AgentModelConfig{
			Provider: cfg.TNVEDAIShadowProvider, APIKey: cfg.TNVEDAIShadowAPIKey, BaseURL: cfg.TNVEDAIShadowBaseURL, Model: cfg.TNVEDAIShadowModel,
			Vision: cfg.TNVEDAIShadowVision, Effort: cfg.TNVEDAIShadowEffort,
			PriceInput: cfg.TNVEDAIShadowPriceInput, PriceCached: cfg.TNVEDAIShadowPriceCached, PriceOutput: cfg.TNVEDAIShadowPriceOutput,
		}), tnvedagent.ShadowSettings{BudgetUSD: cfg.TNVEDAIShadowBudgetUSD, DailyLimit: cfg.TNVEDAIShadowDailyLimit})
	}
	// Поиск по смыслу в базе решений — той же моделью эмбеддингов, что у агента.
	decisions.SetEmbedder(agent.Embedder())
	return agent
}

// StartBackground запускает фоновые задачи: антидемпинговые меры из файла программы (если их ещё
// не загружали), миниатюры записей базы решений без неё, описания фото для поиска похожих и
// переводы документов, прерванные перезапуском.
func (s *Server) StartBackground(ctx context.Context) {
	if err := s.antiDumping.SeedIfEmpty(ctx); err != nil {
		log.Printf("anti-dumping measures: initial load failed: %v", err)
	}
	go s.decisions.FillMissingThumbs(ctx)
	s.agent.StartPhotoWorker(ctx)
	go s.agent.WarmGeneralActs(ctx)
	if err := s.translator.ResumeInterrupted(ctx); err != nil {
		log.Printf("translator: resume interrupted translations: %v", err)
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

	// Подсказки для подбора цен под бюджет в сметах товаров.
	pricing := router.Group("/api/estimates")
	pricing.Use(s.requireUser, s.requireNotTranslator)
	pricing.POST("/price-weights", s.suggestPriceWeights)
	pricing.POST("/price-history", s.priceHistory)

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

	// Таможенный реестр ОИС (ТРОИС): список, логотипы и проверка наименований — всем; загрузка файла
	// реестра и подпись логотипа — администратор.
	trademarkRoutes := router.Group("/api/trademarks")
	trademarkRoutes.Use(s.requireUser)
	trademarkRoutes.GET("", s.listTrademarks)
	trademarkRoutes.GET("/logo", s.trademarkLogo)
	trademarkRoutes.POST("/check", s.checkTrademarks)
	trademarkRoutes.POST("/import", s.requireAdmin, s.importTrademarks)
	trademarkRoutes.PATCH("/logo-name", s.requireAdmin, s.setTrademarkLogoName)

	// Калькулятор утильсбора (перечень № 689, Методика № 448) — справочник в коде, без БД.
	router.GET("/api/util-fee", s.requireUser, s.listUtilFee)
	router.POST("/api/util-fee", s.requireUser, s.calculateUtilFee)
	router.POST("/api/util-fee/rows", s.requireUser, s.calculateUtilFeeRows)

	kzRates := router.Group("/api/tnved-kz-rates")
	kzRates.Use(s.requireUser, s.requireAdmin)
	kzRates.POST("/excel", s.kzRatesExcel)
	kzRates.POST("/import-pdf", s.importKZRatesPDF)
	kzRates.POST("/import", s.importKZRatesExcel)

	productsDirectory := router.Group("/api/product-directory")
	productsDirectory.Use(s.requireUser, s.requireNotTranslator)
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
	goods.Use(s.requireUser, s.requireNotTranslator)
	goods.POST("/parse-xlsx", s.parseGoodsXlsx)
	goods.POST("/calculate", s.calculateGoods)
	goods.POST("/export", s.exportGoods)

	// Сверка инвойса с китайской экспортной декларацией (без сохранения): всем вошедшим, включая переводчика.
	router.POST("/api/export-check", s.requireUser, s.exportCheck)

	// Сметы товаров: список, карточка и сохранённый черновик с результатом.
	goodsEstimates := router.Group("/api/goods-estimates")
	goodsEstimates.Use(s.requireUser, s.requireNotTranslator)
	goodsEstimates.GET("", s.listGoodsEstimates)
	goodsEstimates.POST("", s.createGoodsEstimate)
	goodsEstimates.GET("/:id", s.getGoodsEstimate)
	goodsEstimates.PATCH("/:id", s.updateGoodsEstimate)
	goodsEstimates.DELETE("/:id", s.deleteGoodsEstimate)
	goodsEstimates.PATCH("/:id/draft", s.saveGoodsEstimateDraft)

	// Официальный курс НБРК на дату (?date=ГГГГ-ММ-ДД, ?currency=USD|EUR).
	// Курсы на сегодня — без входа: их показывают страницы входа и регистрации.
	router.GET("/api/rates/today", s.getTodayRates)
	rates := router.Group("/api/rates")
	rates.Use(s.requireUser)
	rates.GET("/nbk", s.getNBKRate)

	// База решений «товар из инвойса → код ТН ВЭД»: проверка, правка и импорт — администратор.
	decisions := router.Group("/api/code-decisions")
	decisions.Use(s.requireUser, s.requireNotTranslator)
	decisions.GET("", s.listCodeDecisions)
	decisions.GET("/stats", s.codeDecisionStats)
	decisions.GET("/estimate/:id", s.estimateDecisionStatus)
	decisions.GET("/:id", s.getCodeDecision)
	decisions.POST("", s.createCodeDecision)
	decisions.POST("/import", s.requireAdmin, s.importCodeDecisions)
	decisions.PATCH("/:id", s.requireAdmin, s.updateCodeDecision)
	decisions.POST("/:id/verify", s.requireAdmin, s.verifyCodeDecision)
	decisions.POST("/:id/reject", s.requireAdmin, s.rejectCodeDecision)
	decisions.DELETE("/:id", s.requireAdmin, s.deleteCodeDecision)

	// ИИ-агент подбора кода ТН ВЭД для строки сметы. Переводчику — только справка с главной
	// (вопрос, ответ, документы, наименование): подбор по сметам и запись в базу решений закрыты.
	agent := router.Group("/api/tnved-agent")
	agent.Use(s.requireUser)
	agent.GET("/status", s.agentStatus)
	agent.GET("/usage", s.requireAdmin, s.agentUsage)
	agent.GET("/log", s.requireAdmin, s.agentLog)
	agent.GET("/log/:id", s.requireAdmin, s.agentLogEntry)
	// База документов: ответы «Документы для ввоза» по кодам, справки tnved.info и акты в кэше — только администратор.
	agent.GET("/documents-base", s.requireAdmin, s.documentsBase)
	agent.GET("/documents-base/codes/:code", s.requireAdmin, s.documentsBaseCode)
	agent.GET("/documents-base/sources", s.requireAdmin, s.documentsSources)
	agent.GET("/sessions", s.requireNotTranslator, s.listAgentSessions)
	agent.POST("/sessions", s.requireNotTranslator, s.startAgentSession)
	agent.GET("/sessions/:id", s.getAgentSession)
	agent.POST("/sessions/:id/begin", s.requireNotTranslator, s.beginAgentSession)
	agent.POST("/sessions/:id/messages", s.messageAgentSession)
	agent.POST("/sessions/:id/rerun", s.requireNotTranslator, s.rerunAgentSession)
	agent.POST("/sessions/:id/documents", s.documentsAgentSession)
	agent.GET("/documents/:code", s.savedCodeDocuments)
	agent.POST("/sessions/:id/cancel", s.requireNotTranslator, s.cancelAgentSession)
	agent.POST("/sessions/:id/apply", s.requireNotTranslator, s.applyAgentSession)
	agent.POST("/sessions/:id/name", s.applyAgentName)
	agent.GET("/reference", s.referenceHistory)
	agent.POST("/reference", s.askReference)
	// «/троис» с главной: вопрос в истории, проверка бренда по ТРОИС в фоне (без подбора кода).
	agent.POST("/reference/trademarks", s.askTrademarks)
	agent.DELETE("/reference/:id", s.deleteReference)
	// Поделиться вопросом чата на главной с коллегой: открыть может автор или администратор, получатель только читает.
	agent.GET("/share-users", s.agentShareUsers)
	agent.GET("/sessions/:id/shares", s.agentShares)
	agent.POST("/sessions/:id/shares", s.shareAgentSession)
	agent.DELETE("/sessions/:id/shares/:userId", s.unshareAgentSession)

	// Подбор наименования и цены керамики — отдельный модуль, не связан со сметами и растаможкой.
	ceramics := router.Group("/api/ceramics")
	ceramics.Use(s.requireUser, s.requireNotTranslator)
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

	// Перевод Word-документов: только переводчики и администратор; документы видят и правят все они.
	translatorDocs := router.Group("/api/translator")
	translatorDocs.Use(s.requireUser, s.requireTranslator)
	translatorDocs.GET("/documents", s.listTranslatorDocuments)
	translatorDocs.POST("/documents", s.uploadTranslatorDocument)
	translatorDocs.GET("/documents/:id", s.getTranslatorDocument)
	translatorDocs.DELETE("/documents/:id", s.deleteTranslatorDocument)
	translatorDocs.POST("/documents/:id/translate", s.translateTranslatorDocument)
	translatorDocs.GET("/documents/:id/export", s.exportTranslatorDocument)
	translatorDocs.PATCH("/segments/:id", s.editTranslatorSegment)
	translatorDocs.POST("/segments/:id/retranslate", s.retranslateTranslatorSegment)
	translatorDocs.GET("/segments/:id/history", s.translatorSegmentHistory)

	// Сервер отдаёт только API (сайт — на Vercel): любой неизвестный адрес — JSON 404.
	router.NoRoute(func(c *gin.Context) {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"message": "Не найдено", "code": "not_found"}})
	})
	return router
}
