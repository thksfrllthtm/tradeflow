package service

// XLSX export of a saved estimate calculation (preliminary, with the revision totals).

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

func (s *CalculationService) Export(ctx context.Context, estimateID, ownerID int64) (*bytes.Buffer, error) {
	result, err := s.Get(ctx, estimateID, ownerID)
	if err != nil {
		return nil, err
	}
	book := excelize.NewFile()
	sheet := book.GetSheetName(0)
	headers := []string{"Предварительный расчёт", "Поставщик", "Название", "Код ТН ВЭД", "Фото URL", "Пары", "Места", "Пар в месте", "Брутто, кг", "Нетто, кг", "Цена пары", "Стоимость", "Валюта", "Доп. расходы", "ТС напрямую, KZT", "Таможенная стоимость, KZT", "Пошлина, KZT", "Сбор, KZT", "База НДС, KZT", "НДС, KZT", "Доставка, USD", "Доставка, KZT", "Статус", "Ошибки", "Предупреждения"}
	for col, header := range headers {
		cell, _ := excelize.CoordinatesToCellName(col+1, 1)
		_ = book.SetCellValue(sheet, cell, header)
	}
	for row, item := range result.Items {
		values := []any{"PRELIMINARY", item.SupplierName, item.StandardName, item.Code, item.PhotoURL, item.Pairs, item.Places, item.PairsPerPlace, item.GrossKG, item.NetKG, item.PricePerPair, item.CostInput, item.CostCurrency, item.ExtraExpenses, item.CustomsValueDirectKZT, item.CustomsValueKZT, item.DutyKZT, item.FeeKZT, item.VATBaseKZT, item.VATKZT, item.DeliveryUSD, item.DeliveryKZT, item.Status, strings.Join(item.Errors, "; "), strings.Join(item.Warnings, "; ")}
		for col, value := range values {
			cell, _ := excelize.CoordinatesToCellName(col+1, row+2)
			_ = book.SetCellValue(sheet, cell, value)
		}
	}
	metaRow := len(result.Items) + 3
	_ = book.SetCellValue(sheet, fmt.Sprintf("A%d", metaRow), "Итоги сохранённой ревизии")
	// Keep the export labels and values aligned with the three totals shown in
	// the screen. Goods value is deliberately not part of the cash total.
	_ = book.SetCellValue(sheet, fmt.Sprintf("B%d", metaRow), fmt.Sprintf("Растаможка (сбор + пошлина + НДС): %s KZT", result.Totals.CustomsPaymentsKZT))
	_ = book.SetCellValue(sheet, fmt.Sprintf("C%d", metaRow), fmt.Sprintf("Доставка: %s USD / %s KZT", result.Totals.TotalDeliveryUSD, result.Totals.DeliveryKZT))
	_ = book.SetCellValue(sheet, fmt.Sprintf("D%d", metaRow), fmt.Sprintf("Всего из кассы: %s KZT", result.Totals.ExpensesKZT))
	_ = book.SetCellValue(sheet, fmt.Sprintf("E%d", metaRow), fmt.Sprintf("Таможенная стоимость: %s KZT", result.Totals.CustomsValueKZT))
	_ = book.SetCellValue(sheet, fmt.Sprintf("F%d", metaRow), fmt.Sprintf("Статус: рассчитано %d из %d", result.Totals.CalculatedCount, result.Totals.TotalCount))
	_ = book.SetCellValue(sheet, fmt.Sprintf("G%d", metaRow), fmt.Sprintf("Формула: %s; округление: %s", result.FormulaVersion, result.RoundingPolicy))
	_ = book.SetCellValue(sheet, fmt.Sprintf("H%d", metaRow), fmt.Sprintf("Пошлина: %s KZT; НДС: %s KZT; Сбор: %s KZT", result.Totals.DutyKZT, result.Totals.VATKZT, result.Totals.FeeKZT))
	return book.WriteToBuffer()
}
