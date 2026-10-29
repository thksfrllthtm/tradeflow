package tnvedagent

import (
	"context"
	"strings"
	"testing"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/trademarks"
)

func testRegistry() matchFunc {
	index := trademarks.NewIndex([]trademarks.Record{
		{RegNo: "1/ТЗ", Name: "SONY", Codes: []string{"851830"}},
		{RegNo: "2/ТЗ", Name: "ADIDAS", Codes: []string{"640411"}},
	})
	return func(_ context.Context, texts []string, code string) ([]trademarks.Hit, error) {
		return index.Match(texts, code), nil
	}
}

func TestPhotoBrandsReadsBrandLine(t *testing.T) {
	transcript := []map[string]any{
		{"role": "user", "content": photoNotePrefix + " (составила модель):\n- Вид товара: кроссовки\n- Бренд: Abibas (похоже на ADIDAS)\n- Материал: текстиль"},
		{"role": "user", "content": photoNotePrefix + ":\n- Бренд: нет"},
		{"role": "user", "content": "Бренд: SONY — не описание фото"},
	}
	brands := photoBrands(transcript)
	if len(brands) != 1 || brands[0].written != "Abibas" || brands[0].similar != "ADIDAS" {
		t.Fatalf("%+v", brands)
	}
}

func TestTrademarkHitsFromNamesAndPhoto(t *testing.T) {
	hits := trademarkHits(context.Background(), testRegistry(), []string{"Кроссовки спортивные", "SONV наушники"},
		[]photoBrand{{written: "Abibas", similar: "ADIDAS"}}, "8518300000")
	if len(hits) != 2 {
		t.Fatalf("%+v", hits)
	}
	if hits[0].RegNo != "1/ТЗ" || hits[0].Kind != trademarks.KindLookalike || !hits[0].SameGoods || hits[0].Found != "SONV" {
		t.Fatalf("SONV: %+v", hits[0])
	}
	// Abibas далёк от ADIDAS по буквам — имитацию заметила модель для фото.
	if hits[1].RegNo != "2/ТЗ" || hits[1].Kind != trademarks.KindLookalike || hits[1].Found != "Abibas" {
		t.Fatalf("Abibas: %+v", hits[1])
	}
}

func TestCheckTrademarksToolAndResult(t *testing.T) {
	model := &scriptedModel{replies: []agentReply{
		{ToolCalls: []agentToolCall{call("c1", "check_trademarks", map[string]string{"query": "S0NY"})}},
		{ToolCalls: []agentToolCall{call("c2", "finish", map[string]any{
			"description": "Наушники.", "name": "Наушники", "confidence": "medium",
			"candidates": []map[string]string{{"code": "6116102000", "verdict": "fits", "reason": "Пример."}},
		})}},
	}}
	runner, events := newTestRunner(model, &fakeAgentData{})
	runner.toolbox.trademarks = testRegistry()
	runner.finalize = func(result *AgentResult) {
		result.Trademarks = trademarkHits(context.Background(), testRegistry(), []string{"SONV"}, nil, result.Best().Code)
	}
	outcome := runner.execute(context.Background())
	if outcome.err != nil || outcome.result == nil {
		t.Fatalf("%+v", outcome)
	}
	var tool agentToolEvent
	var shown *AgentResult
	for _, event := range *events {
		switch content := event.content.(type) {
		case agentToolEvent:
			if content.Tool == "check_trademarks" {
				tool = content
			}
		case *AgentResult:
			shown = content
		}
	}
	if !strings.Contains(tool.Detail, "SONY (похоже)") {
		t.Fatalf("tool event: %+v", tool)
	}
	if shown == nil || len(shown.Trademarks) != 1 || shown.Trademarks[0].RegNo != "1/ТЗ" {
		t.Fatalf("result shown without trademarks: %+v", shown)
	}
}

func TestReadLogoBatch(t *testing.T) {
	logos := []repository.TrademarkLogo{{RegNo: "1/ТЗ", Logo: testPhoto}, {RegNo: "2/ТЗ", Logo: testPhoto}}
	_, server := newFakeProvider(t, textReply(`{"results":[{"index":2,"text":" TOYOTA ","picture":" три овала "},{"index":9,"text":"чужой"}]}`))
	vision := testClient(server, "gemini", "", true)
	agent := NewTNVEDAgentService(nil, vision, vision, nil, AgentSettings{})
	names, _, status := agent.readLogoBatch(context.Background(), logos)
	if status != logoOK || names["1/ТЗ"].Name != "" || names["2/ТЗ"] != (repository.LogoReading{Name: "TOYOTA", Picture: "три овала"}) || len(names) != 2 {
		t.Fatalf("%v %v", status, names)
	}
	// Ответ не JSON — пачка «плохая»: по одному логотипу её отправит readTrademarkLogos.
	_, broken := newFakeProvider(t, textReply("не могу прочитать"))
	bad := testClient(broken, "gemini", "", true)
	agent = NewTNVEDAgentService(nil, bad, bad, nil, AgentSettings{})
	if names, _, status := agent.readLogoBatch(context.Background(), logos); status != logoBad || len(names) != 2 || names["1/ТЗ"].Name != "" {
		t.Fatalf("%v %v", status, names)
	}
}

func TestReadBrandsFromPhoto(t *testing.T) {
	_, server := newFakeProvider(t, textReply("```json\n"+`{"brands":[{"text":" Abibas ","similar":"ADIDAS"},{"text":"","similar":""},{"text":"","similar":"Apple"}],"pictures":["поезд"," "],"goods":" кроссовки "}`+"\n```"))
	vision := testClient(server, "gemini", "", true)
	agent := NewTNVEDAgentService(nil, vision, vision, nil, AgentSettings{})
	spent := agentUsage{}
	reading, err := agent.readBrands(context.Background(), []string{testPhoto}, func(step agentUsage) { spent = step })
	if spent.PromptTokens == 0 {
		t.Fatal("usage not reported")
	}
	brands := reading.brands
	if err == nil && (len(reading.pictures) != 1 || reading.pictures[0] != "поезд" || reading.goods != "кроссовки") {
		t.Fatalf("%+v", reading)
	}
	if err != nil || len(brands) != 2 || brands[0] != (photoBrand{written: "Abibas", similar: "ADIDAS"}) || brands[1].similar != "Apple" {
		t.Fatalf("%v %+v", err, brands)
	}
	// С фото только «похоже на ADIDAS» — знак реестра как возможная имитация.
	hits := trademarkHits(context.Background(), testRegistry(), nil, brands, "")
	if len(hits) != 1 || hits[0].RegNo != "2/ТЗ" || hits[0].Kind != trademarks.KindLookalike || hits[0].Found != "Abibas" {
		t.Fatalf("%+v", hits)
	}
}

func TestReferenceKind(t *testing.T) {
	zero, two := 0, 2
	cases := []struct {
		row       repository.ReferenceSession
		hasResult bool
		want      string
	}{
		{repository.ReferenceSession{TrademarkHits: &zero}, false, "trademarks"},
		{repository.ReferenceSession{TrademarkHits: &two}, false, "trademarks"},
		{repository.ReferenceSession{DocumentsCode: "8205591000"}, false, "documents"},
		{repository.ReferenceSession{DocumentsCode: "8205591000"}, true, "chat"}, // подбор, затем документы
		{repository.ReferenceSession{}, false, "chat"},                           // подбор ещё идёт
	}
	for _, c := range cases {
		if got := referenceKind(c.row, c.hasResult); got != c.want {
			t.Errorf("%+v %v: %s, want %s", c.row, c.hasResult, got, c.want)
		}
	}
}
