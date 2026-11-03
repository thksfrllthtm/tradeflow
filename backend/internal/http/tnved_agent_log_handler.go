package httpapi

// Журнал агента для администратора: все подборы всех сотрудников с итогами основной и теневой модели.

import (
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/tnvedagent"
	"github.com/gin-gonic/gin"
)

// Казахстан — UTC+5: даты периода журнала считаются по местным суткам.
var almaty = time.FixedZone("UTC+5", 5*60*60)

const (
	agentLogDefaultDays = 30
	agentLogPageLimit   = 50
	agentLogMaxLimit    = 200
)

var (
	agentLogModes = map[string]bool{"": true, "row": true, "reference": true}
	agentLogOnly  = map[string]bool{"": true, "disagree": true, "verified": true, "wrong": true, "shadow": true, "documents": true}
)

// parseLogFilter — from и to (даты ГГГГ-ММ-ДД включительно; по умолчанию последние 30 дней), userId, mode,
// only, page (с 1) и limit.
func parseLogFilter(query url.Values, now time.Time) (tnvedagent.AgentLogFilter, error) {
	today := now.In(almaty)
	today = time.Date(today.Year(), today.Month(), today.Day(), 0, 0, 0, 0, almaty)
	filter := tnvedagent.AgentLogFilter{From: today.AddDate(0, 0, -agentLogDefaultDays+1), To: today.AddDate(0, 0, 1), Limit: agentLogPageLimit}
	date := func(name string) (time.Time, bool, error) {
		value := query.Get(name)
		if value == "" {
			return time.Time{}, false, nil
		}
		parsed, err := time.ParseInLocation("2006-01-02", value, almaty)
		if err != nil {
			return time.Time{}, false, &service.ValidationError{Field: name, Message: "Дата — в виде ГГГГ-ММ-ДД"}
		}
		return parsed, true, nil
	}
	if from, ok, err := date("from"); err != nil {
		return filter, err
	} else if ok {
		filter.From = from
	}
	if to, ok, err := date("to"); err != nil {
		return filter, err
	} else if ok {
		filter.To = to.AddDate(0, 0, 1)
	}
	if !filter.From.Before(filter.To) {
		return filter, &service.ValidationError{Field: "to", Message: "Конец периода раньше начала"}
	}
	if value := query.Get("userId"); value != "" {
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil || id <= 0 {
			return filter, &service.ValidationError{Field: "userId", Message: "Сотрудник не найден"}
		}
		filter.UserID = id
	}
	filter.Mode, filter.Only = query.Get("mode"), query.Get("only")
	if !agentLogModes[filter.Mode] {
		return filter, &service.ValidationError{Field: "mode", Message: "Режим — row или reference"}
	}
	if !agentLogOnly[filter.Only] {
		return filter, &service.ValidationError{Field: "only", Message: "Выборка — disagree, verified, wrong, shadow или documents"}
	}
	if value := query.Get("limit"); value != "" {
		limit, err := strconv.Atoi(value)
		if err != nil || limit <= 0 || limit > agentLogMaxLimit {
			return filter, &service.ValidationError{Field: "limit", Message: "Строк на странице — от 1 до 200"}
		}
		filter.Limit = limit
	}
	if value := query.Get("page"); value != "" {
		page, err := strconv.Atoi(value)
		if err != nil || page <= 0 {
			return filter, &service.ValidationError{Field: "page", Message: "Страница — с 1"}
		}
		filter.Offset = (page - 1) * filter.Limit
	}
	return filter, nil
}

func (s *Server) agentLog(c *gin.Context) {
	filter, err := parseLogFilter(c.Request.URL.Query(), time.Now())
	if err != nil {
		s.agentError(c, err)
		return
	}
	page, err := s.agent.Log(c.Request.Context(), filter)
	if err != nil {
		internalError(c)
		return
	}
	c.JSON(http.StatusOK, page)
}

func (s *Server) agentLogEntry(c *gin.Context) {
	id, ok := sessionID(c)
	if !ok {
		return
	}
	entry, err := s.agent.LogEntry(c.Request.Context(), currentUser(c), id)
	if err != nil {
		s.agentError(c, err)
		return
	}
	c.JSON(http.StatusOK, entry)
}
