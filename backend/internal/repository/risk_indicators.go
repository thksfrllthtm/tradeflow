package repository

// Стоимостные индикаторы риска (СУР) КГД РК: профили риска (по файлу на профиль) и индикаторы
// по кодам ТН ВЭД. Загрузка профиля заменяет его целиком.

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type RiskIndicatorProfile struct {
	ID        int64  `gorm:"primaryKey"`
	Number    string `gorm:"not null"`
	FileName  string `gorm:"not null"`
	Groups    JSON   `gorm:"type:jsonb;not null"`
	CodeCount int    `gorm:"not null"`
	// ImportedBy — кто загрузил; ImportedByName заполняют RiskIndicatorProfiles (JOIN с users).
	ImportedBy     *int64    `gorm:"column:imported_by"`
	ImportedAt     time.Time `gorm:"not null"`
	ImportedByName string    `gorm:"->;-:migration;column:imported_by_name"`
}

func (RiskIndicatorProfile) TableName() string { return "risk_indicator_profiles" }

type RiskIndicator struct {
	ProfileID int64  `gorm:"primaryKey"`
	Code      string `gorm:"primaryKey;size:10"`
	// Prices — индикаторы по группам стран профиля (JSON-массив строк, null — индикатора нет).
	Prices JSON `gorm:"type:jsonb;not null"`
}

func (RiskIndicator) TableName() string { return "risk_indicators" }

// RiskIndicatorRow — индикатор кода вместе с профилем, из которого он взят.
type RiskIndicatorRow struct {
	ProfileID     int64
	ProfileNumber string
	Groups        JSON
	Code          string
	Prices        JSON
}

// ReplaceRiskIndicatorProfile сохраняет профиль; профиль с тем же номером удаляется вместе с индикаторами.
func (r *Repository) ReplaceRiskIndicatorProfile(ctx context.Context, profile RiskIndicatorProfile, rows []RiskIndicator) (RiskIndicatorProfile, error) {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("DELETE FROM risk_indicator_profiles WHERE number = ?", profile.Number).Error; err != nil {
			return err
		}
		profile.ImportedAt = time.Now().UTC()
		if err := tx.Omit("ID", "ImportedByName").Create(&profile).Error; err != nil {
			return err
		}
		for index := range rows {
			rows[index].ProfileID = profile.ID
		}
		if len(rows) > 0 {
			return tx.CreateInBatches(rows, 1000).Error
		}
		return nil
	})
	return profile, err
}

// RiskIndicatorProfiles — загруженные профили по номеру.
func (r *Repository) RiskIndicatorProfiles(ctx context.Context) ([]RiskIndicatorProfile, error) {
	var rows []RiskIndicatorProfile
	err := r.db.WithContext(ctx).Table("risk_indicator_profiles AS p").
		Select("p.*, COALESCE(u.name, '') AS imported_by_name").
		Joins("LEFT JOIN users u ON u.id = p.imported_by").
		Order("p.number").Find(&rows).Error
	return rows, err
}

// DeleteRiskIndicatorProfile удаляет профиль и его индикаторы; ErrNotFound, если профиля нет.
func (r *Repository) DeleteRiskIndicatorProfile(ctx context.Context, id int64) error {
	result := r.db.WithContext(ctx).Exec("DELETE FROM risk_indicator_profiles WHERE id = ?", id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// RiskIndicatorsByCodes — индикаторы кодов из всех профилей.
func (r *Repository) RiskIndicatorsByCodes(ctx context.Context, codes []string) ([]RiskIndicatorRow, error) {
	var rows []RiskIndicatorRow
	if len(codes) == 0 {
		return rows, nil
	}
	err := r.db.WithContext(ctx).Table("risk_indicators AS i").
		Select("i.profile_id, p.number AS profile_number, p.groups, i.code, i.prices").
		Joins("JOIN risk_indicator_profiles p ON p.id = i.profile_id").
		Where("i.code IN ?", codes).Order("p.number").Scan(&rows).Error
	return rows, err
}
