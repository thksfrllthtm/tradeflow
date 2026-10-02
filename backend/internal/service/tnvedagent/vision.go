package tnvedagent

// Похожие товары по фото. ИИ (модель агента со зрением) описывает фото — вид товара, материал,
// форма, цвет, особенности; описания записей базы решений составляются один раз в фоне. Перед
// подбором программа ищет похожие записи по отпечатку и описанию, а ИИ сверяет фото лучших
// кандидатов с новым: тот же товар, похожий или другой. Точное совпадение — отдельно (FindSame).

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
)

const (
	photoWorkerInterval = time.Minute
	photoWorkerBatch    = 10
	photoCallTimeout    = 60 * time.Second
	verifyCandidates    = 3
)

const describePhotoPrompt = `Ты описываешь товар на фото, чтобы таможенный декларант нашёл похожие товары в своей базе.
Ответь только JSON:
{"kind":"вид товара 1–2 словами (перчатки, кружка, отвертка)","material":["основные материалы"],"shape":["форма и устройство: 2–4 коротких признака"],"color":["основные цвета"],"features":["другие заметные особенности: покрытие, назначение, комплектность — до 4"]}
Пиши по-русски, коротко, без бренда, артикулов и надписей с упаковки. Если товара на фото нет или его не разобрать — оставь kind пустым.`

const verifyPhotosPrompt = `Первое фото — новый товар, следующие — кандидаты из базы (кандидат 1, 2, …). Для каждого кандидата реши:
same — тот же товар (та же модель; могут отличаться ракурс, фон, свет, упаковка);
similar — товар того же вида, но другая модель (форма, материал, устройство, покрытие, размер);
different — товар другого вида (кружка и миска, перчатки и носки).
Ответь только JSON: {"results":[{"index":1,"verdict":"same|similar|different","why":"до 15 слов по-русски"}]}`

func unwrapJSON(content string) string {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(strings.TrimPrefix(content, "```json"), "```")
	return strings.TrimSpace(strings.TrimSuffix(content, "```"))
}

func cleanList(values []string, limit int) []string {
	out := []string{}
	for _, value := range values {
		if value = service.TruncateRunes(strings.TrimSpace(value), 60); value != "" && len(out) < limit {
			out = append(out, value)
		}
	}
	return out
}

// describePhoto — описание товара на фото.
func (s *TNVEDAgentService) describePhoto(ctx context.Context, photo string) (decisions.PhotoFeatures, agentUsage, error) {
	callCtx, cancel := context.WithTimeout(ctx, photoCallTimeout)
	defer cancel()
	reply, err := s.model.CompleteJSON(callCtx, []agentMessage{
		{Role: "system", Content: describePhotoPrompt},
		{Role: "user", Content: []map[string]any{
			{"type": "text", "text": "Опиши товар на фото."},
			{"type": "image_url", "image_url": map[string]string{"url": photo}},
		}},
	})
	if err != nil {
		return decisions.PhotoFeatures{}, agentUsage{}, err
	}
	var features decisions.PhotoFeatures
	if json.Unmarshal([]byte(unwrapJSON(reply.Content)), &features) != nil {
		return decisions.PhotoFeatures{}, reply.Usage, ai.ErrAIBadResponse
	}
	features = decisions.PhotoFeatures{
		Kind: service.TruncateRunes(strings.TrimSpace(features.Kind), 60), Material: cleanList(features.Material, 4),
		Shape: cleanList(features.Shape, 5), Color: cleanList(features.Color, 4), Features: cleanList(features.Features, 5),
	}
	return features, reply.Usage, nil
}

type photoVerdict struct{ verdict, why string }

