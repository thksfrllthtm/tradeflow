package tnvedagent

import (
	"context"
	"strings"
	"testing"
)

// Программа читает акт под-агента заранее: позиция с кодом — разбором перечня (пункт, документ, оговорка),
// фрагменты — по коду и примечаниям, без повторов окрестностей; весь акт — источник цитат; уже известная
// из общей базы позиция не повторяется; всё это — в задании под-агенту.
func TestReadAct(t *testing.T) {
	run := &documentsRun{data: &fakeAgentData{}, code: "8467298000", codeName: "машины для подрезки живой изгороди и стрижки газонов",
		product: "Триммер садовый электрический для стрижки газонов"}
	toolbox := newAgentToolbox(run.data)
	toolbox.documentsCode = run.code
	task := documentsTask{label: "ТР ТС 010/2011", title: "Решение Коллегии ЕЭК №6 (ТР ТС 010/2011)", url: testListURL}
	preview := run.readAct(context.Background(), toolbox, task)
	if preview.err != nil || preview.chars == 0 || len(preview.positions) != 1 {
		t.Fatalf("preview: %+v", preview)
	}
	position := preview.positions[0]
	if !strings.HasPrefix(position.heading, "5. Средства малой механизации") || position.codes != "из 8467" || !position.partial ||
		position.product != "триммеры, газонокосилки" || position.document != "сертификат соответствия" || position.note != "кроме игрушек" {
		t.Fatalf("position: %+v", position)
	}
	// Пункт, позиция, документ и оговорка — одним фрагментом (окрестности склеены), примечание — отдельным.
	joined := strings.Join(preview.fragments, "\n===\n")
	if len(preview.fragments) != 2 || strings.Count(joined, "из 8467") != 1 ||
		!strings.Contains(preview.fragments[0], "5. Средства малой механизации садово-огородного и лесохозяйственного применения механизированные, в том числе электрические\nтриммеры, газонокосилки\nиз 8467\nсертификат соответствия\nкроме игрушек") ||
		!strings.Contains(preview.fragments[1], "Примечание 2. Настоящий перечень не распространяется") {
		t.Fatalf("fragments (%d): %s", len(preview.fragments), joined)
	}
	if origin := toolbox.quoteOrigin("перечень не распространяется на оборудование, бывшее в употреблении", testListURL, "ТР ТС 010/2011"); origin != "ТР ТС 010/2011" {
		t.Fatalf("quote from the act not accepted: %q", origin)
	}
	if !toolbox.links["https://adilet.zan.kz/rus/docs/H18EK000006"] {
		t.Fatal("links of the act are not allowed")
	}
	message := run.taskMessage(task, preview)
	if !strings.Contains(message, "Программа заранее прочитала акт по ссылке") || !strings.Contains(message, `"codes":"из 8467"`) ||
		!strings.Contains(message, "Примечание 2") || !strings.Contains(message, "Это начало, а не замена проверки") {
		t.Fatalf("task message: %s", message)
	}
	// Та же позиция уже найдена в общей базе — второй раз не даётся.
	known := documentsTask{title: task.title, url: task.url, positions: []actMatch{{codes: "из 8467", product: "триммеры, газонокосилки"}}}
	if again := run.readAct(context.Background(), newAgentToolbox(run.data), known); len(again.positions) != 0 || len(again.fragments) == 0 {
		t.Fatalf("known position repeated: %+v", again)
	}
	// Акт-файл: не прочитан, под-агенту сказано не открывать его снова.
	pdf := run.readAct(context.Background(), newAgentToolbox(run.data), documentsTask{title: "№79", url: testPDFURL})
	if pdf.err == nil || !pdf.unreadable || pdf.chars != 0 {
		t.Fatalf("pdf preview: %+v", pdf)
	}
	if message := run.taskMessage(documentsTask{title: "№79", url: testPDFURL}, pdf); !strings.Contains(message, "не смогла: По ссылке файл") || !strings.Contains(message, "Открывать его снова не нужно") {
		t.Fatalf("pdf task message: %s", message)
	}
	if message := run.taskMessage(documentsTask{title: "Остальные акты"}, run.readAct(context.Background(), newAgentToolbox(run.data), documentsTask{})); strings.Contains(message, "Программа заранее") {
		t.Fatalf("act without a link: %s", message)
	}
}

// Основы для поиска по акту: наименование кода первым, только буквы, слова от четырёх, без повторов основ
// («машины» и «машина» — одна), с лимитом.
func TestProductStems(t *testing.T) {
	if stems := strings.Join(productStems("машины для подрезки", "Посудомоечная машина (встраиваемая) 220В, бытовая, для подрезки"), " "); stems != "маши подр посудомое встраива бытов" {
		t.Fatalf("stems: %q", stems)
	}
	if stems := productStems(strings.Repeat("слово ", 5), "один два три четыре пять шесть семь восемь девять десять одиннадцать двенадцать тринадцать четырнадцать пятнадцать шестнадцать семнадцать восемнадцать"); len(stems) != documentsPreviewWords {
		t.Fatalf("limit: %q", stems)
	}
}
