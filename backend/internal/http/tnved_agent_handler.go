package httpapi

// ИИ-агент подбора кода ТН ВЭД для строки сметы: запуск, опрос новых сообщений чата, ответ
// агенту, «подобрать заново», остановка и «применить код» (запись в базу решений).

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/ai"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/tnvedagent"
	"github.com/gin-gonic/gin"
)

func (s *Server) agentError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ai.ErrAINotConfigured):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"message": "ИИ для подбора кодов не настроен на сервере (ключ TNVED_AI_API_KEY или DEEPSEEK_API_KEY)"}})
	case errors.Is(err, ai.ErrAITimeout), errors.Is(err, ai.ErrAIUnavailable), errors.Is(err, ai.ErrAIRateLimited), errors.Is(err, ai.ErrAIBadResponse),
		errors.Is(err, ai.ErrAIModelUnavailable), errors.Is(err, ai.ErrAIRequestRejected), errors.Is(err, ai.ErrAIUnauthorized):
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"message": "ИИ сейчас не ответил — попробуйте ещё раз через минуту"}})
	case errors.Is(err, tnvedagent.ErrAgentDailyLimit):
		c.JSON(http.StatusTooManyRequests, gin.H{"error": gin.H{"message": fmt.Sprintf("Дневной лимит подборов исчерпан (%d в сутки). Попробуйте завтра или попросите администратора увеличить лимит.", s.config.TNVEDAIDailyLimit)}})
	case errors.Is(err, tnvedagent.ErrAgentBusy):
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{"message": "Подбор ещё идёт — дождитесь ответа или остановите его"}})
	case errors.Is(err, tnvedagent.ErrAgentForbidden), service.IsNotFound(err):
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"message": "Подбор или смета не найдены"}})
	default:
		if validation, ok := err.(*service.ValidationError); ok {
			badRequest(c, validation.Message, validation.Field)
			return
		}
		internalError(c)
	}
}

func sessionID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"message": "Подбор не найден"}})
		return 0, false
	}
	return id, true
}

func (s *Server) agentStatus(c *gin.Context) {
	status, err := s.agent.Status(c.Request.Context(), currentUser(c).ID)
	if err != nil {
		internalError(c)
		return
	}
	c.JSON(http.StatusOK, status)
}

func (s *Server) agentUsage(c *gin.Context) {
	usage, err := s.agent.Usage(c.Request.Context())
	if err != nil {
		internalError(c)
		return
	}
	c.JSON(http.StatusOK, usage)
}

func (s *Server) startAgentSession(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 10<<20)
	var input tnvedagent.AgentStartInput
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, "Некорректный запрос подбора", "")
		return
	}
	view, err := s.agent.Start(c.Request.Context(), currentUser(c), input)
	if err != nil {
		s.agentError(c, err)
		return
	}
	c.JSON(http.StatusCreated, view)
}

// listAgentSessions — состояние подбора по строкам сметы (?estimateId=).
func (s *Server) listAgentSessions(c *gin.Context) {
	estimateID, err := strconv.ParseInt(c.Query("estimateId"), 10, 64)
	if err != nil || estimateID <= 0 {
		badRequest(c, "Не указана смета", "estimateId")
		return
	}
	rows, err := s.agent.RowStatuses(c.Request.Context(), currentUser(c), estimateID)
	if err != nil {
		s.agentError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": rows})
}

func (s *Server) getAgentSession(c *gin.Context) {
	id, ok := sessionID(c)
	if !ok {
		return
	}
	after, _ := strconv.ParseInt(c.DefaultQuery("after", "0"), 10, 64)
	view, err := s.agent.Get(c.Request.Context(), currentUser(c), id, after)
	if err != nil {
		s.agentError(c, err)
		return
	}
	c.JSON(http.StatusOK, view)
}

func (s *Server) messageAgentSession(c *gin.Context) {
	id, ok := sessionID(c)
	if !ok {
		return
	}
	var input struct {
		Text string `json:"text"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, "Напишите сообщение", "text")
		return
	}
	view, err := s.agent.Continue(c.Request.Context(), currentUser(c), id, input.Text)
	if err != nil {
		s.agentError(c, err)
		return
	}
	c.JSON(http.StatusOK, view)
}

func (s *Server) rerunAgentSession(c *gin.Context) {
	id, ok := sessionID(c)
	if !ok {
		return
	}
	view, err := s.agent.Rerun(c.Request.Context(), currentUser(c), id)
	if err != nil {
		s.agentError(c, err)
		return
	}
	c.JSON(http.StatusOK, view)
}

// documentsAgentSession — кнопка «Документы для ввоза» на карточке кода (строка сметы и справочник).
func (s *Server) documentsAgentSession(c *gin.Context) {
	id, ok := sessionID(c)
	if !ok {
		return
	}
	var input struct {
		Code string `json:"code"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, "Укажите код", "code")
		return
	}
	view, err := s.agent.Documents(c.Request.Context(), currentUser(c), id, input.Code)
	if err != nil {
		s.agentError(c, err)
		return
	}
	c.JSON(http.StatusOK, view)
}

