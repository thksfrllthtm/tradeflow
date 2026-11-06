package trademarks

// Поиск знаков реестра в наименованиях товара. Сравниваются слова и группы до 4 слов подряд с
// наименованием знака (или надписью логотипа, которую прочитал ИИ) по двум ключам:
//   - «на вид»: кириллица-двойники → латиница (ТОYОТА → TOYOTA), путаемые символы (0→O, 1 и l→I, rn→m,
//     5→S, VV→W). Совпало только так — написание отличается от реестра: «похоже на знак»;
//   - транслитерация (АРАРАТ → ARARAT): тот же знак другими буквами — «знак из реестра».
// Отличие в одну букву — тоже «похоже»: для латинских знаков от 7 букв любое, иначе — только замена похожих
// букв (SONV → SONY, ADIBAS → ADIDAS): обычные слова (CLEAN → CLEAR) и русские окончания («Детские» →
// «ДЕТСКОЕ») давали бы ложные тревоги.
// Знаки до 3 букв — только точное совпадение.

import (
	"sort"
	"strings"
	"time"
	"unicode"
)

const (
	KindBrand     = "brand"
	KindLookalike = "lookalike"
	maxWindow     = 4
	maxHits       = 5
)

// Record — запись реестра для поиска (без картинки логотипа).
type Record struct {
	RegNo    string
	Name     string
	LogoName string
	// LogoPicture — что изображено на логотипе («поезд, рельсы»), по описанию ИИ.
	LogoPicture    string
	HasLogo        bool
	Holder         string
	Goods          string
	Codes          []string
	ProtectedUntil *time.Time
}

// Hit — знак реестра, найденный в наименованиях товара.
type Hit struct {
	RegNo string `json:"regNo"`
	// Name — наименование знака; для знака-изображения — надпись логотипа, прочитанная ИИ (FromLogo).
	Name           string `json:"name"`
	FromLogo       bool   `json:"fromLogo"`
	HasLogo        bool   `json:"hasLogo"`
	Holder         string `json:"holder"`
	Goods          string `json:"goods"`
	ProtectedUntil string `json:"protectedUntil,omitempty"`
	// Kind — brand: знак как в реестре; lookalike — похожее написание (возможная имитация).
	Kind string `json:"kind"`
	// Found — слова товара, в которых найден знак.
	Found string `json:"found"`
	// SameGoods — товар из той же группы, что в реестре: GoodsMatch code — код в той же товарной позиции
	// (4 знака), description — слово наименования есть в описании товаров записи («наушники»).
	SameGoods  bool   `json:"sameGoods"`
	GoodsMatch string `json:"goodsMatch,omitempty"`
	// OtherGoods — код товара известен, а в реестре знак для других товаров (и описание не совпало).
	OtherGoods bool `json:"otherGoods,omitempty"`
	// ByPicture — найдено сверкой фото товара с логотипом реестра (Why — что похоже), а не по надписи.
	ByPicture bool   `json:"byPicture,omitempty"`
	Why       string `json:"why,omitempty"`
}

type variant struct {
	record   int
	key      string
	raw      string
	translit bool
	fromLogo bool
}

// Index — записи реестра, подготовленные для поиска.
type Index struct {
	records  []Record
	variants []variant
	exact    map[string][]int
	deletes  map[string][]int
	// goods — основы слов описания товаров каждой записи (см. goodsStems).
	goods []map[string]bool
	// pictures — основы слов описания рисунка логотипа.
	pictures []map[string]bool
}

func NewIndex(records []Record) *Index {
	index := &Index{records: records, exact: map[string][]int{}, deletes: map[string][]int{}, goods: make([]map[string]bool, len(records)), pictures: make([]map[string]bool, len(records))}
	for number, record := range records {
		index.goods[number] = goodsStems(record.Goods)
		index.pictures[number] = pictureStems(record.LogoPicture)
		for _, source := range []struct {
			text string
			logo bool
		}{{record.Name, false}, {record.LogoName, true}} {
			for _, part := range nameVariants(source.text) {
				raw := upperAlnum(part)
				seen := map[string]bool{}
				for _, item := range []struct {
					key      string
					translit bool
				}{{visualKey(part), false}, {translitKey(part), true}} {
					if len([]rune(item.key)) < 2 || seen[item.key] {
						continue
					}
					seen[item.key] = true
					position := len(index.variants)
					index.variants = append(index.variants, variant{record: number, key: item.key, raw: raw, translit: item.translit, fromLogo: source.logo})
					index.exact[item.key] = append(index.exact[item.key], position)
					if !item.translit && len([]rune(item.key)) >= 4 {
						for _, deleted := range deletions(item.key) {
							index.deletes[deleted] = append(index.deletes[deleted], position)
						}
					}
				}
			}
		}
	}
	return index
}

