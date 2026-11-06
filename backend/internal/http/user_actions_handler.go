package httpapi

// Журнал действий сотрудников: middleware пишет каждое изменение (POST/PATCH/PUT/DELETE после входа),
// вход и выход пишут свои обработчики. Тело запроса не хранится — только кто, что, над какой записью и итог.

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/gin-gonic/gin"
)

// Журнал хранится полгода: старые записи удаляются при запуске сервера.
const userActionsKeep = 180 * 24 * time.Hour

// actionLabels — подписи изменений для страницы журнала. Каждый маршрут не GET должен быть здесь или в
// notRecorded (это проверяет тест), иначе новый маршрут попадёт в журнал без подписи.
var actionLabels = map[string]string{
	"POST /api/auth/login":  "Вход",
	"POST /api/auth/logout": "Выход",

	"POST /api/employees":       "Создание сотрудника",
	"PATCH /api/employees/:id":  "Изменение сотрудника",
	"DELETE /api/employees/:id": "Удаление сотрудника",

	"POST /api/tnved-nomenclature/import": "Загрузка номенклатуры ТН ВЭД",
	"POST /api/tnved-antidumping/refresh": "Обновление антидемпинговых мер",
	"POST /api/risk-indicators/import":    "Загрузка индикаторов риска",
	"DELETE /api/risk-indicators/:id":     "Удаление индикаторов риска",
	"POST /api/trademarks/import":         "Загрузка реестра ТРОИС",
	"PATCH /api/trademarks/logo-name":     "Подпись логотипа ТРОИС",
	"POST /api/tnved-kz-rates/import-pdf": "Загрузка перечня РК из PDF",
	"POST /api/tnved-kz-rates/import":     "Загрузка перечня РК из Excel",

	"POST /api/product-directory/import":      "Загрузка справочника товаров",
	"POST /api/product-directory":             "Добавление товара в справочник",
	"PATCH /api/product-directory/:id":        "Изменение товара в справочнике",
	"POST /api/product-directory/:id/archive": "Архивация товара",
	"POST /api/product-directory/:id/restore": "Возврат товара из архива",
	"POST /api/product-directory/:id/copy":    "Копирование товара",

	"POST /api/goods-estimates":            "Создание сметы",
	"PATCH /api/goods-estimates/:id":       "Изменение сметы",
	"DELETE /api/goods-estimates/:id":      "Удаление сметы",
	"PATCH /api/goods-estimates/:id/draft": "Сохранение сметы",

	"POST /api/code-decisions":            "Добавление в базу решений",
	"POST /api/code-decisions/import":     "Загрузка базы решений",
	"PATCH /api/code-decisions/:id":       "Изменение решения",
	"POST /api/code-decisions/:id/verify": "Подтверждение решения",
	"POST /api/code-decisions/:id/reject": "Отклонение решения",
	"DELETE /api/code-decisions/:id":      "Удаление решения",

	"POST /api/tnved-agent/sessions":                      "Подбор кода агентом",
	"POST /api/tnved-agent/sessions/:id/begin":            "Запуск агента",
	"POST /api/tnved-agent/sessions/:id/messages":         "Уточнение агенту",
	"POST /api/tnved-agent/sessions/:id/rerun":            "Подбор заново",
	"POST /api/tnved-agent/sessions/:id/documents":        "Документы для ввоза",
	"POST /api/tnved-agent/sessions/:id/cancel":           "Остановка агента",
	"POST /api/tnved-agent/sessions/:id/apply":            "Применение кода агента",
	"POST /api/tnved-agent/sessions/:id/name":             "Применение наименования агента",
	"POST /api/tnved-agent/reference":                     "Вопрос агенту",
	"POST /api/tnved-agent/reference/trademarks":          "Проверка по ТРОИС",
	"DELETE /api/tnved-agent/reference/:id":               "Удаление вопроса",
	"POST /api/tnved-agent/sessions/:id/shares":           "Доступ к вопросу коллеге",
	"DELETE /api/tnved-agent/sessions/:id/shares/:userId": "Закрытие доступа к вопросу",

	"POST /api/ceramics/products":        "Добавление керамики",
	"PATCH /api/ceramics/products/:id":   "Изменение керамики",
	"DELETE /api/ceramics/products/:id":  "Удаление керамики",
	"POST /api/ceramics/products/import": "Загрузка базы керамики",

	"POST /api/translator/documents":                "Загрузка документа на перевод",
	"DELETE /api/translator/documents/:id":          "Удаление документа перевода",
	"POST /api/translator/documents/:id/translate":  "Перевод документа",
	"PATCH /api/translator/segments/:id":            "Правка перевода",
	"POST /api/translator/segments/:id/retranslate": "Повторный перевод фрагмента",
}

