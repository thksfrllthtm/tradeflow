package service

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/shopspring/decimal"
)

var catalogCodePattern = regexp.MustCompile(`^[0-9]{1,32}$`)

type CatalogImportRequest struct {
	Version   CatalogVersionInput    `json:"version"`
	Codes     []CatalogCodeInput     `json:"codes"`
	Documents []CatalogDocumentInput `json:"documents"`
	Rules     []CatalogRuleInput     `json:"rules"`
}
type CatalogVersionInput struct {
	VersionKey  string `json:"versionKey"`
	DatasetKind string `json:"datasetKind"`
	Source      string `json:"source"`
	SourceDate  string `json:"sourceDate"`
	Status      string `json:"status"`
}
type CatalogCodeInput struct {
	Code                string `json:"code"`
	HierarchyLevel      int    `json:"hierarchyLevel"`
	ParentCode          string `json:"parentCode"`
	OfficialDescription string `json:"officialDescription"`
	IsGroup             bool   `json:"isGroup"`
	ValidFrom           string `json:"validFrom"`
	ValidTo             string `json:"validTo"`
	Source              string `json:"source"`
}
type CatalogDocumentInput struct {
	Code       string `json:"code"`
	ParentCode string `json:"parentCode"`
	DocType    string `json:"docType"`
	Title      string `json:"title"`
	Content    string `json:"content"`
	Source     string `json:"source"`
	ValidFrom  string `json:"validFrom"`
	ValidTo    string `json:"validTo"`
}
type CatalogRuleInput struct {
	Code        string          `json:"code"`
	ParentCode  string          `json:"parentCode"`
	RuleKey     string          `json:"ruleKey"`
	PaymentType string          `json:"paymentType"`
	RateType    string          `json:"rateType"`
	Value       json.RawMessage `json:"value"`
	Currency    string          `json:"currency"`
	Unit        string          `json:"unit"`
	Operation   string          `json:"operation"`
	Conditions  map[string]any  `json:"conditions"`
	ValidFrom   string          `json:"validFrom"`
	ValidTo     string          `json:"validTo"`
	Source      string          `json:"source"`
}
type CatalogImportReport struct {
	Valid         bool     `json:"valid"`
	Errors        []string `json:"errors"`
	Warnings      []string `json:"warnings"`
	CodeCount     int      `json:"codeCount"`
	DocumentCount int      `json:"documentCount"`
	RuleCount     int      `json:"ruleCount"`
}

type CatalogService struct{ repository *repository.Repository }

func NewCatalogService(r *repository.Repository) *CatalogService {
	return &CatalogService{repository: r}
}

func parseCatalogDate(value, field string, required bool) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		if required {
			return time.Time{}, fmt.Errorf("%s: дата обязательна", field)
		}
		return time.Time{}, nil
	}
	d, err := time.Parse("2006-01-02", value)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s: используйте дату ГГГГ-ММ-ДД", field)
	}
	return d, nil
}
func parseCatalogDecimal(raw json.RawMessage) (*decimal.Decimal, error) {
	if len(raw) == 0 || string(raw) == "null" || strings.TrimSpace(string(raw)) == "" {
		return nil, nil
	}
	var s string
	if raw[0] == '"' {
		if err := json.Unmarshal(raw, &s); err != nil {
			return nil, err
		}
	} else {
		s = string(raw)
	}
	d, err := decimal.NewFromString(strings.TrimSpace(s))
	if err != nil {
		return nil, fmt.Errorf("некорректное значение ставки")
	}
	return &d, nil
}
func rangesOverlap(a, b time.Time, ae, be *time.Time) bool {
	aEnd := time.Time{}
	if ae != nil {
		aEnd = *ae
	}
	bEnd := time.Time{}
	if be != nil {
		bEnd = *be
	}
	return (aEnd.IsZero() || !b.After(aEnd)) && (bEnd.IsZero() || !a.After(bEnd))
}

