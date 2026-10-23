package decisions

// Похожие товары по фото: мягкий отпечаток (тот же снимок с обрезкой, рамкой, надписью) и
// описание фото от ИИ (вид товара, материал, форма, цвет, особенности) — по нему находится тот же
// товар на другом снимке. Точное совпадение (FindSame) остаётся отдельным и главным.

import (
	"context"
	"encoding/json"
	"log"
	"math"
	"sort"
	"strings"
	"unicode"
)

const (
	// Отличие отпечатков, при котором снимок считается похожим (до samePhotoMaxDistance — тот же).
	similarPhotoMaxDistance = 14
	// Ниже этой похожести (0–100) запись не предлагается.
	SimilarMinScore   = 50
	similarMaxResults = 5
)

// PhotoFeatures — описание фото товара от ИИ. Synonyms, Purpose и Keywords — для поиска: как ещё
// называют товар (в том числе по-английски, как в инвойсах), для чего он и что набрал бы сотрудник.
type PhotoFeatures struct {
	Kind     string   `json:"kind"`
	Synonyms []string `json:"synonyms,omitempty"`
	Purpose  string   `json:"purpose,omitempty"`
	Material []string `json:"material"`
	Shape    []string `json:"shape"`
	Color    []string `json:"color"`
	Features []string `json:"features"`
	Keywords []string `json:"keywords,omitempty"`
}

// SearchText — текст для эмбеддинга: всё, что определяет товар по смыслу; без цвета (у одного
// товара бывают разные цвета, а разные товары — один цвет).
func (f PhotoFeatures) SearchText() string {
	parts := []string{}
	for _, value := range [][]string{{f.Kind}, f.Synonyms, {f.Purpose}, f.Material, f.Shape, f.Features, f.Keywords} {
		for _, item := range value {
			if item = strings.TrimSpace(item); item != "" {
				parts = append(parts, item)
			}
		}
	}
	return strings.Join(parts, ", ")
}

// Summary — описание одной строкой: «перчатки (work gloves) · защита рук · трикотаж, полиуретан · серый».
func (f PhotoFeatures) Summary() string {
	parts := []string{}
	add := func(values ...string) {
		clean := []string{}
		for _, value := range values {
			if value = strings.TrimSpace(value); value != "" {
				clean = append(clean, value)
			}
		}
		if len(clean) > 0 {
			parts = append(parts, strings.Join(clean, ", "))
		}
	}
	kind := strings.TrimSpace(f.Kind)
	if synonyms := strings.Join(trimAll(f.Synonyms), ", "); kind != "" && synonyms != "" {
		kind += " (" + synonyms + ")"
	}
	add(kind)
	add(f.Purpose)
	add(f.Material...)
	add(f.Shape...)
	add(f.Features...)
	add(f.Color...)
	return strings.Join(parts, " · ")
}

func trimAll(values []string) []string {
	clean := []string{}
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			clean = append(clean, value)
		}
	}
	return clean
}

// Empty — описания нет (ИИ не понял, что на фото).
func (f PhotoFeatures) Empty() bool {
	return strings.TrimSpace(f.Kind) == "" && len(f.Material) == 0 && len(f.Features) == 0
}

// ParsePhotoFeatures читает описание из базы; false — описания нет.
func ParsePhotoFeatures(raw []byte) (PhotoFeatures, bool) {
	var features PhotoFeatures
	if len(raw) == 0 || string(raw) == "null" || json.Unmarshal(raw, &features) != nil || features.Empty() {
		return PhotoFeatures{}, false
	}
	return features, true
}

// Вес признака: вид товара решает больше всего, цвет — меньше всего (у одного товара бывают цвета).
var featureWeights = struct{ kind, material, shape, features, color float64 }{4, 2, 1.5, 1.5, 0.5}

// Служебные и общие слова описаний не сравниваются: «набор кружек» и «набор инструментов» — не
// один вид товара.
var featureStopWords = map[string]bool{
	"для": true, "без": true, "при": true, "или": true, "как": true, "под": true, "над": true,
	"набор": true, "комплект": true, "штук": true, "штуки": true, "изделие": true, "товар": true, "предмет": true,
}

// Похожесть товаров одного вида: не ниже порога показа, остальное — по совпадению признаков.
const sameKindBase = 0.45

type featureWord struct {
	word   string
	weight float64
	kind   bool
}

