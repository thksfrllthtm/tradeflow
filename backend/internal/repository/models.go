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
	RoleInvoicer   = "invoicer"   // калькуляторы; справочники только для просмотра
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

type Estimate struct {
	ID             int64           `gorm:"primaryKey"`
	OwnerID        int64           `gorm:"not null"`
	Name           string          `gorm:"size:200;not null"`
	Client         string          `gorm:"size:200"`
	AssessmentDate time.Time       `gorm:"type:date;not null"`
	ImportCountry  string          `gorm:"size:2;not null"`
	Currency       string          `gorm:"size:3;not null"`
	ExchangeRate   decimal.Decimal `gorm:"type:numeric(20,8);not null"`
	Origin         string          `gorm:"size:200"`
	DeliveryTerms  string          `gorm:"size:100"`
	Note           string          `gorm:"type:text"`
	Status         string          `gorm:"size:32;not null"`
	CreatedBy      int64           `gorm:"not null"`
	UpdatedBy      int64           `gorm:"not null"`
	CreatedAt      time.Time       `gorm:"not null"`
	UpdatedAt      time.Time       `gorm:"not null"`
}

func (Estimate) TableName() string { return "estimates" }

type EstimateImport struct {
	ID               int64     `gorm:"primaryKey"`
	EstimateID       int64     `gorm:"not null"`
	OriginalFilename string    `gorm:"size:255;not null"`
	StoredPath       string    `gorm:"not null"`
	SHA256           string    `gorm:"size:64;not null"`
	FileSize         int64     `gorm:"not null"`
	Status           string    `gorm:"size:32;not null"`
	CreatedBy        int64     `gorm:"not null"`
	CreatedAt        time.Time `gorm:"not null"`
	ConfirmedAt      *time.Time
	ConfirmedBy      *int64
}

func (EstimateImport) TableName() string { return "estimate_imports" }

type ImportedProduct struct {
	ID               int64            `gorm:"primaryKey"`
	EstimateID       int64            `gorm:"not null"`
	SourceImportID   int64            `gorm:"not null"`
	SourceSheet      string           `gorm:"size:255;not null"`
	SourceRow        int              `gorm:"not null"`
	SourceValues     JSON             `gorm:"type:jsonb;not null"`
	Name             string           `gorm:"not null"`
	Translation      string           `gorm:"not null"`
	SKU              string           `gorm:"size:255;not null"`
	Quantity         *decimal.Decimal `gorm:"type:numeric(20,6)"`
	QuantityUnit     string           `gorm:"size:64;not null"`
	Pairs            *decimal.Decimal `gorm:"type:numeric(30,12)" json:"pairs,omitempty"`
	Places           *decimal.Decimal `gorm:"type:numeric(30,12)" json:"places,omitempty"`
	PairsPerPlace    *decimal.Decimal `gorm:"column:pairs_per_place;type:numeric(30,12)" json:"pairsPerPlace,omitempty"`
	UnitPrice        *decimal.Decimal `gorm:"type:numeric(20,8)"`
	LineTotal        *decimal.Decimal `gorm:"type:numeric(20,8)"`
	Currency         string           `gorm:"size:3;not null"`
	Weight           *decimal.Decimal `gorm:"type:numeric(20,6)"`
	GrossWeight      *decimal.Decimal `gorm:"type:numeric(20,6)"`
	GrossWeightUnit  string           `gorm:"size:8;not null"`
	GrossWeightBasis string           `gorm:"size:16;not null"`
	NetWeight        *decimal.Decimal `gorm:"type:numeric(20,6)"`
	NetWeightUnit    string           `gorm:"size:8;not null"`
	NetWeightBasis   string           `gorm:"size:16;not null"`
	Material         string           `gorm:"type:text;not null"`
	Characteristics  string           `gorm:"type:text;not null"`
	PhotoURL         string           `gorm:"type:text;not null"`
	// Keep explicit column names in sync with migration 000006. GORM's
	// default acronym conversion would otherwise query tnved_code (without the
	// separator), while the versioned schema uses tn_ved_code.
	TNVEDCode              string    `gorm:"column:tn_ved_code;size:10;not null" json:"tnVedCode"`
	TNVEDCodeSource        string    `gorm:"column:tn_ved_code_source;size:16;not null" json:"tnVedCodeSource"`
	TNVEDDirectoryEntryID  *int64    `gorm:"column:tnved_directory_entry_id" json:"tnvedDirectoryEntryId,omitempty"`
	TNVEDDirectorySnapshot JSON      `gorm:"column:tnved_directory_snapshot;type:jsonb;not null" json:"tnvedDirectorySnapshot"`
	AIDataRevision         int       `gorm:"column:ai_data_revision;not null" json:"aiDataRevision"`
	AdditionalFields       JSON      `gorm:"type:jsonb;not null"`
	PriceBasis             string    `gorm:"size:32;not null"`
	PriceUnitLabel         string    `gorm:"size:100;not null"`
	WeightBasis            string    `gorm:"size:32;not null"`
	ImportStatus           string    `gorm:"size:32;not null"`
	Warnings               JSON      `gorm:"type:jsonb;not null"`
	Excluded               bool      `gorm:"not null"`
	ExclusionReason        string    `gorm:"type:text;not null"`
	CreatedBy              int64     `gorm:"not null"`
	UpdatedBy              int64     `gorm:"not null"`
	CreatedAt              time.Time `gorm:"not null"`
	UpdatedAt              time.Time `gorm:"not null"`
	// ImageAssetIDs is populated by the service for API responses and is not a database column.
	ImageAssetIDs []int64 `gorm:"-" json:"-"`
}

