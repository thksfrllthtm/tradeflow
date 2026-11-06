package tnvedagent

// Проверка бренда по ТРОИС с главной («/троис»): надпись и фото товара сверяются с реестром — по буквам,
// по похожему написанию и по рисункам логотипов.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/ai"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/decisions"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/trademarks"
	"github.com/shopspring/decimal"
)

// BrandCheck — проверка бренда по ТРОИС с главной («/троис»): что ИИ увидел на фото и знаки реестра.
type BrandCheck struct {
	// Brands — бренды с фото: «SONV (похоже на SONY)»; Pictures — что нарисовано на знаках товара.
	Brands   []string         `json:"brands"`
	Pictures []string         `json:"pictures"`
	Hits     []trademarks.Hit `json:"hits"`
	// Compared — со сколькими логотипами реестра ИИ сверил фото (похожий рисунок или та же группа товаров).
	Compared int `json:"compared"`
	// PhotoNote — что не получилось с фото ("" — всё в порядке или фото нет).
	PhotoNote string `json:"photoNote,omitempty"`
	// TextNote — что ИИ решил о надписи, которой нет в реестре: похожа ли на известный бренд ("" — не спрашивали).
	TextNote string `json:"textNote,omitempty"`
}

const readBrandPrompt = `На фото — товар, упаковка или этикетка. Найди все бренды и знаки для таможенной проверки товарных знаков.
- brands: text — только видимые буквы надписи, буква в букву, как на товаре: если написано SONV, так и пиши, не исправляй на SONY. Не дописывай слова, которых на фото нет, даже если знаешь полное название (видно «ҚАЗАҚСТАН ТЕМІР» — так и пиши, без «ЖОЛЫ»). Логотип без надписи — название бренда, только если уверенно узнаёшь знак. similar — известный бренд, если написание или логотип на него похожи, но отличаются (SONV → SONY, Abibas → ADIDAS); иначе пусто. Брендов не видно — пустой список.
- pictures — заметные рисунки на логотипе, упаковке, этикетке и самом товаре (персонажи, животные, предметы, здания): существительные по-русски в единственном числе (поезд, юрта, медведь, рыба, корона); не форма и не цвет; рисунков нет — пустой список.
- goods — что за товар и его общая категория, как в перечнях товаров, по-русски через запятую (конфеты, кондитерские изделия; наушники, аудиотехника; кроссовки, обувь); на фото только логотип — пусто.
Не выдумывай. Ответь только JSON: {"brands":[{"text":"SONV","similar":"SONY"}],"pictures":["поезд"],"goods":"наушники"}`

// similarBrandPrompt — надпись не совпала с реестром: сделана ли она под известный бренд (текстовый запрос без
// фото; для фото то же спрашивается у модели для фото в readBrandPrompt).
const similarBrandPrompt = `Сотрудник таможенного брокера проверяет надпись с товара по реестру товарных знаков. По буквам она не совпала ни с одним знаком. Реши, сделана ли надпись под известный бренд: похожее написание, замена или перестановка букв, цифры вместо букв, другой алфавит (Abibas → ADIDAS, SONV → SONY, Pumma → PUMA, Найк → NIKE). Обычное слово, вид товара или незнакомое название — пусто. Не угадывай: пусто лучше выдуманного.
Ответь только JSON: {"similar":"ADIDAS"} или {"similar":""}`

// similarBrand — известный бренд, под который сделана надпись (пусто — не похожа), по модели рассуждений.
func (s *TNVEDAgentService) similarBrand(ctx context.Context, text string, spent func(agentUsage, bool)) (string, error) {
	callCtx, cancel := context.WithTimeout(ctx, photoCallTimeout)
	defer cancel()
	reply, err := s.model.CompleteJSON(callCtx, []agentMessage{{Role: "system", Content: similarBrandPrompt}, {Role: "user", Content: "Надпись: " + text}})
	spent(reply.Usage, false)
	if err != nil {
		return "", err
	}
	var answer struct {
		Similar string `json:"similar"`
	}
	if json.Unmarshal([]byte(ai.UnwrapJSON(reply.Content)), &answer) != nil {
		return "", ai.ErrAIBadResponse
	}
	return service.TruncateRunes(strings.TrimSpace(answer.Similar), 100), nil
}