// featureWords — слова описания с весом поля (слово из нескольких полей — с большим весом).
func featureWords(f PhotoFeatures) []featureWord {
	byWord := map[string]*featureWord{}
	order := []string{}
	add := func(weight float64, isKind bool, values ...string) {
		for _, value := range values {
			for _, word := range strings.FieldsFunc(strings.ReplaceAll(strings.ToLower(value), "ё", "е"), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
				if len([]rune(word)) < 3 || featureStopWords[word] {
					continue
				}
				entry, ok := byWord[word]
				if !ok {
					entry = &featureWord{word: word}
					byWord[word] = entry
					order = append(order, word)
				}
				entry.weight = math.Max(entry.weight, weight)
				entry.kind = entry.kind || isKind
			}
		}
	}
	// Синонимы — тот же вид товара другими словами: «кельма» в одном описании и «мастерок» в другом.
	add(featureWeights.kind, true, f.Kind)
	add(featureWeights.kind, true, f.Synonyms...)
	add(featureWeights.material, false, f.Material...)
	add(featureWeights.shape, false, f.Shape...)
	add(featureWeights.features, false, f.Features...)
	add(featureWeights.features, false, f.Purpose)
	add(featureWeights.features, false, f.Keywords...)
	add(featureWeights.color, false, f.Color...)
	words := make([]featureWord, 0, len(order))
	for _, word := range order {
		words = append(words, *byWord[word])
	}
	return words
}

// sameWordRoot — одно слово в разных формах: общее начало не короче 5 букв (у коротких слов —
// всё слово, но не меньше 4): «ладони» и «ладоней», «полиуретан» и «полиуретановое».
func sameWordRoot(a, b string) bool {
	ra, rb := []rune(a), []rune(b)
	common := 0
	for common < len(ra) && common < len(rb) && ra[common] == rb[common] {
		common++
	}
	need := min(5, len(ra), len(rb))
	return common >= need && common >= 4 || a == b
}

func matched(word featureWord, others []featureWord) bool {
	for _, other := range others {
		if sameWordRoot(word.word, other.word) {
			return true
		}
	}
	return false
}

// sharedWeight — доля совпавших слов обоих наборов с весами полей (0…1).
func sharedWeight(wordsA, wordsB []featureWord) float64 {
	var total, common float64
	for _, pair := range [2][2][]featureWord{{wordsA, wordsB}, {wordsB, wordsA}} {
		for _, word := range pair[0] {
			total += word.weight
			if matched(word, pair[1]) {
				common += word.weight
			}
		}
	}
	if total == 0 {
		return 0
	}
	return common / total
}

// FeatureSimilarity — похожесть описаний 0…1. Вид товара решает главное: товары одного вида
// («кружка» и «кружки») похожи не меньше чем на sameKindBase, выше — по совпадению материала,
// формы, особенностей и цвета; подробность одного из описаний мало что меняет. Если вид не совпал,
// похожесть — половина общей доли слов (миска и тарелка из одного материала — не одно и то же).
func FeatureSimilarity(a, b PhotoFeatures) float64 {
	wordsA, wordsB := featureWords(a), featureWords(b)
	if len(wordsA) == 0 || len(wordsB) == 0 {
		return 0
	}
	kindA, kindB := []featureWord{}, []featureWord{}
	restA, restB := []featureWord{}, []featureWord{}
	for _, word := range wordsA {
		if word.kind {
			kindA = append(kindA, word)
		} else {
			restA = append(restA, word)
		}
	}
	for _, word := range wordsB {
		if word.kind {
			kindB = append(kindB, word)
		} else {
			restB = append(restB, word)
		}
	}
	if len(kindA) == 0 || len(kindB) == 0 {
		return sharedWeight(wordsA, wordsB)
	}
	for _, word := range kindA {
		if matched(word, kindB) {
			return sameKindBase + (1-sameKindBase)*sharedWeight(restA, restB)
		}
	}
	return sharedWeight(wordsA, wordsB) / 2
}

// photoSimilarity — похожесть снимков по отпечатку 0…1 (0 — не похожи).
func photoSimilarity(hashes, other []string) (float64, bool) {
	best := -1
	for _, left := range hashes {
		for _, right := range other {
			if distance := photoHashDistance(left, right); distance >= 0 && (best < 0 || distance < best) {
				best = distance
			}
		}
	}
	if best < 0 || best > similarPhotoMaxDistance {
		return 0, false
	}
	return 1 - float64(best)/32, true
}

// SimilarMatch — похожий товар из базы решений: похожесть 0–100 и почему.
type SimilarMatch struct {
	Decision      CodeDecisionView `json:"decision"`
	Score         int              `json:"score"`
	ByPhoto       bool             `json:"byPhoto"`
	ByDescription bool             `json:"byDescription"`
	Description   string           `json:"description"`
	// Визуальная сверка ИИ: same — тот же товар, similar — похожий, different — другой; "" — не сверялось.
	Verdict string `json:"verdict"`
	Why     string `json:"why"`
}

