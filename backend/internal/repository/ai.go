package repository

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"gorm.io/gorm"
)

func (r *Repository) CreateProductAIAnalysis(ctx context.Context, analysis ProductAIAnalysis) (ProductAIAnalysis, error) {
	if err := r.db.WithContext(ctx).Create(&analysis).Error; err != nil {
		return ProductAIAnalysis{}, err
	}
	return analysis, nil
}

func (r *Repository) LatestProductAIAnalysisForOwner(ctx context.Context, productID, ownerID int64) (ProductAIAnalysis, error) {
	var analysis ProductAIAnalysis
	err := r.db.WithContext(ctx).
		Joins("JOIN imported_products ON imported_products.id = product_ai_analyses.product_id").
		Joins("JOIN estimates ON estimates.id = imported_products.estimate_id").
		Where("product_ai_analyses.product_id = ? AND estimates.owner_id = ?", productID, ownerID).
		Order("product_ai_analyses.created_at DESC, product_ai_analyses.id DESC").First(&analysis).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ProductAIAnalysis{}, ErrNotFound
	}
	return analysis, err
}

func (r *Repository) MarkProductAIAnalysesRecheck(ctx context.Context, productID int64) error {
	return r.db.WithContext(ctx).Model(&ProductAIAnalysis{}).Where("product_id = ?", productID).Updates(map[string]any{"needs_recheck": true}).Error
}

func (r *Repository) ProductAIDialogueForOwner(ctx context.Context, productID, ownerID int64) (ProductAIDialogue, error) {
	if _, err := r.ProductByIDForOwner(ctx, productID, ownerID); err != nil {
		return ProductAIDialogue{}, err
	}
	var dialogue ProductAIDialogue
	err := r.db.WithContext(ctx).Where("product_id = ?", productID).First(&dialogue).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		dialogue = ProductAIDialogue{ProductID: productID, CreatedBy: ownerID, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
		if err := r.db.WithContext(ctx).Create(&dialogue).Error; err != nil {
			// A concurrent first message may have created the unique dialogue.
			if err := r.db.WithContext(ctx).Where("product_id = ?", productID).First(&dialogue).Error; err != nil {
				return ProductAIDialogue{}, err
			}
		}
		err = nil
	}
	return dialogue, err
}

func (r *Repository) ListProductAIMessagesForOwner(ctx context.Context, productID, ownerID int64, limit int) ([]ProductAIMessage, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	if _, err := r.ProductByIDForOwner(ctx, productID, ownerID); err != nil {
		return nil, err
	}
	var messages []ProductAIMessage
	err := r.db.WithContext(ctx).
		Joins("JOIN product_ai_dialogues ON product_ai_dialogues.id = product_ai_messages.dialogue_id").
		Where("product_ai_dialogues.product_id = ?", productID).
		Order("product_ai_messages.created_at DESC, product_ai_messages.id DESC").
		Limit(limit).Find(&messages).Error
	for left, right := 0, len(messages)-1; left < right; left, right = left+1, right-1 {
		messages[left], messages[right] = messages[right], messages[left]
	}
	return messages, err
}

func (r *Repository) ProductAIMessageByRequest(ctx context.Context, productID, ownerID int64, requestID string) (ProductAIMessage, error) {
	if _, err := r.ProductByIDForOwner(ctx, productID, ownerID); err != nil {
		return ProductAIMessage{}, err
	}
	var message ProductAIMessage
	err := r.db.WithContext(ctx).
		Joins("JOIN product_ai_dialogues ON product_ai_dialogues.id = product_ai_messages.dialogue_id").
		Where("product_ai_dialogues.product_id = ? AND product_ai_messages.client_request_id = ?", productID, requestID).
		Order("product_ai_messages.id DESC").First(&message).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ProductAIMessage{}, ErrNotFound
	}
	return message, err
}

func (r *Repository) CreateProductAIMessage(ctx context.Context, message ProductAIMessage) (ProductAIMessage, error) {
	if len(message.AttachmentAssetIDs) == 0 {
		message.AttachmentAssetIDs = JSON([]byte("[]"))
	}
	if len(message.Response) == 0 {
		message.Response = JSON([]byte("{}"))
	}
	if !json.Valid(message.AttachmentAssetIDs) || !json.Valid(message.Response) {
		return ProductAIMessage{}, errors.New("invalid ai message json")
	}
	if err := r.db.WithContext(ctx).Create(&message).Error; err != nil {
		return ProductAIMessage{}, err
	}
	return message, nil
}

func (r *Repository) UpdateProductAIMessage(ctx context.Context, id int64, status string, response JSON) error {
	return r.db.WithContext(ctx).Model(&ProductAIMessage{}).Where("id = ?", id).Updates(map[string]any{"status": status, "response": response}).Error
}

func (r *Repository) TouchProductAIDialogue(ctx context.Context, dialogueID int64) error {
	return r.db.WithContext(ctx).Model(&ProductAIDialogue{}).Where("id = ?", dialogueID).Update("updated_at", time.Now().UTC()).Error
}

func (r *Repository) BumpProductAIRevision(ctx context.Context, productID, ownerID int64) error {
	result := r.db.WithContext(ctx).Model(&ImportedProduct{}).
		Where("id = ? AND EXISTS (SELECT 1 FROM estimates WHERE estimates.id = imported_products.estimate_id AND estimates.owner_id = ?)", productID, ownerID).
		Updates(map[string]any{"ai_data_revision": gorm.Expr("ai_data_revision + 1"), "updated_by": ownerID, "updated_at": time.Now().UTC()})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrNotFound
	}
	return nil
}

func (r *Repository) UpdateProductTNVEDCode(ctx context.Context, productID, ownerID int64, code, source string) (ImportedProduct, error) {
	result := r.db.WithContext(ctx).Model(&ImportedProduct{}).
		Where("id = ? AND EXISTS (SELECT 1 FROM estimates WHERE estimates.id = imported_products.estimate_id AND estimates.owner_id = ?)", productID, ownerID).
		Updates(map[string]any{"tn_ved_code": code, "tn_ved_code_source": source, "updated_by": ownerID, "updated_at": time.Now().UTC()})
	if result.Error != nil {
		return ImportedProduct{}, result.Error
	}
	if result.RowsAffected == 0 {
		return ImportedProduct{}, ErrNotFound
	}
	product, err := r.ProductByIDForOwner(ctx, productID, ownerID)
	if err != nil {
		return ImportedProduct{}, err
	}
	assets, err := r.ImageAssetIDsForProducts(ctx, []int64{productID})
	if err != nil {
		return ImportedProduct{}, err
	}
	product.ImageAssetIDs = assets[productID]
	return product, nil
}