// Match — знаки реестра в наименованиях товара (до 5): сначала той же товарной позиции, что code.
func (index *Index) Match(texts []string, code string) []Hit {
	code = digitsOnly(code)
	itemStems := goodsStems(strings.Join(texts, " "))
	best := map[int]Hit{}
	for _, text := range texts {
		words := splitWords(text)
		for start := range words {
			for size := 1; size <= maxWindow && start+size <= len(words); size++ {
				window := strings.Join(words[start:start+size], " ")
				for _, found := range index.matchWindow(window) {
					hit := index.hit(found, window, code, itemStems)
					if current, ok := best[found.record]; !ok || hitRank(hit) < hitRank(current) {
						best[found.record] = hit
					}
				}
			}
		}
	}
	hits := make([]Hit, 0, len(best))
	for _, hit := range best {
		hits = append(hits, hit)
	}
	return SortHits(hits)
}

// SortHits — сначала той же товарной позиции, затем знак как в реестре, затем длиннее совпадение; до 5.
func SortHits(hits []Hit) []Hit {
	sort.SliceStable(hits, func(i, j int) bool {
		if hitRank(hits[i]) != hitRank(hits[j]) {
			return hitRank(hits[i]) < hitRank(hits[j])
		}
		if len(hits[i].Found) != len(hits[j].Found) {
			return len(hits[i].Found) > len(hits[j].Found)
		}
		return hits[i].RegNo < hits[j].RegNo
	})
	if len(hits) > maxHits {
		hits = hits[:maxHits]
	}
	return hits
}

func hitRank(hit Hit) int {
	rank := 0
	if !hit.SameGoods {
		rank += 2
	}
	if hit.Kind != KindBrand {
		rank++
	}
	return rank
}

type found struct {
	record int
	kind   string
	logo   bool
}

func (index *Index) matchWindow(window string) []found {
	raw, visual, translit := upperAlnum(window), visualKey(window), translitKey(window)
	result := []found{}
	add := func(position int, kind string) {
		item := index.variants[position]
		result = append(result, found{record: item.record, kind: kind, logo: item.fromLogo})
	}
	for _, position := range index.exact[visual] {
		if item := index.variants[position]; !item.translit {
			kind := KindBrand
			if item.raw != raw {
				kind = KindLookalike
			}
			add(position, kind)
		}
	}
	for _, position := range index.exact[translit] {
		if item := index.variants[position]; item.translit {
			add(position, KindBrand)
		}
	}
	if len([]rune(visual)) < 3 {
		return result
	}
	// Одна буква: вставка (ключ = слово без буквы), пропуск (слово = ключ без буквы), замена.
	positions := append([]int{}, index.deletes[visual]...)
	for _, deleted := range deletions(visual) {
		positions = append(positions, index.exact[deleted]...)
		positions = append(positions, index.deletes[deleted]...)
	}
	seen := map[int]bool{}
	for _, position := range positions {
		item := index.variants[position]
		if seen[position] || item.translit || item.key == visual {
			continue
		}
		seen[position] = true
		if similarSpelling([]rune(visual), []rune(item.key), !isCyrillic(item.raw) && !isCyrillic(raw)) {
			add(position, KindLookalike)
		}
	}
	return result
}

