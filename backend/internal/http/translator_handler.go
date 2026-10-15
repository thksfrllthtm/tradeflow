package httpapi

// Раздел «Переводчик»: документы .docx и их перевод — всем сотрудникам; удаляет документ
// тот, кто загрузил, или администратор.

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/ai"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/translator"
	"github.com/gin-gonic/gin"
)

func translatorID(c *gin.Context) (int64, bool) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		translatorNotFound(c)
		return 0, false
	}
	return id, true
}

func translatorNotFound(c *gin.Context) {
	c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"message": "Документ не найден"}})
}

func translatorError(c *gin.Context, err error) {
	var validation *service.ValidationError
	switch {
	case errors.As(err, &validation):
		badRequest(c, validation.Message, validation.Field)
	case service.IsNotFound(err):
		translatorNotFound(c)
	case errors.Is(err, ai.ErrAINotConfigured):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": gin.H{"message": ai.AIErrorMessage(err), "code": "ai_not_configured"}})
	case isAIError(err):
		c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"message": ai.AIErrorMessage(err), "code": "ai_error"}})
	case errors.Is(err, translator.ErrForbidden):
		c.JSON(http.StatusForbidden, gin.H{"error": gin.H{"message": "Удалить документ может тот, кто его загрузил, или администратор", "code": "forbidden"}})
	default:
		internalError(c)
	}
}

func (s *Server) listTranslatorDocuments(c *gin.Context) {
	items, err := s.translator.List(c.Request.Context(), c.Query("q"))
	if err != nil {
		internalError(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}

func (s *Server) uploadTranslatorDocument(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, translator.MaxFileBytes+1<<20)
	file, header, err := c.Request.FormFile("file")
	if err != nil {
		var sizeErr *http.MaxBytesError
		if errors.As(err, &sizeErr) {
			badRequest(c, "Файл больше 20 МБ. Уменьшите его (например, сожмите картинки в Word) или разделите на части", "file")
			return
		}
		badRequest(c, "Выберите документ Word в формате .docx", "file")
		return
	}
	defer file.Close()
	data, err := io.ReadAll(file)
	if err != nil {
		internalError(c)
		return
	}
	id, err := s.translator.Upload(c.Request.Context(), currentUser(c).ID, header.Filename, data, c.PostForm("sourceLang"), c.PostForm("targetLang"))
	if err != nil {
		translatorError(c, err)
		return
	}
	c.JSON(http.StatusCreated, gin.H{"id": id})
}

func (s *Server) getTranslatorDocument(c *gin.Context) {
	id, ok := translatorID(c)
	if !ok {
		return
	}
	// since — время прошлого ответа: во время перевода страница забирает только изменённые сегменты.
	since, _ := time.Parse(time.RFC3339Nano, c.Query("since"))
	view, err := s.translator.Get(c.Request.Context(), id, since)
	if err != nil {
		translatorError(c, err)
		return
	}
	c.JSON(http.StatusOK, view)
}

func (s *Server) translateTranslatorDocument(c *gin.Context) {
	id, ok := translatorID(c)
	if !ok {
		return
	}
	if err := s.translator.Translate(c.Request.Context(), id); err != nil {
		translatorError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) deleteTranslatorDocument(c *gin.Context) {
	id, ok := translatorID(c)
	if !ok {
		return
	}
	if err := s.translator.Delete(c.Request.Context(), currentUser(c), id); err != nil {
		translatorError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

// isAIError — ошибка ИИ-провайдера при переводе сегмента («Перевести заново»).
func isAIError(err error) bool {
	for _, target := range []error{ai.ErrAIUnauthorized, ai.ErrAIRateLimited, ai.ErrAITimeout, ai.ErrAIRequestRejected,
		ai.ErrAIModelUnavailable, ai.ErrAIBadResponse, ai.ErrAIRequestTooLarge, ai.ErrAIUnavailable} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

func (s *Server) editTranslatorSegment(c *gin.Context) {
	id, ok := translatorID(c)
	if !ok {
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 256<<10)
	var edit translator.SegmentEdit
	if err := c.ShouldBindJSON(&edit); err != nil {
		badRequest(c, "Не удалось прочитать правку", "targetText")
		return
	}
	segment, err := s.translator.EditSegment(c.Request.Context(), currentUser(c).ID, id, edit)
	if err != nil {
		translatorError(c, err)
		return
	}
	c.JSON(http.StatusOK, segment)
}

func (s *Server) retranslateTranslatorSegment(c *gin.Context) {
	id, ok := translatorID(c)
	if !ok {
		return
	}
	segment, err := s.translator.RetranslateSegment(c.Request.Context(), id)
	if err != nil {
		translatorError(c, err)
		return
	}
	c.JSON(http.StatusOK, segment)
}

func (s *Server) translatorSegmentHistory(c *gin.Context) {
	id, ok := translatorID(c)
	if !ok {
		return
	}
	versions, err := s.translator.SegmentHistory(c.Request.Context(), id)
	if err != nil {
		translatorError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": versions})
}

// exportTranslatorDocument — .docx с переводом (mode=target) или двуязычный (mode=bilingual).
func (s *Server) exportTranslatorDocument(c *gin.Context) {
	id, ok := translatorID(c)
	if !ok {
		return
	}
	name, data, err := s.translator.Export(c.Request.Context(), id, c.Query("mode") == "bilingual")
	if err != nil {
		translatorError(c, err)
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="translation.docx"; filename*=UTF-8''%s`, url.PathEscape(name)))
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.wordprocessingml.document", data)
}
