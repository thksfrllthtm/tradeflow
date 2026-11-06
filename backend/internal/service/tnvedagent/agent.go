package tnvedagent

// ИИ-агент подбора кода ТН ВЭД для строки сметы. Работает на сервере в фоне по кругу
// «модель → инструмент → результат» с лимитом шагов; каждый шаг сразу пишется в чат
// (tnved_agent_messages), браузер забирает новые сообщения опросом. Перед запуском ищется тот же
// товар в базе решений: проверенная запись показывается сразу, без ИИ.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/ai"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/decisions"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/directory"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/trademarks"
	"github.com/shopspring/decimal"
)

const (
	agentMaxSteps = 12
	// С интернетом агент читает страницы целиком и сравнивает источники — больше шагов.
	agentWebMaxSteps = 24
	// Модель рассуждений (Pro, с размышлениями) отвечает медленнее Flash.
	agentRunTimeout = 15 * time.Minute
	// «Документы для ввоза»: под-агенты по актам параллельно (documents.go); время — с запасом на
	// размышления модели рассуждений (Pro) и медленные сайты актов.
	agentDocumentsTimeout = 12 * time.Minute
	agentParallelRuns     = 3
	agentMaxPhotos        = 3
	agentMaxPhotoBytes    = 2 << 20
	agentMaxUserMessage   = 1000
)

var (
	ErrAgentBusy       = errors.New("agent session is running")
	ErrAgentDailyLimit = errors.New("agent daily limit reached")
	ErrAgentForbidden  = errors.New("agent session belongs to another user")
)

// Казахстан — UTC+5: дневной лимит считается по местным суткам.
var almaty = time.FixedZone("UTC+5", 5*60*60)

type AgentSettings struct {
	DailyLimit int
	NamingRule string
	// WebSearchKey — ключ Tavily: поиск в интернете для подбора ("" — без интернета).
	WebSearchKey string
}

// AgentItemInput — строка сметы, для которой подбирается код.
type AgentItemInput struct {
	Name         string `json:"name"`
	SupplierName string `json:"supplierName"`
	// OtherNames — другие колонки наименования из файла (например, китайское и русское).
	OtherNames []string `json:"otherNames,omitempty"`
	// Details — «Данные» из файла: свойства товара с подписью колонки («Материал: сталь», «Модель: X-200»).
	Details      []string `json:"details,omitempty"`
	Code         string   `json:"code"`
	Quantity     string   `json:"quantity"`
	Places       string   `json:"places"`
	GrossKg      string   `json:"grossKg"`
	NetKg        string   `json:"netKg"`
	PricePerUnit string   `json:"pricePerUnit"`
	Unit         string   `json:"unit"`
}

type AgentStartInput struct {
	EstimateID int64          `json:"estimateId"`
	ItemID     string         `json:"itemId"`
	Item       AgentItemInput `json:"item"`
	Photos     []string       `json:"photos"`
	// Auto — запустить агента сразу после поиска в базе («Подобрать коды ИИ» для всех строк).
	// Без него подбор останавливается на «ready», и агента запускает сотрудник.
	Auto bool `json:"auto"`
}

type AgentMessageView struct {
	ID        int64           `json:"id"`
	Kind      string          `json:"kind"`
	Content   json.RawMessage `json:"content"`
	CreatedAt time.Time       `json:"createdAt"`
}

type AgentSessionView struct {
	ID int64 `json:"id"`
	// Mode — row (строка сметы) или reference (чат-справочник), Title — первый вопрос справочника.
	Mode       string             `json:"mode"`
	Title      string             `json:"title"`
	EstimateID *int64             `json:"estimateId"`
	ItemID     string             `json:"itemId"`
	Status     string             `json:"status"`
	Error      string             `json:"error"`
	Item       AgentItemInput     `json:"item"`
	Photos     []string           `json:"photos,omitempty"`
	Result     *AgentResult       `json:"result"`
	Messages   []AgentMessageView `json:"messages"`
	Steps      int                `json:"steps"`
	Runs       int                `json:"runs"`
	CostUSD    string             `json:"costUsd"`
	Tokens     int64              `json:"tokens"`
	Model      string             `json:"model"`
	CreatedAt  time.Time          `json:"createdAt"`
	UpdatedAt  time.Time          `json:"updatedAt"`
	// TrademarkCheck — вопрос «/троис»: ответ — сообщение «trademarks», уточнений нет.
	TrademarkCheck bool `json:"trademarkCheck,omitempty"`
	// Shared — вопрос коллеги, открытый сотруднику только для чтения; SharedBy — имя автора ("" — не заполнено).
	Shared   bool   `json:"shared,omitempty"`
	SharedBy string `json:"sharedBy,omitempty"`
}

