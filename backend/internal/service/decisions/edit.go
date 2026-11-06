package decisions

// Правка, проверка, отклонение и удаление записи администратором.

import (
	"context"
	"strings"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
)

// Verify — «Проверено» с необязательной правкой. Если у записи есть код агента, отмечается,
// прав ли был агент.
func (s *CodeDecisionService) Verify(ctx context.Context, adminID, id int64, edit CodeDecisionEdit) (CodeDecisionView, error) {
	return s.edit(ctx, adminID, id, edit, "verified")
}

// Update — правка без смены статуса.
func (s *CodeDecisionService) Update(ctx context.Context, adminID, id int64, edit CodeDecisionEdit) (CodeDecisionView, error) {
	return s.edit(ctx, adminID, id, edit, "")
}

func (s *CodeDecisionService) Reject(ctx context.Context, adminID, id int64, note string) (CodeDecisionView, error) {
	edit := CodeDecisionEdit{}
	if strings.TrimSpace(note) != "" {
		edit.Note = &note
	}
	return s.edit(ctx, adminID, id, edit, "rejected")
}

func (s *CodeDecisionService) edit(ctx context.Context, adminID, id int64, edit CodeDecisionEdit, status string) (CodeDecisionView, error) {
	current, err := s.repository.CodeDecisionByID(ctx, id)
	if err != nil {
		return CodeDecisionView{}, err
	}
	changes := map[string]any{"updated_at": time.Now().UTC()}
	code := current.Code
	if edit.Code != nil {
		code = NormalizeCode(*edit.Code)
		if !tnvedCodePattern.MatchString(code) {
			return CodeDecisionView{}, &service.ValidationError{Field: "code", Message: "Код ТН ВЭД — 10 цифр"}
		}
		changes["code"] = code
	}
	if edit.FinalName != nil {
		changes["final_name"] = cleanText(*edit.FinalName, maxDecisionNameRunes)
	}
	if edit.InvoiceName != nil {
		changes["invoice_name"] = cleanText(*edit.InvoiceName, maxDecisionNameRunes)
	}
	if edit.SupplierName != nil {
		changes["supplier_name"] = cleanText(*edit.SupplierName, maxDecisionNameRunes)
	}
	if edit.Reason != nil {
		changes["reason"] = cleanText(*edit.Reason, maxDecisionTextRunes)
	}
	if edit.Note != nil {
		changes["note"] = cleanText(*edit.Note, maxDecisionTextRunes)
	}
	if edit.UnitPriceUSD != nil {
		price, err := optionalDecimalText(*edit.UnitPriceUSD, "unitPriceUsd")
		if err != nil {
			return CodeDecisionView{}, err
		}
		if price == nil {
			changes["unit_price_usd"] = nil
		} else {
			changes["unit_price_usd"] = *price
		}
	}
	if edit.ProductDirectoryID != nil {
		if *edit.ProductDirectoryID > 0 {
			changes["product_directory_id"] = *edit.ProductDirectoryID
		} else {
			changes["product_directory_id"] = nil
		}
	}
	if status != "" {
		changes["status"] = status
		if status == "verified" {
			now := time.Now().UTC()
			changes["verified_by"], changes["verified_at"] = adminID, now
		} else {
			changes["verified_by"], changes["verified_at"] = nil, nil
		}
	}
	finalStatus := current.Status
	if status != "" {
		finalStatus = status
	}
	// Оценка агента — только у проверенной записи, где агент что-то предлагал.
	if current.AgentCode != "" && finalStatus == "verified" {
		changes["agent_correct"] = current.AgentCode == code
	} else if finalStatus != "verified" {
		changes["agent_correct"] = nil
	}
	if err := s.repository.UpdateCodeDecision(ctx, id, changes); err != nil {
		return CodeDecisionView{}, err
	}
	return s.Get(ctx, id)
}

func (s *CodeDecisionService) Delete(ctx context.Context, id int64) error {
	return s.repository.DeleteCodeDecision(ctx, id)
}