// brandInput — текст и фото «/троис» после проверки.
func brandInput(input ReferenceInput) (string, []string, error) {
	text := service.TruncateRunes(strings.TrimSpace(input.Text), maxReferenceQuestion)
	photos := []string{}
	for _, photo := range input.Photos {
		if len(photos) == agentMaxPhotos {
			break
		}
		if !decisions.ValidPhoto(photo, agentMaxPhotoBytes) {
			return "", nil, &service.ValidationError{Field: "photos", Message: "Фото — JPEG или PNG до 2 МБ"}
		}
		photos = append(photos, photo)
	}
	if text == "" && len(photos) == 0 {
		return "", nil, &service.ValidationError{Field: "text", Message: "Напишите бренд или добавьте фото: /троис SONV"}
	}
	return text, photos, nil
}

// AskTrademarks — «/троис» в чате-справочнике: вопрос в истории, как обычный; проверка идёт в фоне, ход и
// ответ («trademarks») забирает Get. Подбор кода не запускается и в дневной лимит не считается.
func (s *TNVEDAgentService) AskTrademarks(ctx context.Context, user repository.User, input ReferenceInput) (AgentSessionView, error) {
	if s.trademarks == nil {
		return AgentSessionView{}, &service.ValidationError{Field: "text", Message: "Реестр ТРОИС не подключён"}
	}
	text, photos, err := brandInput(input)
	if err != nil {
		return AgentSessionView{}, err
	}
	item := AgentItemInput{Name: text}
	itemJSON, _ := json.Marshal(item)
	photosJSON, _ := json.Marshal(photos)
	zero := 0
	now := time.Now().UTC()
	session, err := s.repository.CreateAgentSession(ctx, repository.TNVEDAgentSession{
		UserID: user.ID, Mode: modeReference, Title: referenceTitle(text), Status: "running", TrademarkHits: &zero,
		Item: repository.JSON(itemJSON), Photos: repository.JSON(photosJSON), Transcript: repository.JSON("[]"),
		Result: repository.JSON("null"), CostUSD: decimal.Zero, VisionCostUSD: decimal.Zero, Model: s.modelName(), CreatedAt: now, UpdatedAt: now,
	})
	if err != nil {
		return AgentSessionView{}, err
	}
	s.emit(ctx, session.ID, "input", map[string]any{"item": item, "photos": len(photos), "ai": true})
	s.launchBrandCheck(session, text, photos)
	return s.Get(ctx, user, session.ID, 0)
}

// launchBrandCheck — проверка «/троис» в фоне: шаги — «tool», ответ — «trademarks», расход — в сессию.
// Остановка сотрудником (Cancel) прерывает запросы к ИИ.
func (s *TNVEDAgentService) launchBrandCheck(session repository.TNVEDAgentSession, text string, photos []string) {
	ctx, cancel := context.WithTimeout(context.Background(), brandCheckTimeout)
	s.mu.Lock()
	s.cancels[session.ID] = cancel
	s.mu.Unlock()
	go func() {
		defer func() {
			cancel()
			s.mu.Lock()
			delete(s.cancels, session.ID)
			s.mu.Unlock()
		}()
		store := context.Background()
		usage := usageOf(session)
		emit := func(kind string, content any) { s.emit(store, session.ID, kind, content) }
		result, err := s.checkBrand(ctx, text, photos, func(event agentToolEvent) { emit("tool", event) },
			func(step agentUsage, vision bool) {
				model := s.model
				if vision {
					model = s.visionModel
				}
				usage.add(step, model.Cost(step), vision)
			})
		changes := usage.changes()
		switch {
		case errors.Is(ctx.Err(), context.Canceled):
			// Остановлено сотрудником: статус уже «остановлен».
		case err != nil:
			message := ai.AIErrorMessage(err)
			changes["status"], changes["error"] = "error", message
			emit("error", map[string]string{"message": message})
		default:
			emit("trademarks", result)
			changes["status"], changes["trademark_hits"] = "done", len(result.Hits)
		}
		_ = s.repository.UpdateAgentSession(store, session.ID, changes)
	}()
}

