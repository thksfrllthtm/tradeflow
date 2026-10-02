package httpapi

// Employee management (administrators only): list, create, change name/email/role/password,
// block, delete.

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/auth"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/employees"
	"github.com/gin-gonic/gin"
)

func (s *Server) listEmployees(c *gin.Context) {
	employees, err := s.employees.List(c.Request.Context())
	if err != nil {
		internalError(c)
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": employees})
}

func (s *Server) createEmployee(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 12<<10)
	var input employees.EmployeeInput
	if err := c.ShouldBindJSON(&input); err != nil {
		badRequest(c, "Заполните имя, email, пароль и роль", "")
		return
	}
	employee, err := s.employees.Create(c.Request.Context(), input)
	if !handleEmployeeError(c, err) {
		return
	}
	c.JSON(http.StatusCreated, gin.H{"item": employee})
}

func (s *Server) updateEmployee(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"message": "Сотрудник не найден"}})
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 12<<10)
	var update employees.EmployeeUpdate
	if err := c.ShouldBindJSON(&update); err != nil {
		badRequest(c, "Некорректные данные сотрудника", "")
		return
	}
	employee, err := s.employees.Update(c.Request.Context(), currentUser(c), id, update)
	if !handleEmployeeError(c, err) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"item": employee})
}

// deleteEmployee removes an employee; their estimates and directory entries pass to the
// administrator. Answers with the number of estimates moved.
func (s *Server) deleteEmployee(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id < 1 {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"message": "Сотрудник не найден"}})
		return
	}
	moved, err := s.employees.Delete(c.Request.Context(), currentUser(c), id)
	if !handleEmployeeError(c, err) {
		return
	}
	c.JSON(http.StatusOK, gin.H{"movedEstimates": moved})
}

// handleEmployeeError writes the error response and reports whether the
// request may continue.
func handleEmployeeError(c *gin.Context, err error) bool {
	if err == nil {
		return true
	}
	var validation *service.ValidationError
	switch {
	case errors.As(err, &validation):
		badRequest(c, validation.Message, validation.Field)
	case errors.Is(err, auth.ErrEmailAlreadyExists):
		c.JSON(http.StatusConflict, gin.H{"error": gin.H{"message": "Сотрудник с таким email уже есть", "field": "email"}})
	case errors.Is(err, repository.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"message": "Сотрудник не найден"}})
	default:
		internalError(c)
	}
	return false
}
