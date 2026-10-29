package repository

// Таможенный реестр объектов интеллектуальной собственности (ТРОИС): записи из файла КГД. Загрузка
// заменяет реестр целиком; надписи логотипов (ИИ или администратор) переносятся по номеру.

import (
	"context"
	"strings"
	"time"

	"gorm.io/gorm"
)

type Trademark struct {
	RegNo           string `gorm:"primaryKey;column:reg_no"`
	Name            string
	Logo            *string
	LogoName        string
	LogoNameSource  string
	LogoPicture     string
	LogoReadAt      *time.Time
	Goods           string
	Codes           string
	Holder          string
	Document        string
	ProtectedUntil  *time.Time
	Representatives string
	ImportedBy      *int64
	ImportedAt      time.Time
}

func (Trademark) TableName() string { return "trademarks" }

// TrademarkRow — запись без картинки логотипа (HasLogo — картинка есть).
type TrademarkRow struct {
	RegNo           string
	Name            string
	HasLogo         bool
	LogoName        string
	LogoNameSource  string
	LogoPicture     string
	LogoReadAt      *time.Time
	Goods           string
	Codes           string
	Holder          string
	Document        string
	ProtectedUntil  *time.Time
	Representatives string
}

const trademarkRowColumns = "reg_no, name, logo IS NOT NULL AS has_logo, logo_name, logo_name_source, logo_picture, logo_read_at, goods, codes, holder, document, protected_until, representatives"

// TrademarkSummary — сколько записей и логотипов, сколько логотипов ИИ уже смотрел, когда и кто загрузил.
type TrademarkSummary struct {
	Total          int64
	Logos          int64
	LogosRead      int64
	ImportedAt     *time.Time
	ImportedByName string
}

// TrademarkLogo — логотип для ИИ.
type TrademarkLogo struct {
	RegNo string
	Logo  string
}

// ReplaceTrademarks заменяет реестр; надписи логотипов прежних записей с теми же номерами сохраняются.
func (r *Repository) ReplaceTrademarks(ctx context.Context, rows []Trademark, importedBy int64) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var previous []Trademark
		if err := tx.Select("reg_no, logo_name, logo_name_source, logo_picture, logo_read_at").
			Where("logo_name_source <> '' OR logo_read_at IS NOT NULL").Find(&previous).Error; err != nil {
			return err
		}
		kept := map[string]Trademark{}
		for _, row := range previous {
			kept[row.RegNo] = row
		}
		now := time.Now().UTC()
		for index := range rows {
			rows[index].ImportedBy, rows[index].ImportedAt = &importedBy, now
			if old, ok := kept[rows[index].RegNo]; ok && rows[index].Logo != nil {
				rows[index].LogoName, rows[index].LogoNameSource, rows[index].LogoPicture, rows[index].LogoReadAt = old.LogoName, old.LogoNameSource, old.LogoPicture, old.LogoReadAt
			}
		}
		if err := tx.Exec("DELETE FROM trademarks").Error; err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.CreateInBatches(rows, 100).Error
	})
}

// TrademarkRows — все записи без картинок (для поиска в наименованиях).
func (r *Repository) TrademarkRows(ctx context.Context) ([]TrademarkRow, error) {
	var rows []TrademarkRow
	err := r.db.WithContext(ctx).Table("trademarks").Select(trademarkRowColumns).Order("reg_no").Scan(&rows).Error
	return rows, err
}

// SearchTrademarks — записи по части номера, наименования, надписи логотипа или правообладателя.
// onlyUnnamed — знаки-изображения без надписи (их администратор подписывает вручную).
func (r *Repository) SearchTrademarks(ctx context.Context, query string, onlyUnnamed bool, limit, offset int) ([]TrademarkRow, int64, error) {
	base := r.db.WithContext(ctx).Table("trademarks")
	if text := strings.TrimSpace(query); text != "" {
		like := "%" + escapeLike(text) + "%"
		base = base.Where("reg_no ILIKE ? OR name ILIKE ? OR logo_name ILIKE ? OR holder ILIKE ?", like, like, like, like)
	}
	if onlyUnnamed {
		base = base.Where("name = '' AND logo_name = ''")
	}
	var total int64
	if err := base.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []TrademarkRow
	err := base.Select(trademarkRowColumns).Order("reg_no DESC").Limit(limit).Offset(offset).Scan(&rows).Error
	return rows, total, err
}

