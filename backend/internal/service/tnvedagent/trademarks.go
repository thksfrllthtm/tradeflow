package tnvedagent

// ТРОИС в подборе: после итога программа ищет знаки реестра в наименованиях строки и в бренде из
// описания фото (строка «Бренд: …»); фоновый обработчик фото подписывает логотипы реестра (надпись и что
// нарисовано) — по ним ищутся бренды в наименованиях и похожие рисунки на фото товара («/троис»).

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/ai"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/trademarks"
	"github.com/shopspring/decimal"
)

const (
	// logoBatch — логотипов реестра в одном запросе к модели для фото.
	logoBatch  = 10
	brandLabel = "бренд:"
	// pictureCandidates — сколько логотипов реестра сверяются с фото товара за одну проверку.
	pictureCandidates = 20
	// brandCheckTimeout — «/троис» целиком (в фоне); brandCompareMinTime — меньше осталось — сверку не начинать.
	brandCheckTimeout   = 3 * time.Minute
	brandCompareMinTime = 25 * time.Second
)

const readLogosPrompt = `Перед тобой пронумерованные логотипы — изображения товарных знаков из таможенного реестра. Для каждого:
- text — бренд: есть надпись — перепиши её буква в букву, как на логотипе (латиница или кириллица), без слоганов, адресов и мелкого текста; надписи нет, но знак уверенно узнаётся (эмблема Toyota, «галочка» Nike) — название бренда; не уверен или знак незнакомый — пусто. Не угадывай: пусто лучше выдуманного.
- picture — что нарисовано на знаке, 1–6 существительных через запятую, по-русски, в единственном числе: предметы, животные, люди, растения (поезд, рельсы, лев, корона, крыло, лист, человек, бутылка). Не форма и не цвет. Знак — только надпись или абстрактная фигура — пусто.
Ответь только JSON: {"results":[{"index":1,"text":"TOYOTA","picture":""}]}`

// checkTrademarks — знаки ТРОИС для итога: наименования строки и бренд из описания фото; код — лучший код итога.
func (s *TNVEDAgentService) checkTrademarks(ctx context.Context, session repository.TNVEDAgentSession, transcript []map[string]any, result *AgentResult) []trademarks.Hit {
	if s.trademarks == nil {
		return nil
	}
	code := ""
	if best := result.Best(); best != nil {
		code = best.Code
	}
	return trademarkHits(ctx, s.trademarks.Match, sessionNames(session), photoBrands(transcript), code)
}

type matchFunc func(ctx context.Context, texts []string, code string) ([]trademarks.Hit, error)

// trademarkHits — знаки в наименованиях и брендах с фото; «похоже на SONY» от модели для фото — знак,
// под который сделано написание на товаре (возможная имитация, даже если буквы далеки: Abibas).
func trademarkHits(ctx context.Context, match matchFunc, names []string, brands []photoBrand, code string) []trademarks.Hit {
	texts := append([]string{}, names...)
	for _, brand := range brands {
		texts = append(texts, brand.written)
	}
	hits, err := match(ctx, texts, code)
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	for _, hit := range hits {
		seen[hit.RegNo] = true
	}
	for _, brand := range brands {
		if brand.similar == "" {
			continue
		}
		similar, err := match(ctx, []string{brand.similar}, code)
		if err != nil {
			continue
		}
		for _, hit := range similar {
			if !seen[hit.RegNo] {
				seen[hit.RegNo] = true
				hit.Kind, hit.Found = trademarks.KindLookalike, firstNonEmpty(brand.written, brand.similar)
				hits = append(hits, hit)
			}
		}
	}
	if len(hits) == 0 {
		return nil
	}
	return trademarks.SortHits(hits)
}

type photoBrand struct{ written, similar string }

// photoBrands — бренды из строки «Бренд: SONV (похоже на SONY)» описания фото («нет» — бренда нет).
func photoBrands(transcript []map[string]any) []photoBrand {
	brands := []photoBrand{}
	for _, message := range transcript {
		text, ok := message["content"].(string)
		if !ok || message["role"] != "user" || !strings.HasPrefix(text, photoNotePrefix) {
			continue
		}
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(line), "-•* "))
			if !strings.HasPrefix(strings.ToLower(line), brandLabel) {
				continue
			}
			value := strings.TrimSpace(line[len(brandLabel):])
			if value == "" || strings.HasPrefix(strings.ToLower(value), "нет") {
				continue
			}
			brand := photoBrand{written: value}
			if index := strings.Index(strings.ToLower(value), "похоже на"); index >= 0 {
				brand.written = strings.TrimSpace(strings.TrimRight(strings.TrimSpace(value[:index]), "(,;—-"))
				brand.similar = strings.Trim(strings.TrimSpace(value[index+len("похоже на"):]), "()«»\"., ")
			}
			brands = append(brands, brand)
		}
	}
	return brands
}

