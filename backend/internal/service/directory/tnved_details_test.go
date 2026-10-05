package directory

import (
	"strings"
	"testing"
)

func TestCleanTWSPageKeepsTreeNotesAndDropsBelarus(t *testing.T) {
	page := `<html><head><title>x</title></head><body><header>Меню Войти</header>
	<div>актуальность информации : 01.10.2026</div><div>РАЗДЕЛ VII Пластмассы (гр. 39-40) всего 404 кода, адвалорные ставки 0–15%</div>
	<div>4015 Одежда из вулканизованной резины:</div><div>4015 19 000 0 прочие</div><div>Полное дерево кодов</div>
	<div>Дополнительная единица измерения</div><div>Пара (код 715)</div>
	<div>Дополнительные признаки</div><div>Обязательность указания дополнительных сведений</div><div>наличие текстильных материалов</div>
	<div>Дополнительные признаки, утратившие силу</div><div>Указ Президента Республики Беларусь № 107</div>
	<div>Актуальные ставки сборов</div><div>Сбор : 120.00 BYN</div>
	<div>Меры нетарифного регулирования</div><div>ТР ТС 019/2011</div>
	<div>Дополнительная информация</div><div>Пояснения к ТНВЭД: В данную товарную позицию включаются перчатки</div>
	<h4>ОБЩИЕ ПОЛОЖЕНИЯ</h4><div>подвал</div><script>var x = "актуальность информации"</script></body></html>`
	text := cleanTWSPage(page)
	for _, want := range []string{"4015 19 000 0 прочие", "Пара (код 715)", "наличие текстильных материалов", "включаются перчатки", "ТР ТС 019/2011"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	for _, unwanted := range []string{"BYN", "Беларусь", "всего 404", "адвалорные", "подвал", "Войти", "01.10.2026"} {
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
