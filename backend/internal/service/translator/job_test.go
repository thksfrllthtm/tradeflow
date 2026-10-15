package translator

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/ai"
)

// fakeStore — база для задачи перевода в памяти процесса. Сохранение повторяет условие SQL:
// заблокированный или правленный сегмент не перезаписывается.
type fakeStore struct {
	mu       sync.Mutex
	segments []repository.TranslatorSegment
	cache    map[string]string
}

func newFakeStore(sources ...string) *fakeStore {
	store := &fakeStore{cache: map[string]string{}}
	for i, source := range sources {
		store.segments = append(store.segments, repository.TranslatorSegment{ID: int64(i + 1), DocumentID: 1, Idx: i, SourceText: source, Status: "machine"})
	}
	return store
}

func (s *fakeStore) TranslatorSegments(context.Context, int64) ([]repository.TranslatorSegment, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]repository.TranslatorSegment(nil), s.segments...), nil
}

func (s *fakeStore) TranslatorCache(_ context.Context, hash string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if text, ok := s.cache[hash]; ok {
		return text, nil
	}
	return "", repository.ErrNotFound
}

func (s *fakeStore) SaveTranslatorCache(_ context.Context, hash, translation string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cache[hash] = translation
	return nil
}

func (s *fakeStore) SaveMachineTranslation(_ context.Context, segment repository.TranslatorSegment) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	current := &s.segments[segment.ID-1]
	if current.Locked || current.Status != "machine" {
		return nil
	}
	current.MTText, current.QA = segment.MTText, segment.QA
	return nil
}

// scriptedModel отвечает по сценарию: перевод — функция от сегмента, ошибки — по тексту.
type scriptedModel struct {
	mu      sync.Mutex
	calls   []string
	answer  func(segment string, attempt int) string
	failing map[string]error
}

func (m *scriptedModel) Configured() bool { return true }
func (m *scriptedModel) Name() string     { return "scripted" }

func (m *scriptedModel) Translate(_ context.Context, _, user string) (string, error) {
	segment := user[strings.LastIndex(user, "Переведи сегмент:\n")+len("Переведи сегмент:\n"):]
	m.mu.Lock()
	m.calls = append(m.calls, segment)
	attempt := 0
	for _, call := range m.calls {
		if call == segment {
			attempt++
		}
	}
	m.mu.Unlock()
	if err := m.failing[segment]; err != nil {
		return "", err
	}
	if m.answer != nil {
		return m.answer(segment, attempt), nil
	}
	return testWords[segment], nil
}

// testWords — перевод сценарной модели по умолчанию.
var testWords = map[string]string{"一": "один", "二": "два", "三": "три", "四": "четыре", "合同": "Договор", "发票": "Счёт"}

var testDocument = repository.TranslatorDocumentSummary{ID: 1, SourceLang: "zh", TargetLang: "ru"}

func TestJobSkipsLockedAndTranslatedSegments(t *testing.T) {
	store := newFakeStore("一", "二", "三", "四")
	store.segments[1].Locked, store.segments[1].Status, store.segments[1].TargetText = true, "edited", "правка"
	store.segments[2].Locked, store.segments[2].Status = true, "confirmed"
	store.segments[3].MTText = "уже переведён"
	model := &scriptedModel{}
	failed, err := runJob(context.Background(), store, model, testDocument)
	if failed != 0 || err != nil {
		t.Fatalf("failed %d, err %v", failed, err)
	}
	if len(model.calls) != 1 || model.calls[0] != "一" {
		t.Fatalf("model calls %v, want only the pending segment", model.calls)
	}
	got := []string{store.segments[0].MTText, store.segments[1].MTText, store.segments[2].MTText, store.segments[3].MTText}
	if strings.Join(got, "|") != "один|||уже переведён" || store.segments[1].TargetText != "правка" {
		t.Fatalf("segments %v, target %q", got, store.segments[1].TargetText)
	}
}

