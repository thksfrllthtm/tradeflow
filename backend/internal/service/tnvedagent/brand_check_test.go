package tnvedagent

import (
	"context"
	"testing"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service/trademarks"
)

func TestReadBrandsFromPhoto(t *testing.T) {
	_, server := newFakeProvider(t, textReply("```json\n"+`{"brands":[{"text":" Abibas ","similar":"ADIDAS"},{"text":"","similar":""},{"text":"","similar":"Apple"}],"pictures":["поезд"," "],"goods":" кроссовки "}`+"\n```"))
	vision := testClient(server, "gemini", "", true)
	agent := NewTNVEDAgentService(nil, vision, vision, nil, AgentSettings{})
	spent := agentUsage{}
	reading, err := agent.readBrands(context.Background(), []string{testPhoto}, func(step agentUsage, _ bool) { spent = step })
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

// Текст без фото: надпись, которой нет в реестре, модель рассуждений сверяет с известными брендами, расход —
// не на модель для фото.
func TestSimilarBrandForText(t *testing.T) {
	_, server := newFakeProvider(t, textReply("```json\n"+`{"similar":" ADIDAS "}`+"\n```"))
	agent := NewTNVEDAgentService(nil, testClient(server, "deepseek", "", false), nil, nil, AgentSettings{})
	spent, vision := agentUsage{}, true
	similar, err := agent.similarBrand(context.Background(), "abibaS", func(step agentUsage, v bool) { spent, vision = step, v })
	if err != nil || similar != "ADIDAS" || spent.PromptTokens == 0 || vision {
		t.Fatalf("%v %q %+v vision=%v", err, similar, spent, vision)
	}
	hits := trademarkHits(context.Background(), testRegistry(), nil, []photoBrand{{written: "abibaS", similar: similar}}, "")
	if len(hits) != 1 || hits[0].RegNo != "2/ТЗ" || hits[0].Kind != trademarks.KindLookalike || hits[0].Found != "abibaS" {
		t.Fatalf("%+v", hits)
	}
	_, broken := newFakeProvider(t, textReply("не знаю"))
	agent = NewTNVEDAgentService(nil, testClient(broken, "deepseek", "", false), nil, nil, AgentSettings{})
	if _, err := agent.similarBrand(context.Background(), "abibaS", func(agentUsage, bool) {}); err == nil {
		t.Fatal("ответ не JSON должен быть ошибкой")
	}
}
