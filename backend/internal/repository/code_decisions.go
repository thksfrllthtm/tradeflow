package repository

// База решений: проверенные примеры «товар из инвойса → код ТН ВЭД». Список читается без фото
// (только миниатюра), карточка — с фото.

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type CodeDecision struct {
	ID            int64            `gorm:"primaryKey"`
	Status        string           `gorm:"size:16;not null"`
	Source        string           `gorm:"size:16;not null"`
	InvoiceName   string           `gorm:"not null"`
	SupplierName  string           `gorm:"not null"`
	FinalName     string           `gorm:"not null"`
	Code          string           `gorm:"size:10;not null"`
	Reason        string           `gorm:"not null"`
	Quote         string           `gorm:"not null"`
	SourceURL     string           `gorm:"column:source_url;size:500;not null"`
	RejectedCodes JSON             `gorm:"type:jsonb;not null"`
	UnitNetKg     *decimal.Decimal `gorm:"type:numeric(14,6)"`
	UnitPriceUSD  *decimal.Decimal `gorm:"column:unit_price_usd;type:numeric(14,4)"`
	Photos        JSON             `gorm:"type:jsonb;not null"`
	Thumb         string           `gorm:"not null"`
	PhotoHashes   JSON             `gorm:"type:jsonb;not null"`
	// Описание фото от ИИ (decisions.PhotoFeatures) и когда оно составлено.
	PhotoFeatures      JSON       `gorm:"type:jsonb;not null"`
	PhotoFeaturesAt    *time.Time `gorm:"column:photo_features_at"`
	AgentCode          string     `gorm:"size:10;not null"`
	AgentCorrect       *bool      `gorm:"column:agent_correct"`
	EstimateID         *int64     `gorm:"column:estimate_id"`
	ItemID             string     `gorm:"size:64;not null"`
	SessionID          *int64     `gorm:"column:session_id"`
	ProductDirectoryID *int64     `gorm:"column:product_directory_id"`
	Note               string     `gorm:"not null"`
	CreatedBy          *int64     `gorm:"column:created_by"`
	VerifiedBy         *int64     `gorm:"column:verified_by"`
	VerifiedAt         *time.Time `gorm:"column:verified_at"`
	CreatedAt          time.Time  `gorm:"not null"`
	UpdatedAt          time.Time  `gorm:"not null"`
	// Filled by list and card queries.
	CreatedByName  string `gorm:"->;-:migration"`
	VerifiedByName string `gorm:"->;-:migration"`
}

func (CodeDecision) TableName() string { return "code_decisions" }

type CodeDecisionFilter struct {
	Status string
	Query  string
	// AgentWrong — только решения, где администратор исправил код агента.
	AgentWrong bool
	// AlsoIDs — записи, найденные по смыслу (эмбеддинги): показываются вместе с найденными по словам Query.
	AlsoIDs []int64
	Limit   int
	Offset  int
}

// Колонки списка: всё, кроме полных фото.
const codeDecisionListColumns = "d.id, d.status, d.source, d.invoice_name, d.supplier_name, d.final_name, d.code, d.reason, d.quote, " +
	"d.source_url, d.rejected_codes, d.unit_net_kg, d.unit_price_usd, '[]'::jsonb AS photos, d.thumb, '[]'::jsonb AS photo_hashes, d.photo_features, d.photo_features_at, " +
	"d.agent_code, d.agent_correct, d.estimate_id, d.item_id, d.session_id, d.product_directory_id, d.note, d.created_by, " +
	"d.verified_by, d.verified_at, d.created_at, d.updated_at, COALESCE(NULLIF(cu.name, ''), cu.email, '') AS created_by_name, " +
	"COALESCE(NULLIF(vu.name, ''), vu.email, '') AS verified_by_name"

func codeDecisionQuery(db *gorm.DB) *gorm.DB {
	return db.Table("code_decisions AS d").
		Joins("LEFT JOIN users cu ON cu.id = d.created_by").
		Joins("LEFT JOIN users vu ON vu.id = d.verified_by")
}

// wordsWhere: every word's stem occurs in one of the columns.
func wordsWhere(query *gorm.DB, text string, columns ...string) *gorm.DB {
	expr, args := wordsExpr(text, columns...)
	return query.Where(expr, args...)
}