// notRecorded — POST без изменений данных (расчёты, проверки, поиск, выгрузки) и регистрация (на проде закрыта).
var notRecorded = map[string]bool{
	"POST /api/auth/register":             true,
	"POST /api/estimates/price-weights":   true,
	"POST /api/estimates/price-history":   true,
	"POST /api/tnved-nomenclature/lookup": true,
	"POST /api/trademarks/check":          true,
	"POST /api/util-fee":                  true,
	"POST /api/util-fee/rows":             true,
	"POST /api/tnved-kz-rates/excel":      true,
	"POST /api/goods/parse-xlsx":          true,
	"POST /api/goods/calculate":           true,
	"POST /api/goods/export":              true,
	"POST /api/export-check":              true,
	"POST /api/ceramics/lookup":           true,
	"POST /api/ceramics/analyze":          true,
	"POST /api/ceramics/price":            true,
}

// recordAction — после обработчика: изменение от вошедшего сотрудника (его кладёт requireUser) пишется
// в журнал, в том числе отказ (403, ошибка проверки).
func (s *Server) recordAction(c *gin.Context) {
	c.Next()
	value, signedIn := c.Get(userContextKey)
	key := c.Request.Method + " " + c.FullPath()
	if !signedIn || c.Request.Method == http.MethodGet || c.FullPath() == "" || notRecorded[key] {
		return
	}
	user := value.(repository.User)
	s.writeAction(c, &user.ID, user.Email)
}

// writeAction пишет действие текущего запроса; ошибка записи не ломает ответ сотруднику.
func (s *Server) writeAction(c *gin.Context, userID *int64, email string) {
	values := make([]string, len(c.Params))
	for i, param := range c.Params {
		values[i] = param.Value
	}
	// Запрос мог оборваться (сотрудник закрыл вкладку), а запись всё равно нужна.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), 5*time.Second)
	defer cancel()
	err := s.actions.RecordUserAction(ctx, repository.UserAction{
		UserID: userID, Email: email, Method: c.Request.Method, Route: c.FullPath(),
		Target: strings.Join(values, " / "), Status: c.Writer.Status(), IP: c.ClientIP(),
	})
	if err != nil {
		log.Printf("user actions: %v", err)
	}
}

func (s *Server) listUserActions(c *gin.Context) {
	// Тот же разбор периода, сотрудника и страницы, что у журнала агента.
	parsed, err := parseLogFilter(c.Request.URL.Query(), time.Now())
	if err != nil {
		s.agentError(c, err)
		return
	}
	rows, total, err := s.actions.UserActions(c.Request.Context(), repository.UserActionFilter{
		From: parsed.From, To: parsed.To, UserID: parsed.UserID, Limit: parsed.Limit, Offset: parsed.Offset,
	})
	if err != nil {
		internalError(c)
		return
	}
	items := make([]gin.H, len(rows))
	for i, row := range rows {
		label, ok := actionLabels[row.Method+" "+row.Route]
		if !ok {
			label = row.Method + " " + row.Route
		}
		items[i] = gin.H{
			"id": row.ID, "userId": row.UserID, "user": row.UserName, "email": row.Email, "action": label,
			"target": row.Target, "status": row.Status, "ip": row.IP, "count": row.Count,
			"createdAt": row.CreatedAt, "updatedAt": row.UpdatedAt,
		}
	}
	c.JSON(http.StatusOK, gin.H{"rows": items, "total": total})
}

// CleanupUserActions удаляет записи журнала старше полугода.
func (s *Server) CleanupUserActions(ctx context.Context) {
	if err := s.actions.DeleteUserActionsBefore(ctx, time.Now().Add(-userActionsKeep)); err != nil {
		log.Printf("user actions: cleanup: %v", err)
	}
}
