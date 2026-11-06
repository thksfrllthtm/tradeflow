package repository

// Результаты последних обновлений справочников (data_update_results) для страницы «Обновление данных».

import (
	"context"
	"encoding/json"
	"time"
)

// DataUpdateResult — последний результат набора; Result — JSON ответа загрузки, UpdatedByName — текущее имя.
type DataUpdateResult struct {
	Dataset       string
	Result        json.RawMessage
	UpdatedAt     time.Time
	UpdatedByName string
}

// SaveDataUpdate заменяет результат набора новым.
func (r *Repository) SaveDataUpdate(ctx context.Context, dataset string, userID int64, result json.RawMessage) error {
	return r.db.WithContext(ctx).Exec(`INSERT INTO data_update_results (dataset, result, updated_by, updated_at)
		VALUES (?, ?::jsonb, ?, now())
		ON CONFLICT (dataset) DO UPDATE SET result = EXCLUDED.result, updated_by = EXCLUDED.updated_by, updated_at = EXCLUDED.updated_at`,
		dataset, string(result), userID).Error
}

func (r *Repository) DataUpdates(ctx context.Context) ([]DataUpdateResult, error) {
	var rows []struct {
		Dataset       string
		Result        string
		UpdatedAt     time.Time
		UpdatedByName string
	}
	err := r.db.WithContext(ctx).Raw(`SELECT d.dataset, d.result::text AS result, d.updated_at, COALESCE(u.name, '') AS updated_by_name
		FROM data_update_results d LEFT JOIN users u ON u.id = d.updated_by`).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make([]DataUpdateResult, len(rows))
	for i, row := range rows {
		out[i] = DataUpdateResult{Dataset: row.Dataset, Result: json.RawMessage(row.Result), UpdatedAt: row.UpdatedAt, UpdatedByName: row.UpdatedByName}
	}
	return out, nil
}
