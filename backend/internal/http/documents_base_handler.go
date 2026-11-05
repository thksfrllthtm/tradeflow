package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// documentsBase — коды с сохранёнными ответами «Документы для ввоза» (?q= — начало кода или слова товара).
func (s *Server) documentsBase(c *gin.Context) {
	page, err := s.agent.DocumentsBase(c.Request.Context(), c.Query("q"))
	if err != nil {
		internalError(c)
		return
	}
	c.JSON(http.StatusOK, page)
}

// documentsBaseCode — карточка кода: все ответы, справка tnved.info и акты из ответов.
func (s *Server) documentsBaseCode(c *gin.Context) {
	entry, err := s.agent.DocumentsBaseCode(c.Request.Context(), c.Param("code"))
	if err != nil {
		s.agentError(c, err)
		return
	}
	c.JSON(http.StatusOK, entry)
}

// documentsSources — акты и документы в кэше программы (?q= — слова заголовка или адреса).
func (s *Server) documentsSources(c *gin.Context) {
	items, err := s.agent.DocumentsSources(c.Request.Context(), c.Query("q"))
	if err != nil {
		internalError(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": items})
}