// Embedder — вектор текста для поиска по смыслу (модель эмбеддингов); nil — поиск только по словам.
type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

// Сколько ближайших по вектору записей смотреть.
const nearestLimit = 20

// VectorScore — похожесть по смыслу 0…1 из косинусной близости эмбеддингов Gemini. Замер 2026-10-23
// (gemini-embedding-001, 768): один вид товара — 0.81–0.85, разные товары — 0.55–0.68 (чайник и
// кружка — 0.68). Порог показа SimilarMinScore (0.5) соответствует близости 0.745.
func VectorScore(cosine float64) float64 {
	return math.Max(0, math.Min(1, (cosine-0.62)/0.25))
}

// nearestByMeaning — записи, близкие по смыслу описания: id → похожесть 0…1. Без модели
// эмбеддингов, без описания или при ошибке ИИ — пусто (поиск идёт по словам, как раньше).
func (s *CodeDecisionService) nearestByMeaning(ctx context.Context, text string) map[int64]float64 {
	scores := map[int64]float64{}
	if s.embedder == nil || strings.TrimSpace(text) == "" {
		return scores
	}
	vector, err := s.embedder.Embed(ctx, text)
	if err != nil {
		log.Printf("decisions: embedding failed: %v", err)
		return scores
	}
	neighbors, err := s.repository.NearestDecisions(ctx, vector, nearestLimit)
	if err != nil {
		log.Printf("decisions: nearest by vector failed: %v", err)
		return scores
	}
	for _, neighbor := range neighbors {
		if score := VectorScore(neighbor.Similarity); score*100 >= SimilarMinScore {
			scores[neighbor.ID] = score
		}
	}
	return scores
}

// FindSimilar — похожие товары по фото: мягкий отпечаток, описание фото (по словам и по смыслу через
// эмбеддинги). Записи из skip (уже найденные точно) не повторяются. Лучшие — первыми, проверенные
// выше непроверенных при равенстве.
func (s *CodeDecisionService) FindSimilar(ctx context.Context, photoHashes []string, features *PhotoFeatures, skip map[int64]bool) ([]SimilarMatch, error) {
	keys, err := s.repository.CodeDecisionKeys(ctx)
	if err != nil {
		return nil, err
	}
	byMeaning := map[int64]float64{}
	if features != nil {
		byMeaning = s.nearestByMeaning(ctx, features.SearchText())
	}
	type hit struct {
		score         float64
		byPhoto, byFx bool
		description   string
	}
	hits := map[int64]hit{}
	for _, key := range keys {
		if skip[key.ID] {
			continue
		}
		var hashes []string
		_ = json.Unmarshal(key.PhotoHashes, &hashes)
		photoScore, byPhoto := photoSimilarity(photoHashes, hashes)
		var fxScore float64
		description := ""
		if stored, ok := ParsePhotoFeatures(key.PhotoFeatures); ok {
			description = stored.Summary()
			if features != nil {
				fxScore = FeatureSimilarity(*features, stored)
			}
		}
		// По смыслу надёжнее, чем по словам: синонимы и другой язык не мешают.
		fxScore = math.Max(fxScore, byMeaning[key.ID])
		score := math.Max(photoScore, fxScore)
		// Совпали и снимок, и описание — увереннее.
		if photoScore > 0 && fxScore > 0 {
			score = math.Min(1, score+0.1*math.Min(photoScore, fxScore))
		}
		if score*100 >= SimilarMinScore {
			hits[key.ID] = hit{score: score, byPhoto: byPhoto, byFx: fxScore*100 >= SimilarMinScore, description: description}
		}
	}
	ids := make([]int64, 0, len(hits))
	for id := range hits {
		ids = append(ids, id)
	}
	rows, err := s.repository.CodeDecisionsByIDs(ctx, ids)
	if err != nil {
		return nil, err
	}
	views, err := s.views(ctx, rows)
	if err != nil {
		return nil, err
	}
	matches := make([]SimilarMatch, 0, len(views))
	for _, view := range views {
		h := hits[view.ID]
		matches = append(matches, SimilarMatch{Decision: view, Score: int(math.Round(h.score * 100)), ByPhoto: h.byPhoto, ByDescription: h.byFx, Description: h.description})
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Score != matches[j].Score {
			return matches[i].Score > matches[j].Score
		}
		return matches[i].Decision.Status == "verified" && matches[j].Decision.Status != "verified"
	})
	if len(matches) > similarMaxResults {
		matches = matches[:similarMaxResults]
	}
	return matches, nil
}
