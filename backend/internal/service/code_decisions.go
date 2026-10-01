package service

// База решений: проверенные примеры «товар из инвойса → код ТН ВЭД». Запись создаёт любой
// сотрудник (из подбора агентом или вручную) со статусом «Не проверено»; проверяет, исправляет,
// отклоняет и импортирует старые инвойсы только администратор. Исправление кода агента
// отмечается — по нему считается точность агента.

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/shopspring/decimal"
)

const (
	maxDecisionPhotos     = 3
	maxDecisionPhotoBytes = 2 << 20
	maxDecisionThumbBytes = 120 << 10
	maxDecisionImportRows = 200
	maxDecisionTextRunes  = 2000
	maxDecisionNameRunes  = 500
)

var tnvedCodePattern = regexp.MustCompile(`^\d{10}$`)

type CodeDecisionService struct{ repository *repository.Repository }

func NewCodeDecisionService(r *repository.Repository) *CodeDecisionService {
	return &CodeDecisionService{repository: r}
}

type RejectedCode struct {
	Code string `json:"code"`
	Why  string `json:"why"`
}

type CodeDecisionView struct {
	ID                 int64             `json:"id"`
	Status             string            `json:"status"`
	Source             string            `json:"source"`
	InvoiceName        string            `json:"invoiceName"`
	SupplierName       string            `json:"supplierName"`
	FinalName          string            `json:"finalName"`
	Code               string            `json:"code"`
	Reason             string            `json:"reason"`
	Quote              string            `json:"quote"`
	SourceURL          string            `json:"sourceUrl"`
	RejectedCodes      []RejectedCode    `json:"rejectedCodes"`
	UnitNetKg          string            `json:"unitNetKg"`
	UnitPriceUSD       string            `json:"unitPriceUsd"`
	Photos             []string          `json:"photos"`
	Thumb              string            `json:"thumb"`
	AgentCode          string            `json:"agentCode"`
	AgentCorrect       *bool             `json:"agentCorrect"`
	EstimateID         *int64            `json:"estimateId,omitempty"`
	ItemID             string            `json:"itemId,omitempty"`
	SessionID          *int64            `json:"sessionId,omitempty"`
	ProductDirectoryID *int64            `json:"productDirectoryId,omitempty"`
	Note               string            `json:"note"`
	CreatedByName      string            `json:"createdByName"`
	VerifiedByName     string            `json:"verifiedByName"`
	VerifiedAt         *time.Time        `json:"verifiedAt"`
	CreatedAt          time.Time         `json:"createdAt"`
	UpdatedAt          time.Time         `json:"updatedAt"`
	Nomenclature       *NomenclatureCode `json:"nomenclature"`
}

type CodeDecisionInput struct {
	InvoiceName        string         `json:"invoiceName"`
	SupplierName       string         `json:"supplierName"`
	FinalName          string         `json:"finalName"`
	Code               string         `json:"code"`
	Reason             string         `json:"reason"`
	Quote              string         `json:"quote"`
	SourceURL          string         `json:"sourceUrl"`
	RejectedCodes      []RejectedCode `json:"rejectedCodes"`
	UnitNetKg          string         `json:"unitNetKg"`
	UnitPriceUSD       string         `json:"unitPriceUsd"`
	Photos             []string       `json:"photos"`
	Thumb              string         `json:"thumb"`
	AgentCode          string         `json:"agentCode"`
	EstimateID         *int64         `json:"estimateId"`
	ItemID             string         `json:"itemId"`
	SessionID          *int64         `json:"sessionId"`
	ProductDirectoryID *int64         `json:"productDirectoryId"`
	Note               string         `json:"note"`
}

// CodeDecisionEdit — правка администратора; nil-поля не меняются.
type CodeDecisionEdit struct {
	Code               *string `json:"code"`
	FinalName          *string `json:"finalName"`
	InvoiceName        *string `json:"invoiceName"`
	SupplierName       *string `json:"supplierName"`
	Reason             *string `json:"reason"`
	Note               *string `json:"note"`
	ProductDirectoryID *int64  `json:"productDirectoryId"`
}

