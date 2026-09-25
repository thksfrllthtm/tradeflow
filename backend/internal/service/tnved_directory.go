package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/shopspring/decimal"
)

var ErrTNVEDDirectoryConflict = errors.New("tnved directory entry already exists")
var tnvedDirectoryCodePattern = regexp.MustCompile(`^[0-9]{10}$`)
var directoryCurrencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

type RateInput struct {
	Type      string `json:"type"`
	Value     string `json:"value"` // backwards-compatible alias for percent
	Percent   string `json:"percent"`
	Amount    string `json:"amount"`
	Currency  string `json:"currency"`
	Unit      string `json:"unit"`
	Basis     string `json:"basis"`
	ValidFrom string `json:"validFrom"`
	ValidTo   string `json:"validTo"`
	CheckedAt string `json:"checkedAt"`
	Source    string `json:"source"`
}

type TNVEDDirectoryInput struct {
	Category           string    `json:"category"`
	Name               string    `json:"name"`
	NameZH             string    `json:"nameZh"`
	Code               string    `json:"code"`
	CompanyCoefficient string    `json:"companyCoefficient"`
	AntiDumpingPercent string    `json:"antiDumpingPercent"`
	SynonymsRU         []string  `json:"synonymsRu"`
	SynonymsZH         []string  `json:"synonymsZh"`
	SynonymsEN         []string  `json:"synonymsEn"`
	VerificationStatus string    `json:"verificationStatus"`
	Duty               RateInput `json:"duty"`
	VAT                RateInput `json:"vat"`
}

type TNVEDDirectoryFilter struct {
	Query           string
	Category        string
	Status          string
	IncludeArchived bool
}

type TNVEDDirectoryService struct{ repository *repository.Repository }

func NewTNVEDDirectoryService(r *repository.Repository) *TNVEDDirectoryService {
	return &TNVEDDirectoryService{repository: r}
}

func normalizeTNVEDSynonyms(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, value)
	}
	return result
}

func parseDirectoryDate(value, field string) (*time.Time, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, nil
	}
	d, err := time.Parse("2006-01-02", value)
	if err != nil {
		return nil, fmt.Errorf("%s: используйте дату ГГГГ-ММ-ДД", field)
	}
	return &d, nil
}