// readTrademarkLogos — одна порция подписей логотипов ТРОИС. Временная недоступность ИИ — повтор в
// следующий раз. Плохой ответ на пачку — логотипы отправляются по одному: без надписи помечается только
// тот, на котором ИИ ошибся (его подпишет администратор), а не вся пачка.
func (s *TNVEDAgentService) readTrademarkLogos(ctx context.Context) {
	if s.trademarks == nil || !s.visionModel.Configured() || !s.visionModel.Vision() {
		return
	}
	logos, err := s.trademarks.LogosToRead(ctx, logoBatch)
	if err != nil || len(logos) == 0 {
		return
	}
	readings, cost, ok := s.readLogoBatch(ctx, logos)
	if ok == logoBad && len(logos) > 1 {
		readings = map[string]repository.LogoReading{}
		for _, logo := range logos {
			single, step, status := s.readLogoBatch(ctx, []repository.TrademarkLogo{logo})
			cost = cost.Add(step)
			if status == logoRetry {
				break // ИИ недоступен — остальные в следующий раз
			}
			readings[logo.RegNo] = single[logo.RegNo]
		}
	} else if ok == logoRetry {
		return
	}
	if len(readings) == 0 {
		return
	}
	read := 0
	for _, reading := range readings {
		if reading.Name != "" {
			read++
		}
	}
	if err := s.trademarks.SaveLogoReadings(ctx, readings); err != nil {
		log.Printf("trademark logos: save failed: %v", err)
		return
	}
	log.Printf("trademark logos: read %d of %d, $%s", read, len(readings), cost.StringFixed(4))
}

type logoStatus int

const (
	logoOK logoStatus = iota
	// logoBad — ИИ ответил не по формату или отказал (повтор той же пачки не поможет).
	logoBad
	// logoRetry — ИИ недоступен (сеть, лимит): повтор позже.
	logoRetry
)

// readLogoBatch — прочитанное на логотипах пачки по номеру (пустое — не прочитал) и стоимость запроса.
func (s *TNVEDAgentService) readLogoBatch(ctx context.Context, logos []repository.TrademarkLogo) (map[string]repository.LogoReading, decimal.Decimal, logoStatus) {
	content := []map[string]any{}
	for index, logo := range logos {
		content = append(content,
			map[string]any{"type": "text", "text": fmt.Sprintf("Логотип %d:", index+1)},
			map[string]any{"type": "image_url", "image_url": map[string]string{"url": logo.Logo}})
	}
	callCtx, cancel := context.WithTimeout(ctx, photoCallTimeout)
	defer cancel()
	reply, err := s.visionModel.CompleteJSON(callCtx, []agentMessage{{Role: "system", Content: readLogosPrompt}, {Role: "user", Content: content}})
	cost := s.visionModel.Cost(reply.Usage)
	if err != nil {
		if errors.Is(err, ai.ErrAIBadResponse) || errors.Is(err, ai.ErrAIRequestRejected) {
			return emptyReadings(logos), cost, logoBad
		}
		return nil, cost, logoRetry
	}
	var answer struct {
		Results []struct {
			Index   int    `json:"index"`
			Text    string `json:"text"`
			Picture string `json:"picture"`
		} `json:"results"`
	}
	if json.Unmarshal([]byte(ai.UnwrapJSON(reply.Content)), &answer) != nil {
		return emptyReadings(logos), cost, logoBad
	}
	readings := emptyReadings(logos)
	for _, result := range answer.Results {
		if result.Index >= 1 && result.Index <= len(logos) {
			readings[logos[result.Index-1].RegNo] = repository.LogoReading{Name: strings.TrimSpace(result.Text), Picture: strings.TrimSpace(result.Picture)}
		}
	}
	return readings, cost, logoOK
}

func emptyReadings(logos []repository.TrademarkLogo) map[string]repository.LogoReading {
	readings := map[string]repository.LogoReading{}
	for _, logo := range logos {
		readings[logo.RegNo] = repository.LogoReading{}
	}
	return readings
}
