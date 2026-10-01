package httpapi

// ИИ-агент подбора кода ТН ВЭД для строки сметы: запуск, опрос новых сообщений чата, ответ
// агенту, «подобрать заново», остановка и «применить код» (запись в базу решений).

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/gin-gonic/gin"
)

func (s *Server) agentError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrAINotConfigured):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"message": "ИИ для подбора кодов не настроен на сервере (ключ TNVED_AI_API_KEY или DEEPSEEK_API_KEY)"}})
	case errors.Is(err, service.ErrAgentDailyLimit):
		c.JSON(http.StatusTooManyRequests, gin.H{"error": gin.H{"message": fmt.Sprintf("Дневной лимит подборов исчерпан (%d в сутки). Попробуйте завтра или попросите администратора увеличить лимит.", s.config.TNVEDAIDailyLimit)}})
	case errors.Is(err, service.ErrAgentBusy):
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{"message": "Подбор ещё идёт — дождитесь ответа или остановите его"}})
	case errors.Is(err, service.ErrAgentForbidden), service.IsNotFound(err):
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
	var input service.AgentStartInput
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
	var input service.AgentApplyInput
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