func parseDirectoryRate(rate RateInput, field string, allowed ...string) (percent, amount *decimal.Decimal, from, to, checkedAt *time.Time, err error) {
	rate.Type = strings.TrimSpace(rate.Type)
	valid := false
	for _, option := range allowed {
		if rate.Type == option {
			valid = true
		}
	}
	if !valid {
		return nil, nil, nil, nil, nil, fmt.Errorf("%s.type имеет недопустимое значение", field)
	}
	from, err = parseDirectoryDate(rate.ValidFrom, field+".validFrom")
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	to, err = parseDirectoryDate(rate.ValidTo, field+".validTo")
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	if from != nil && to != nil && to.Before(*from) {
		return nil, nil, nil, nil, nil, fmt.Errorf("%s: дата окончания раньше даты начала", field)
	}
	checkedAt, err = parseDirectoryDate(rate.CheckedAt, field+".checkedAt")
	if err != nil {
		return nil, nil, nil, nil, nil, err
	}
	if rate.Type == "unknown" || rate.Type == "other" || rate.Type == "exempt" {
		if strings.TrimSpace(rate.Percent) != "" || strings.TrimSpace(rate.Amount) != "" || strings.TrimSpace(rate.Value) != "" || strings.TrimSpace(rate.Currency) != "" || strings.TrimSpace(rate.Unit) != "" {
			return nil, nil, nil, nil, nil, fmt.Errorf("%s.value оставьте пустым для неизвестного или неподдерживаемого типа", field)
		}
		return nil, nil, from, to, checkedAt, nil
	}
	percentText := strings.TrimSpace(rate.Percent)
	if rate.Type == "percentage" && percentText != "" && strings.TrimSpace(rate.Value) != "" {
		return nil, nil, nil, nil, nil, fmt.Errorf("%s: используйте только одно поле percent или value", field)
	}
	if percentText == "" && rate.Type == "percentage" {
		percentText = strings.TrimSpace(rate.Value)
	}
	parse := func(text, valueField string) (*decimal.Decimal, error) {
		text = strings.TrimSpace(strings.ReplaceAll(text, ",", "."))
		if text == "" {
			return nil, fmt.Errorf("%s обязательно", valueField)
		}
		parts := strings.Split(text, ".")
		if len(parts) > 2 || (len(parts) == 2 && len(parts[1]) > 12) {
			return nil, fmt.Errorf("%s: не более 12 знаков после запятой", valueField)
		}
		d, parseErr := decimal.NewFromString(text)
		if parseErr != nil || d.IsNegative() {
			return nil, fmt.Errorf("%s должно быть неотрицательным числом", valueField)
		}
		return &d, nil
	}
	if rate.Type == "percentage" {
		value, parseErr := parse(percentText, field+".percent")
		if parseErr != nil || value.GreaterThan(decimal.NewFromInt(100)) {
			if parseErr != nil {
				return nil, nil, nil, nil, nil, parseErr
			}
			return nil, nil, nil, nil, nil, fmt.Errorf("%s.percent должно быть от 0 до 100 процентов", field)
		}
		if strings.TrimSpace(rate.Amount) != "" || rate.Currency != "" || rate.Unit != "" {
			return nil, nil, nil, nil, nil, fmt.Errorf("%s.amount/currency/unit доступны только для ставки за единицу", field)
		}
		return value, nil, from, to, checkedAt, nil
	}
	if rate.Type != "specific" && rate.Type != "combined_minimum" {
		return nil, nil, nil, nil, nil, fmt.Errorf("%s.type имеет недопустимое значение", field)
	}
	if rate.Type == "specific" && strings.TrimSpace(rate.Percent) != "" {
		return nil, nil, nil, nil, nil, fmt.Errorf("%s.percent доступно только для комбинированной ставки", field)
	}
	if rate.Type == "specific" && strings.TrimSpace(rate.Value) != "" {
		return nil, nil, nil, nil, nil, fmt.Errorf("%s.value доступно только для процентной ставки", field)
	}
	amount, parseErr := parse(rate.Amount, field+".amount")
	if parseErr != nil {
		return nil, nil, nil, nil, nil, parseErr
	}
	currency := strings.ToUpper(strings.TrimSpace(rate.Currency))
	if !directoryCurrencyPattern.MatchString(currency) || strings.TrimSpace(rate.Unit) == "" {
		return nil, nil, nil, nil, nil, fmt.Errorf("%s: укажите валюту (3 буквы) и единицу расчёта", field)
	}
	if rate.Type == "combined_minimum" {
		percentText = strings.TrimSpace(rate.Percent)
		if percentText == "" {
			percentText = strings.TrimSpace(rate.Value)
		}
		percent, parseErr = parse(percentText, field+".percent")
		if parseErr != nil || percent.GreaterThan(decimal.NewFromInt(100)) {
			if parseErr != nil {
				return nil, nil, nil, nil, nil, parseErr
			}
			return nil, nil, nil, nil, nil, fmt.Errorf("%s.percent должно быть от 0 до 100 процентов", field)
		}
	}
	return percent, amount, from, to, checkedAt, nil
}