type CodeDecisionStats struct {
	Pending  int64 `json:"pending"`
	Verified int64 `json:"verified"`
	Rejected int64 `json:"rejected"`
	// Точность агента по решениям, проверенным за 90 дней.
	AgentChecked int64 `json:"agentChecked"`
	AgentCorrect int64 `json:"agentCorrect"`
}

func (s *CodeDecisionService) Stats(ctx context.Context) (CodeDecisionStats, error) {
	stats, err := s.repository.CodeDecisionStats(ctx)
	return CodeDecisionStats(stats), err
}

func (s *CodeDecisionService) List(ctx context.Context, filter repository.CodeDecisionFilter) ([]CodeDecisionView, int64, error) {
	if filter.Limit <= 0 || filter.Limit > 200 {
		filter.Limit = 50
	}
	if filter.Offset < 0 {
		filter.Offset = 0
	}
	switch filter.Status {
	case "", "pending", "verified", "rejected":
	default:
		filter.Status = ""
	}
	rows, total, err := s.repository.ListCodeDecisions(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	views, err := s.views(ctx, rows)
	return views, total, err
}

func (s *CodeDecisionService) Get(ctx context.Context, id int64) (CodeDecisionView, error) {
	row, err := s.repository.CodeDecisionByID(ctx, id)
	if err != nil {
		return CodeDecisionView{}, err
	}
	views, err := s.views(ctx, []repository.CodeDecision{row})
	if err != nil {
		return CodeDecisionView{}, err
	}
	return views[0], nil
}

// views — записи для ответа вместе с текущим состоянием кода в номенклатуре.
func (s *CodeDecisionService) views(ctx context.Context, rows []repository.CodeDecision) ([]CodeDecisionView, error) {
	codes := make([]string, 0, len(rows))
	for _, row := range rows {
		codes = append(codes, row.Code)
	}
	found, err := NewTNVEDNomenclatureService(s.repository).Lookup(ctx, codes)
	if err != nil {
		return nil, err
	}
	byCode := make(map[string]*NomenclatureCode, len(found))
	for index := range found {
		byCode[found[index].Code] = &found[index]
	}
	views := make([]CodeDecisionView, 0, len(rows))
	for _, row := range rows {
		view := CodeDecisionView{
			ID: row.ID, Status: row.Status, Source: row.Source, InvoiceName: row.InvoiceName, SupplierName: row.SupplierName,
			FinalName: row.FinalName, Code: row.Code, Reason: row.Reason, Quote: row.Quote, SourceURL: row.SourceURL,
			UnitNetKg: decimalText(row.UnitNetKg), UnitPriceUSD: decimalText(row.UnitPriceUSD), Thumb: row.Thumb,
			AgentCode: row.AgentCode, AgentCorrect: row.AgentCorrect, EstimateID: row.EstimateID, ItemID: row.ItemID,
			SessionID: row.SessionID, ProductDirectoryID: row.ProductDirectoryID, Note: row.Note,
			CreatedByName: row.CreatedByName, VerifiedByName: row.VerifiedByName, VerifiedAt: row.VerifiedAt,
			CreatedAt: row.CreatedAt, UpdatedAt: row.UpdatedAt, Nomenclature: byCode[row.Code],
			RejectedCodes: []RejectedCode{}, Photos: []string{},
		}
		_ = json.Unmarshal(row.RejectedCodes, &view.RejectedCodes)
		_ = json.Unmarshal(row.Photos, &view.Photos)
		if view.RejectedCodes == nil {
			view.RejectedCodes = []RejectedCode{}
		}
		if view.Photos == nil {
			view.Photos = []string{}
		}
		views = append(views, view)
	}
	return views, nil
}

func cleanText(value string, limit int) string {
	return truncateRunes(strings.TrimSpace(value), limit)
}

// normalizeCode: «6116 10 200 0», «6116.10.200.0» → «6116102000».
func normalizeCode(value string) string {
	return strings.NewReplacer(" ", "", ".", "", " ", "").Replace(strings.TrimSpace(value))
}

func optionalDecimalText(value string, field string) (*decimal.Decimal, error) {
	text := strings.ReplaceAll(strings.TrimSpace(value), ",", ".")
	if text == "" {
		return nil, nil
	}
	parsed, err := decimal.NewFromString(text)
	if err != nil || parsed.IsNegative() {
		return nil, &ValidationError{Field: field, Message: "Число должно быть неотрицательным"}
	}
	return &parsed, nil
}

func validPhoto(value string, limit int) bool {
	if len(value) > limit*4/3+64 {
		return false
	}
	_, mime, ok := decodeDataURL(value)
	return ok && (mime == "image/jpeg" || mime == "image/png" || mime == "image/webp" || mime == "image/gif")
}

// decisionRow проверяет ввод и собирает запись базы; хеши фото считаются здесь.
func decisionRow(input CodeDecisionInput, source string) (repository.CodeDecision, error) {
	code := normalizeCode(input.Code)
	if !tnvedCodePattern.MatchString(code) {
		return repository.CodeDecision{}, &ValidationError{Field: "code", Message: "Код ТН ВЭД — 10 цифр"}
	}
	agentCode := normalizeCode(input.AgentCode)
	if agentCode != "" && !tnvedCodePattern.MatchString(agentCode) {
		agentCode = ""
	}
	invoiceName := cleanText(input.InvoiceName, maxDecisionNameRunes)
	finalName := cleanText(input.FinalName, maxDecisionNameRunes)
	supplierName := cleanText(input.SupplierName, maxDecisionNameRunes)
	if invoiceName == "" && finalName == "" && supplierName == "" {
		return repository.CodeDecision{}, &ValidationError{Field: "invoiceName", Message: "Укажите наименование товара"}
	}
	if len(input.Photos) > maxDecisionPhotos {
		input.Photos = input.Photos[:maxDecisionPhotos]
	}
	photos := make([]string, 0, len(input.Photos))
	hashes := make([]string, 0, len(input.Photos))
	for _, photo := range input.Photos {
		if !validPhoto(photo, maxDecisionPhotoBytes) {
			return repository.CodeDecision{}, &ValidationError{Field: "photos", Message: "Фото — JPEG или PNG до 2 МБ"}
		}
		photos = append(photos, photo)
		if hash := PhotoHash(photo); hash != "" {
			hashes = append(hashes, hash)
		}
	}
	thumb := input.Thumb
	if thumb != "" && !validPhoto(thumb, maxDecisionThumbBytes) {
		thumb = ""
	}
	rejected := make([]RejectedCode, 0, len(input.RejectedCodes))
	for _, item := range input.RejectedCodes {
		if other := normalizeCode(item.Code); tnvedCodePattern.MatchString(other) && other != code && len(rejected) < 5 {
			rejected = append(rejected, RejectedCode{Code: other, Why: cleanText(item.Why, 500)})
		}
	}
	netKg, err := optionalDecimalText(input.UnitNetKg, "unitNetKg")
	if err != nil {
		return repository.CodeDecision{}, err
	}
	price, err := optionalDecimalText(input.UnitPriceUSD, "unitPriceUsd")
	if err != nil {
		return repository.CodeDecision{}, err
	}
	photosJSON, _ := json.Marshal(photos)
	hashesJSON, _ := json.Marshal(hashes)
	rejectedJSON, _ := json.Marshal(rejected)
	now := time.Now().UTC()
	return repository.CodeDecision{
		Status: "pending", Source: source, InvoiceName: invoiceName, SupplierName: supplierName, FinalName: finalName,
		Code: code, Reason: cleanText(input.Reason, maxDecisionTextRunes), Quote: cleanText(input.Quote, maxDecisionTextRunes),
		SourceURL: cleanText(input.SourceURL, 500), RejectedCodes: repository.JSON(rejectedJSON), UnitNetKg: netKg, UnitPriceUSD: price,
		Photos: repository.JSON(photosJSON), Thumb: thumb, PhotoHashes: repository.JSON(hashesJSON), AgentCode: agentCode,
		EstimateID: input.EstimateID, ItemID: cleanText(input.ItemID, 64), SessionID: input.SessionID,
		ProductDirectoryID: input.ProductDirectoryID, Note: cleanText(input.Note, maxDecisionTextRunes), CreatedAt: now, UpdatedAt: now,
	}, nil
}

// Create — запись «Не проверено» от сотрудника (source: agent или manual).
func (s *CodeDecisionService) Create(ctx context.Context, userID int64, input CodeDecisionInput, source string) (CodeDecisionView, error) {
	if source != "agent" {
		source = "manual"
	}
	row, err := decisionRow(input, source)
	if err != nil {
		return CodeDecisionView{}, err
	}
	row.CreatedBy = &userID
	created, err := s.repository.CreateCodeDecisions(ctx, []repository.CodeDecision{row})
	if err != nil {
		return CodeDecisionView{}, err
	}
	return s.Get(ctx, created[0].ID)
}

type DecisionImportError struct {
	Row     int    `json:"row"`
	Message string `json:"message"`
}

// Import — строки старых инвойсов с правильными кодами: сразу «Проверено» администратором.
// Строки с ошибками пропускаются и возвращаются с номером.
func (s *CodeDecisionService) Import(ctx context.Context, adminID int64, rows []CodeDecisionInput) (int, int, []DecisionImportError, error) {
	if len(rows) == 0 {
		return 0, 0, nil, &ValidationError{Field: "rows", Message: "Нет строк для импорта"}
	}
	if len(rows) > maxDecisionImportRows {
		return 0, 0, nil, &ValidationError{Field: "rows", Message: fmt.Sprintf("Не больше %d строк за раз", maxDecisionImportRows)}
	}
	keys, err := s.repository.CodeDecisionKeys(ctx)
	if err != nil {
		return 0, 0, nil, err
	}
	// Тот же код с тем же наименованием уже есть (или повторяется в файле) — строка пропускается.
	known := map[string]bool{}
	for _, key := range keys {
		known[decisionImportKey(key.Code, key.FinalName, key.InvoiceName, key.SupplierName)] = true
	}
	skipped := 0
	now := time.Now().UTC()
	valid := make([]repository.CodeDecision, 0, len(rows))
	problems := []DecisionImportError{}
	for index, input := range rows {
		row, err := decisionRow(input, "import")
		if err != nil {
			message := err.Error()
			if validation, ok := err.(*ValidationError); ok {
				message = validation.Message
			}
			problems = append(problems, DecisionImportError{Row: index + 1, Message: message})
			continue
		}
		key := decisionImportKey(row.Code, row.FinalName, row.InvoiceName, row.SupplierName)
		if known[key] {
			skipped++
			continue
		}
		known[key] = true
		row.Status, row.CreatedBy, row.VerifiedBy, row.VerifiedAt = "verified", &adminID, &adminID, &now
		valid = append(valid, row)
	}
	created, err := s.repository.CreateCodeDecisions(ctx, valid)
	return len(created), skipped, problems, err
}

func decisionImportKey(code, finalName, invoiceName, supplierName string) string {
	return code + "|" + normalizeDecisionName(finalName) + "|" + normalizeDecisionName(invoiceName) + "|" + normalizeDecisionName(supplierName)
}

// Verify — «Проверено» с необязательной правкой. Если у записи есть код агента, отмечается,
// прав ли был агент.
func (s *CodeDecisionService) Verify(ctx context.Context, adminID, id int64, edit CodeDecisionEdit) (CodeDecisionView, error) {
	return s.edit(ctx, adminID, id, edit, "verified")
}

// Update — правка без смены статуса.
func (s *CodeDecisionService) Update(ctx context.Context, adminID, id int64, edit CodeDecisionEdit) (CodeDecisionView, error) {
	return s.edit(ctx, adminID, id, edit, "")
}

func (s *CodeDecisionService) Reject(ctx context.Context, adminID, id int64, note string) (CodeDecisionView, error) {
	edit := CodeDecisionEdit{}
	if strings.TrimSpace(note) != "" {
		edit.Note = &note
	}
	return s.edit(ctx, adminID, id, edit, "rejected")
}

func (s *CodeDecisionService) edit(ctx context.Context, adminID, id int64, edit CodeDecisionEdit, status string) (CodeDecisionView, error) {
	current, err := s.repository.CodeDecisionByID(ctx, id)
	if err != nil {
		return CodeDecisionView{}, err
	}
	changes := map[string]any{"updated_at": time.Now().UTC()}
	code := current.Code
	if edit.Code != nil {
		code = normalizeCode(*edit.Code)
		if !tnvedCodePattern.MatchString(code) {
			return CodeDecisionView{}, &ValidationError{Field: "code", Message: "Код ТН ВЭД — 10 цифр"}
		}
		changes["code"] = code
	}
	if edit.FinalName != nil {
		changes["final_name"] = cleanText(*edit.FinalName, maxDecisionNameRunes)
	}
	if edit.InvoiceName != nil {
		changes["invoice_name"] = cleanText(*edit.InvoiceName, maxDecisionNameRunes)
	}
	if edit.SupplierName != nil {
		changes["supplier_name"] = cleanText(*edit.SupplierName, maxDecisionNameRunes)
	}
	if edit.Reason != nil {
		changes["reason"] = cleanText(*edit.Reason, maxDecisionTextRunes)
	}
	if edit.Note != nil {
		changes["note"] = cleanText(*edit.Note, maxDecisionTextRunes)
	}
	if edit.ProductDirectoryID != nil {
		if *edit.ProductDirectoryID > 0 {
			changes["product_directory_id"] = *edit.ProductDirectoryID
		} else {
			changes["product_directory_id"] = nil
		}
	}
	if status != "" {
		changes["status"] = status
		if status == "verified" {
			now := time.Now().UTC()
			changes["verified_by"], changes["verified_at"] = adminID, now
		} else {
			changes["verified_by"], changes["verified_at"] = nil, nil
		}
	}
	finalStatus := current.Status
	if status != "" {
		finalStatus = status
	}
	// Оценка агента — только у проверенной записи, где агент что-то предлагал.
	if current.AgentCode != "" && finalStatus == "verified" {
		changes["agent_correct"] = current.AgentCode == code
	} else if finalStatus != "verified" {
		changes["agent_correct"] = nil
	}
	if err := s.repository.UpdateCodeDecision(ctx, id, changes); err != nil {
		return CodeDecisionView{}, err
	}
	return s.Get(ctx, id)
}

func (s *CodeDecisionService) Delete(ctx context.Context, id int64) error {
	return s.repository.DeleteCodeDecision(ctx, id)
}

// normalizeDecisionName: регистр, ё/е, знаки препинания и лишние пробелы не важны.
func normalizeDecisionName(value string) string {
	var builder strings.Builder
	space := false
	for _, r := range strings.ReplaceAll(strings.ToLower(value), "ё", "е") {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if space && builder.Len() > 0 {
				builder.WriteByte(' ')
			}
			builder.WriteRune(r)
			space = false
			continue
		}
		space = true
	}
	return builder.String()
}