// checkBrand — бренды и рисунки знаков с фото (модель для фото), текст сотрудника по ТРОИС и сверка фото с
// логотипами реестра с похожим рисунком или той же группы товаров. Надпись, которой нет в реестре, модель
// рассуждений сверяет с известными брендами (Abibas → ADIDAS). step — шаг для чата, spent — расход (vision —
// модели для фото).
func (s *TNVEDAgentService) checkBrand(ctx context.Context, text string, photos []string, step func(agentToolEvent), spent func(agentUsage, bool)) (BrandCheck, error) {
	result := BrandCheck{Brands: []string{}, Pictures: []string{}, Hits: []trademarks.Hit{}}
	reading := brandReading{}
	vision := len(photos) > 0 && s.visionModel.Configured() && s.visionModel.Vision()
	if len(photos) > 0 && !vision {
		result.PhotoNote = "ИИ для фото не настроен — проверен только текст"
	}
	if vision {
		read, err := s.readBrands(ctx, photos, spent)
		if err != nil && text == "" {
			return BrandCheck{}, err
		}
		if err != nil {
			result.PhotoNote = "Фото не прочиталось — проверен только текст"
			vision = false
		}
		reading = read
	}
	for _, brand := range reading.brands {
		label := brand.written
		if brand.similar != "" {
			label = firstNonEmpty(label, "логотип") + " (похоже на " + brand.similar + ")"
		}
		result.Brands = append(result.Brands, label)
	}
	result.Pictures = append(result.Pictures, reading.pictures...)
	if vision {
		detail := firstNonEmpty(strings.Join(result.Brands, ", "), "надписей бренда нет")
		if len(result.Pictures) > 0 {
			detail += " · рисунок: " + strings.Join(result.Pictures, ", ")
		}
		step(agentToolEvent{Tool: "photo", Label: "Бренды и знаки на фото", Detail: detail})
	}
	names := []string{}
	for _, name := range []string{text, reading.goods} {
		if name != "" {
			names = append(names, name)
		}
	}
	code := nonDigits.ReplaceAllString(text, "")
	if hits := trademarkHits(ctx, s.trademarks.Match, names, reading.brands, code); hits != nil {
		result.Hits = hits
	}
	detail := "в реестре нет"
	if len(result.Hits) > 0 {
		detail = trademarkSummary(result.Hits)
	}
	step(agentToolEvent{Tool: "check_trademarks", Label: "ТРОИС: поиск по надписям", Detail: detail})
	if text != "" && len(result.Hits) == 0 && s.model.Configured() {
		similar, err := s.similarBrand(ctx, text, spent)
		switch {
		case err != nil && ctx.Err() != nil:
			return result, ctx.Err()
		case err != nil:
			result.TextNote = "Похожесть на известный бренд ИИ не проверил — проверено только написание"
			step(agentToolEvent{Tool: "check_trademarks", Label: "Похоже на известный бренд?", Detail: "ИИ не ответил", Failed: true})
		case similar == "":
			result.TextNote = "По мнению ИИ, на известный бренд написание не похоже"
			step(agentToolEvent{Tool: "check_trademarks", Label: "Похоже на известный бренд?", Detail: "нет"})
		default:
			result.TextNote = "По мнению ИИ, написание похоже на " + similar
			detail := "в реестре нет"
			if hits := trademarkHits(ctx, s.trademarks.Match, nil, []photoBrand{{written: text, similar: similar}}, code); hits != nil {
				result.Hits, detail = hits, trademarkSummary(hits)
			}
			step(agentToolEvent{Tool: "check_trademarks", Label: "Похоже на известный бренд? " + similar, Detail: detail})
		}
	}
	if !vision {
		return result, nil
	}
	// Незнакомый знак без надписи: фото товара сверяется с логотипами реестра — если на сверку ещё есть время.
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < brandCompareMinTime {
		result.PhotoNote = "Сверка с логотипами реестра пропущена: ИИ долго отвечал — попробуйте ещё раз"
		return result, nil
	}
	found := map[string]bool{}
	for _, hit := range result.Hits {
		found[hit.RegNo] = true
	}
	candidates, err := s.trademarks.PictureCandidates(ctx, reading.pictures, names, code, found, pictureCandidates)
	if err != nil || len(candidates) == 0 {
		return result, nil
	}
	result.Compared = len(candidates)
	matched, err := s.comparePictures(ctx, photos, candidates, spent)
	if err != nil {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		result.PhotoNote = "Сверка с логотипами реестра не удалась — попробуйте ещё раз"
		step(agentToolEvent{Tool: "check_trademarks", Label: fmt.Sprintf("Сверка фото с %d логотипами реестра", len(candidates)), Detail: "не удалась", Failed: true})
		return result, nil
	}
	detail = "совпадений нет"
	if len(matched) > 0 {
		detail = trademarkSummary(matched)
	}
	step(agentToolEvent{Tool: "check_trademarks", Label: fmt.Sprintf("Сверка фото с %d логотипами реестра", len(candidates)), Detail: detail})
	result.Hits = append(result.Hits, matched...)
	return result, nil
}

type brandReading struct {
	brands   []photoBrand
	pictures []string
	goods    string
}