// Сегмент, заблокированный правкой во время перевода, машинный перевод не перезаписывает.
func TestJobDoesNotOverwriteSegmentLockedDuringTranslation(t *testing.T) {
	store := newFakeStore("一")
	model := &scriptedModel{answer: func(string, int) string {
		store.mu.Lock()
		store.segments[0].Locked, store.segments[0].Status, store.segments[0].TargetText = true, "edited", "правка сотрудника"
		store.mu.Unlock()
		return "машинный"
	}}
	if _, err := runJob(context.Background(), store, model, testDocument); err != nil {
		t.Fatal(err)
	}
	if store.segments[0].MTText != "" || store.segments[0].TargetText != "правка сотрудника" {
		t.Fatalf("segment %#v", store.segments[0])
	}
}

func TestJobTakesCacheWithoutModel(t *testing.T) {
	store := newFakeStore("合同", "发票")
	model := &scriptedModel{}
	if _, err := runJob(context.Background(), store, model, testDocument); err != nil {
		t.Fatal(err)
	}
	if len(model.calls) != 2 || store.segments[0].MTText != "Договор" || store.segments[1].MTText != "Счёт" {
		t.Fatalf("calls %v, segments %#v", model.calls, store.segments)
	}

	// Тот же текст в новом документе берётся из кэша — модель больше не вызывается.
	again := newFakeStore("发票", "合同")
	again.cache = store.cache
	if _, err := runJob(context.Background(), again, model, testDocument); err != nil {
		t.Fatal(err)
	}
	if len(model.calls) != 2 || again.segments[0].MTText != "Счёт" || again.segments[1].MTText != "Договор" {
		t.Fatalf("calls %v, segments %#v", model.calls, again.segments)
	}
}

func TestJobRetriesOnceWhenTagsBreak(t *testing.T) {
	store := newFakeStore("价格<g1>100 USD</g1>", "数量<x1/>件")
	model := &scriptedModel{answer: func(segment string, attempt int) string {
		if strings.HasPrefix(segment, "价格") {
			if attempt == 1 {
				return "Цена 100 USD" // тег потерян — повтор
			}
			return "Цена <g1>100 USD</g1>"
		}
		return "Количество шт." // тег потерян оба раза
	}}
	if _, err := runJob(context.Background(), store, model, testDocument); err != nil {
		t.Fatal(err)
	}
	if len(model.calls) != 4 {
		t.Fatalf("calls %v", model.calls)
	}
	fixed, broken := store.segments[0], store.segments[1]
	if fixed.MTText != "Цена <g1>100 USD</g1>" || len(fixed.QA) != 0 {
		t.Fatalf("fixed %#v", fixed)
	}
	var warnings []qaWarning
	json.Unmarshal(broken.QA, &warnings)
	if broken.MTText != "Количество шт." || len(warnings) != 1 || warnings[0].Code != "tags" {
		t.Fatalf("broken %#v", broken)
	}
	if _, cached := store.cache[hashText("scripted", "zh", "ru", "数量<x1/>件")]; cached {
		t.Fatal("translation with broken tags must not be cached")
	}
}

func TestJobCountsFailedSegmentsAndStopsOnFatalError(t *testing.T) {
	store := newFakeStore("一", "二", "三")
	model := &scriptedModel{failing: map[string]error{"二": ai.ErrAIUnavailable}}
	failed, err := runJob(context.Background(), store, model, testDocument)
	if failed != 1 || err != ai.ErrAIUnavailable || store.segments[0].MTText == "" || store.segments[1].MTText != "" || store.segments[2].MTText == "" {
		t.Fatalf("failed %d, err %v, segments %#v", failed, err, store.segments)
	}

	store = newFakeStore("一", "二")
	model = &scriptedModel{failing: map[string]error{"一": ai.ErrAIUnauthorized, "二": ai.ErrAIUnauthorized}}
	if _, err := runJob(context.Background(), store, model, testDocument); err != ai.ErrAIUnauthorized {
		t.Fatalf("expected fatal error, got %v", err)
	}
}

