package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
)

// Список, открытие и автосохранение сметы товаров частями: фото строк хранятся отдельно
// и передаются, только когда изменились.

const maxPhotosPerRow = 20

func (s *GoodsEstimateService) ListEstimates(ctx context.Context, userID int64) ([]GlovesEstimateSummary, error) {
	if s.repository == nil {
		return []GlovesEstimateSummary{}, nil
	}
	rows, err := s.repository.ListGoodsEstimates(ctx, userID)
	if err != nil {
		return nil, err
	}
	owners, err := estimateOwnerNames(ctx, s.repository, userID)
	if err != nil {
		return nil, err
	}
	result := make([]GlovesEstimateSummary, 0, len(rows))
	for _, row := range rows {
		summary := goodsSummary(row)
		summary.OwnerName = owners[row.UserID]
		result = append(result, summary)
	}
	return result, nil
}

func goodsSummary(row repository.GoodsEstimateSummary) GlovesEstimateSummary {
	status := "Черновик"
	if row.HasResult {
		status = "Рассчитано"
	}
	return GlovesEstimateSummary{
		ID: row.ID, Name: row.Name, Client: row.Client, AssessmentDate: row.AssessmentDate.Format("2006-01-02"),
		Status: status, CreatedAt: row.CreatedAt.UTC().Format(time.RFC3339), UpdatedAt: row.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// GetEstimate returns the whole estimate: rows get their photos back.
func (s *GoodsEstimateService) GetEstimate(ctx context.Context, id, userID int64) (GlovesEstimateRecord, error) {
	if s.repository == nil {
		return GlovesEstimateRecord{}, repository.ErrNotFound
	}
	row, err := s.repository.GoodsEstimateByIDForUser(ctx, id, userID)
	if err != nil {
		return GlovesEstimateRecord{}, err
	}
	record, err := decodeGoodsEstimate(row)
	if err != nil {
		return GlovesEstimateRecord{}, err
	}
	photos, err := s.repository.GoodsEstimatePhotos(ctx, id)
	if err != nil {
		return GlovesEstimateRecord{}, err
	}
	for index := range record.Input.Items {
		item := &record.Input.Items[index]
		if list := photos[item.ID]; len(list) > 0 {
			item.PhotoURLs = list
			item.PhotoURL = list[0]
		}
	}
	return record, nil
}

// GoodsDraftPatch is what autosave sends: only the changed parts of the draft. A missing
// part stays as it is on the server.
type GoodsDraftPatch struct {
	// Changed input fields (parameters, rows without photos, Excel source).
	Input map[string]json.RawMessage `json:"input"`
	// New calculation result; null — the result is gone (rows changed after calculation).
	Result json.RawMessage `json:"result"`
	// Photos of changed rows by row id; an empty list removes the row's photos.
	Photos map[string][]string `json:"photos"`
}

// inputFields are the JSON names of GlovesCalculationInput: only they may be patched.
var inputFields = func() map[string]bool {
	fields := map[string]bool{}
	kind := reflect.TypeOf(GlovesCalculationInput{})
	for index := 0; index < kind.NumField(); index++ {
		if name := strings.Split(kind.Field(index).Tag.Get("json"), ",")[0]; name != "" && name != "-" {
			fields[name] = true
		}
	}
	return fields
}()

// PatchEstimateDraft overwrites the changed parts of the estimate draft (autosave).
func (s *GoodsEstimateService) PatchEstimateDraft(ctx context.Context, id, userID int64, patch GoodsDraftPatch) (GlovesEstimateSummary, error) {
	if s.repository == nil {
		return GlovesEstimateSummary{}, repository.ErrNotFound
	}
	if userID < 1 && userID != repository.AllUsers {
		return GlovesEstimateSummary{}, &ValidationError{Field: "draft", Message: "Пользователь не определён"}
	}
	change := repository.GoodsDraftPatch{UpdatedAt: time.Now().UTC()}
	if len(patch.Input) > 0 {
		if err := prepareInputPatch(patch.Input, &change); err != nil {
			return GlovesEstimateSummary{}, err
		}
	}
	if len(patch.Result) > 0 {
		result, err := prepareResultPatch(patch.Result)
		if err != nil {
			return GlovesEstimateSummary{}, err
		}
		change.Result = result
	}
	if len(patch.Photos) > 0 {
		for itemID, photos := range patch.Photos {
			if strings.TrimSpace(itemID) == "" || len(itemID) > 128 {
				return GlovesEstimateSummary{}, &ValidationError{Field: "photos", Message: "Фото без строки сметы"}
			}
			if len(photos) > maxPhotosPerRow {
				return GlovesEstimateSummary{}, &ValidationError{Field: "photos", Message: fmt.Sprintf("У строки не может быть больше %d фото", maxPhotosPerRow)}
			}
			for _, photo := range photos {
				if strings.TrimSpace(photo) == "" {
					return GlovesEstimateSummary{}, &ValidationError{Field: "photos", Message: "Пустое фото"}
				}
				if err := validateGlovesPhotoData(photo); err != nil {
					return GlovesEstimateSummary{}, &ValidationError{Field: "photos", Message: err.Error()}
				}
			}
		}
		change.Photos = patch.Photos
	}
	row, err := s.repository.PatchGoodsEstimateDraft(ctx, id, userID, change)
	if err != nil {
		return GlovesEstimateSummary{}, err
	}
	return goodsSummary(row), nil
}

// prepareInputPatch checks the changed input fields by decoding them into the input type.
// Rows are stored without photos; the calculation date also updates the estimate date.
func prepareInputPatch(fields map[string]json.RawMessage, change *repository.GoodsDraftPatch) error {
	invalid := &ValidationError{Field: "input", Message: "Проверьте данные сметы"}
	for name := range fields {
		if !inputFields[name] {
			return &ValidationError{Field: "input", Message: fmt.Sprintf("Неизвестное поле сметы: %s", name)}
		}
	}
	encoded, err := json.Marshal(fields)
	if err != nil {
		return invalid
	}
	var input GlovesCalculationInput
	if err := json.Unmarshal(encoded, &input); err != nil {
		return invalid
	}
	if _, ok := fields["items"]; ok {
		if len(input.Items) > MaxRowsPerSheet {
			return &ValidationError{Field: "items", Message: fmt.Sprintf("В смете не может быть больше %d строк", MaxRowsPerSheet)}
		}
		ids := make([]string, 0, len(input.Items))
		seen := make(map[string]bool, len(input.Items))
		for index := range input.Items {
			item := &input.Items[index]
			item.ID = strings.TrimSpace(item.ID)
			if item.ID == "" || len(item.ID) > 128 || seen[item.ID] {
				return &ValidationError{Field: "items", Message: fmt.Sprintf("Строка %d: нет уникального id", index+1)}
			}
			seen[item.ID] = true
			ids = append(ids, item.ID)
			item.PhotoURL, item.PhotoURLs = "", nil
		}
		items, err := json.Marshal(input.Items)
		if err != nil {
			return invalid
		}
		fields["items"] = items
		change.ItemIDs = ids
	}
	if _, ok := fields["assessmentDate"]; ok {
		date, err := parseGlovesEstimateDate(input.AssessmentDate)
		if err != nil {
			return err
		}
		change.AssessmentDate = &date
	}
	data, err := json.Marshal(fields)
	if err != nil {
		return invalid
	}
	change.Input = repository.JSON(data)
	return nil
}

func prepareResultPatch(raw json.RawMessage) (repository.JSON, error) {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return repository.JSON("{}"), nil
	}
	var result GlovesCalculationResult
	if err := json.Unmarshal(raw, &result); err != nil {
		return nil, &ValidationError{Field: "result", Message: "Проверьте результат расчёта"}
	}
	data, err := json.Marshal(result)
	if err != nil {
		return nil, &ValidationError{Field: "result", Message: "Проверьте результат расчёта"}
	}
	return repository.JSON(data), nil
}