func (s *CatalogService) Validate(input CatalogImportRequest) (CatalogImportReport, error) {
	r := CatalogImportReport{Valid: true, CodeCount: len(input.Codes), DocumentCount: len(input.Documents), RuleCount: len(input.Rules)}
	v := input.Version
	if strings.TrimSpace(v.VersionKey) == "" {
		r.Errors = append(r.Errors, "version.versionKey обязателен")
	}
	if v.DatasetKind != "demo" && v.DatasetKind != "real" {
		r.Errors = append(r.Errors, "version.datasetKind должен быть real или demo")
	}
	if strings.TrimSpace(v.Source) == "" {
		r.Errors = append(r.Errors, "version.source обязателен")
	}
	if v.Status == "" {
		v.Status = "active"
	}
	if v.Status != "active" && v.Status != "archived" {
		r.Errors = append(r.Errors, "version.status должен быть active или archived")
	}
	if _, err := parseCatalogDate(v.SourceDate, "version.sourceDate", false); err != nil {
		r.Errors = append(r.Errors, err.Error())
	}
	seen := map[string]bool{}
	for _, c := range input.Codes {
		seen[c.Code] = true
	}
	seenOnce := map[string]bool{}
	for i, c := range input.Codes {
		p := fmt.Sprintf("codes[%d]", i)
		if !catalogCodePattern.MatchString(c.Code) {
			r.Errors = append(r.Errors, p+".code должен содержать только цифры (строка)")
		}
		if strings.TrimSpace(c.ParentCode) != "" && !seen[c.ParentCode] {
			r.Errors = append(r.Errors, p+".parentCode ссылается на неизвестный код")
		}
		if seenOnce[c.Code] {
			r.Errors = append(r.Errors, p+" дублирует код")
		}
		seenOnce[c.Code] = true
		if c.HierarchyLevel < 0 {
			r.Errors = append(r.Errors, p+".hierarchyLevel не может быть отрицательным")
		}
		if c.ParentCode != "" && !catalogCodePattern.MatchString(c.ParentCode) {
			r.Errors = append(r.Errors, p+".parentCode некорректен")
		}
		if strings.TrimSpace(c.OfficialDescription) == "" {
			r.Errors = append(r.Errors, p+".officialDescription обязателен")
		}
		if strings.TrimSpace(c.Source) == "" {
			r.Errors = append(r.Errors, p+".source обязателен")
		}
		from, e := parseCatalogDate(c.ValidFrom, p+".validFrom", true)
		if e != nil {
			r.Errors = append(r.Errors, e.Error())
		}
		to, e := parseCatalogDate(c.ValidTo, p+".validTo", false)
		if e != nil {
			r.Errors = append(r.Errors, e.Error())
		}
		if !to.IsZero() && !from.IsZero() && to.Before(from) {
			r.Errors = append(r.Errors, p+" период действия некорректен")
		}
		_ = from
		_ = to
	}
	for i, d := range input.Documents {
		p := fmt.Sprintf("documents[%d]", i)
		if d.Code == "" && d.ParentCode == "" {
			r.Errors = append(r.Errors, p+" укажите code или parentCode")
		}
		if d.Code != "" && !seen[d.Code] {
			r.Errors = append(r.Errors, p+" ссылается на неизвестный code")
		}
		if d.DocType != "note" && d.DocType != "explanation" && d.DocType != "exception" {
			r.Errors = append(r.Errors, p+".docType некорректен")
		}
		if strings.TrimSpace(d.Content) == "" {
			r.Errors = append(r.Errors, p+".content обязателен")
		}
		if strings.TrimSpace(d.Source) == "" {
			r.Errors = append(r.Errors, p+".source обязателен")
		}
		from, e := parseCatalogDate(d.ValidFrom, p+".validFrom", true)
		if e != nil {
			r.Errors = append(r.Errors, e.Error())
		}
		to, e := parseCatalogDate(d.ValidTo, p+".validTo", false)
		if e != nil {
			r.Errors = append(r.Errors, e.Error())
		}
		if !to.IsZero() && !from.IsZero() && to.Before(from) {
			r.Errors = append(r.Errors, p+" период действия некорректен")
		}
	}
	ruleSeen := map[string]repository.CalculationRule{}
	ruleTargets := map[string][]repository.CalculationRule{}
	for i, ru := range input.Rules {
		p := fmt.Sprintf("rules[%d]", i)
		if ru.Code == "" && ru.ParentCode == "" {
			r.Errors = append(r.Errors, p+" укажите code или parentCode")
		}
		if ru.Code != "" && !seen[ru.Code] {
			r.Errors = append(r.Errors, p+" ссылается на неизвестный code")
		}
		if ru.RuleKey == "" {
			r.Errors = append(r.Errors, p+".ruleKey обязателен")
		}
		if _, ok := ruleSeen[ru.RuleKey]; ok {
			r.Errors = append(r.Errors, p+" дублирует ruleKey")
		}
		from, e := parseCatalogDate(ru.ValidFrom, p+".validFrom", true)
		if e != nil {
			r.Errors = append(r.Errors, e.Error())
		}
		to, e := parseCatalogDate(ru.ValidTo, p+".validTo", false)
		if e != nil {
			r.Errors = append(r.Errors, e.Error())
		}
		if !to.IsZero() && !from.IsZero() && to.Before(from) {
			r.Errors = append(r.Errors, p+" период действия некорректен")
		}
		if ru.RateType != "ad_valorem" && ru.RateType != "specific" && ru.RateType != "combined" && ru.RateType != "fixed" && ru.RateType != "unknown" {
			r.Errors = append(r.Errors, p+".rateType некорректен")
		}
		val, e := parseCatalogDecimal(ru.Value)
		if e != nil {
			r.Errors = append(r.Errors, p+".value: "+e.Error())
		}
		if ru.RateType != "unknown" && val == nil {
			r.Errors = append(r.Errors, p+".value обязателен; неизвестную ставку обозначайте rateType=unknown, а не нулём")
		}
		if val != nil && val.IsNegative() {
			r.Errors = append(r.Errors, p+".value не может быть отрицательным")
		}
		if val != nil && val.IsZero() && strings.TrimSpace(ru.Source) == "" {
			r.Errors = append(r.Errors, p+" нулевая ставка требует source")
		}
		if strings.TrimSpace(ru.Source) == "" {
			r.Errors = append(r.Errors, p+".source обязателен")
		}
		if ru.RateType == "specific" && (ru.Currency == "" || ru.Unit == "") {
			r.Errors = append(r.Errors, p+" для specific обязательны currency и unit")
		}
		if ru.RateType == "combined" && ru.Operation == "" {
			r.Errors = append(r.Errors, p+" для combined обязательна operation")
		}
		cr := repository.CalculationRule{RuleKey: ru.RuleKey, PaymentType: ru.PaymentType, ValidFrom: from, ValidTo: ptrTime(to), Source: ru.Source}
		if old, ok := ruleSeen[ru.RuleKey]; ok && rangesOverlap(old.ValidFrom, cr.ValidFrom, old.ValidTo, cr.ValidTo) {
			r.Errors = append(r.Errors, p+" пересекается с другим правилом")
		}
		target := firstNonEmpty(ru.Code, ru.ParentCode) + "\x00" + ru.PaymentType
		for _, old := range ruleTargets[target] {
			if rangesOverlap(old.ValidFrom, cr.ValidFrom, old.ValidTo, cr.ValidTo) && old.RuleKey != cr.RuleKey {
				r.Errors = append(r.Errors, p+" пересекается по периоду с правилом "+old.RuleKey)
			}
		}
		ruleTargets[target] = append(ruleTargets[target], cr)
		ruleSeen[ru.RuleKey] = cr
	}
	r.Valid = len(r.Errors) == 0
	return r, nil
}
func ptrTime(t time.Time) *time.Time {
	if t.IsZero() {
		return nil
	}
	return &t
}