// verifyPhotos — визуальная сверка нового фото с фото кандидатов (по id записи).
func (s *TNVEDAgentService) verifyPhotos(ctx context.Context, photo string, candidates []repository.DecisionPhoto) (map[int64]photoVerdict, agentUsage, error) {
	content := []map[string]any{{"type": "text", "text": "Новый товар:"}, {"type": "image_url", "image_url": map[string]string{"url": photo}}}
	ids := []int64{}
	for _, candidate := range candidates {
		var photos []string
		if json.Unmarshal(candidate.Photos, &photos) != nil || len(photos) == 0 {
			continue
		}
		ids = append(ids, candidate.ID)
		content = append(content, map[string]any{"type": "text", "text": fmt.Sprintf("Кандидат %d:", len(ids))}, map[string]any{"type": "image_url", "image_url": map[string]string{"url": photos[0]}})
	}
	if len(ids) == 0 {
		return nil, agentUsage{}, nil
	}
	callCtx, cancel := context.WithTimeout(ctx, photoCallTimeout)
	defer cancel()
	reply, err := s.model.CompleteJSON(callCtx, []agentMessage{{Role: "system", Content: verifyPhotosPrompt}, {Role: "user", Content: content}})
	if err != nil {
		return nil, agentUsage{}, err
	}
	var answer struct {
		Results []struct {
			Index   int    `json:"index"`
			Verdict string `json:"verdict"`
			Why     string `json:"why"`
		} `json:"results"`
	}
	if json.Unmarshal([]byte(unwrapJSON(reply.Content)), &answer) != nil {
		return nil, reply.Usage, ai.ErrAIBadResponse
	}
	verdicts := map[int64]photoVerdict{}
	for _, result := range answer.Results {
		if result.Index < 1 || result.Index > len(ids) {
			continue
		}
		switch result.Verdict {
		case "same", "similar", "different":
			verdicts[ids[result.Index-1]] = photoVerdict{result.Verdict, service.TruncateRunes(strings.TrimSpace(result.Why), 200)}
		}
	}
	return verdicts, reply.Usage, nil
}

var verdictRank = map[string]int{"same": 0, "similar": 1, "": 2}

// similarByPhoto — похожие по фото записи базы для строки подбора: сообщение «similar» в чат и
// подсказка для модели. Без фото — ничего; без ИИ со зрением — только по отпечатку.
func (s *TNVEDAgentService) similarByPhoto(ctx, store context.Context, session repository.TNVEDAgentSession) (string, agentUsage) {
	var photos []string
	var item AgentItemInput
	_ = json.Unmarshal(session.Photos, &photos)
	_ = json.Unmarshal(session.Item, &item)
	usage := agentUsage{}
	if len(photos) == 0 {
		return "", usage
	}
	addUsage := func(step agentUsage) {
		usage.PromptTokens += step.PromptTokens
		usage.CachedTokens += step.CachedTokens
		usage.CompletionTokens += step.CompletionTokens
	}
	hashes := []string{}
	for _, photo := range photos {
		if hash := decisions.PhotoHash(photo); hash != "" {
			hashes = append(hashes, hash)
		}
	}
	vision := s.model.Configured() && s.model.Vision()
	var features *decisions.PhotoFeatures
	description := ""
	if vision {
		if described, step, err := s.describePhoto(ctx, photos[0]); err == nil && !described.Empty() {
			addUsage(step)
			features = &described
			description = described.Summary()
		} else {
			addUsage(step)
		}
	}
	// Точные совпадения уже показаны отдельно — не повторяем их.
	skip := map[int64]bool{}
	if exact, err := s.decisions.FindSame(store, []string{item.Name, item.SupplierName}, hashes); err == nil {
		for _, match := range exact {
			skip[match.Decision.ID] = true
		}
	}
	matches, err := s.decisions.FindSimilar(store, hashes, features, skip)
	if err != nil {
		return "", usage
	}
	if vision && len(matches) > 0 {
		ids := []int64{}
		for index, match := range matches {
			if index < verifyCandidates {
				ids = append(ids, match.Decision.ID)
			}
		}
		if candidates, err := s.repository.DecisionPhotos(store, ids); err == nil {
			if verdicts, step, err := s.verifyPhotos(ctx, photos[0], candidates); err == nil {
				addUsage(step)
				kept := matches[:0]
				for _, match := range matches {
					if verdict, ok := verdicts[match.Decision.ID]; ok {
						if verdict.verdict == "different" {
							continue
						}
						match.Verdict, match.Why = verdict.verdict, verdict.why
					}
					kept = append(kept, match)
				}
				matches = kept
				sortByVerdict(matches)
			} else {
				addUsage(step)
			}
		}
	}
	s.emit(store, session.ID, "similar", map[string]any{"matches": matches, "description": description})
	if len(matches) == 0 {
		return "", usage
	}
	lines := []string{}
	for _, match := range matches {
		status := "не проверено"
		if match.Decision.Status == "verified" {
			status = "проверено администратором"
		}
		verdict := ""
		switch match.Verdict {
		case "same":
			verdict = ", по фото — тот же товар"
		case "similar":
			verdict = ", по фото — похожий товар"
		}
		lines = append(lines, fmt.Sprintf("- похожесть %d%% (%s%s): «%s» → %s, «%s»", match.Score, status, verdict,
			firstNonEmpty(match.Decision.InvoiceName, match.Decision.SupplierName, match.Decision.FinalName), match.Decision.Code, match.Decision.FinalName))
	}
	hint := "Похожие по фото товары из базы решений (подсказка, проверь сам):\n" + strings.Join(lines, "\n")
	if description != "" {
		hint = "Описание фото: " + description + "\n" + hint
	}
	return hint, usage
}