func validateDirectoryInput(input TNVEDDirectoryInput) (repository.TNVEDDirectoryEntry, error) {
	var err error
	if strings.TrimSpace(input.Duty.Type) == "" {
		input.Duty.Type = "unknown"
	}
	if strings.TrimSpace(input.VAT.Type) == "" {
		input.VAT.Type = "unknown"
	}
	entry := repository.TNVEDDirectoryEntry{
		Category: strings.TrimSpace(input.Category), Name: strings.TrimSpace(input.Name), NameZH: strings.TrimSpace(input.NameZH), Code: strings.TrimSpace(input.Code),
		VerificationStatus: strings.TrimSpace(input.VerificationStatus), DutyType: strings.TrimSpace(input.Duty.Type), VATType: strings.TrimSpace(input.VAT.Type),
	}
	if entry.Category == "" {
		entry.Category = "Товары"
	}
	if len([]rune(entry.Category)) > 64 {
		return entry, &ValidationError{Field: "category", Message: "Категория слишком длинная"}
	}
	if !tnvedDirectoryCodePattern.MatchString(entry.Code) {
		return entry, &ValidationError{Field: "code", Message: "Код ТН ВЭД должен содержать ровно 10 цифр"}
	}
	// A TN ВЭД card is a code/rates card now. The optional descriptions are
	// still useful for search, but a blank description must not block saving.
	if entry.Name == "" {
		entry.Name = entry.Code
	}
	if len([]rune(entry.Name)) > 300 {
		return entry, &ValidationError{Field: "name", Message: "Описание слишком длинное"}
	}
	if entry.VerificationStatus == "" {
		entry.VerificationStatus = "needs_review"
	}
	if entry.VerificationStatus != "needs_review" && entry.VerificationStatus != "operator_verified" {
		return entry, &ValidationError{Field: "verificationStatus", Message: "Укажите статус проверки"}
	}
	if len([]rune(entry.NameZH)) > 300 {
		return entry, &ValidationError{Field: "nameZh", Message: "Китайское название слишком длинное"}
	}
	entry.CompanyCoefficient, err = parseOptionalNonNegative(input.CompanyCoefficient, "companyCoefficient")
	if err != nil {
		return entry, err
	}
	entry.AntiDumpingPercent, err = parseOptionalNonNegative(input.AntiDumpingPercent, "antiDumpingPercent")
	if err != nil {
		return entry, err
	}
	if entry.AntiDumpingPercent != nil && entry.AntiDumpingPercent.GreaterThan(decimal.NewFromInt(100)) {
		return entry, &ValidationError{Field: "antiDumpingPercent", Message: "Антидемпинговая ставка должна быть от 0 до 100 процентов"}
	}
	entry.SynonymsRU = repository.JSON(mustJSON(normalizeTNVEDSynonyms(input.SynonymsRU)))
	entry.SynonymsZH = repository.JSON(mustJSON(normalizeTNVEDSynonyms(input.SynonymsZH)))
	entry.SynonymsEN = repository.JSON(mustJSON(normalizeTNVEDSynonyms(input.SynonymsEN)))
	dutyPercent, dutyAmount, from, to, checkedAt, err := parseDirectoryRate(input.Duty, "duty", "unknown", "percentage", "specific", "combined_minimum")
	if err != nil {
		return entry, &ValidationError{Field: "duty", Message: err.Error()}
	}
	entry.DutyPercent, entry.DutyAmount, entry.DutyCurrency, entry.DutyUnit = dutyPercent, dutyAmount, strings.ToUpper(strings.TrimSpace(input.Duty.Currency)), strings.TrimSpace(input.Duty.Unit)
	entry.DutyValidFrom, entry.DutyValidTo, entry.DutyCheckedAt, entry.DutySource = from, to, checkedAt, strings.TrimSpace(input.Duty.Source)
	vatPercent, _, from, to, checkedAt, err := parseDirectoryRate(input.VAT, "vat", "unknown", "percentage", "exempt")
	if err != nil {
		return entry, &ValidationError{Field: "vat", Message: err.Error()}
	}
	entry.VATPercent, entry.VATBasis = vatPercent, strings.TrimSpace(input.VAT.Basis)
	if entry.VATType == "exempt" && entry.VATBasis == "" {
		return entry, &ValidationError{Field: "vat.basis", Message: "Для освобождения укажите основание"}
	}
	if entry.VATType != "exempt" && entry.VATBasis != "" {
		return entry, &ValidationError{Field: "vat.basis", Message: "Основание доступно только для освобождения"}
	}
	entry.VATValidFrom, entry.VATValidTo, entry.VATCheckedAt, entry.VATSource = from, to, checkedAt, strings.TrimSpace(input.VAT.Source)
	return entry, nil
}

func parseOptionalNonNegative(value, field string) (*decimal.Decimal, error) {
	value = strings.TrimSpace(strings.ReplaceAll(value, ",", "."))
	if value == "" {
		return nil, nil
	}
	parts := strings.Split(value, ".")
	if len(parts) > 2 || (len(parts) == 2 && len(parts[1]) > 12) {
		return nil, &ValidationError{Field: field, Message: "Укажите не более 12 знаков после запятой"}
	}
	d, err := decimal.NewFromString(value)
	if err != nil || d.IsNegative() {
		return nil, &ValidationError{Field: field, Message: "Значение должно быть неотрицательным числом"}
	}
	return &d, nil
}

