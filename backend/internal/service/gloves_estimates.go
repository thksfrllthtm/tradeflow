package service

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
)

type GlovesService struct{ repository *repository.Repository }

// NewGlovesService accepts the repository optionally to keep the service easy
// to construct in isolated tests. In the running server it is always supplied
// so directory selections are re-read and verified on the server.
func NewGlovesService(repositories ...*repository.Repository) *GlovesService {
	var repo *repository.Repository
	if len(repositories) > 0 {
		repo = repositories[0]
	}
	return &GlovesService{repository: repo}
}

func normalizeGlovesEstimateInput(input GlovesEstimateInput) (string, string, time.Time, error) {
	name := strings.TrimSpace(input.Name)
	if name == "" {
		return "", "", time.Time{}, &ValidationError{Field: "name", Message: "Название сметы обязательно"}
	}
	if len([]rune(name)) > 200 {
		return "", "", time.Time{}, &ValidationError{Field: "name", Message: "Название не должно быть длиннее 200 символов"}
	}
	client := strings.TrimSpace(input.Client)
	if len([]rune(client)) > 200 {
		return "", "", time.Time{}, &ValidationError{Field: "client", Message: "Клиент не должен быть длиннее 200 символов"}
	}
	date, err := parseGlovesEstimateDate(input.AssessmentDate)
	if err != nil {
		return "", "", time.Time{}, err
	}
	return name, client, date, nil
}

func parseGlovesEstimateDate(value string) (time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Now().UTC().Truncate(24 * time.Hour), nil
	}
	date, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, &ValidationError{Field: "assessmentDate", Message: "Укажите дату в формате ГГГГ-ММ-ДД"}
	}
	return date, nil
}

func validateGlovesPhotoData(value string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	comma := strings.IndexByte(value, ',')
	if !strings.HasPrefix(strings.ToLower(value), "data:image/") || comma < 0 {
		return fmt.Errorf("фото должно быть встроенным JPG или PNG")
	}
	header := strings.ToLower(value[:comma])
	if !(strings.HasPrefix(header, "data:image/jpeg;base64") || strings.HasPrefix(header, "data:image/png;base64")) {
		return fmt.Errorf("поддерживаются только JPG и PNG")
	}
	decoded, err := base64.StdEncoding.DecodeString(value[comma+1:])
	if err != nil {
		return fmt.Errorf("фото повреждено")
	}
	if len(decoded) > MaxImageBytes {
		return fmt.Errorf("фото больше %d МБ", MaxImageBytes/(1024*1024))
	}
	return nil
}

// estimateOwnerNames returns employee names for the administrator's list of
// all estimates; for an ordinary employee's own list it returns nothing.
func estimateOwnerNames(ctx context.Context, repo *repository.Repository, userID int64) (map[int64]string, error) {
	if userID != repository.AllUsers {
		return nil, nil
	}
	users, err := repo.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	names := make(map[int64]string, len(users))
	for _, user := range users {
		names[user.ID] = user.Name
		if names[user.ID] == "" {
			names[user.ID] = user.Email
		}
	}
	return names, nil
}