// sortByVerdict: «тот же товар» — первым, затем «похожий», затем без сверки; внутри — по похожести.
func sortByVerdict(matches []decisions.SimilarMatch) {
	for i := 1; i < len(matches); i++ {
		for j := i; j > 0; j-- {
			left, right := matches[j-1], matches[j]
			if verdictRank[left.Verdict] < verdictRank[right.Verdict] || (verdictRank[left.Verdict] == verdictRank[right.Verdict] && left.Score >= right.Score) {
				break
			}
			matches[j-1], matches[j] = right, left
		}
	}
}

// StartPhotoWorker в фоне составляет описания фото записей базы решений (по 10 раз в минуту).
func (s *TNVEDAgentService) StartPhotoWorker(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(photoWorkerInterval)
		defer ticker.Stop()
		for {
			s.describeDecisionPhotos(ctx)
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()
}

// describeDecisionPhotos — одна порция описаний. Временная недоступность ИИ — повтор в следующий
// раз; фото, которое ИИ не смог описать, помечается и больше не отправляется.
func (s *TNVEDAgentService) describeDecisionPhotos(ctx context.Context) {
	if !s.model.Configured() || !s.model.Vision() {
		return
	}
	rows, err := s.repository.DecisionsNeedingPhotoFeatures(ctx, photoWorkerBatch)
	if err != nil || len(rows) == 0 {
		return
	}
	described := 0
	var cost float64
	for _, row := range rows {
		var photos []string
		if json.Unmarshal(row.Photos, &photos) != nil || len(photos) == 0 {
			_ = s.repository.SaveDecisionPhotoFeatures(ctx, row.ID, repository.JSON("null"))
			continue
		}
		features, usage, err := s.describePhoto(ctx, photos[0])
		cost += s.model.Cost(usage)
		if err != nil && !errors.Is(err, ai.ErrAIBadResponse) && !errors.Is(err, ai.ErrAIRequestRejected) {
			break
		}
		data := []byte("null")
		if err == nil && !features.Empty() {
			data, _ = json.Marshal(features)
			described++
		}
		_ = s.repository.SaveDecisionPhotoFeatures(ctx, row.ID, repository.JSON(data))
	}
	if described > 0 {
		log.Printf("photo features: described %d decision photos, $%.4f", described, cost)
	}
}