func (ImportedProduct) TableName() string { return "imported_products" }

type ProductAIAnalysis struct {
	ID            int64     `gorm:"primaryKey" json:"id"`
	ProductID     int64     `gorm:"not null" json:"productId"`
	UserID        int64     `gorm:"not null" json:"userId"`
	Model         string    `gorm:"size:128;not null" json:"model"`
	Status        string    `gorm:"size:32;not null" json:"status"`
	InputSnapshot JSON      `gorm:"type:jsonb;not null" json:"inputSnapshot"`
	Result        JSON      `gorm:"type:jsonb;not null" json:"result"`
	CreatedAt     time.Time `gorm:"not null" json:"createdAt"`
	NeedsRecheck  bool      `gorm:"not null" json:"needsRecheck"`
}

func (ProductAIAnalysis) TableName() string { return "product_ai_analyses" }

type ProductAIDialogue struct {
	ID        int64     `gorm:"primaryKey" json:"id"`
	ProductID int64     `gorm:"not null;unique" json:"productId"`
	CreatedBy int64     `gorm:"not null" json:"createdBy"`
	CreatedAt time.Time `gorm:"not null" json:"createdAt"`
	UpdatedAt time.Time `gorm:"not null" json:"updatedAt"`
}

func (ProductAIDialogue) TableName() string { return "product_ai_dialogues" }

type ProductAIMessage struct {
	ID                 int64     `gorm:"primaryKey" json:"id"`
	DialogueID         int64     `gorm:"not null" json:"dialogueId"`
	UserID             *int64    `json:"userId"`
	Author             string    `gorm:"size:16;not null" json:"author"`
	Content            string    `gorm:"type:text;not null" json:"content"`
	AttachmentAssetIDs JSON      `gorm:"type:jsonb;not null" json:"attachmentAssetIds"`
	Response           JSON      `gorm:"type:jsonb;not null" json:"response"`
	Model              string    `gorm:"size:128;not null" json:"model"`
	Status             string    `gorm:"size:32;not null" json:"status"`
	ClientRequestID    *string   `gorm:"size:128" json:"clientRequestId"`
	AnalysisID         *int64    `json:"analysisId"`
	ProductRevision    int       `gorm:"not null" json:"productRevision"`
	CreatedAt          time.Time `gorm:"not null" json:"createdAt"`
}

func (ProductAIMessage) TableName() string { return "product_ai_messages" }

type ImportAsset struct {
	ID          int64     `gorm:"primaryKey"`
	ImportID    int64     `gorm:"not null"`
	SourceSheet string    `gorm:"size:255;not null"`
	SourceCell  string    `gorm:"size:32;not null"`
	SourceRow   int       `gorm:"not null"`
	StoredPath  string    `gorm:"not null"`
	MIMEType    string    `gorm:"size:100;not null"`
	FileSize    int64     `gorm:"not null"`
	Kind        string    `gorm:"size:32;not null"`
	CreatedAt   time.Time `gorm:"not null"`
}

func (ImportAsset) TableName() string { return "import_assets" }

type ImportedProductImage struct {
	ProductID int64 `gorm:"primaryKey"`
	AssetID   int64 `gorm:"primaryKey"`
}

func (ImportedProductImage) TableName() string { return "imported_product_images" }

type ImportWorkspace struct {
	ID             int64     `gorm:"primaryKey"`
	ImportID       int64     `gorm:"not null;unique"`
	Sheet          string    `gorm:"size:255;not null"`
	HeaderRow      int       `gorm:"not null"`
	DataStartRow   int       `gorm:"not null"`
	Columns        JSON      `gorm:"type:jsonb;not null"`
	Rows           JSON      `gorm:"type:jsonb;not null"`
	DeletedRows    JSON      `gorm:"type:jsonb;not null"`
	DeletedColumns JSON      `gorm:"type:jsonb;not null"`
	Batch          JSON      `gorm:"type:jsonb;not null"`
	PriceBasis     string    `gorm:"size:32;not null"`
	PriceUnitLabel string    `gorm:"size:100;not null"`
	Revision       int       `gorm:"not null"`
	UpdatedBy      int64     `gorm:"not null"`
	UpdatedAt      time.Time `gorm:"not null"`
}

