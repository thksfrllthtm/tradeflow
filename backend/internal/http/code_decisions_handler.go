package httpapi

// База решений «товар из инвойса → код ТН ВЭД»: смотрят и пополняют все сотрудники, проверяют,
// исправляют, отклоняют, удаляют и импортируют старые инвойсы — администраторы.

import (
	"net/http"
	"strconv"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/decisions"
	"github.com/gin-gonic/gin"
)

func decisionID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		decisionNotFound(c)
		return 0, false
	}
	return id, true
}

func decisionNotFound(c *gin.Context) {
	c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"message": "Запись базы решений не найдена"}})
}

// decisionError отвечает на ошибку сервиса: проверка ввода — 400, нет записи — 404.
func decisionError(c *gin.Context, err error) {
	if validation, ok := err.(*service.ValidationError); ok {
		badRequest(c, validation.Message, validation.Field)
		return
	}
	if service.IsNotFound(err) {
		decisionNotFound(c)
		return
	}
	internalError(c)
}

func (s *Server) listCodeDecisions(c *gin.Context) {
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "50"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))
	items, total, err := s.decisions.List(c.Request.Context(), repository.CodeDecisionFilter{
		Status: c.Query("status"), Query: c.Query("q"), AgentWrong: c.Query("agentWrong") == "true", Limit: limit, Offset: offset,
	})
	if err != nil {
		decisionError(c, err)
		return
	}
	stats, err := s.decisions.Stats(c.Request.Context())
	if err != nil {
		internalError(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "total": total, "stats": stats})
}

func (s *Server) codeDecisionStats(c *gin.Context) {
	stats, err := s.decisions.Stats(c.Request.Context())
	if err != nil {
		internalError(c)
		return
	}
	c.JSON(http.StatusOK, stats)
}

func (s *Server) getCodeDecision(c *gin.Context) {
	id, ok := decisionID(c)
	if !ok {
		return
	}
	view, err := s.decisions.Get(c.Request.Context(), id)
	if err != nil {
		decisionError(c, err)
		return
	}
	c.JSON(http.StatusOK, view)
}

// createCodeDecision — «В базу решений» от сотрудника: запись «Не проверено».
func (s *Server) createCodeDecision(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 10<<20)
	var input decisions.CodeDecisionInput
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, "Некорректная запись базы решений", "")
		return
	}
	view, err := s.decisions.Create(c.Request.Context(), currentUser(c).ID, input, "manual")
	if err != nil {
		decisionError(c, err)
		return
	}
	c.JSON(http.StatusCreated, view)
}

func (s *Server) bindDecisionEdit(c *gin.Context) (decisions.CodeDecisionEdit, bool) {
	var edit decisions.CodeDecisionEdit
	if c.Request.ContentLength == 0 {
		return edit, true
	}
	if err := c.ShouldBindJSON(&edit); err != nil {
		badRequest(c, "Некорректная правка", "")
		return edit, false
	}
	return edit, true
}

func (s *Server) updateCodeDecision(c *gin.Context) {
	id, ok := decisionID(c)
	if !ok {
		return
	}
	edit, ok := s.bindDecisionEdit(c)
	if !ok {
		return
	}
	view, err := s.decisions.Update(c.Request.Context(), currentUser(c).ID, id, edit)
	if err != nil {
		decisionError(c, err)
		return
	}
	c.JSON(http.StatusOK, view)
}

func (s *Server) verifyCodeDecision(c *gin.Context) {
	id, ok := decisionID(c)
	if !ok {
		return
	}
	edit, ok := s.bindDecisionEdit(c)
	if !ok {
		return
	}
	view, err := s.decisions.Verify(c.Request.Context(), currentUser(c).ID, id, edit)
	if err != nil {
		decisionError(c, err)
		return
	}
	c.JSON(http.StatusOK, view)
}

func (s *Server) rejectCodeDecision(c *gin.Context) {
	id, ok := decisionID(c)
	if !ok {
		return
	}
	var input struct {
		Note string `json:"note"`
	}
	if c.Request.ContentLength > 0 {
		if err := c.ShouldBindJSON(&input); err != nil {
			badRequest(c, "Некорректный запрос", "")
			return
		}
	}
	view, err := s.decisions.Reject(c.Request.Context(), currentUser(c).ID, id, input.Note)
	if err != nil {
		decisionError(c, err)
		return
	}
	c.JSON(http.StatusOK, view)
}

func (s *Server) deleteCodeDecision(c *gin.Context) {
	id, ok := decisionID(c)
	if !ok {
		return
	}
	if err := s.decisions.Delete(c.Request.Context(), id); err != nil {
		decisionError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// importCodeDecisions — часть строк старого инвойса (браузер шлёт частями по 50 с фото).
func (s *Server) importCodeDecisions(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 60<<20)
	var input struct {
		Rows []decisions.CodeDecisionInput `json:"rows"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, "Некорректные строки импорта", "rows")
		return
	}
	created, skipped, problems, err := s.decisions.Import(c.Request.Context(), currentUser(c).ID, input.Rows)
	if err != nil {
		decisionError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"created": created, "skipped": skipped, "errors": problems})
}
