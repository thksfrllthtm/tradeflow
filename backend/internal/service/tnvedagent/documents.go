package tnvedagent

// «Документы для ввоза» (кнопка на карточке кода, команда «/документы»). Один агент на всё делал 6–11 ходов
// подряд и 2,5–4 минуты: сам искал код в перечнях, читал Правила условного выпуска, повторял итог. Теперь
// программа открывает справку tnved.info, находит позиции перечней общей базы по коду и считает условный
// выпуск, а каждый акт справки проверяет свой под-агент — параллельно, с короткой перепиской и итогом только
// по своему акту. Перед запуском под-агента программа сама читает его акт и отдаёт ему позиции перечня с
// кодом и фрагменты по коду, словам товара и примечаниям (readAct): под-агент начинает с готовых находок,
// а не с десятка поисков по документу, но проверить акт сам всё равно обязан. Итог собирает программа.
// Вопросов сотруднику под-агенты не задают: неизвестный признак — «возможно» и что уточнить в requirement.

import (
	"context"
	"errors"
	"fmt"
	"log"
	"regexp"
	"strings"
	"sync"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/ai"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/directory"
)

const (
	// Под-агентов одновременно; ходов у каждого (акт читается сам, не только справка); заданий (лишние акты
	// склеиваются в одно).
	documentsParallel = 6
	documentsActSteps = 8
	documentsMaxActs  = 10
	// documentsPhotoRunes — описание фото в сообщении под-агенту.
	documentsPhotoRunes = 1500
	// documentsPreviewFragments — фрагментов акта, которые программа находит под-агенту заранее;
	// documentsPreviewWords — слов товара и наименования кода в поиске по акту.
	documentsPreviewFragments = 12
	documentsPreviewWords     = 16
	// helpRestrictions — группа справки с мерами нетарифного регулирования: по акту на под-агента. Остальные
	// группы (льготы по НДС, примечания, прочие сведения, ТРОИС) — не документы на ввоз: не проверяются,
	// ТРОИС смета проверяет сама.
	helpRestrictions = "Меры нетарифного регулирования (ограничения)"
	// conditionalReleaseQuote — п. 4 Правил РК: основание записи «условный выпуск» (проверяется по странице Правил).
	conditionalReleaseQuote = "Подтверждение соответствия требованиям технических регламентов, соблюдения запретов и ограничений в части технического регулирования осуществляется в течение 60 (шестидесяти) календарных дней с даты помещения товаров под таможенную процедуру выпуска для внутреннего потребления"
)

var (
	helpActLine       = regexp.MustCompile(`^\d+\. `)
	helpActNumber     = regexp.MustCompile(`№\s*(\d+)`)
	helpActRegulation = regexp.MustCompile(`ТР\s+(?:ТС|ЕАЭС)\s+\d{3}/\d{4}`)
	// documentsNotesPhrases — обороты примечаний и исключений перечня, которыми программа ищет в акте
	// заранее. Обороты, а не основы слов: «исключен решением…» — поправка, а не исключение.
	documentsNotesPhrases = []string{"не распространяется", "за исключением", "не подлеж", "область применения", "примечани"}
)