// SameProductMatch — запись базы о том же товаре и по чему он узнан.
type SameProductMatch struct {
	Decision CodeDecisionView `json:"decision"`
	ByPhoto  bool             `json:"byPhoto"`
	ByName   bool             `json:"byName"`
}

// FindSame ищет в базе тот же товар: то же фото или то же наименование (из инвойса или поставщика).
// Сначала проверенные записи.
func (s *CodeDecisionService) FindSame(ctx context.Context, names []string, photoHashes []string) ([]SameProductMatch, error) {
	keys, err := s.repository.CodeDecisionKeys(ctx)
	if err != nil {
		return nil, err
	}
	wanted := map[string]bool{}
	for _, name := range names {
		for _, variant := range decisionNameVariants(name) {
			wanted[variant] = true
		}
	}
	type hit struct{ byPhoto, byName bool }
	hits := map[int64]hit{}
	ids := []int64{}
	for _, key := range keys {
		var hashes []string
		_ = json.Unmarshal(key.PhotoHashes, &hashes)
		byPhoto := len(photoHashes) > 0 && samePhoto(photoHashes, hashes)
		byName := false
		for _, name := range []string{key.InvoiceName, key.SupplierName, key.FinalName} {
			for _, variant := range decisionNameVariants(name) {
				byName = byName || wanted[variant]
			}
		}
		if byPhoto || byName {
			hits[key.ID] = hit{byPhoto, byName}
			ids = append(ids, key.ID)
		}
	}
	rows, err := s.repository.CodeDecisionsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	views, err := s.views(ctx, rows)
	if err != nil {
		return nil, err
	}
	matches := make([]SameProductMatch, 0, len(views))
	for _, view := range views {
		matches = append(matches, SameProductMatch{Decision: view, ByPhoto: hits[view.ID].byPhoto, ByName: hits[view.ID].byName})
	}
	// Проверенные — первыми, затем по фото, затем новые.
	sortMatches(matches)
	if len(matches) > 5 {
		matches = matches[:5]
	}
	return matches, nil
}

