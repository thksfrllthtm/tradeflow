package decisions

// Новые записи базы решений: проверка ввода, создание сотрудником и импорт администратором.

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/shopspring/decimal"
)

func cleanText(value string, limit int) string {
	return service.TruncateRunes(strings.TrimSpace(value), limit)
}

// normalizeCode: «6116 10 200 0», «6116.10.200.0» → «6116102000».
func NormalizeCode(value string) string {
	return strings.NewReplacer(" ", "", ".", "", " ", "").Replace(strings.TrimSpace(value))
}

// decimalLimit — потолок для цены и нетто. Нетто хранится в NUMERIC(14,6), туда влезает меньше 10^8;
// число больше — ошибка ввода, а не 500 от базы.
var decimalLimit = decimal.New(1, 8)

func optionalDecimalText(value string, field string) (*decimal.Decimal, error) {
	text := strings.ReplaceAll(strings.TrimSpace(value), ",", ".")
	if text == "" {
		return nil, nil
	}
	parsed, err := decimal.NewFromString(text)
	if err != nil || parsed.IsNegative() {
		return nil, &service.ValidationError{Field: field, Message: "Число должно быть неотрицательным"}
	}
	if parsed.GreaterThanOrEqual(decimalLimit) {
		return nil, &service.ValidationError{Field: field, Message: "Слишком большое число"}
	}
	return &parsed, nil
}

func ValidPhoto(value string, limit int) bool {
	if len(value) > limit*4/3+64 {
		return false
	}
	_, mime, ok := decodeDataURL(value)
	return ok && (mime == "image/jpeg" || mime == "image/png" || mime == "image/webp" || mime == "image/gif")
}