// wordsExpr — условие «каждое слово запроса встречается в одной из колонок» и его аргументы.
func wordsExpr(text string, columns ...string) (string, []any) {
	conditions := []string{}
	args := []any{}
	for _, word := range strings.Fields(text) {
		pattern := "%" + escapeLike(SearchStem(strings.ToLower(word))) + "%"
		parts := make([]string, 0, len(columns))
		for _, column := range columns {
			parts = append(parts, column+" ILIKE ?")
			args = append(args, pattern)
		}
		conditions = append(conditions, "("+strings.Join(parts, " OR ")+")")
	}
	if len(conditions) == 0 {
		return "TRUE", nil
	}
	return strings.Join(conditions, " AND "), args
}

func (r *Repository) ListCodeDecisions(ctx context.Context, filter CodeDecisionFilter) ([]CodeDecision, int64, error) {
	query := codeDecisionQuery(r.db.WithContext(ctx))
	listOrder := clause.Expr{SQL: decisionListOrder}
	if filter.Status != "" {
		query = query.Where("d.status = ?", filter.Status)
	}
	if filter.AgentWrong {
		query = query.Where("d.agent_correct = false")
	}
	if text := strings.TrimSpace(filter.Query); text != "" {
		if digitsQuery.MatchString(text) {
			query = query.Where("d.code LIKE ?", strings.NewReplacer(" ", "", ".", "").Replace(text)+"%")
		} else {
			where, order := decisionTextSearch(text, filter.AlsoIDs)
			query = query.Where(where.SQL, where.Vars...)
			order.SQL += ", " + decisionListOrder
			listOrder = order
		}
	}
	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []CodeDecision
	err := query.Select(codeDecisionListColumns).Order(clause.OrderBy{Expression: listOrder}).Limit(filter.Limit).Offset(filter.Offset).Scan(&rows).Error
	return rows, total, err
}

const decisionListOrder = "d.updated_at DESC, d.id DESC"

// Из описания фото ищется только «что это за товар». Материал и особенности не участвуют:
// «картонная упаковка» есть почти у всех, и «карта» находила рулетки и наждачку.
const decisionPhotoSearchText = "concat_ws(' ', d.photo_features->>'kind', d.photo_features->>'purpose', " +
	"d.photo_features->'synonyms', d.photo_features->'keywords')"

// decisionTextSearch — условие поиска по словам (alsoIDs — найденные по смыслу) и порядок по
// релевантности: совпало наименование → совпало описание фото → только по смыслу.
func decisionTextSearch(text string, alsoIDs []int64) (where, order clause.Expr) {
	byName, nameArgs := wordsExpr(text, "d.invoice_name", "d.supplier_name", "d.final_name")
	byWords, wordArgs := wordsExpr(text, "d.invoice_name", "d.supplier_name", "d.final_name", decisionPhotoSearchText)
	order = clause.Expr{
		SQL:  "CASE WHEN " + byName + " THEN 0 WHEN " + byWords + " THEN 1 ELSE 2 END",
		Vars: append(append([]any{}, nameArgs...), wordArgs...),
	}
	where = clause.Expr{SQL: byWords, Vars: wordArgs}
	if len(alsoIDs) > 0 {
		where = clause.Expr{SQL: "(" + byWords + ") OR d.id IN ?", Vars: append(append([]any{}, wordArgs...), alsoIDs)}
	}
	return where, order
}

func (r *Repository) CodeDecisionByID(ctx context.Context, id int64) (CodeDecision, error) {
	var rows []CodeDecision
	err := codeDecisionQuery(r.db.WithContext(ctx)).
		Select("d.*, COALESCE(NULLIF(cu.name, ''), cu.email, '') AS created_by_name, COALESCE(NULLIF(vu.name, ''), vu.email, '') AS verified_by_name").
		Where("d.id = ?", id).Scan(&rows).Error
	if err != nil {
		return CodeDecision{}, err
	}
	if len(rows) == 0 {
		return CodeDecision{}, ErrNotFound
	}
	return rows[0], nil
}