// documentsTools — инструменты под-агента: итог по мерам акта, страница по ссылке и поиск в документе.
func documentsTools() []agentTool {
	return []agentTool{
		tool("finish", "Итог по документам для ввоза по этому акту. Вызывается один раз в конце.", objectSchema(map[string]any{
			"description": stringProperty("Короткое резюме сотруднику, 1–2 предложения: какие документы и разрешения по этому акту нужны для ввоза этого товара"),
			"documents": map[string]any{"type": "array", "description": "По одной записи на меру этого акта (и из прочитанных по ссылкам документов), включая неподходящие (applies no)", "items": objectSchema(map[string]any{
				"kind":        map[string]any{"type": "string", "enum": []string{"cc", "dc", "sgr", "other"}, "description": "Вид документа: cc — сертификат соответствия (СС), dc — декларация соответствия (ДС), sgr — свидетельство о государственной регистрации (СГР), other — прочие меры (санэпиднадзор без СГР, лицензия, квота, ветеринарный контроль…)"},
				"measure":     stringProperty("Вид меры полностью, с большой буквы, без сокращений: для техрегламента — номер и название в кавычках («Сертификат соответствия ТР ТС 010/2011 «О безопасности машин и оборудования»», «Декларация соответствия ТР ТС 019/2011 «О безопасности средств индивидуальной защиты»»); для санэпиднадзора — раздел Решения КТС №299 и нужно ли СГР («Санитарно-эпидемиологический надзор (раздел I, без СГР)»); прочие — «Ветеринарный контроль», «Лицензия», «Тарифная квота», «Экспортный контроль»"),
				"act":         stringProperty("Акт, например «Решение КТС №299 от 28.05.2010»"),
				"url":         stringProperty("Ссылка на акт или документ — только адрес с открытых страниц; пусто, если нет"),
				"requirement": stringProperty("Что конкретно нужно для этого товара, 1–2 предложения; для maybe — что уточнить"),
				"applies":     map[string]any{"type": "string", "enum": []string{"yes", "maybe", "no"}, "description": "Относится ли мера к этому товару по пояснению под актом. Зависит от назначения товара (контакт с пищей, для детей, бытовое) — всегда yes и nameHint, даже если в наименовании назначение другое; решает декларант"},
				"basis":       stringProperty("Дословная цитата (5–40 слов) из справки tnved.info, акта или страницы кода, на которой основан вывод applies: пояснение под мерой («Относится к товарам: …») или пункт перечня. Программа проверит цитату; без неё «нужно» не принимается"),
				"nameHint":    stringProperty("Если мера зависит от признака товара (бытовое или промышленное назначение, контакт с пищей, напряжение, детский товар): при каком реальном признаке она не применяется, какое наименование тогда указать в декларации и какими документами поставщика это подтвердить. Пусто, если от наименования не зависит"),
			}, "kind", "measure", "act", "requirement", "applies", "basis")},
			"notes": stringProperty("Короткое дополнение; пусто, если нечего добавить"),
		}, "description", "documents")),
		tool("open_link", "Открыть ссылку акта или ссылку с уже открытой страницы: начало текста страницы и ссылки с неё. Большой документ целиком не отдаётся — ищи в нём через search_in_document.",
			objectSchema(map[string]any{"url": stringProperty("Полный адрес https://…")}, "url")),
		tool("search_in_document", "Найти в документе по ссылке фрагменты со словами запроса (с соседними абзацами). Для больших актов (перечни, единые требования) вместо чтения целиком.",
			objectSchema(map[string]any{
				"url":   stringProperty("Адрес документа — ссылка акта или с открытой страницы"),
				"query": stringProperty("2–5 слов: вид товара, материал, код или группа ТН ВЭД, номер раздела"),
			}, "url", "query")),
	}
}

// helpAct — акт из справки tnved.info: название, ссылка, строки мер и пояснений (с отступом), группа справки.
type helpAct struct {
	Title, URL, Body, Group string
}

// splitHelpActs — акты из текста справки (формат directory.parseGoodsHelp): заголовок группы — строка без
// отступа с «:» на конце, акт — «N. Название — url» (нумерация в каждой группе заново), строки без отступа
// после акта — продолжение названия, с отступом — меры и пояснения. Блок «Условный выпуск (РК)», который
// дописывает goodsHelp, актом не считается.
func splitHelpActs(text string) []helpAct {
	acts := []helpAct{}
	group, open := "", false
	var title, body []string
	flush := func() {
		if !open {
			return
		}
		open = false
		act := helpAct{Group: group, Body: strings.TrimSpace(strings.Join(body, "\n"))}
		name := strings.Join(title, " ")
		if index := strings.LastIndex(name, " — https://"); index >= 0 {
			if address, ok := directory.CanonicalWebURL(strings.TrimSpace(name[index+len(" — "):])); ok {
				act.URL, name = address, name[:index]
			}
		}
		act.Title = strings.TrimSpace(helpActLine.ReplaceAllString(name, ""))
		acts = append(acts, act)
	}
	for _, line := range strings.Split(text, "\n") {
		switch {
		case strings.TrimSpace(line) == "":
		case strings.HasPrefix(line, " "):
			if open {
				body = append(body, strings.TrimSpace(line))
			}
		case strings.HasSuffix(line, ":"):
			flush()
			group = strings.TrimSuffix(line, ":")
		case helpActLine.MatchString(line) && group != "" && !strings.HasPrefix(group, "Условный выпуск"):
			flush()
			open, title, body = true, []string{line}, nil
		case open:
			title = append(title, strings.TrimSpace(line))
		}
	}
	flush()
	return acts
}