// decisionRow проверяет ввод и собирает запись базы; хеши фото считаются здесь.
func decisionRow(input CodeDecisionInput, source string) (repository.CodeDecision, error) {
	code := NormalizeCode(input.Code)
	if !tnvedCodePattern.MatchString(code) {
		return repository.CodeDecision{}, &service.ValidationError{Field: "code", Message: "Код ТН ВЭД — 10 цифр"}
	}
	agentCode := NormalizeCode(input.AgentCode)
	if agentCode != "" && !tnvedCodePattern.MatchString(agentCode) {
		agentCode = ""
	}
	invoiceName := cleanText(input.InvoiceName, maxDecisionNameRunes)
	finalName := cleanText(input.FinalName, maxDecisionNameRunes)
	supplierName := cleanText(input.SupplierName, maxDecisionNameRunes)
	if invoiceName == "" && finalName == "" && supplierName == "" {
		return repository.CodeDecision{}, &service.ValidationError{Field: "invoiceName", Message: "Укажите наименование товара"}
	}
	if len(input.Photos) > maxDecisionPhotos {
		input.Photos = input.Photos[:maxDecisionPhotos]
	}
	photos := make([]string, 0, len(input.Photos))
	hashes := make([]string, 0, len(input.Photos))
	for _, photo := range input.Photos {
		if !ValidPhoto(photo, maxDecisionPhotoBytes) {
			return repository.CodeDecision{}, &service.ValidationError{Field: "photos", Message: "Фото — JPEG или PNG до 2 МБ"}
		}
		photos = append(photos, photo)
		if hash := PhotoHash(photo); hash != "" {
			hashes = append(hashes, hash)
		}
	}
	thumb := input.Thumb
	if thumb != "" && !ValidPhoto(thumb, maxDecisionThumbBytes) {
		thumb = ""
	}
	if thumb == "" && len(photos) > 0 {
		thumb = MakeThumb(photos[0])
	}
	rejected := make([]RejectedCode, 0, len(input.RejectedCodes))
	for _, item := range input.RejectedCodes {
		if other := NormalizeCode(item.Code); tnvedCodePattern.MatchString(other) && other != code && len(rejected) < 5 {
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
	// Тот же товар этой сметы уже сохранён (из этой или другой строки) — вторая запись не нужна.
	// ponytail: проверка без блокировки — два одновременных нажатия в одной смете могут дать две записи.
	if row.EstimateID != nil {
		saved, err := s.repository.CodeDecisionsByEstimate(ctx, *row.EstimateID)
		if err != nil {
			return CodeDecisionView{}, err
		}
		var hashes []string
		_ = json.Unmarshal(row.PhotoHashes, &hashes)
		if match := findSame(saved, row.Code, decisionNameKey(row.FinalName, row.InvoiceName), hashes); match != nil {
			view, err := s.Get(ctx, match.ID)
			view.Duplicate = true
			return view, err
		}
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

// ImportOptions — режим импорта. Импорт идёт порциями: Replace ставится только на первой (удалить
// всю базу), KeepDuplicates — на каждой порции замены: файл загружается как есть, повторы (тот же
// код, наименование и похожее фото) не пропускаются — их уже свернул сборщик файла.
type ImportOptions struct {
	Replace        bool `json:"replace"`
	KeepDuplicates bool `json:"keepDuplicates"`
}

// Import — строки Excel базы решений с правильными кодами: сразу «Проверено» администратором.
// Строки с ошибками пропускаются и возвращаются с номером.
func (s *CodeDecisionService) Import(ctx context.Context, adminID int64, rows []CodeDecisionInput, options ImportOptions) (int, int, []DecisionImportError, error) {
	if len(rows) == 0 {
		return 0, 0, nil, &service.ValidationError{Field: "rows", Message: "Нет строк для импорта"}
	}
	if len(rows) > maxDecisionImportRows {
		return 0, 0, nil, &service.ValidationError{Field: "rows", Message: fmt.Sprintf("Не больше %d строк за раз", maxDecisionImportRows)}
	}
	if options.Replace {
		deleted, err := s.repository.DeleteAllCodeDecisions(ctx)
		if err != nil {
			return 0, 0, nil, err
		}
		log.Printf("decisions: base cleared before import by user %d, %d records deleted", adminID, deleted)
	}
	keys, err := s.repository.CodeDecisionKeys(ctx)
	if err != nil {
		return 0, 0, nil, err
	}
	// Тот же код с тем же наименованием и похожим фото уже есть (или повторяется в файле) — строка
	// пропускается. По ключу хранятся отпечатки фото каждой такой записи.
	known := map[string][][]string{}
	for _, key := range keys {
		var hashes []string
		_ = json.Unmarshal(key.PhotoHashes, &hashes)
		id := decisionImportKey(key.Code, key.FinalName, key.InvoiceName, key.SupplierName)
		known[id] = append(known[id], hashes)
	}
	skipped := 0
	now := time.Now().UTC()
	valid := make([]repository.CodeDecision, 0, len(rows))
	problems := []DecisionImportError{}
	for index, input := range rows {
		row, err := decisionRow(input, "import")
		if err != nil {
			message := err.Error()
			if validation, ok := err.(*service.ValidationError); ok {
				message = validation.Message
			}
			problems = append(problems, DecisionImportError{Row: index + 1, Message: message})
			continue
		}
		key := decisionImportKey(row.Code, row.FinalName, row.InvoiceName, row.SupplierName)
		var hashes []string
		_ = json.Unmarshal(row.PhotoHashes, &hashes)
		if !options.KeepDuplicates && importDuplicate(known[key], hashes) {
			skipped++
			continue
		}
		known[key] = append(known[key], hashes)
		row.Status, row.CreatedBy, row.VerifiedBy, row.VerifiedAt = "verified", &adminID, &adminID, &now
		valid = append(valid, row)
	}
	created, err := s.repository.CreateCodeDecisions(ctx, valid)
	return len(created), skipped, problems, err
}

func decisionImportKey(code, finalName, invoiceName, supplierName string) string {
	return code + "|" + NormalizeDecisionName(finalName) + "|" + NormalizeDecisionName(invoiceName) + "|" + NormalizeDecisionName(supplierName)
}

// importDuplicate — среди записей с тем же ключом есть тот же товар: как sameProduct, фото
// сравниваются, только если они есть у обеих сторон.
func importDuplicate(saved [][]string, hashes []string) bool {
	for _, other := range saved {
		if len(hashes) == 0 || len(other) == 0 || samePhoto(hashes, other) {
			return true
		}
	}
	return false
}
