package service

// Common service errors: invalid input for a field and missing records.

import (
	"errors"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
)

type ValidationError struct {
	Field   string
	Message string
}

func (e *ValidationError) Error() string { return e.Message }

func IsNotFound(err error) bool {
	return errors.Is(err, repository.ErrNotFound)
}
