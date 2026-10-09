package decisions

// Один товар сметы — одна запись базы решений. В разных сметах товар сохраняется заново (рулетка
// красная — не синяя), а в одной смете строки того же товара повторно не записываются.

import (
	"context"
	"encoding/json"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
)

// EstimateDecision — товар строки уже в базе решений: запись и строка сметы, из которой её сохранили.
type EstimateDecision struct {
	DecisionID int64  `json:"decisionId"`
	ItemID     string `json:"itemId"`
	Status     string `json:"status"`
}

// decisionNameKey — наименование товара для сравнения: для декларации, иначе из инвойса.
func decisionNameKey(finalName, invoiceName string) string {
	if name := NormalizeDecisionName(finalName); name != "" {
		return name
	}
	return NormalizeDecisionName(invoiceName)
}

// sameProduct — в одной смете это тот же товар: тот же код и наименование, а если фото есть у обоих —
// похожий отпечаток (dHash считается по оттенкам серого, так что цвет снимка не важен).
func sameProduct(code, name string, hashes []string, decision repository.CodeDecision) bool {
	if code != decision.Code || name == "" || name != decisionNameKey(decision.FinalName, decision.InvoiceName) {
		return false
	}
	var other []string
	_ = json.Unmarshal(decision.PhotoHashes, &other)
	return len(hashes) == 0 || len(other) == 0 || samePhoto(hashes, other)
}

func findSame(decisions []repository.CodeDecision, code, name string, hashes []string) *repository.CodeDecision {
	for index := range decisions {
		if sameProduct(code, name, hashes, decisions[index]) {
			return &decisions[index]
		}
	}
	return nil
}

// EstimateStatus — какие строки сметы уже в базе решений (сами или как тот же товар другой строки).
// scope — владелец сметы или repository.AllUsers для администратора.
func (s *CodeDecisionService) EstimateStatus(ctx context.Context, estimateID, scope int64) (map[string]EstimateDecision, error) {
	estimate, err := s.repository.GoodsEstimateByIDForUser(ctx, estimateID, scope)
	if err != nil {
		return nil, err
	}
	saved, err := s.repository.CodeDecisionsByEstimate(ctx, estimateID)
	if err != nil || len(saved) == 0 {
		return map[string]EstimateDecision{}, err
	}
	var input struct {
		Items []struct {
			ID           string `json:"id"`
			Code         string `json:"code"`
			Name         string `json:"name"`
			StandardName string `json:"standardName"`
		} `json:"items"`
	}
	_ = json.Unmarshal(estimate.InputSnapshot, &input)
	var photos map[string][]string
	status := map[string]EstimateDecision{}
	for _, item := range input.Items {
		code, name := NormalizeCode(item.Code), decisionNameKey(item.StandardName, item.Name)
		// Фото строки читаются, только если у неё есть кандидат по коду и наименованию.
		if findSame(saved, code, name, nil) == nil {
			continue
		}
		if photos == nil {
			if photos, err = s.repository.GoodsEstimatePhotos(ctx, estimateID); err != nil {
				return nil, err
			}
		}
		hashes := []string{}
		if list := photos[item.ID]; len(list) > 0 {
			if hash := PhotoHash(list[0]); hash != "" {
				hashes = append(hashes, hash)
			}
		}
		if match := findSame(saved, code, name, hashes); match != nil {
			status[item.ID] = EstimateDecision{DecisionID: match.ID, ItemID: match.ItemID, Status: match.Status}
		}
	}
	return status, nil
}