func (s *CatalogService) Apply(ctx context.Context, input CatalogImportRequest) (repository.ClassifierVersion, CatalogImportReport, error) {
	report, _ := s.Validate(input)
	if !report.Valid {
		return repository.ClassifierVersion{}, report, &ValidationError{Field: "catalog", Message: strings.Join(report.Errors, "; ")}
	}
	vDate, _ := parseCatalogDate(input.Version.SourceDate, "sourceDate", false)
	v := repository.ClassifierVersion{VersionKey: input.Version.VersionKey, DatasetKind: input.Version.DatasetKind, Source: input.Version.Source, SourceDate: ptrTime(vDate), LoadedAt: time.Now().UTC(), Status: input.Version.Status}
	if v.Status == "" {
		v.Status = "active"
	}
	codes := make([]repository.ClassifierCode, 0, len(input.Codes))
	for _, c := range input.Codes {
		from, _ := parseCatalogDate(c.ValidFrom, "", true)
		to, _ := parseCatalogDate(c.ValidTo, "", false)
		codes = append(codes, repository.ClassifierCode{Code: c.Code, HierarchyLevel: c.HierarchyLevel, ParentCode: c.ParentCode, OfficialDescription: c.OfficialDescription, IsGroup: c.IsGroup, ValidFrom: from, ValidTo: ptrTime(to), Source: c.Source})
	}
	docs := make([]repository.ClassifierDocument, 0, len(input.Documents))
	for _, d := range input.Documents {
		from, _ := parseCatalogDate(d.ValidFrom, "", true)
		to, _ := parseCatalogDate(d.ValidTo, "", false)
		docs = append(docs, repository.ClassifierDocument{ParentCode: firstNonEmpty(d.ParentCode, d.Code), DocType: d.DocType, Title: d.Title, Content: d.Content, Source: d.Source, ValidFrom: from, ValidTo: ptrTime(to)})
	}
	rules := make([]repository.CalculationRule, 0, len(input.Rules))
	for _, ru := range input.Rules {
		from, _ := parseCatalogDate(ru.ValidFrom, "", true)
		to, _ := parseCatalogDate(ru.ValidTo, "", false)
		val, _ := parseCatalogDecimal(ru.Value)
		cond, _ := json.Marshal(ru.Conditions)
		rules = append(rules, repository.CalculationRule{ParentCode: firstNonEmpty(ru.ParentCode, ru.Code), RuleKey: ru.RuleKey, PaymentType: ru.PaymentType, RateType: ru.RateType, Value: val, Currency: ru.Currency, Unit: ru.Unit, Operation: ru.Operation, Conditions: repository.JSON(cond), ValidFrom: from, ValidTo: ptrTime(to), Source: ru.Source})
	}
	saved, err := s.repository.ImportCatalog(ctx, v, codes, docs, rules)
	return saved, report, err
}
func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func (s *CatalogService) Search(ctx context.Context, q, date string, includeDemo bool) ([]repository.CatalogSearchRow, error) {
	d := time.Now().UTC()
	if date != "" {
		var err error
		d, err = time.Parse("2006-01-02", date)
		if err != nil {
			return nil, &ValidationError{Field: "date", Message: "Дата оценки должна быть в формате ГГГГ-ММ-ДД"}
		}
	}
	return s.repository.SearchCatalog(ctx, q, d, includeDemo)
}
func (s *CatalogService) Details(ctx context.Context, code, date string, includeDemo bool) ([]repository.CatalogDetails, error) {
	d := time.Now().UTC()
	if date != "" {
		var err error
		d, err = time.Parse("2006-01-02", date)
		if err != nil {
			return nil, &ValidationError{Field: "date", Message: "Дата оценки должна быть в формате ГГГГ-ММ-ДД"}
		}
	}
	return s.repository.CatalogDetails(ctx, code, d, includeDemo)
}

