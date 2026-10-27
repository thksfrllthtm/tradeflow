package tnvedagent

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
)

// Лимит подборов, ответ агента по коду есть — он и показывается, с датой и товаром.
func TestSavedDocumentsResult(t *testing.T) {
	row := repository.CodeDocuments{Code: "8467292000", Product: "дрель", Description: "нужен СС",
		Documents: repository.JSON(`[{"kind":"cc","measure":"сертификат"}]`), CreatedAt: time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)}
	result := savedDocumentsResult(row)
	if result.DocumentsCode != "8467292000" || len(result.Documents) != 1 || result.Documents[0].Kind != "cc" || !strings.Contains(result.Notes, "от 20.10.2026 по товару «дрель»") {
		t.Fatalf("saved %+v", result)
	}
}

// Настоящая страница TWS.BY (пульверизаторы): записи ТР ТС с документом; утратившие силу, белорусское
// «Сертифицирование», санитарные меры и ТР ТС 003/2011 без «Требуется документ» не берутся.
func TestTWSRegulations(t *testing.T) {
	page, err := os.ReadFile("testdata/tws_8424200000.txt")
	if err != nil {
		t.Fatal(err)
	}
	regulations := twsRegulations(string(page))
	if len(regulations) != 8 {
		t.Fatalf("regulations %d: %+v", len(regulations), regulations)
	}
	first := regulations[0]
	if first.code != "8424200000" || !strings.Contains(first.act, "№ 2") || first.document != "Сертификат соответствия" ||
		len(first.items) != 2 || !strings.Contains(first.items[1], "пистолеты") {
		t.Fatalf("first %+v", first)
	}
	for _, regulation := range regulations {
		if strings.Contains(regulation.act, "№ 91") || strings.Contains(regulation.act, "003/2011") {
			t.Fatalf("expired or without document: %+v", regulation)
		}
	}
}

func TestGeneralDocuments(t *testing.T) {
	page, _ := os.ReadFile("testdata/tws_8424200000.txt")
	result := generalDocuments("8424200000", string(page))
	kinds := map[string]int{}
	for _, document := range result.Documents {
		kinds[document.Kind]++
	}
	// Пульверизаторы: СС и ДС возможны, условный выпуск; СГР по №299 нет; утильсбора нет.
	if kinds["cc"] == 0 || kinds["dc"] == 0 || kinds["conditional"] != 1 || kinds["sgr"] != 0 || kinds["other"] != 0 ||
		!strings.Contains(result.Description, "СС/ДС: возможно нужны") || !strings.Contains(result.Description, "СГР: не нужно") ||
		!strings.Contains(result.Description, "утильсбор: кода нет") {
		t.Fatalf("sprayers %+v", result)
	}
	// Краски 3208: СГР по разделу II (позиция без «из»), утильсбора нет; ТР ТС на странице нет — СС/ДС не нужны.
	paint := generalDocuments("3208109000", "Меры нетарифного регулирования\nСанитарные меры\n32\n- Экстракты\nРешение Комиссии Таможенного Союза № 299\n")
	if !strings.Contains(paint.Description, "СС/ДС: не нужны") || !strings.Contains(paint.Description, "СГР: возможно") || paint.Documents[0].Kind != "sgr" {
		t.Fatalf("paint %+v", paint)
	}
	// 7326 — позиция «из» (скребки для языка): СГР только для них.
	if hooks := generalDocuments("7326909807", ""); !strings.Contains(hooks.Description, "СГР: только если товар — Скребки для языка") {
		t.Fatalf("7326 %+v", hooks)
	}
	// Шины 4011: утильсбор нужен; страница не открылась — СС/ДС под вопросом.
	tyres := generalDocuments("4011100003", "")
	if !strings.Contains(tyres.Description, "утильсбор: код в перечне") || !strings.Contains(tyres.Description, "страница кода TWS.BY не открылась") {
		t.Fatalf("tyres %+v", tyres)
	}
}

// Раздел II Решения №299 перенесён целиком: 94 позиции (диапазон 3901–3911 развёрнут).
func TestSGRPositionsCount(t *testing.T) {
	if len(sgrPositions) != 94 {
		t.Fatalf("sgr positions %d", len(sgrPositions))
	}
}
