package repository

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// JSON is a PostgreSQL JSONB value with explicit database conversion.
type JSON json.RawMessage

func (j *JSON) Scan(value any) error {
	if value == nil {
		*j = JSON("null")
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return fmt.Errorf("cannot scan %T into JSON", value)
	}
	copyValue := make([]byte, len(bytes))
	copy(copyValue, bytes)
	*j = JSON(copyValue)
	return nil
}

func (j JSON) Value() (driver.Value, error) {
	if len(j) == 0 {
		return "null", nil
	}
	if !json.Valid(j) {
		return nil, fmt.Errorf("invalid JSON")
	}
	return string(j), nil
}

func (j JSON) MarshalJSON() ([]byte, error) {
	if !json.Valid(j) {
		return []byte("null"), nil
	}
	return j, nil
}

type User struct {
	ID           int64      `gorm:"primaryKey"`
	Name         string     `gorm:"size:120;not null" json:"name"`
	Email        string     `gorm:"size:320;not null;unique"`
	PasswordHash string     `gorm:"not null"`
	Role         string     `gorm:"size:16;not null"`
	DisabledAt   *time.Time `gorm:"column:disabled_at"`
	CreatedAt    time.Time  `gorm:"not null"`
	UpdatedAt    time.Time  `gorm:"not null"`
}

// Роли сотрудников (users.role).
const (
	RoleAdmin      = "admin"      // полный доступ, управление сотрудниками и справочниками
	RoleInvoicer   = "invoicer"   // калькуляторы; справочники для просмотра, кроме добавления кода ТН ВЭД без статуса «Проверено»
	RoleTranslator = "translator" // пока те же права, что у инвойсера
)

func ValidRole(role string) bool {
	return role == RoleAdmin || role == RoleInvoicer || role == RoleTranslator
}

func (u User) IsAdmin() bool { return u.Role == RoleAdmin }

func (User) TableName() string { return "users" }

type Session struct {
	ID        int64     `gorm:"primaryKey"`
	TokenHash string    `gorm:"size:64;not null;unique"`
	UserID    int64     `gorm:"not null"`
	ExpiresAt time.Time `gorm:"not null"`
	CreatedAt time.Time `gorm:"not null"`
}

func (Session) TableName() string { return "sessions" }

// TNVEDDirectoryEntry is an operator-managed overlay. It stores friendly
// names and preliminary rate parameters without changing immutable classifier
// versions or already saved estimates.
type TNVEDDirectoryEntry struct {
	ID                 int64            `gorm:"primaryKey" json:"id"`
	Category           string           `gorm:"size:64;not null" json:"category"`
	Name               string           `gorm:"size:300;not null" json:"name"`
	NameZH             string           `gorm:"column:name_zh;type:text;not null" json:"nameZh"`
	Code               string           `gorm:"size:10;not null" json:"code"`
	CompanyCoefficient *decimal.Decimal `gorm:"column:company_coefficient;type:numeric(30,12)" json:"companyCoefficient,omitempty"`
	AntiDumpingPercent *decimal.Decimal `gorm:"column:anti_dumping_percent;type:numeric(30,12)" json:"antiDumpingPercent,omitempty"`
	SynonymsRU         JSON             `gorm:"column:synonyms_ru;type:jsonb;not null" json:"synonymsRu"`
	SynonymsZH         JSON             `gorm:"column:synonyms_zh;type:jsonb;not null" json:"synonymsZh"`
	SynonymsEN         JSON             `gorm:"column:synonyms_en;type:jsonb;not null" json:"synonymsEn"`
	VerificationStatus string           `gorm:"column:verification_status;size:24;not null" json:"verificationStatus"`
	DutyType           string           `gorm:"column:duty_type;size:24;not null" json:"dutyType"`
	DutyPercent        *decimal.Decimal `gorm:"column:duty_percent;type:numeric(30,12)" json:"dutyPercent,omitempty"`
	DutyAmount         *decimal.Decimal `gorm:"column:duty_amount;type:numeric(30,12)" json:"dutyAmount,omitempty"`
	DutyCurrency       string           `gorm:"column:duty_currency;size:3;not null" json:"dutyCurrency"`
	DutyUnit           string           `gorm:"column:duty_unit;size:64;not null" json:"dutyUnit"`
	DutyValidFrom      *time.Time       `gorm:"column:duty_valid_from;type:date" json:"dutyValidFrom,omitempty"`
	DutyValidTo        *time.Time       `gorm:"column:duty_valid_to;type:date" json:"dutyValidTo,omitempty"`
	DutyCheckedAt      *time.Time       `gorm:"column:duty_checked_at;type:date" json:"dutyCheckedAt,omitempty"`
	DutySource         string           `gorm:"column:duty_source;type:text;not null" json:"dutySource"`
	VATType            string           `gorm:"column:vat_type;size:16;not null" json:"vatType"`
	VATPercent         *decimal.Decimal `gorm:"column:vat_percent;type:numeric(30,12)" json:"vatPercent,omitempty"`
	VATBasis           string           `gorm:"column:vat_basis;type:text;not null" json:"vatBasis"`
	VATValidFrom       *time.Time       `gorm:"column:vat_valid_from;type:date" json:"vatValidFrom,omitempty"`
	VATValidTo         *time.Time       `gorm:"column:vat_valid_to;type:date" json:"vatValidTo,omitempty"`
	VATCheckedAt       *time.Time       `gorm:"column:vat_checked_at;type:date" json:"vatCheckedAt,omitempty"`
	VATSource          string           `gorm:"column:vat_source;type:text;not null" json:"vatSource"`
	ArchivedAt         *time.Time       `gorm:"column:archived_at" json:"archivedAt,omitempty"`
	ArchivedBy         *int64           `gorm:"column:archived_by" json:"archivedBy,omitempty"`
	CreatedBy          int64            `gorm:"column:created_by;not null" json:"createdBy"`
	UpdatedBy          int64            `gorm:"column:updated_by;not null" json:"updatedBy"`
	CreatedAt          time.Time        `gorm:"not null" json:"createdAt"`
	UpdatedAt          time.Time        `gorm:"not null" json:"updatedAt"`
}

