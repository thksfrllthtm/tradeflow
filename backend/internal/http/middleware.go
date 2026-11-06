package httpapi

// Access middleware: session check, role checks and the owner filter for estimates.

import (
	"net/http"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/gin-gonic/gin"
)

func (s *Server) requireUser(c *gin.Context) {
	token, err := c.Cookie(s.config.SessionCookieName)
	if err != nil || token == "" {
		unauthorized(c)
		return
	}
	user, err := s.auth.UserForToken(c.Request.Context(), token)
	if err != nil {
		unauthorized(c)
		return
	}
	c.Set(userContextKey, user)
	c.Next()
}

// requireAdmin follows requireUser on routes only administrators may use:
// employee management and changes to the shared directories.
func (s *Server) requireAdmin(c *gin.Context) {
	if !currentUser(c).IsAdmin() {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": gin.H{"message": "Недостаточно прав: действие доступно только администратору", "code": "forbidden"}})
		return
	}
	c.Next()
}

// requireTranslator follows requireUser on the «Переводчик» section: translators and administrators only.
func (s *Server) requireTranslator(c *gin.Context) {
	if role := currentUser(c).Role; role != repository.RoleAdmin && role != repository.RoleTranslator {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": gin.H{"message": "Раздел «Переводчик» доступен переводчикам и администратору", "code": "forbidden"}})
		return
	}
	c.Next()
}

// requireNotTranslator follows requireUser on estimates, the product directory, the decision base,
// ceramics and agent runs for estimate rows: a translator works only with documents.
func (s *Server) requireNotTranslator(c *gin.Context) {
	if currentUser(c).Role == repository.RoleTranslator {
		c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": gin.H{"message": "Раздел недоступен переводчику", "code": "forbidden"}})
		return
	}
	c.Next()
}

// estimateScope is the owner filter for estimates: an administrator works with
// every employee's estimates, everyone else only with their own.
func estimateScope(c *gin.Context) int64 {
	user := currentUser(c)
	if user.IsAdmin() {
		return repository.AllUsers
	}
	return user.ID
}
