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
	"regexp"
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

// Size — сколько записей в индексе.
func (index *Index) Size() int { return len(index.records) }

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

// similarSpelling — ключи отличаются одной буквой (см. правила в начале файла). Любая одна буква — только
// у латиницы (latin): в русском одна буква — обычно другое слово или окончание («Детские» и «ДЕТСКОЕ»).
func similarSpelling(word, key []rune, latin bool) bool {
	if len(word) == len(key) {
		diff := -1
		for index := range word {
			if word[index] != key[index] {
				if diff >= 0 {
					return false
				}
				diff = index
			}
		}
		return diff >= 0 && ((latin && len(key) >= 7) || (len(key) >= 4 && confusable(word[diff], key[diff])))
	}
	if !latin || len(key) < 7 || len(word)-len(key) > 1 || len(key)-len(word) > 1 {
		return false
	}
	short, long := word, key
	if len(short) > len(long) {
		short, long = long, short
	}
	for index := range long {
		if string(long[:index])+string(long[index+1:]) == string(short) {
			return true
		}
	}
	return false
}

var confusablePairs = map[[2]rune]bool{}

func init() {
	for _, group := range []string{"OQD", "IJ", "UV", "VY", "EF", "CG", "MN", "BD", "SZ", "PR", "KX", "ИЙ", "ШЩ", "ЬЪБ"} {
		letters := []rune(group)
		for _, a := range letters {
			for _, b := range letters {
				if a != b {
					confusablePairs[[2]rune{a, b}] = true
				}
			}
		}
	}
}

func confusable(a, b rune) bool { return confusablePairs[[2]rune{a, b}] }

func deletions(key string) []string {
	runes := []rune(key)
	out := make([]string, 0, len(runes))
	for index := range runes {
		out = append(out, string(runes[:index])+string(runes[index+1:]))
	}
	return out
}

// nameVariants — наименование знака целиком и его части: «ARIEL ЭЙРИЛ» — ещё ARIEL и ЭЙРИЛ, «X / Y» и
// «X (Y)» — X и Y.
func nameVariants(name string) []string {
	parts := strings.FieldsFunc(name, func(r rune) bool { return strings.ContainsRune("/()[],;«»\"“”\n", r) })
	out := []string{}
	seen := map[string]bool{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if letterCount(value) > 0 && !seen[value] && len(splitWords(value)) <= maxWindow {
			seen[value] = true
			out = append(out, value)
		}
	}
	add(name)
	for _, part := range parts {
		add(part)
		// «BALLANTINE’S FINEST B», «ESSE EXCHANGE M»: буква через пробел в конце — пометка варианта знака,
		// в наименовании товара её обычно нет.
		if match := trailingLetter.FindStringSubmatch(strings.TrimSpace(part)); match != nil {
			add(match[1])
		}
		latin, cyrillic := []string{}, []string{}
		for _, word := range splitWords(part) {
			if isCyrillic(word) {
				cyrillic = append(cyrillic, word)
			} else {
				latin = append(latin, word)
			}
		}
		// Части другого алфавита — отдельный знак, только если в каждой от 3 букв («ВД-40» не даёт «40»).
		if letterCount(strings.Join(latin, "")) >= 3 && letterCount(strings.Join(cyrillic, "")) >= 3 {
			add(strings.Join(latin, " "))
			add(strings.Join(cyrillic, " "))
		}
	}
	return out
}

var trailingLetter = regexp.MustCompile(`^(.*\S)\s+\pL\.?$`)

func isCyrillic(word string) bool {
	cyrillic, latin := 0, 0
	for _, r := range word {
		switch {
		case unicode.Is(unicode.Cyrillic, r):
			cyrillic++
		case unicode.Is(unicode.Latin, r):
			latin++
		}
	}
	return cyrillic > latin
}

func splitWords(text string) []string {
	return strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '$' && r != '@' && r != '|'
	})
}

func upperAlnum(text string) string {
	var builder strings.Builder
	for _, r := range strings.ToUpper(text) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

var lookalikes = map[rune]rune{
	'А': 'A', 'В': 'B', 'Е': 'E', 'Ё': 'E', 'К': 'K', 'М': 'M', 'Н': 'H', 'О': 'O', 'Р': 'P', 'С': 'C', 'Т': 'T',
	'У': 'Y', 'Х': 'X', 'І': 'I', 'Ѕ': 'S', 'Ј': 'J', '0': 'O', '1': 'I', '|': 'I', 'L': 'I', '5': 'S', '8': 'B', '$': 'S', '@': 'A',
}

// visualKey — как слово выглядит: двойники и путаемые символы сведены к одной латинской букве (без учёта
// регистра: l и I в строчном и заглавном виде — одна буква).
func visualKey(text string) string {
	text = strings.ReplaceAll(strings.ToLower(text), "rn", "m")
	var builder strings.Builder
	for _, r := range strings.ToUpper(text) {
		if mapped, ok := lookalikes[r]; ok {
			r = mapped
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
		}
	}
	return strings.ReplaceAll(builder.String(), "VV", "W")
}

var translit = map[rune]string{
	'А': "A", 'Б': "B", 'В': "V", 'Г': "G", 'Д': "D", 'Е': "E", 'Ё': "E", 'Ж': "ZH", 'З': "Z", 'И': "I", 'Й': "Y",
	'К': "K", 'Л': "L", 'М': "M", 'Н': "N", 'О': "O", 'П': "P", 'Р': "R", 'С': "S", 'Т': "T", 'У': "U", 'Ф': "F",
	'Х': "H", 'Ц': "TS", 'Ч': "CH", 'Ш': "SH", 'Щ': "SCH", 'Ъ': "", 'Ы': "Y", 'Ь': "", 'Э': "E", 'Ю': "YU", 'Я': "YA",
	'Ә': "A", 'Ғ': "G", 'Қ': "K", 'Ң': "N", 'Ө': "O", 'Ұ': "U", 'Ү': "U", 'Һ': "H", 'І': "I",
}

// translitKey — латиницей: кириллица транслитерацией, латиница как есть.
func translitKey(text string) string {
	var builder strings.Builder
	for _, r := range strings.ToUpper(text) {
		if latin, ok := translit[r]; ok {
			builder.WriteString(latin)
		} else if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func digitsOnly(value string) string {
	var builder strings.Builder
	for _, r := range value {
		if r >= '0' && r <= '9' {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

func shortText(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "…"
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