// AgentRowStatus — состояние подбора строки для значка в таблице.
type AgentRowStatus struct {
	SessionID int64  `json:"sessionId"`
	ItemID    string `json:"itemId"`
	Status    string `json:"status"`
	BestCode  string `json:"bestCode"`
	Verdict   string `json:"verdict"`
}

type AgentStatus struct {
	Configured bool   `json:"configured"`
	Model      string `json:"model"`
	Vision     bool   `json:"vision"`
	DailyLimit int    `json:"dailyLimit"`
	UsedToday  int64  `json:"usedToday"`
}

type TNVEDAgentService struct {
	repository *repository.Repository
	// model — модель рассуждений (цикл агента, итог, справочник, документы); visionModel — модель для
	// фото (описание, вопросы по фото, похожие товары, перевод наименования). Без отдельной модели для
	// фото — один и тот же клиент.
	model       *AgentModelClient
	visionModel *AgentModelClient
	// documentsModel — модель под-агентов «Документов для ввоза» (другой уровень размышлений); nil — model.
	documentsModel *AgentModelClient
	decisions      *decisions.CodeDecisionService
	data           agentData
	settings       AgentSettings
	search         webSearcher
	slots          chan struct{}
	mu             sync.Mutex
	cancels        map[int64]context.CancelFunc
	// trademarks — реестр ТРОИС (nil — не подключён): проверка брендов и подписи логотипов.
	trademarks *trademarks.Service
	// shadow — модель теневого подбора (shadow.go); nil — тени нет. shadowSlots — её параллельность, отдельно от slots.
	shadow         shadowModel
	shadowSettings ShadowSettings
	shadowSlots    chan struct{}
}

// SetTrademarks подключает реестр ТРОИС: инструмент check_trademarks, проверку итога и чтение логотипов.
func (s *TNVEDAgentService) SetTrademarks(registry *trademarks.Service) { s.trademarks = registry }

// SetDocumentsModel — модель под-агентов «Документов для ввоза» (TNVED_AI_DOCUMENTS_EFFORT).
func (s *TNVEDAgentService) SetDocumentsModel(model *AgentModelClient) { s.documentsModel = model }

// NewTNVEDAgentService — visionModel nil: всё на одной модели model, как раньше.
func NewTNVEDAgentService(r *repository.Repository, model, visionModel *AgentModelClient, decisions *decisions.CodeDecisionService, settings AgentSettings) *TNVEDAgentService {
	if settings.DailyLimit <= 0 {
		settings.DailyLimit = 100
	}
	if visionModel == nil {
		visionModel = model
	}
	return &TNVEDAgentService{
		repository: r, model: model, visionModel: visionModel, decisions: decisions, settings: settings, search: newTavilySearcher(settings.WebSearchKey),
		data:        repositoryAgentData{repository: r, nomenclature: directory.NewTNVEDNomenclatureService(r), details: directory.NewDetailsFetcher(r), web: directory.NewWebPageFetcher(r)},
		slots:       make(chan struct{}, agentParallelRuns),
		shadowSlots: make(chan struct{}, shadowParallelRuns),
		cancels:     map[int64]context.CancelFunc{},
	}
}

// twoModels — фото смотрит отдельная модель, а модель рассуждений фото не получает.
func (s *TNVEDAgentService) twoModels() bool { return s.visionModel != s.model }

// modelName — модели подбора для сессии и статуса: «deepseek-flash + deepseek-v4-pro».
// Модель, которая смотрит фото сама (Claude), — без модели для фото в названии: та только описывает фото
// для поиска в базе и для тени.
func (s *TNVEDAgentService) modelName() string {
	if s.twoModels() && !s.model.Vision() {
		return s.visionModel.Name() + " + " + s.model.Name()
	}
	return s.model.Name()
}

// sessionUsage — расход подбора: всего (обе модели) и отдельно модели для фото.
type sessionUsage struct {
	total, vision    agentUsage
	cost, visionCost decimal.Decimal
}

func usageOf(session repository.TNVEDAgentSession) sessionUsage {
	return sessionUsage{
		total:  agentUsage{PromptTokens: session.PromptTokens, CachedTokens: session.CachedTokens, CompletionTokens: session.CompletionTokens},
		vision: agentUsage{PromptTokens: session.VisionPromptTokens, CachedTokens: session.VisionCachedTokens, CompletionTokens: session.VisionCompletionTokens},
		cost:   session.CostUSD, visionCost: session.VisionCostUSD,
	}
}

func (u *sessionUsage) add(step agentUsage, cost decimal.Decimal, vision bool) {
	add := func(to *agentUsage) {
		to.PromptTokens += step.PromptTokens
		to.CachedTokens += step.CachedTokens
		to.CompletionTokens += step.CompletionTokens
	}
	add(&u.total)
	u.cost = u.cost.Add(cost)
	if vision {
		add(&u.vision)
		u.visionCost = u.visionCost.Add(cost)
	}
}

