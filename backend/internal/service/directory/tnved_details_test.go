package directory

import (
	"strings"
	"testing"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
)

// Страница кода целиком: дерево, признаки, меры, классификационные решения, пояснения к позиции и общие
// положения группы и раздела — без шапки, подвала, утративших силу признаков, сборов и норм Беларуси.
func TestCleanTWSPageKeepsWholePageAndDropsBelarus(t *testing.T) {
	longNotes := strings.Repeat("Общие положения группы 82: в группу не включаются инструменты для станков. ", 200)
	page := `<html><head><title>x</title></head><body><header>Меню Войти</header>
	<div>актуальность информации : 01.10.2026</div><div>РАЗДЕЛ VII Пластмассы (гр. 39-40) всего 404 кода, адвалорные ставки 0–15%</div>
	<div>4015 Одежда из вулканизованной резины:</div><div>4015 19 000 0 прочие</div><div>Полное дерево кодов</div>
	<div>Дополнительная единица измерения</div><div>Пара (код 715)</div>
	<div>Дополнительные признаки</div><div>Обязательность указания дополнительных сведений</div><div>наличие текстильных материалов</div>
	<div>Дополнительные признаки, утратившие силу</div><div>Указ Президента № 107 (утратил силу)</div>
	<div>Актуальные ставки сборов</div><div>Сбор : 120.00</div>
	<div>Меры нетарифного регулирования</div><div>ТР ТС 019/2011</div><div>Постановление Совмина Республики Беларусь № 5</div>
	<div>Классификационные решения (1)</div><div>Решение ЕЭК: перчатки трикотажные с покрытием — 6116</div>
	<div>Дополнительная информация</div><div>Пояснения к ТНВЭД: В данную товарную позицию включаются перчатки</div>
	<h4>ОБЩИЕ ПОЛОЖЕНИЯ</h4><div>` + longNotes + `</div><div>Footer menu</div><div>подвал</div><script>var x = "актуальность информации"</script></body></html>`
	text := cleanTWSPage(page)
	for _, want := range []string{"4015 19 000 0 прочие", "Пара (код 715)", "наличие текстильных материалов", "включаются перчатки", "ТР ТС 019/2011",
		"Решение ЕЭК: перчатки трикотажные", "ОБЩИЕ ПОЛОЖЕНИЯ", strings.TrimSpace(longNotes)} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q", service.TruncateRunes(want, 80))
		}
	}
	for _, unwanted := range []string{"Беларусь", "утратил силу", "Сбор : 120.00", "всего 404", "адвалорные", "подвал", "Footer menu", "Войти", "01.10.2026", "Полное дерево кодов"} {
		if strings.Contains(text, unwanted) {
			t.Errorf("unexpected %q in:\n%s", unwanted, text)
		}
	}
}

// В шаблонах htmlDropBlocks стоял невидимый backspace вместо \b: скрипты, стили и <head> оставались в тексте.
func TestHTMLToTextDropsScriptsStylesAndHead(t *testing.T) {
	text := htmlToText(`<html><head><title>Заголовок</title><meta charset="utf-8"></head><body><style type="text/css">.a{color:red}</style>
	<header>Шапка</header><p>Текст страницы</p><script type="text/javascript">var secret = 1</script><noscript>Включите JS</noscript><svg viewBox="0 0 1 1"><path d="M0"/></svg></body></html>`)
	if !strings.Contains(text, "Текст страницы") || !strings.Contains(text, "Шапка") {
		t.Fatalf("text lost: %q", text)
	}
	for _, unwanted := range []string{"Заголовок", "color:red", "secret", "Включите JS", "M0"} {
		if strings.Contains(text, unwanted) {
			t.Errorf("unexpected %q in %q", unwanted, text)
		}
	}
}

// Настоящая страница кода TWS.BY (3918 10 100 0, 2026-10-16): классификационные решения, решения ЕЭК и общие
// положения пояснений — целиком; подвала и белорусских норм нет.
func TestCleanTWSPageRealPage(t *testing.T) {
	text := cleanTWSPage(string(readTestdata(t, "tws_code_3918101000.html")))
	for _, want := range []string{"3918 10 100 0", "Классификационные решения", "Композитное напольное покрытие", "Террасная доска",
		"Решение Коллегии Евразийской Экономической Комиссии № 56", "ОБЩИЕ ПОЛОЖЕНИЯ"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q", want)
		}
	}
	for _, unwanted := range []string{"Footer menu", "Беларусь", "Войти", "Заказать Обратный Звонок"} {
		if strings.Contains(text, unwanted) {
			t.Errorf("unexpected %q", unwanted)
		}
	}
	if runes := len([]rune(text)); runes < 30_000 || runes > detailsMaxRunes {
		t.Errorf("длина %d", runes)
	}
}
