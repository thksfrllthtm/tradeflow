package service

// Classification of calculation messages into blocking errors and warnings.

import (
	"strings"
)

func unsupportedSchemeIssue(issue string) bool {
	return strings.HasPrefix(issue, "Пошлина поддерживается только процентная") ||
		strings.HasPrefix(issue, "НДС поддерживается только процентный")
}

func filterRateWarnings(values []string) []string {
	result := values[:0]
	for _, value := range values {
		if strings.Contains(value, "Пошлина") || strings.Contains(value, "НДС") {
			continue
		}
		result = append(result, value)
	}
	return result
}

func splitCalculationIssues(values []string) (errorsList, warningsList []string) {
	for _, value := range values {
		if isBlockingCalculationIssue(value) {
			errorsList = append(errorsList, value)
		} else {
			warningsList = append(warningsList, value)
		}
	}
	if errorsList == nil {
		errorsList = []string{}
	}
	if warningsList == nil {
		warningsList = []string{}
	}
	return errorsList, warningsList
}

func isBlockingCalculationIssue(value string) bool {
	for _, prefix := range []string{
		"Количество ", "Пар в месте", "Общий вес:", "Вес нетто:",
		"Количество товара ", "Количество товара в одном месте",
		"Вес упаковки:", "Расчётный вес нетто", "Вес нетто не может",
		"Общий вес не может", "Доставка:", "Доставка не может",
		"ИТС:", "Укажите цену за единицу товара", "Для стоимости укажите", "Для брутто укажите",
		"Для нетто укажите", "Не выбрана карточка перчаток",
		"Распределение доставки:", "Стоимость товара не может",
		"Цена за пару не может", "Дополнительные расходы не могут",
		"Цена за единицу не может", "Для стоимости укажите стоимость товара либо количество товара и цену за единицу",
		"Не выбрана карточка ТН ВЭД",
		"Таможенная стоимость в тенге не может", "Для таможенной стоимости нужна",
		"Валюта стоимости не поддерживается", "Для стоимости в EUR укажите",
		"Пошлина поддерживается только процентная", "НДС поддерживается только процентный",
	} {
		if strings.HasPrefix(value, prefix) {
			return true
		}
	}
	return false
}