func (ImportWorkspace) TableName() string { return "import_workspaces" }

// ClassifierVersion identifies one immutable classifier/rules dataset.
type ClassifierVersion struct {
	ID          int64      `gorm:"primaryKey" json:"id"`
	VersionKey  string     `gorm:"size:128;not null;unique" json:"versionKey"`
	DatasetKind string     `gorm:"size:16;not null" json:"datasetKind"`
	Source      string     `gorm:"type:text;not null" json:"source"`
	SourceDate  *time.Time `gorm:"type:date" json:"sourceDate,omitempty"`
	LoadedAt    time.Time  `gorm:"not null" json:"loadedAt"`
	Status      string     `gorm:"size:16;not null" json:"status"`
}

func (ClassifierVersion) TableName() string { return "classifier_versions" }

type ClassifierCode struct {
	ID                  int64      `gorm:"primaryKey" json:"id"`
	VersionID           int64      `gorm:"not null" json:"versionId"`
	Code                string     `gorm:"size:32;not null" json:"code"`
	HierarchyLevel      int        `gorm:"not null" json:"hierarchyLevel"`
	ParentCode          string     `gorm:"size:32;not null" json:"parentCode"`
	OfficialDescription string     `gorm:"type:text;not null" json:"officialDescription"`
	IsGroup             bool       `gorm:"not null" json:"isGroup"`
	ValidFrom           time.Time  `gorm:"type:date;not null" json:"validFrom"`
	ValidTo             *time.Time `gorm:"type:date" json:"validTo,omitempty"`
	Source              string     `gorm:"type:text;not null" json:"source"`
}

func (ClassifierCode) TableName() string { return "classifier_codes" }

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

type ClassifierDocument struct {
	ID         int64      `gorm:"primaryKey" json:"id"`
	VersionID  int64      `gorm:"not null" json:"versionId"`
	CodeID     *int64     `json:"codeId,omitempty"`
	ParentCode string     `gorm:"size:32;not null" json:"parentCode"`
	DocType    string     `gorm:"size:32;not null" json:"docType"`
	Title      string     `gorm:"size:255;not null" json:"title"`
	Content    string     `gorm:"type:text;not null" json:"content"`
	Source     string     `gorm:"type:text;not null" json:"source"`
	ValidFrom  time.Time  `gorm:"type:date;not null" json:"validFrom"`
	ValidTo    *time.Time `gorm:"type:date" json:"validTo,omitempty"`
}

func (ClassifierDocument) TableName() string { return "classifier_documents" }

type CalculationRule struct {
	ID          int64            `gorm:"primaryKey" json:"id"`
	VersionID   int64            `gorm:"not null" json:"versionId"`
	CodeID      *int64           `json:"codeId,omitempty"`
	ParentCode  string           `gorm:"size:32;not null" json:"parentCode"`
	RuleKey     string           `gorm:"size:128;not null" json:"ruleKey"`
	PaymentType string           `gorm:"size:64;not null" json:"paymentType"`
	RateType    string           `gorm:"size:32;not null" json:"rateType"`
	Value       *decimal.Decimal `gorm:"type:numeric(30,12)" json:"value,omitempty"`
	Currency    string           `gorm:"size:3;not null" json:"currency"`
	Unit        string           `gorm:"size:64;not null" json:"unit"`
	Operation   string           `gorm:"size:32;not null" json:"operation"`
	Conditions  JSON             `gorm:"type:jsonb;not null" json:"conditions"`
	ValidFrom   time.Time        `gorm:"type:date;not null" json:"validFrom"`
	ValidTo     *time.Time       `gorm:"type:date" json:"validTo,omitempty"`
	Source      string           `gorm:"type:text;not null" json:"source"`
}

func (CalculationRule) TableName() string { return "calculation_rules" }

