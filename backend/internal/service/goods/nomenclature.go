package goods

import (
	"context"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service"
	"github.com/east-trans-logistic/import-calculator/backend/internal/service/calculation"
)

// loadNomenclatureRates читает номенклатуру для всех кодов сметы одним запросом.
func (s *GoodsService) loadNomenclatureRates(ctx context.Context, codes []string) (calculation.NomenclatureRates, error) {
	rates := calculation.NomenclatureRates{ByCode: map[string]repository.TNVEDNomenclatureCode{}}
	latest, err := s.repository.LatestTNVEDNomenclatureImport(ctx)
	if service.IsNotFound(err) {
		return rates, nil
	}
	if err != nil {
		return rates, err
	}
	rates.Loaded = true
	if latest.ActualDate != nil {
		rates.ActualDate = latest.ActualDate.Format("02.01.2006")
	}
	rows, err := s.repository.TNVEDNomenclatureByCodes(ctx, codes)
	if err != nil {
		return rates, err
	}
	for _, row := range rows {
		rates.ByCode[row.Code] = row
	}
	return rates, nil
}
