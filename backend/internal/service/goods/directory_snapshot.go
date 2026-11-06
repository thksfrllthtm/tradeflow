package goods

// Снимок карточки товара для строки сметы: перечитывается из справочника на сервере.

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
)

func (s *GoodsService) resolveDirectorySnapshot(ctx context.Context, item GoodsItemInput, genericItems bool) (map[string]any, error) {
	if s.repository == nil {
		// Unit tests can exercise the arithmetic without a database. Production
		// servers always provide a repository and take the trusted path below.
		return item.DirectorySnapshot, nil
	}
	if item.ProductDirectoryID == nil || *item.ProductDirectoryID < 1 {
		// Строка без карточки товара: код — из ячейки, ставки — из номенклатуры по коду.
		return nil, nil
	}
	product, err := s.repository.ProductDirectoryEntryByID(ctx, *item.ProductDirectoryID)
	if err != nil {
		if service.IsNotFound(err) {
			return nil, fmt.Errorf("карточка товара не найдена")
		}
		return nil, fmt.Errorf("не удалось загрузить карточку товара")
	}
	if product.ArchivedAt != nil {
		return nil, fmt.Errorf("выбранная карточка товара архивирована")
	}
	if strings.TrimSpace(product.Code) == "" {
		return nil, fmt.Errorf("у выбранного товара не указан код ТН ВЭД")
	}
	if !genericItems && strings.TrimSpace(product.Category) != "Перчатки" {
		return nil, fmt.Errorf("выбранная карточка относится не к категории «Перчатки»")
	}
	// Название, код и ИТС — из карточки товара; пошлина, НДС и антидемпинг — из номенклатуры
	// и реестра ЕЭК по коду (ApplyNomenclatureRates).
	snapshot := map[string]any{
		"category": product.Category, "name": strings.TrimSpace(product.Name), "code": product.Code,
		"verificationStatus": product.VerificationStatus, "selectedAt": time.Now().UTC().Format(time.RFC3339),
		// Отмечает карточку товара: только тогда строка берёт название товара вместо названия из Excel.
		"productDirectoryId": product.ID,
	}
	if strings.TrimSpace(product.NameZH) != "" {
		snapshot["nameZh"] = strings.TrimSpace(product.NameZH)
	}
	if product.CompanyCoefficient != nil {
		snapshot["companyCoefficient"] = service.DecimalText(product.CompanyCoefficient)
	}
	return snapshot, nil
}

func directoryDisplayName(snapshot map[string]any) string {
	russian, _ := snapshot["name"].(string)
	chinese, _ := snapshot["nameZh"].(string)
	russian = strings.TrimSpace(russian)
	chinese = strings.TrimSpace(chinese)
	if russian != "" && chinese != "" {
		return russian + " / " + chinese
	}
	if russian != "" {
		return russian
	}
	return chinese
}