func (r *Repository) TrademarkSummary(ctx context.Context) (TrademarkSummary, error) {
	var summary TrademarkSummary
	err := r.db.WithContext(ctx).Raw(`SELECT count(*) AS total, count(logo) AS logos, count(logo_read_at) FILTER (WHERE logo IS NOT NULL) AS logos_read,
		max(t.imported_at) AS imported_at, COALESCE(max(u.name), '') AS imported_by_name
		FROM trademarks t LEFT JOIN users u ON u.id = t.imported_by`).Scan(&summary).Error
	return summary, err
}

// TrademarkLogoImage — картинка логотипа; ErrNotFound — записи или картинки нет.
func (r *Repository) TrademarkLogoImage(ctx context.Context, regNo string) (string, error) {
	var row struct{ Logo *string }
	if err := r.db.WithContext(ctx).Table("trademarks").Select("logo").Where("reg_no = ?", regNo).Scan(&row).Error; err != nil {
		return "", err
	}
	if row.Logo == nil || *row.Logo == "" {
		return "", ErrNotFound
	}
	return *row.Logo, nil
}

// TrademarkLogos — логотипы записей по номерам (для сверки с фото товара).
func (r *Repository) TrademarkLogos(ctx context.Context, regNos []string) ([]TrademarkLogo, error) {
	var rows []TrademarkLogo
	if len(regNos) == 0 {
		return rows, nil
	}
	err := r.db.WithContext(ctx).Table("trademarks").Select("reg_no, logo").Where("reg_no IN ? AND logo IS NOT NULL", regNos).Scan(&rows).Error
	return rows, err
}

// TrademarkLogosToRead — логотипы, которые ИИ ещё не смотрел.
func (r *Repository) TrademarkLogosToRead(ctx context.Context, limit int) ([]TrademarkLogo, error) {
	var rows []TrademarkLogo
	err := r.db.WithContext(ctx).Table("trademarks").Select("reg_no, logo").
		Where("logo IS NOT NULL AND logo_read_at IS NULL").Order("reg_no").Limit(limit).Scan(&rows).Error
	return rows, err
}

// LogoReading — что ИИ прочитал на логотипе: надпись ("" — не прочитал) и что изображено.
type LogoReading struct {
	Name    string
	Picture string
}

// SaveTrademarkLogoReadings — прочитанное ИИ по номеру; подпись администратора не меняется.
func (r *Repository) SaveTrademarkLogoReadings(ctx context.Context, readings map[string]LogoReading) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		for regNo, reading := range readings {
			source := "ai"
			if reading.Name == "" {
				source = ""
			}
			if err := tx.Exec(`UPDATE trademarks SET logo_read_at = ?, logo_picture = ?,
				logo_name = CASE WHEN logo_name_source = 'admin' THEN logo_name ELSE ? END,
				logo_name_source = CASE WHEN logo_name_source = 'admin' THEN logo_name_source ELSE ? END
				WHERE reg_no = ?`, now, reading.Picture, reading.Name, source, regNo).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// SetTrademarkLogoName — подпись логотипа администратором (пусто — снять подпись); ErrNotFound — нет записи.
func (r *Repository) SetTrademarkLogoName(ctx context.Context, regNo, name string) error {
	source := "admin"
	if name == "" {
		source = ""
	}
	result := r.db.WithContext(ctx).Exec("UPDATE trademarks SET logo_name = ?, logo_name_source = ? WHERE reg_no = ?", name, source, regNo)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}