func TestTagHelpers(t *testing.T) {
	for _, test := range []struct {
		source, translation string
		match               bool
	}{
		{"a <g1>b</g1><x2/>", "<x2/>А <g1>Б</g1>", true},
		{"a <g1>b</g1>", "А Б", false},
		{"a <g1>b</g1>", "<g1>А</g1> <g1>Б</g1>", false},
		{"a <g1>b</g1>", "А </g1>Б<g1>", false},
		{"a", "А <x1/>", false},
		{"a", "А", true},
	} {
		if got := tagsMatch(test.source, test.translation); got != test.match {
			t.Errorf("tagsMatch(%q, %q) = %v", test.source, test.translation, got)
		}
	}
	if got := normalizeTagged("A & B <g1>x < y &amp; z</g1> &lt;ok&gt;"); got != "A &amp; B <g1>x &lt; y &amp; z</g1> &lt;ok&gt;" {
		t.Errorf("normalizeTagged: %q", got)
	}
}

// Модель придумала тег там, где его нет, или оставила текст на китайском: один повтор, потом предупреждение.
func TestJobRetriesInventedTagsAndUntranslatedText(t *testing.T) {
	store := newFakeStore("中文版 材料", "回抛轮具及程序确认。")
	model := &scriptedModel{answer: func(segment string, attempt int) string {
		if strings.HasPrefix(segment, "中文版") {
			if attempt == 1 {
				return "<gN>中文版</gN> Материал"
			}
			return "Китайская версия, материал"
		}
		return segment // не переведено оба раза
	}}
	if _, err := runJob(context.Background(), store, model, testDocument); err != nil {
		t.Fatal(err)
	}
	if len(model.calls) != 4 {
		t.Fatalf("calls %v", model.calls)
	}
	if store.segments[0].MTText != "Китайская версия, материал" || len(store.segments[0].QA) != 0 {
		t.Fatalf("fixed %#v", store.segments[0])
	}
	var warnings []qaWarning
	json.Unmarshal(store.segments[1].QA, &warnings)
	if len(warnings) != 1 || warnings[0].Code != "untranslated" {
		t.Fatalf("warnings %v", warnings)
	}
	if len(store.cache) != 1 {
		t.Fatalf("only the good translation is cached, got %d", len(store.cache))
	}
}

func TestTranslationProblems(t *testing.T) {
	for _, test := range []struct {
		source, translation, from, to string
		codes                         []string
	}{
		{"中文版", "&lt;gN&gt;Версия&lt;/gN&gt;", "zh", "ru", []string{"tags"}},
		{"规格", "Спецификация规格", "zh", "ru", []string{"untranslated"}},
		{"<g1>规格</g1>", "<g1>Спецификация</g1>", "zh", "ru", nil},
		{"Договор", "Договор", "ru", "kk", nil}, // одна письменность — не проверяется
		{"Договор поставки", "Supply agreement", "ru", "en", nil},
		{"Договор", "Contract (Договор)", "ru", "en", []string{"untranslated"}},
		{"Contract", "Contract", "en", "ru", nil}, // латиница в переводе допустима
	} {
		var codes []string
		for _, problem := range translationProblems(test.source, test.translation, test.from, test.to) {
			codes = append(codes, problem.Code)
		}
		if strings.Join(codes, ",") != strings.Join(test.codes, ",") {
			t.Errorf("%q → %q: got %v, want %v", test.source, test.translation, codes, test.codes)
		}
	}
}

// Без проблем — пустой список: в базе qa = [], а не null (страница читает qa.length).
func TestTranslationProblemsMarshalAsEmptyList(t *testing.T) {
	data, _ := json.Marshal(translationProblems("Договор", "Contract", "ru", "en"))
	if string(data) != "[]" {
		t.Fatalf("got %s", data)
	}
}