// changes — поля сессии с расходом.
func (u sessionUsage) changes() map[string]any {
	return map[string]any{
		"prompt_tokens": u.total.PromptTokens, "cached_tokens": u.total.CachedTokens, "completion_tokens": u.total.CompletionTokens, "cost_usd": u.cost.Round(6),
		"vision_prompt_tokens": u.vision.PromptTokens, "vision_cached_tokens": u.vision.CachedTokens, "vision_completion_tokens": u.vision.CompletionTokens, "vision_cost_usd": u.visionCost.Round(6),
	}
}

// WarmGeneralActs — тексты перечней общей базы «Документов для ввоза» в базу заранее (в фоне при запуске).
func (s *TNVEDAgentService) WarmGeneralActs(ctx context.Context) {
	if data, ok := s.data.(repositoryAgentData); ok {
		data.web.WarmGeneralActs(ctx)
	}
}

// RecoverInterrupted — подборы, прерванные перезапуском сервера, помечаются ошибкой.
func (s *TNVEDAgentService) RecoverInterrupted(ctx context.Context) error {
	const message = "Подбор прерван перезапуском сервера. Нажмите «Подобрать заново»."
	if err := s.repository.ReadyInterruptedSearches(ctx); err != nil {
		return err
	}
	if err := s.repository.FailRunningShadowRuns(ctx, "Теневой подбор прерван перезапуском сервера."); err != nil {
		return err
	}
	ids, err := s.repository.FailRunningAgentSessions(ctx, message)
	for _, id := range ids {
		s.emit(ctx, id, "error", map[string]string{"message": message})
	}
	return err
}

func dayStart(now time.Time) time.Time {
	local := now.In(almaty)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, almaty)
}

func (s *TNVEDAgentService) Status(ctx context.Context, userID int64) (AgentStatus, error) {
	used, err := s.repository.CountAgentRunsSince(ctx, userID, dayStart(time.Now()))
	if err != nil {
		return AgentStatus{}, err
	}
	return AgentStatus{Configured: s.model.Configured(), Model: s.modelName(), Vision: s.visionModel.Configured() && s.visionModel.Vision(), DailyLimit: s.settings.DailyLimit, UsedToday: used}, nil
}

func (s *TNVEDAgentService) checkLimit(ctx context.Context, userID int64) error {
	if !s.model.Configured() {
		return ai.ErrAINotConfigured
	}
	used, err := s.repository.CountAgentRunsSince(ctx, userID, dayStart(time.Now()))
	if err != nil {
		return err
	}
	if used >= int64(s.settings.DailyLimit) {
		return ErrAgentDailyLimit
	}
	return nil
}

func scopeFor(user repository.User) int64 {
	if user.Role == repository.RoleAdmin {
		return repository.AllUsers
	}
	return user.ID
}

func (s *TNVEDAgentService) emit(ctx context.Context, sessionID int64, kind string, content any) {
	data, err := json.Marshal(content)
	if err != nil {
		return
	}
	_, _ = s.repository.AddAgentMessage(ctx, sessionID, kind, repository.JSON(data))
}

// sessionNames — наименования строки подбора (в справочнике — вопрос сотрудника).
func sessionNames(session repository.TNVEDAgentSession) []string {
	var item AgentItemInput
	_ = json.Unmarshal(session.Item, &item)
	return item.names()
}

// sessionFileCode — код из файла поставщика у строки подбора ("" — не было; в справочнике — нет).
func sessionFileCode(session repository.TNVEDAgentSession) string {
	var item AgentItemInput
	_ = json.Unmarshal(session.Item, &item)
	return item.Code
}

// names — все наименования строки: из инвойса, поставщика и других колонок файла.
func (item AgentItemInput) names() []string {
	return append([]string{item.Name, item.SupplierName}, item.OtherNames...)
}

func cleanItem(item AgentItemInput) AgentItemInput {
	clean := func(value string, limit int) string { return service.TruncateRunes(strings.TrimSpace(value), limit) }
	name, supplier := clean(item.Name, 500), clean(item.SupplierName, 500)
	others := []string{}
	for _, other := range item.OtherNames {
		if other = clean(other, 500); other != "" && other != name && other != supplier && len(others) < 2 {
			others = append(others, other)
		}
	}
	details := []string{}
	for _, detail := range item.Details {
		if detail = clean(detail, 300); detail != "" && len(details) < 10 {
			details = append(details, detail)
		}
	}
	return AgentItemInput{
		Name: name, SupplierName: supplier, OtherNames: others, Details: details, Code: decisions.NormalizeCode(clean(item.Code, 20)),
		Quantity: clean(item.Quantity, 30), Places: clean(item.Places, 30), GrossKg: clean(item.GrossKg, 30), NetKg: clean(item.NetKg, 30),
		PricePerUnit: clean(item.PricePerUnit, 30), Unit: clean(item.Unit, 20),
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