func (TNVEDDirectoryEntry) TableName() string { return "tnved_directory_entries" }

// ProductDirectoryEntry is a reusable product card. Unlike an imported row,
// it is shared between estimates and keeps only the operator's master data.
// The linked TN VED card is copied into an estimate as an immutable snapshot.
type ProductDirectoryEntry struct {
	ID                    int64                `gorm:"primaryKey" json:"id"`
	Category              string               `gorm:"size:64;not null" json:"category"`
	Name                  string               `gorm:"size:300;not null" json:"name"`
	NameZH                string               `gorm:"column:name_zh;type:text;not null" json:"nameZh"`
	Unit                  string               `gorm:"size:64;not null" json:"unit"`
	CompanyCoefficient    *decimal.Decimal     `gorm:"column:company_coefficient;type:numeric(30,12)" json:"companyCoefficient,omitempty"`
	SynonymsRU            JSON                 `gorm:"column:synonyms_ru;type:jsonb;not null" json:"synonymsRu"`
	SynonymsZH            JSON                 `gorm:"column:synonyms_zh;type:jsonb;not null" json:"synonymsZh"`
	SynonymsEN            JSON                 `gorm:"column:synonyms_en;type:jsonb;not null" json:"synonymsEn"`
	TNVEDDirectoryEntryID *int64               `gorm:"column:tnved_directory_entry_id" json:"tnvedDirectoryEntryId,omitempty"`
	TNVEDDirectoryEntry   *TNVEDDirectoryEntry `gorm:"foreignKey:TNVEDDirectoryEntryID" json:"-"`
	VerificationStatus    string               `gorm:"column:verification_status;size:24;not null" json:"verificationStatus"`
	ArchivedAt            *time.Time           `gorm:"column:archived_at" json:"archivedAt,omitempty"`
	ArchivedBy            *int64               `gorm:"column:archived_by" json:"archivedBy,omitempty"`
	CreatedBy             int64                `gorm:"column:created_by;not null" json:"createdBy"`
	UpdatedBy             int64                `gorm:"column:updated_by;not null" json:"updatedBy"`
	CreatedAt             time.Time            `gorm:"not null" json:"createdAt"`
	UpdatedAt             time.Time            `gorm:"not null" json:"updatedAt"`
}

func (ProductDirectoryEntry) TableName() string { return "product_directory_entries" }

// GoodsEstimate is a goods estimate: parameters and rows (input_snapshot, without photos)
// and the last calculation result. Row photos live in goods_estimate_photos.
type GoodsEstimate struct {
	ID             int64     `gorm:"primaryKey" json:"id"`
	UserID         int64     `gorm:"column:user_id;not null;index" json:"userId"`
	Name           string    `gorm:"size:200;not null" json:"name"`
	Client         string    `gorm:"size:200;not null" json:"client"`
	AssessmentDate time.Time `gorm:"column:assessment_date;type:date;not null" json:"assessmentDate"`
	InputSnapshot  JSON      `gorm:"column:input_snapshot;type:jsonb;not null" json:"inputSnapshot"`
	ResultSnapshot JSON      `gorm:"column:result_snapshot;type:jsonb;not null" json:"resultSnapshot"`
	CreatedAt      time.Time `gorm:"column:created_at;not null" json:"createdAt"`
	UpdatedAt      time.Time `gorm:"column:updated_at;not null" json:"updatedAt"`
}

func (GoodsEstimate) TableName() string { return "goods_estimates" }

// GoodsEstimateSummary is a goods estimate without rows, result and photos: the estimates list.
type GoodsEstimateSummary struct {
	ID             int64
	UserID         int64
	Name           string
	Client         string
	AssessmentDate time.Time
	HasResult      bool
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// GoodsEstimatePhoto holds the photos of one estimate row (data URLs, the first is the main one).
type GoodsEstimatePhoto struct {
	EstimateID int64  `gorm:"column:estimate_id;primaryKey"`
	ItemID     string `gorm:"column:item_id;primaryKey"`
	Photos     JSON   `gorm:"column:photos;type:jsonb;not null"`
}

func (GoodsEstimatePhoto) TableName() string { return "goods_estimate_photos" }