func mustJSON(value []string) []byte {
	data, err := json.Marshal(value)
	if err != nil {
		return []byte("[]")
	}
	return data
}

// decodeJSONStrings is deliberately tolerant of legacy null values while the
// schema itself always writes arrays.
func decodeJSONStrings(raw repository.JSON) []string {
	var values []string
	if len(raw) == 0 || string(raw) == "null" {
		return values
	}
	// The operator service stores arrays; use a small JSON decoder without
	// exposing database internals in the HTTP layer.
	_ = json.Unmarshal(raw, &values)
	return values
}

func (s *TNVEDDirectoryService) List(ctx context.Context, filter TNVEDDirectoryFilter) ([]repository.TNVEDDirectoryEntry, error) {
	entries, err := s.repository.ListTNVEDDirectory(ctx, filter.Category, filter.Status, filter.IncludeArchived)
	if err != nil {
		return nil, err
	}
	q := strings.ToLower(strings.TrimSpace(filter.Query))
	if q == "" {
		return entries, nil
	}
	result := entries[:0]
	for _, entry := range entries {
		parts := []string{entry.Code, entry.Name, entry.NameZH, entry.Category}
		parts = append(parts, decodeJSONStrings(entry.SynonymsRU)...)
		parts = append(parts, decodeJSONStrings(entry.SynonymsZH)...)
		parts = append(parts, decodeJSONStrings(entry.SynonymsEN)...)
		matched := false
		for _, part := range parts {
			if strings.Contains(strings.ToLower(part), q) {
				matched = true
				break
			}
		}
		if matched {
			result = append(result, entry)
		}
	}
	return result, nil
}

func (s *TNVEDDirectoryService) Create(ctx context.Context, userID int64, input TNVEDDirectoryInput) (repository.TNVEDDirectoryEntry, error) {
	entry, err := validateDirectoryInput(input)
	if err != nil {
		return repository.TNVEDDirectoryEntry{}, err
	}
	entry.CreatedBy, entry.UpdatedBy = userID, userID
	entry.CreatedAt, entry.UpdatedAt = time.Now().UTC(), time.Now().UTC()
	saved, err := s.repository.CreateTNVEDDirectoryEntry(ctx, entry)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "tnved_directory_active_name_idx") {
		return repository.TNVEDDirectoryEntry{}, ErrTNVEDDirectoryConflict
	}
	return saved, err
}

func (s *TNVEDDirectoryService) Update(ctx context.Context, id, userID int64, input TNVEDDirectoryInput) (repository.TNVEDDirectoryEntry, error) {
	entry, err := validateDirectoryInput(input)
	if err != nil {
		return repository.TNVEDDirectoryEntry{}, err
	}
	entry.ID, entry.UpdatedBy, entry.UpdatedAt = id, userID, time.Now().UTC()
	old, err := s.repository.TNVEDDirectoryEntryByID(ctx, id)
	if err != nil {
		return repository.TNVEDDirectoryEntry{}, err
	}
	entry.CreatedBy, entry.CreatedAt = old.CreatedBy, old.CreatedAt
	entry.ArchivedAt, entry.ArchivedBy = old.ArchivedAt, old.ArchivedBy
	saved, err := s.repository.UpdateTNVEDDirectoryEntry(ctx, entry)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "tnved_directory_active_name_idx") {
		return repository.TNVEDDirectoryEntry{}, ErrTNVEDDirectoryConflict
	}
	return saved, err
}

func (s *TNVEDDirectoryService) Archive(ctx context.Context, id, userID int64) error {
	return s.repository.ArchiveTNVEDDirectoryEntry(ctx, id, userID)
}

func (s *TNVEDDirectoryService) Restore(ctx context.Context, id, userID int64) error {
	if err := s.repository.RestoreTNVEDDirectoryEntry(ctx, id, userID); err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "tnved_directory_active_name_idx") {
			return ErrTNVEDDirectoryConflict
		}
		return err
	}
	return nil
}