// decisionNameVariants — по чему наименование узнаётся: целиком и по частям «русское / китайское»
// (так записаны наименования компании). Слишком короткое («перчатки», «钳子») слишком общее.
func decisionNameVariants(name string) []string {
	parts := []string{name}
	if left, right, ok := strings.Cut(name, "/"); ok {
		parts = append(parts, left, right)
	}
	variants := []string{}
	for _, part := range parts {
		normalized := normalizeDecisionName(part)
		minimum := 8
		if strings.IndexFunc(normalized, func(r rune) bool { return unicode.Is(unicode.Han, r) }) >= 0 {
			minimum = 4
		}
		if len([]rune(normalized)) >= minimum {
			variants = append(variants, normalized)
		}
	}
	return variants
}

// AgreedVerifiedCode — код, на котором сходятся все проверенные записи о том же товаре; без ИИ
// обходимся, только если такой код один (иначе одно и то же наименование оформлялось по-разному).
func AgreedVerifiedCode(matches []SameProductMatch) (string, bool) {
	code := ""
	for _, match := range matches {
		if match.Decision.Status != "verified" {
			continue
		}
		if code != "" && code != match.Decision.Code {
			return "", false
		}
		code = match.Decision.Code
	}
	return code, code != ""
}

func sortMatches(matches []SameProductMatch) {
	rank := func(match SameProductMatch) int {
		value := 0
		if match.Decision.Status != "verified" {
			value += 2
		}
		if !match.ByPhoto {
			value++
		}
		return value
	}
	for i := 1; i < len(matches); i++ {
		for j := i; j > 0; j-- {
			left, right := matches[j-1], matches[j]
			if rank(left) < rank(right) || (rank(left) == rank(right) && !left.Decision.UpdatedAt.Before(right.Decision.UpdatedAt)) {
				break
			}
			matches[j-1], matches[j] = right, left
		}
	}
}