func (r *Repository) CreateCodeDecisions(ctx context.Context, rows []CodeDecision) ([]CodeDecision, error) {
	if len(rows) == 0 {
		return rows, nil
	}
	err := r.db.WithContext(ctx).Omit("CreatedByName", "VerifiedByName").CreateInBatches(&rows, 200).Error
	return rows, err
}

func (r *Repository) UpdateCodeDecision(ctx context.Context, id int64, changes map[string]any) error {
	result := r.db.WithContext(ctx).Model(&CodeDecision{}).Where("id = ?", id).Updates(changes)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteAllCodeDecisions очищает базу решений (импорт с заменой). Возвращает число удалённых записей.
func (r *Repository) DeleteAllCodeDecisions(ctx context.Context) (int64, error) {
	result := r.db.WithContext(ctx).Exec("DELETE FROM code_decisions")
	return result.RowsAffected, result.Error
}

func (r *Repository) DeleteCodeDecision(ctx context.Context, id int64) error {
	result := r.db.WithContext(ctx).Where("id = ?", id).Delete(&CodeDecision{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

// SearchCodeDecisions — примеры для агента: не отклонённые, где встречается хотя бы одно слово запроса
// (по основе). Сверху — больше совпавших слов, среди равных — проверенные и свежие. Все слова сразу
// не требуются: «отвертка крестовая» должна находить образец «Ручной инструмент: Отвертка».
func (r *Repository) SearchCodeDecisions(ctx context.Context, text string, limit int) ([]CodeDecision, error) {
	const haystack = "(d.invoice_name || ' ' || d.supplier_name || ' ' || d.final_name)"
	parts, args := []string{}, []any{}
	for _, word := range strings.Fields(strings.ToLower(text)) {
		if stem := SearchStem(word); len([]rune(stem)) >= 2 && len(parts) < 8 {
			parts = append(parts, "(CASE WHEN "+haystack+" ILIKE ? THEN 1 ELSE 0 END)")
			args = append(args, "%"+escapeLike(stem)+"%")
		}
	}
	var rows []CodeDecision
	if len(parts) == 0 {
		return rows, nil
	}
	score := strings.Join(parts, " + ")
	query := codeDecisionQuery(r.db.WithContext(ctx)).Where("d.status <> 'rejected'").Where("("+score+") > 0", args...)
	err := query.Select(codeDecisionListColumns).
		Order(clause.OrderBy{Expression: gorm.Expr("("+score+") DESC, CASE d.status WHEN 'verified' THEN 0 ELSE 1 END, d.updated_at DESC", args...)}).
		Limit(limit).Scan(&rows).Error
	return rows, err
}

// CodeDecisionsByIDs — карточки без фото (для сравнения по хешам фото и по наименованию).
func (r *Repository) CodeDecisionsByIDs(ctx context.Context, ids []int64) ([]CodeDecision, error) {
	var rows []CodeDecision
	if len(ids) == 0 {
		return rows, nil
	}
	err := codeDecisionQuery(r.db.WithContext(ctx)).Select(codeDecisionListColumns).Where("d.id IN ?", ids).Scan(&rows).Error
	return rows, err
}

// CodeDecisionKey — то, по чему ищется тот же товар: хеши фото и наименования.
type CodeDecisionKey struct {
	ID            int64
	Status        string
	Code          string
	InvoiceName   string
	SupplierName  string
	FinalName     string
	PhotoHashes   JSON
	PhotoFeatures JSON
}

func (r *Repository) CodeDecisionKeys(ctx context.Context) ([]CodeDecisionKey, error) {
	var rows []CodeDecisionKey
	err := r.db.WithContext(ctx).Table("code_decisions").Select("id, status, code, invoice_name, supplier_name, final_name, photo_hashes, photo_features").
		Where("status <> 'rejected'").Scan(&rows).Error
	return rows, err
}

type CodeDecisionStats struct {
	Pending      int64
	Verified     int64
	Rejected     int64
	AgentChecked int64
	AgentCorrect int64
}

// CodeDecisionStats — счётчики по статусам и точность агента по проверенным за 90 дней.
func (r *Repository) CodeDecisionStats(ctx context.Context) (CodeDecisionStats, error) {
	var stats CodeDecisionStats
	err := r.db.WithContext(ctx).Raw(`SELECT
		COUNT(*) FILTER (WHERE status = 'pending') AS pending,
		COUNT(*) FILTER (WHERE status = 'verified') AS verified,
		COUNT(*) FILTER (WHERE status = 'rejected') AS rejected,
		COUNT(*) FILTER (WHERE agent_correct IS NOT NULL AND verified_at > now() - interval '90 days') AS agent_checked,
		COUNT(*) FILTER (WHERE agent_correct AND verified_at > now() - interval '90 days') AS agent_correct
		FROM code_decisions`).Scan(&stats).Error
	return stats, err
}

// SearchDirectoriesForAgent — опыт компании в справочнике товаров: наименования, синонимы и код.
type DirectoryHint struct {
	Kind               string
	Name               string
	NameZH             string
	Code               string
	VerificationStatus string
}

func (r *Repository) SearchDirectoriesForAgent(ctx context.Context, text string, limit int) ([]DirectoryHint, error) {
	products := r.db.WithContext(ctx).Table("product_directory_entries AS p").Where("p.archived_at IS NULL")
	products = wordsWhere(products, text, "p.name", "p.name_zh", "p.synonyms_ru::text", "p.synonyms_zh::text", "p.synonyms_en::text")
	var productRows []DirectoryHint
	if err := products.Select("'product' AS kind, p.name, p.name_zh, p.code, p.verification_status").
		Order("p.updated_at DESC").Limit(limit).Scan(&productRows).Error; err != nil {
		return nil, err
	}
	return productRows, nil
}

// DeletePendingDecisionsForSession — непроверенная запись подбора заменяется новой при повторном «Применить».
func (r *Repository) DeletePendingDecisionsForSession(ctx context.Context, sessionID int64) error {
	return r.db.WithContext(ctx).Where("session_id = ? AND status = 'pending'", sessionID).Delete(&CodeDecision{}).Error
}

// SetPendingDecisionNameForSession — наименование непроверенной записи подбора (после «Применить наименование»).
func (r *Repository) SetPendingDecisionNameForSession(ctx context.Context, sessionID int64, name string) error {
	return r.db.WithContext(ctx).Model(&CodeDecision{}).Where("session_id = ? AND status = 'pending'", sessionID).
		Updates(map[string]any{"final_name": name, "updated_at": time.Now().UTC()}).Error
}

// DecisionPhoto — первое фото записи базы решений.
type DecisionPhoto struct {
	ID     int64
	Photos JSON
}

// DecisionsNeedingPhotoFeatures — записи с фото, для которых ИИ ещё не составил описание.
func (r *Repository) DecisionsNeedingPhotoFeatures(ctx context.Context, limit int) ([]DecisionPhoto, error) {
	var rows []DecisionPhoto
	err := r.db.WithContext(ctx).Table("code_decisions").Select("id, photos").
		Where("photo_features_at IS NULL AND photos <> '[]'::jsonb AND status <> 'rejected'").
		Order("id DESC").Limit(limit).Scan(&rows).Error
	return rows, err
}

// DecisionPhotos — фото записей (для визуальной сверки похожих товаров).
func (r *Repository) DecisionPhotos(ctx context.Context, ids []int64) ([]DecisionPhoto, error) {
	var rows []DecisionPhoto
	if len(ids) == 0 {
		return rows, nil
	}
	err := r.db.WithContext(ctx).Table("code_decisions").Select("id, photos").Where("id IN ?", ids).Scan(&rows).Error
	return rows, err
}

// DecisionsMissingThumb — записи с фото без миниатюры, по возрастанию id после after.
func (r *Repository) DecisionsMissingThumb(ctx context.Context, after int64, limit int) ([]DecisionPhoto, error) {
	var rows []DecisionPhoto
	err := r.db.WithContext(ctx).Table("code_decisions").Select("id, photos").
		Where("id > ? AND thumb = '' AND photos <> '[]'::jsonb", after).
		Order("id").Limit(limit).Scan(&rows).Error
	return rows, err
}

// DecisionThumbs — миниатюры записей по id (без миниатюры — нет в карте).
func (r *Repository) DecisionThumbs(ctx context.Context, ids []int64) (map[int64]string, error) {
	var rows []struct {
		ID    int64
		Thumb string
	}
	err := r.db.WithContext(ctx).Table("code_decisions").Select("id, thumb").Where("id IN ? AND thumb <> ''", ids).Scan(&rows).Error
	thumbs := make(map[int64]string, len(rows))
	for _, row := range rows {
		thumbs[row.ID] = row.Thumb
	}
	return thumbs, err
}

// SaveDecisionThumb сохраняет миниатюру записи.
func (r *Repository) SaveDecisionThumb(ctx context.Context, id int64, thumb string) error {
	return r.db.WithContext(ctx).Model(&CodeDecision{}).Where("id = ?", id).Update("thumb", thumb).Error
}

// SaveDecisionPhotoFeatures сохраняет описание фото (null — описать не удалось, больше не пробуем).
// Вектор описания сбрасывается: фоновый обработчик посчитает его заново.
func (r *Repository) SaveDecisionPhotoFeatures(ctx context.Context, id int64, features JSON) error {
	return r.db.WithContext(ctx).Model(&CodeDecision{}).Where("id = ?", id).
		Updates(map[string]any{"photo_features": features, "photo_features_at": time.Now().UTC(), "photo_embedding": nil}).Error
}

// vectorText — вектор в записи pgvector: «[0.1,0.2,…]».
func vectorText(vector []float32) string {
	parts := make([]string, len(vector))
	for index, value := range vector {
		parts[index] = strconv.FormatFloat(float64(value), 'g', -1, 32)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

type DecisionFeatures struct {
	ID            int64
	PhotoFeatures JSON
}

// DecisionsNeedingEmbedding — записи с описанием фото, но без его вектора.
func (r *Repository) DecisionsNeedingEmbedding(ctx context.Context, limit int) ([]DecisionFeatures, error) {
	var rows []DecisionFeatures
	err := r.db.WithContext(ctx).Table("code_decisions").Select("id, photo_features").
		Where("photo_embedding IS NULL AND jsonb_typeof(photo_features) = 'object' AND status <> 'rejected'").
		Order("id DESC").Limit(limit).Scan(&rows).Error
	return rows, err
}

// SaveDecisionEmbedding сохраняет вектор описания фото.
func (r *Repository) SaveDecisionEmbedding(ctx context.Context, id int64, vector []float32) error {
	return r.db.WithContext(ctx).Exec("UPDATE code_decisions SET photo_embedding = ?::vector WHERE id = ?", vectorText(vector), id).Error
}

type DecisionNeighbor struct {
	ID int64
	// Similarity — косинусная близость векторов (1 — одинаковые).
	Similarity float64
}

// NearestDecisions — записи, ближайшие по вектору описания (не отклонённые), лучшие первыми.
// Без индекса: записей сотни, перебор быстрее. ponytail: HNSW-индекс, когда записей станут десятки тысяч.
func (r *Repository) NearestDecisions(ctx context.Context, vector []float32, limit int) ([]DecisionNeighbor, error) {
	var rows []DecisionNeighbor
	text := vectorText(vector)
	err := r.db.WithContext(ctx).Raw(`SELECT id, 1 - (photo_embedding <=> ?::vector) AS similarity FROM code_decisions
		WHERE photo_embedding IS NOT NULL AND status <> 'rejected' ORDER BY photo_embedding <=> ?::vector LIMIT ?`, text, text, limit).Scan(&rows).Error
	return rows, err
}

// CodeDecisionsByEstimate — записи, сохранённые из строк сметы (кроме отклонённых): для проверки,
// не сохранён ли уже тот же товар этой сметы.
func (r *Repository) CodeDecisionsByEstimate(ctx context.Context, estimateID int64) ([]CodeDecision, error) {
	var rows []CodeDecision
	err := r.db.WithContext(ctx).Select("id", "status", "code", "invoice_name", "final_name", "photo_hashes", "item_id").
		Where("estimate_id = ? AND status <> 'rejected'", estimateID).Order("id").Find(&rows).Error
	return rows, err
}