// CustomsCalculation is the editable, preliminary customs calculation for one
// estimate (one declaration). Items contain a server-calculated snapshot of
// imported products and directory parameters so later directory edits do not
// change an existing calculation silently.
type CustomsCalculation struct {
	ID               int64            `gorm:"primaryKey" json:"id"`
	EstimateID       int64            `gorm:"not null;unique" json:"estimateId"`
	AssessmentDate   time.Time        `gorm:"type:date;not null" json:"assessmentDate"`
	Origin           string           `gorm:"size:200;not null" json:"origin"`
	USDKZT           decimal.Decimal  `gorm:"column:usd_kzt;type:numeric(30,12);not null" json:"usdKzt"`
	EURKZT           *decimal.Decimal `gorm:"column:eur_kzt;type:numeric(30,12)" json:"eurKzt,omitempty"`
	Wagons           *decimal.Decimal `gorm:"type:numeric(30,12)" json:"wagons,omitempty"`
	WagonTariffUSD   decimal.Decimal  `gorm:"column:wagon_tariff_usd;type:numeric(30,12);not null" json:"wagonTariffUsd"`
	DeliveryMode     string           `gorm:"column:delivery_mode;size:16;not null" json:"deliveryMode"`
	DeliveryTotalUSD *decimal.Decimal `gorm:"column:delivery_total_usd;type:numeric(30,12)" json:"deliveryTotalUsd,omitempty"`
	PackageWeightKG  decimal.Decimal  `gorm:"column:package_weight_kg;type:numeric(30,12);not null" json:"packageWeightKg"`
	CustomsFeeKZT    *decimal.Decimal `gorm:"column:customs_fee_kzt;type:numeric(30,12)" json:"customsFeeKzt,omitempty"`
	FeeIncluded      bool             `gorm:"column:fee_included;not null" json:"feeIncluded"`
	CashUSD          *decimal.Decimal `gorm:"column:cash_usd;type:numeric(30,12)" json:"cashUsd,omitempty"`
	Items            JSON             `gorm:"type:jsonb;not null" json:"items"`
	Totals           JSON             `gorm:"type:jsonb;not null" json:"totals"`
	FormulaVersion   string           `gorm:"column:formula_version;size:64;not null" json:"formulaVersion"`
	RoundingPolicy   string           `gorm:"column:rounding_policy;size:64;not null" json:"roundingPolicy"`
	InputSnapshot    JSON             `gorm:"column:input_snapshot;type:jsonb;not null" json:"inputSnapshot"`
	CalculatedAt     *time.Time       `gorm:"column:calculated_at" json:"calculatedAt,omitempty"`
	CreatedBy        int64            `gorm:"not null" json:"createdBy"`
	UpdatedBy        int64            `gorm:"not null" json:"updatedBy"`
	CreatedAt        time.Time        `gorm:"not null" json:"createdAt"`
	UpdatedAt        time.Time        `gorm:"not null" json:"updatedAt"`
}

func (CustomsCalculation) TableName() string { return "customs_calculations" }

// CustomsCalculationRevision preserves every explicitly saved calculation
// snapshot. Reading a calculation never rebuilds it from current rates or
// formulas; a new snapshot is created only by an explicit save/recalculate.
type CustomsCalculationRevision struct {
	ID             int64     `gorm:"primaryKey"`
	CalculationID  int64     `gorm:"column:calculation_id;not null"`
	RevisionNo     int64     `gorm:"column:revision_no;not null"`
	FormulaVersion string    `gorm:"column:formula_version;size:64;not null"`
	RoundingPolicy string    `gorm:"column:rounding_policy;size:64;not null"`
	InputSnapshot  JSON      `gorm:"column:input_snapshot;type:jsonb;not null"`
	ResultSnapshot JSON      `gorm:"column:result_snapshot;type:jsonb;not null"`
	CreatedBy      int64     `gorm:"column:created_by;not null"`
	CreatedAt      time.Time `gorm:"column:created_at;not null"`
}

func (CustomsCalculationRevision) TableName() string { return "customs_calculation_revisions" }

// GlovesCalculationDraft is a per-employee working copy for the standalone
// gloves calculator.  It is deliberately separate from estimate calculations:
// a gloves draft is not a declaration and can be reused for the next shipment.
type GlovesCalculationDraft struct {
	ID             int64     `gorm:"primaryKey" json:"id"`
	UserID         int64     `gorm:"column:user_id;not null;unique" json:"userId"`
	InputSnapshot  JSON      `gorm:"column:input_snapshot;type:jsonb;not null" json:"inputSnapshot"`
	ResultSnapshot JSON      `gorm:"column:result_snapshot;type:jsonb;not null" json:"resultSnapshot"`
	CreatedAt      time.Time `gorm:"column:created_at;not null" json:"createdAt"`
	UpdatedAt      time.Time `gorm:"column:updated_at;not null" json:"updatedAt"`
}

func (GlovesCalculationDraft) TableName() string { return "gloves_calculation_drafts" }

// GlovesEstimate is an independent estimate list for the standalone gloves
// calculator. It deliberately does not use the regular estimates table.
type GlovesEstimate struct {
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

func (GlovesEstimate) TableName() string { return "gloves_estimates" }

// GoodsEstimate is the independent storage for the copied gloves
// calculator. It intentionally has its own table so the original gloves
// estimates cannot be changed from the copied section.
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

func (GoodsEstimate) TableName() string { return "gloves_copy_estimates" }