// documentsTask — задание под-агенту: акт справки с мерами или перечень общей базы, где встретился код.
type documentsTask struct {
	label, title, url, body string
	// positions — позиции перечня этого акта с кодом (нашла программа); general — акта в справке нет.
	positions []actMatch
	general   bool
}

// documentsTaskLabel — короткая подпись акта для шагов чата: техрегламент, номер решения или начало названия.
func documentsTaskLabel(text string) string {
	if regulation := helpActRegulation.FindString(text); regulation != "" {
		return strings.Join(strings.Fields(regulation), " ")
	}
	if number := helpActNumber.FindStringSubmatch(text); number != nil {
		return "№" + number[1]
	}
	return service.TruncateRunes(strings.TrimSpace(text), 40)
}

// actMatchesHelp — позиция общей базы относится к акту справки: та же ссылка, иначе номер решения (и номер
// техрегламента, если он есть в названии перечня) встречается в названии или мерах акта; №299 — по номеру.
func actMatchesHelp(match actMatch, act helpAct) bool {
	if match.act.URL == act.URL {
		return true
	}
	text := act.Title + " " + act.Body
	if match.act.URL == directory.SGRActURL {
		return regexp.MustCompile(`№\s*299\b`).MatchString(text)
	}
	number := helpActNumber.FindStringSubmatch(match.act.Text)
	if number == nil || !regexp.MustCompile(`№\s*`+number[1]+`\b`).MatchString(text) {
		return false
	}
	regulation := actRegulation.FindString(match.act.Text)
	return regulation == "" || strings.Contains(text, regulation)
}

// documentsTasks — задания: по акту группы «ограничения» справки и по перечню общей базы с кодом, которого в
// справке нет; больше documentsMaxActs — хвост в одно задание.
func documentsTasks(acts []helpAct, matches []actMatch) []documentsTask {
	tasks := []documentsTask{}
	used := map[string]bool{}
	for _, act := range acts {
		if act.Group != helpRestrictions {
			continue
		}
		task := documentsTask{label: documentsTaskLabel(act.Title + " " + act.Body), title: act.Title, url: act.URL, body: act.Body}
		for _, match := range matches {
			if actMatchesHelp(match, act) {
				task.positions = append(task.positions, match)
				used[match.act.URL] = true
			}
		}
		tasks = append(tasks, task)
	}
	index := map[string]int{}
	for _, match := range matches {
		if used[match.act.URL] {
			continue
		}
		if _, ok := index[match.act.URL]; !ok {
			index[match.act.URL] = len(tasks)
			tasks = append(tasks, documentsTask{label: documentsTaskLabel(match.act.Text), title: match.act.Text, url: match.act.URL, general: true})
		}
		tasks[index[match.act.URL]].positions = append(tasks[index[match.act.URL]].positions, match)
	}
	if len(tasks) > documentsMaxActs {
		merged := documentsTask{label: "Остальные акты", title: "Остальные акты справки и перечни"}
		var parts []string
		for _, task := range tasks[documentsMaxActs-1:] {
			parts = append(parts, task.title+urlSuffix(task.url)+"\n"+task.body)
			merged.positions = append(merged.positions, task.positions...)
		}
		merged.body = strings.Join(parts, "\n\n")
		tasks = append(tasks[:documentsMaxActs-1], merged)
	}
	return tasks
}

func urlSuffix(address string) string {
	if address == "" {
		return ""
	}
	return " — " + address
}