type ApplicableInput struct {
	Code       string         `json:"code"`
	Date       string         `json:"date"`
	Conditions map[string]any `json:"conditions"`
	AllowDemo  bool           `json:"allowDemo"`
}
type ApplicableResult struct {
	Status  string                       `json:"status"`
	Message string                       `json:"message,omitempty"`
	Rules   []repository.CalculationRule `json:"rules,omitempty"`
	Missing []string                     `json:"missing,omitempty"`
}

func conditionMatches(rule repository.CalculationRule, input map[string]any) (bool, []string) {
	var req map[string]any
	_ = json.Unmarshal(rule.Conditions, &req)
	missing := []string{}
	for k, v := range req {
		iv, ok := input[k]
		if !ok {
			missing = append(missing, k)
			continue
		}
		rb, _ := json.Marshal(v)
		ib, _ := json.Marshal(iv)
		if string(rb) != string(ib) {
			return false, nil
		}
	}
	return len(missing) == 0, missing
}
func (s *CatalogService) Applicable(ctx context.Context, in ApplicableInput) (ApplicableResult, error) {
	d := time.Now().UTC()
	if in.Date != "" {
		var err error
		d, err = time.Parse("2006-01-02", in.Date)
		if err != nil {
			return ApplicableResult{}, &ValidationError{Field: "date", Message: "Дата оценки должна быть в формате ГГГГ-ММ-ДД"}
		}
	}
	rules, codes, versions, err := s.repository.RulesForCode(ctx, in.Code, d, true)
	if err != nil {
		return ApplicableResult{Status: "not_found", Message: "Конечный код не найден на указанную дату"}, nil
	}
	for _, c := range codes {
		if c.IsGroup {
			return ApplicableResult{Status: "group_code", Message: "Группировочный код нельзя использовать для расчёта"}, nil
		}
	}
	if !in.AllowDemo {
		nonDemo := false
		demoIDs := map[int64]bool{}
		for _, v := range versions {
			if v.DatasetKind != "demo" {
				nonDemo = true
			} else {
				demoIDs[v.ID] = true
			}
		}
		if !nonDemo {
			return ApplicableResult{Status: "demo_blocked", Message: "Для DEMO-данных требуется явное разрешение; реальные правила отсутствуют"}, nil
		}
		filtered := rules[:0]
		for _, rule := range rules {
			if !demoIDs[rule.VersionID] {
				filtered = append(filtered, rule)
			}
		}
		rules = filtered
	}
	matched := []repository.CalculationRule{}
	missingSet := map[string]bool{}
	for _, r := range rules {
		ok, missing := conditionMatches(r, in.Conditions)
		if ok {
			matched = append(matched, r)
		} else {
			for _, m := range missing {
				missingSet[m] = true
			}
		}
	}
	if len(matched) == 0 {
		missing := make([]string, 0, len(missingSet))
		for k := range missingSet {
			missing = append(missing, k)
		}
		sort.Strings(missing)
		return ApplicableResult{Status: "ambiguous", Message: "Недостаточно условий партии", Missing: missing}, nil
	}
	if len(matched) > 1 {
		return ApplicableResult{Status: "ambiguous", Message: "Подходит несколько правил; условия противоречат друг другу", Rules: matched}, nil
	}
	return ApplicableResult{Status: "ok", Rules: matched}, nil
}