func (index *Index) hit(found found, window, code string, itemStems map[string]bool) Hit {
	record := index.records[found.record]
	name, fromLogo := record.Name, false
	if found.logo || name == "" {
		name, fromLogo = record.LogoName, true
	}
	hit := Hit{
		RegNo: record.RegNo, Name: name, FromLogo: fromLogo, HasLogo: record.HasLogo, Holder: shortText(record.Holder, 160),
		Goods: shortText(record.Goods, 200), Kind: found.kind, Found: window,
	}
	if record.ProtectedUntil != nil {
		hit.ProtectedUntil = record.ProtectedUntil.Format("2006-01-02")
	}
	if len(code) >= 4 {
		for _, registered := range record.Codes {
			if len(registered) >= 4 && registered[:4] == code[:4] {
				hit.SameGoods, hit.GoodsMatch = true, "code"
				break
			}
		}
	}
	if !hit.SameGoods {
		for stem := range itemStems {
			if index.goods[found.record][stem] {
				hit.SameGoods, hit.GoodsMatch = true, "description"
				break
			}
		}
	}
	hit.OtherGoods = !hit.SameGoods && len(code) >= 4
	return hit
}

// genericStems — общие слова описаний товаров («изделия», «средства»): по ним группа товаров не совпадает.
var genericStems = map[string]bool{
	"издел": true, "товар": true, "средс": true, "прочи": true, "предм": true, "матер": true, "класс": true,
	"части": true, "детал": true, "прина": true, "издан": true, "проду": true, "всех": true,
	"други": true, "друго": true, "также": true, "котор": true, "испол": true, "видов": true,
}

// goodsStems — основы слов (первые 5 букв, слова от 4 букв, кроме общих) для сравнения наименования товара
// с описанием товаров записи реестра: «наушники беспроводные» и «Наушники; микрофоны» — общее «наушн».
func goodsStems(text string) map[string]bool {
	stems := map[string]bool{}
	for _, word := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) }) {
		runes := []rune(word)
		if len(runes) < 4 {
			continue
		}
		stem := string(runes[:min(5, len(runes))])
		if !genericStems[stem] {
			stems[stem] = true
		}
	}
	return stems
}

// pictureStop — слова описания рисунка, которые есть почти у любого логотипа (форма, цвет, «надпись»).
var pictureStop = map[string]bool{
	"логот": true, "надпи": true, "изобр": true, "знак": true, "букв": true, "буква": true, "текст": true,
	"круг": true, "круга": true, "кругл": true, "полос": true, "линия": true, "линии": true, "форма": true,
	"форме": true, "стили": true, "абстр": true, "эмбле": true, "симво": true, "рисун": true, "рамка": true,
	"фоне": true, "цвет": true, "цвета": true, "квадр": true, "прямо": true, "треуг": true, "овал": true,
}

// pictureStems — основы слов описания рисунка без общих слов.
func pictureStems(text string) map[string]bool {
	stems := goodsStems(text)
	for stem := range stems {
		if pictureStop[stem] {
			delete(stems, stem)
		}
	}
	return stems
}

// PictureCandidates — логотипы реестра для сверки с фото товара (до limit, лучшие первыми): с похожим
// рисунком (общее слово в описании рисунка: «поезд») и той же группы товаров (по коду или описанию).
// exclude — уже найденные по надписи.
func (index *Index) PictureCandidates(pictures, texts []string, code string, exclude map[string]bool, limit int) []Hit {
	code = digitsOnly(code)
	wanted, itemStems := pictureStems(strings.Join(pictures, " ")), goodsStems(strings.Join(texts, " "))
	type scored struct {
		hit   Hit
		score int
	}
	list := []scored{}
	for number, record := range index.records {
		if !record.HasLogo || exclude[record.RegNo] {
			continue
		}
		score := 0
		for stem := range wanted {
			if index.pictures[number][stem] {
				score += 2
			}
		}
		hit := index.hit(found{record: number, kind: KindLookalike}, "", code, itemStems)
		if hit.SameGoods {
			score++
		}
		if score > 0 {
			hit.ByPicture = true
			list = append(list, scored{hit, score})
		}
	}
	sort.SliceStable(list, func(i, j int) bool {
		if list[i].score != list[j].score {
			return list[i].score > list[j].score
		}
		return list[i].hit.RegNo < list[j].hit.RegNo
	})
	hits := []Hit{}
	for _, item := range list[:min(len(list), limit)] {
		hits = append(hits, item.hit)
	}
	return hits
}

func letterCount(text string) int {
	count := 0
	for _, r := range text {
		if unicode.IsLetter(r) {
			count++
		}
	}
	return count
}