// Copy creates a new operator entry from an existing one. A suffix makes the
// operation safe to repeat while keeping the original immutable for saved
// calculations. The caller may provide a replacement name when needed.
func (s *TNVEDDirectoryService) Copy(ctx context.Context, id, userID int64, name string) (repository.TNVEDDirectoryEntry, error) {
	source, err := s.repository.TNVEDDirectoryEntryByID(ctx, id)
	if err != nil {
		return repository.TNVEDDirectoryEntry{}, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = source.Name + " (копия)"
	}
	input := TNVEDDirectoryInput{
		Category: source.Category, Name: name, NameZH: source.NameZH, Code: source.Code,
		CompanyCoefficient: decimalString(source.CompanyCoefficient),
		AntiDumpingPercent: decimalString(source.AntiDumpingPercent),
		SynonymsRU:         decodeJSONStrings(source.SynonymsRU), SynonymsZH: decodeJSONStrings(source.SynonymsZH), SynonymsEN: decodeJSONStrings(source.SynonymsEN),
		VerificationStatus: source.VerificationStatus,
		Duty:               RateInput{Type: source.DutyType, Percent: decimalString(source.DutyPercent), Amount: decimalString(source.DutyAmount), Currency: source.DutyCurrency, Unit: source.DutyUnit, ValidFrom: dateString(source.DutyValidFrom), ValidTo: dateString(source.DutyValidTo), CheckedAt: dateString(source.DutyCheckedAt), Source: source.DutySource},
		VAT:                RateInput{Type: source.VATType, Percent: decimalString(source.VATPercent), Basis: source.VATBasis, ValidFrom: dateString(source.VATValidFrom), ValidTo: dateString(source.VATValidTo), CheckedAt: dateString(source.VATCheckedAt), Source: source.VATSource},
	}
	entry, err := validateDirectoryInput(input)
	if err != nil {
		return repository.TNVEDDirectoryEntry{}, err
	}
	now := time.Now().UTC()
	entry.CreatedBy, entry.UpdatedBy, entry.CreatedAt, entry.UpdatedAt = userID, userID, now, now
	saved, err := s.repository.CreateTNVEDDirectoryEntry(ctx, entry)
	if err != nil && strings.Contains(strings.ToLower(err.Error()), "tnved_directory_active_name_idx") {
		return repository.TNVEDDirectoryEntry{}, ErrTNVEDDirectoryConflict
	}
	return saved, err
}

// DirectorySnapshot returns the immutable set of operator-entered values that
// is embedded into a calculation.  Keeping this construction in one place
// prevents the standalone gloves calculator from trusting a client-provided
// snapshot or accidentally omitting a rate field.
func DirectorySnapshot(entry repository.TNVEDDirectoryEntry) map[string]any {
	return map[string]any{
		"entryId": entry.ID, "category": entry.Category, "name": entry.Name, "nameZh": entry.NameZH, "code": entry.Code,
		"companyCoefficient": decimalString(entry.CompanyCoefficient), "antiDumpingPercent": decimalString(entry.AntiDumpingPercent),
		"duty":               map[string]any{"type": entry.DutyType, "percent": decimalString(entry.DutyPercent), "amount": decimalString(entry.DutyAmount), "currency": entry.DutyCurrency, "unit": entry.DutyUnit, "validFrom": dateString(entry.DutyValidFrom), "validTo": dateString(entry.DutyValidTo), "checkedAt": dateString(entry.DutyCheckedAt), "source": entry.DutySource, "sourceKind": "operator"},
		"vat":                map[string]any{"type": entry.VATType, "percent": decimalString(entry.VATPercent), "basis": entry.VATBasis, "validFrom": dateString(entry.VATValidFrom), "validTo": dateString(entry.VATValidTo), "checkedAt": dateString(entry.VATCheckedAt), "source": entry.VATSource, "sourceKind": "operator"},
		"verificationStatus": entry.VerificationStatus, "selectedAt": time.Now().UTC().Format(time.RFC3339),
	}
}

func decimalString(value *decimal.Decimal) string {
	if value == nil {
		return ""
	}
	return value.String()
}

func dateString(value *time.Time) string {
	if value == nil {
		return ""
	}
	return value.Format("2006-01-02")
}