// readBrands — бренды, рисунки знаков и вид товара на фото по ответу модели для фото.
func (s *TNVEDAgentService) readBrands(ctx context.Context, photos []string, spent func(agentUsage, bool)) (brandReading, error) {
	callCtx, cancel := context.WithTimeout(ctx, photoCallTimeout)
	defer cancel()
	reply, err := s.visionModel.CompleteJSON(callCtx, []agentMessage{
		{Role: "system", Content: readBrandPrompt},
		{Role: "user", Content: photoContent("Найди бренды и знаки на фото.", photos)},
	})
	spent(reply.Usage, true)
	if err != nil {
		return brandReading{}, err
	}
	log.Printf("brand check: photos %d, $%s", len(photos), s.visionModel.Cost(reply.Usage).StringFixed(4))
	var answer struct {
		Brands []struct {
			Text    string `json:"text"`
			Similar string `json:"similar"`
		} `json:"brands"`
		Pictures []string `json:"pictures"`
		Goods    string   `json:"goods"`
	}
	if json.Unmarshal([]byte(ai.UnwrapJSON(reply.Content)), &answer) != nil {
		return brandReading{}, ai.ErrAIBadResponse
	}
	reading := brandReading{brands: []photoBrand{}, pictures: cleanList(answer.Pictures, 8), goods: service.TruncateRunes(strings.TrimSpace(answer.Goods), 60)}
	for _, brand := range answer.Brands {
		written := service.TruncateRunes(strings.TrimSpace(brand.Text), 100)
		similar := service.TruncateRunes(strings.TrimSpace(brand.Similar), 100)
		if (written != "" || similar != "") && len(reading.brands) < 10 {
			reading.brands = append(reading.brands, photoBrand{written: written, similar: similar})
		}
	}
	return reading, nil
}

const comparePicturesPrompt = `Первые фото — товар. Дальше — пронумерованные товарные знаки из таможенного реестра. Для каждого знака реши, есть ли он на товаре:
same — этот знак есть на товаре (тот же логотип, эмблема или надпись; допустимы другой цвет и размер);
similar — на товаре знак, похожий на этот до степени смешения: та же композиция и рисунок при другом тексте или деталях — возможная имитация;
no — нет.
Сравнивай только знаки, а не товары: просто похожий предмет (другой поезд, другой лев) — no. Не уверен — no.
Ответь только JSON: {"results":[{"index":1,"verdict":"no","why":"до 15 слов по-русски"}]}`

// comparePictures — знаки-кандидаты, которые модель для фото нашла на товаре (тот же или похожий).
func (s *TNVEDAgentService) comparePictures(ctx context.Context, photos []string, candidates []trademarks.Hit, spent func(agentUsage, bool)) ([]trademarks.Hit, error) {
	regNos := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		regNos = append(regNos, candidate.RegNo)
	}
	logos, err := s.trademarks.AILogos(ctx, regNos)
	if err != nil {
		return nil, err
	}
	content := photoContent("Товар:", photos)
	shown := []trademarks.Hit{}
	for _, candidate := range candidates {
		if logo := logos[candidate.RegNo]; logo != "" {
			shown = append(shown, candidate)
			content = append(content,
				map[string]any{"type": "text", "text": fmt.Sprintf("Знак %d:", len(shown))},
				map[string]any{"type": "image_url", "image_url": map[string]string{"url": logo}})
		}
	}
	if len(shown) == 0 {
		return nil, nil
	}
	callCtx, cancel := context.WithTimeout(ctx, photoCallTimeout)
	defer cancel()
	reply, err := s.visionModel.CompleteJSON(callCtx, []agentMessage{{Role: "system", Content: comparePicturesPrompt}, {Role: "user", Content: content}})
	spent(reply.Usage, true)
	if err != nil {
		return nil, err
	}
	log.Printf("brand check: compared with %d logos, $%s", len(shown), s.visionModel.Cost(reply.Usage).StringFixed(4))
	var answer struct {
		Results []struct {
			Index   int    `json:"index"`
			Verdict string `json:"verdict"`
			Why     string `json:"why"`
		} `json:"results"`
	}
	if json.Unmarshal([]byte(ai.UnwrapJSON(reply.Content)), &answer) != nil {
		return nil, ai.ErrAIBadResponse
	}
	matched := []trademarks.Hit{}
	for _, result := range answer.Results {
		if result.Index < 1 || result.Index > len(shown) || (result.Verdict != "same" && result.Verdict != "similar") {
			continue
		}
		hit := shown[result.Index-1]
		hit.Kind, hit.Found = trademarks.KindLookalike, "изображение на товаре"
		if result.Verdict == "same" {
			hit.Kind = trademarks.KindBrand
		}
		hit.Why = service.TruncateRunes(strings.TrimSpace(result.Why), 200)
		matched = append(matched, hit)
	}
	return matched, nil
}