// documentsRun — один запуск «Документов для ввоза»: подготовка программой, под-агенты по актам, сборка.
// Без базы: шаги и расход уходят через emit и onStep (их вызовы сериализованы).
type documentsRun struct {
	model  agentModel
	data   agentData
	search webSearcher
	code   string
	// product — что за товар (из подбора или наименования строки); photo — описание фото, если было.
	product, photo string
	codeName       string
	emit           func(kind string, content any)
	onStep         func(usage agentUsage)
	// secondOpinion — ответ другой модели по тем же документам (nil — нет): спорные меры перед показом
	// становятся «возможно» с её доводом; secondOpinionName — как её назвать в заметках.
	secondOpinion     func(ctx context.Context, main *AgentResult) *AgentResult
	secondOpinionName string
	mu                sync.Mutex
}

type documentsTaskResult struct {
	task   documentsTask
	result *AgentResult
	err    error
	// positions — позиции перечня с кодом, найденные программой (из справки и самого акта): если
	// под-агент не ответил, запись в итоге строится по ним.
	positions []actMatch
}

func (r *documentsTaskResult) failed() bool { return r.err != nil || r.result == nil }

func (d *documentsRun) execute(ctx context.Context) agentOutcome {
	toolbox := newAgentToolbox(d.data)
	toolbox.documentsCode, toolbox.search = d.code, d.search
	if items, err := d.data.LookupCodes(ctx, []string{d.code}); err == nil && len(items) > 0 {
		d.codeName = treeName(items[0].Name)
	}
	page, event, helpErr := toolbox.goodsHelp(ctx, d.code)
	d.emit("tool", event)
	var acts []helpAct
	if helpErr == nil {
		acts = splitHelpActs(page.Text)
	}
	matches, unread, total := toolbox.generalMatches(ctx, d.code)
	d.emit("tool", agentToolEvent{Tool: "general", Label: "Перечни общей базы", Detail: fmt.Sprintf("позиций с кодом %d в %d перечнях", len(matches), total)})
	tasks := documentsTasks(acts, matches)
	results := make([]documentsTaskResult, len(tasks))
	var wg sync.WaitGroup
	slots := make(chan struct{}, documentsParallel)
	for index, task := range tasks {
		wg.Add(1)
		go func(index int, task documentsTask) {
			defer wg.Done()
			select {
			case slots <- struct{}{}:
				defer func() { <-slots }()
			case <-ctx.Done():
				results[index] = documentsTaskResult{task: task, err: ctx.Err()}
				return
			}
			results[index] = d.runTask(ctx, task, toolbox)
		}(index, task)
	}
	// Правила условного выпуска — для цитаты-основания (кеш 30 дней): параллельно с под-агентами.
	var rules directory.WebPage
	wg.Add(1)
	go func() {
		defer wg.Done()
		rules, _ = d.data.WebPage(ctx, conditionalReleaseURL)
	}()
	wg.Wait()
	// Тайм-аут или остановка: итог из ⚠-записей не собирается и в базу по коду не попадает.
	if ctx.Err() != nil {
		return agentOutcome{err: ctx.Err()}
	}
	failed := 0
	var firstErr error
	for index := range results {
		if results[index].failed() {
			failed++
			if firstErr == nil && results[index].err != nil {
				firstErr = results[index].err
			}
		}
	}
	if len(tasks) > 0 && failed == len(tasks) {
		if firstErr == nil {
			firstErr = ai.ErrAIBadResponse
		}
		return agentOutcome{err: firstErr}
	}
	result := mergeDocuments(d.code, results, matches, rules.Text, helpErr, unread, total)
	if d.secondOpinion != nil {
		if other := d.secondOpinion(ctx, result); other != nil {
			applySecondOpinion(result, other.Documents, d.secondOpinionName, rules.Text, helpErr, total)
		}
	}
	d.emit("result", result)
	return agentOutcome{status: "done", result: result, partial: failed > 0}
}

