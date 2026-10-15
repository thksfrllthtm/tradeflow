package httpapi

// Authentication: login/register/logout, session cookie, per-IP/email attempt limit and the requireUser middleware.

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/auth"
	"github.com/gin-gonic/gin"
)

type authRate struct {
	Window time.Time
	Count  int
}

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

func (s *Server) login(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8<<10)
	var input struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		var sizeErr *http.MaxBytesError
		if errors.As(err, &sizeErr) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": gin.H{"message": "Данные формы слишком большие", "code": "auth_request_too_large"}})
			return
		}
		badRequest(c, "Введите email и пароль", "")
		return
	}
	if !s.allowAuthAttempt(c.ClientIP(), input.Email) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": gin.H{"message": "Слишком много попыток. Повторите через минуту", "code": "auth_rate_limited"}})
		return
	}
	user, token, expiresAt, err := s.auth.Login(c.Request.Context(), input.Email, input.Password)
	if err != nil {
		if err == auth.ErrInvalidCredentials {
			c.JSON(http.StatusUnauthorized, gin.H{"error": gin.H{"message": "Неверный email или пароль", "code": "invalid_credentials"}})
			return
		}
		internalError(c)
		return
	}
	s.setSessionCookie(c, token, expiresAt)
	c.JSON(http.StatusOK, gin.H{"user": userResponse(user)})
}

func (s *Server) setSessionCookie(c *gin.Context, token string, expiresAt time.Time) {
	c.SetSameSite(http.SameSiteLaxMode)
	maxAge := int(time.Until(expiresAt).Seconds())
	if maxAge < 1 {
		maxAge = 1
	}
	c.SetCookie(s.config.SessionCookieName, token, maxAge, "/", "", s.config.CookieSecure, true)
}

func (s *Server) allowAuthAttempt(ip, email string) bool {
	key := strings.ToLower(strings.TrimSpace(ip)) + "|" + strings.ToLower(strings.TrimSpace(email))
	now := time.Now().UTC()
	s.authMu.Lock()
	defer s.authMu.Unlock()
	if s.authRates == nil {
		s.authRates = make(map[string]authRate)
	}
	for existingKey, existingRate := range s.authRates {
		if !existingRate.Window.IsZero() && now.Sub(existingRate.Window) >= time.Minute {
			delete(s.authRates, existingKey)
		}
	}
	rate := s.authRates[key]
	if rate.Window.IsZero() || now.Sub(rate.Window) >= time.Minute {
		rate = authRate{Window: now}
	}
	if rate.Count >= 10 {
		s.authRates[key] = rate
		return false
	}
	rate.Count++
	s.authRates[key] = rate
	return true
}

func (s *Server) register(c *gin.Context) {
	if s.config.RegistrationDisabled {
		c.JSON(http.StatusForbidden, gin.H{"error": gin.H{"message": "Регистрация закрыта. Попросите администратора создать вам аккаунт", "code": "registration_disabled"}})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 12<<10)
	var input struct {
		Name     string `json:"name"`
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if err := c.ShouldBindJSON(&input); err != nil {
		var sizeErr *http.MaxBytesError
		if errors.As(err, &sizeErr) {
			c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": gin.H{"message": "Данные формы слишком большие", "code": "auth_request_too_large"}})
			return
		}
		badRequest(c, "Заполните имя, email и пароль", "")
		return
	}
	if !s.allowAuthAttempt(c.ClientIP(), input.Email) {
		c.JSON(http.StatusTooManyRequests, gin.H{"error": gin.H{"message": "Слишком много попыток. Повторите через минуту", "code": "auth_rate_limited"}})
		return
	}
	user, token, expiresAt, err := s.auth.Register(c.Request.Context(), input.Name, input.Email, input.Password)
	if err != nil {
		if err == auth.ErrEmailAlreadyExists {
			c.JSON(http.StatusConflict, gin.H{"error": gin.H{"message": "Аккаунт с таким email уже существует", "field": "email", "code": "email_exists"}})
			return
		}
		if validation, ok := err.(*service.ValidationError); ok {
			badRequest(c, validation.Message, validation.Field)
			return
		}
		internalError(c)
		return
	}
	s.setSessionCookie(c, token, expiresAt)
	c.JSON(http.StatusCreated, gin.H{"user": userResponse(user)})
}

func (s *Server) logout(c *gin.Context) {
	if token, err := c.Cookie(s.config.SessionCookieName); err == nil {
		if err := s.auth.Logout(c.Request.Context(), token); err != nil {
			internalError(c)
			return
		}
	}
	c.SetSameSite(http.SameSiteLaxMode)
	c.SetCookie(s.config.SessionCookieName, "", -1, "/", "", s.config.CookieSecure, true)
	c.Status(http.StatusNoContent)
}

func (s *Server) me(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"user": userResponse(currentUser(c))})
}

func userResponse(user repository.User) gin.H {
	return gin.H{"id": user.ID, "name": user.Name, "email": user.Email, "role": user.Role}
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

// estimateScope is the owner filter for estimates: an administrator works with
// every employee's estimates, everyone else only with their own.
func estimateScope(c *gin.Context) int64 {
	user := currentUser(c)
	if user.IsAdmin() {
		return repository.AllUsers
	}
	return user.ID
}