func (s *Server) cancelAgentSession(c *gin.Context) {
	id, ok := sessionID(c)
	if !ok {
		return
	}
	view, err := s.agent.Cancel(c.Request.Context(), currentUser(c), id)
	if err != nil {
		s.agentError(c, err)
		return
	}
	c.JSON(http.StatusOK, view)
}

func (s *Server) applyAgentSession(c *gin.Context) {
	id, ok := sessionID(c)
	if !ok {
		return
	}
	var input tnvedagent.AgentApplyInput
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, "Не указан код", "code")
		return
	}
	view, err := s.agent.Apply(c.Request.Context(), currentUser(c), id, input)
	if err != nil {
		s.agentError(c, err)
		return
	}
	c.JSON(http.StatusOK, view)
}

func (s *Server) applyAgentName(c *gin.Context) {
	id, ok := sessionID(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8<<10)
	var input tnvedagent.AgentNameInput
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, "Укажите наименование", "name")
		return
	}
	name, err := s.agent.ApplyName(c.Request.Context(), currentUser(c), id, input)
	if err != nil {
		s.agentError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"name": name})
}

// beginAgentSession — запуск агента после поиска в базе решений.
func (s *Server) beginAgentSession(c *gin.Context) {
	id, ok := sessionID(c)
	if !ok {
		return
	}
	view, err := s.agent.Begin(c.Request.Context(), currentUser(c), id)
	if err != nil {
		s.agentError(c, err)
		return
	}
	c.JSON(http.StatusOK, view)
}

// Чат-справочник на главной: вопрос о товаре (фото и/или текст), история вопросов сотрудника.
func (s *Server) askReference(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 10<<20)
	var input tnvedagent.ReferenceInput
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, "Некорректный вопрос", "")
		return
	}
	view, err := s.agent.AskReference(c.Request.Context(), currentUser(c), input)
	if err != nil {
		s.agentError(c, err)
		return
	}
	c.JSON(http.StatusCreated, view)
}

func (s *Server) referenceHistory(c *gin.Context) {
	items, err := s.agent.ReferenceHistory(c.Request.Context(), currentUser(c))
	if err != nil {
		internalError(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (s *Server) deleteReference(c *gin.Context) {
	id, ok := sessionID(c)
	if !ok {
		return
	}
	if err := s.agent.DeleteReference(c.Request.Context(), currentUser(c), id); err != nil {
		s.agentError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// savedCodeDocuments — последний сохранённый ответ «Документы для ввоза» по коду (карточка базы решений).
func (s *Server) savedCodeDocuments(c *gin.Context) {
	saved, err := s.agent.SavedDocuments(c.Request.Context(), c.Param("code"))
	if errors.Is(err, repository.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"message": "Документов по этому коду ещё нет"}})
		return
	}
	if err != nil {
		s.agentError(c, err)
		return
	}
	c.JSON(http.StatusOK, saved)
}

func (s *Server) askTrademarks(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 10<<20)
	var input tnvedagent.ReferenceInput
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, "Некорректный запрос", "")
		return
	}
	view, err := s.agent.AskTrademarks(c.Request.Context(), currentUser(c), input)
	if err != nil {
		s.agentError(c, err)
		return
	}
	c.JSON(http.StatusCreated, view)
}