// ParseCatalogCSV accepts a compact code-only CSV contract. Header: versionKey,datasetKind,source,code,hierarchyLevel,parentCode,officialDescription,isGroup,validFrom,validTo.
func ParseCatalogCSV(r io.Reader) (CatalogImportRequest, error) {
	cr := csv.NewReader(r)
	hdr, err := cr.Read()
	if err != nil {
		return CatalogImportRequest{}, err
	}
	idx := map[string]int{}
	for i, h := range hdr {
		idx[strings.TrimSpace(h)] = i
	}
	required := []string{"versionKey", "datasetKind", "source", "code", "hierarchyLevel", "officialDescription", "isGroup", "validFrom"}
	for _, h := range required {
		if _, ok := idx[h]; !ok {
			return CatalogImportRequest{}, fmt.Errorf("CSV: отсутствует колонка %s", h)
		}
	}
	var out CatalogImportRequest
	for {
		row, e := cr.Read()
		if e == io.EOF {
			break
		}
		if e != nil {
			return out, e
		}
		get := func(k string) string {
			if i, ok := idx[k]; ok && i < len(row) {
				return row[i]
			}
			return ""
		}
		if out.Version.VersionKey == "" {
			out.Version = CatalogVersionInput{VersionKey: get("versionKey"), DatasetKind: get("datasetKind"), Source: get("source"), Status: "active"}
		}
		out.Codes = append(out.Codes, CatalogCodeInput{Code: get("code"), HierarchyLevel: atoi(get("hierarchyLevel")), ParentCode: get("parentCode"), OfficialDescription: get("officialDescription"), IsGroup: strings.EqualFold(get("isGroup"), "true"), ValidFrom: get("validFrom"), ValidTo: get("validTo"), Source: get("source")})
	}
	return out, nil
}
func atoi(s string) int { var n int; fmt.Sscanf(s, "%d", &n); return n }