// runTask — под-агент по одному акту: своя toolbox (засеяна справкой и перечнями — цитаты из них проходят
// проверку), короткая переписка, инструменты только по документам, без вопросов сотруднику. В чат уходят
// только его шаги с подписью акта; частичный итог — нет.
func (d *documentsRun) runTask(ctx context.Context, task documentsTask, seed *agentToolbox) documentsTaskResult {
	toolbox := newAgentToolbox(d.data)
	toolbox.documentsCode, toolbox.search = d.code, d.search
	// Документы — дело серьёзное: итог по акту со ссылкой не принимается, пока под-агент не заглянул в сам акт.
	toolbox.requireActRead = task.url != ""
	for address, text := range seed.web {
		toolbox.web[address] = text
	}
	for address := range seed.links {
		toolbox.links[address] = true
	}
	for address, page := range seed.docs {
		toolbox.docs[address] = page
	}
	preview := d.readAct(ctx, toolbox, task)
	if task.url != "" {
		event := agentToolEvent{Tool: "act", Label: "[" + task.label + "] Акт прочитан программой", URL: task.url,
			Detail: fmt.Sprintf("позиций с кодом %d, фрагментов %d · %d символов", len(task.positions)+len(preview.positions), len(preview.fragments), preview.chars)}
		if preview.err != nil {
			_, detail := webProblem(preview.err)
			event.Label, event.Detail, event.Failed = "["+task.label+"] Акт не прочитан программой", detail, !errors.Is(preview.err, directory.ErrWebPageFile)
		}
		d.mu.Lock()
		d.emit("tool", event)
		d.mu.Unlock()
	}
	// Файл или страница скриптом: под-агент её тоже не прочитает — пусть сразу отдаст ссылку сотруднику.
	toolbox.requireActRead = toolbox.requireActRead && !preview.unreadable
	runner := &agentRunner{
		model: d.model, toolbox: toolbox, system: tnvedDocumentsActPrompt,
		transcript: []map[string]any{{"role": "user", "content": d.taskMessage(task, preview)}},
		tools:      documentsTools(), documents: true, maxSteps: documentsActSteps,
		emit: func(kind string, content any) {
			event, ok := content.(agentToolEvent)
			if kind != "tool" || !ok {
				// Заметки исполнителя («закончил без итога», «не дал итог за отведённые шаги») в чат не идут,
				// но в журнале сервера нужны: иначе причину «⚠ агент не ответил» не узнать.
				if text, isText := content.(map[string]string); kind == "notice" && isText {
					log.Printf("documents %s [%s]: %s", d.code, task.label, text["text"])
				}
				return
			}
			event.Label = "[" + task.label + "] " + event.Label
			d.mu.Lock()
			defer d.mu.Unlock()
			d.emit("tool", event)
		},
		onStep: func(_ []map[string]any, usage agentUsage) {
			d.mu.Lock()
			defer d.mu.Unlock()
			d.onStep(usage)
		},
	}
	outcome := runner.execute(ctx)
	if outcome.result == nil && outcome.err == nil {
		log.Printf("documents %s [%s]: без итога, последний ответ модели: %s", d.code, task.label, service.TruncateRunes(lastAssistantText(runner.transcript), 300))
	}
	positions := append(append([]actMatch{}, task.positions...), preview.positions...)
	return documentsTaskResult{task: task, result: outcome.result, err: outcome.err, positions: positions}
}

// lastAssistantText — текст последнего ответа модели в переписке (вызовы инструментов — именами).
func lastAssistantText(transcript []map[string]any) string {
	for index := len(transcript) - 1; index >= 0; index-- {
		message := transcript[index]
		if message["role"] != "assistant" {
			continue
		}
		text, _ := message["content"].(string)
		if calls, ok := message["tool_calls"].([]any); ok && len(calls) > 0 {
			names := []string{}
			for _, call := range calls {
				object, _ := call.(map[string]any)
				function, _ := object["function"].(map[string]any)
				name, _ := function["name"].(string)
				names = append(names, name)
			}
			text = strings.TrimSpace(text + " [инструменты: " + strings.Join(names, ", ") + "]")
		}
		return text
	}
	return ""
}
