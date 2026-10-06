package directory

import "testing"

func TestDirectoryCellParsers(t *testing.T) {
	for value, want := range map[string]string{"31.01.2026": "2026-01-31", "2026-01-31": "2026-01-31", "46053": "2026-01-31"} {
		if got, err := cellDate(value); err != nil || got != want {
			t.Errorf("cellDate(%q) = %q, %v", value, got, err)
		}
	}
	if _, err := cellDate("вчера"); err == nil {
		t.Error("text date must be rejected")
	}
}

// Товар хранит свой код ТН ВЭД: обязателен, 10 цифр (пробелы между группами допускаются).
func TestProductCodeValidation(t *testing.T) {
	entry, err := validateProductDirectoryInput(ProductDirectoryInput{Name: "Лопата", Code: "8201 10 000 0"})
	if err != nil || entry.Code != "8201100000" {
		t.Fatalf("entry %q, err %v", entry.Code, err)
	}
	for _, code := range []string{"", "82011000", "820110000a"} {
		if _, err := validateProductDirectoryInput(ProductDirectoryInput{Name: "Лопата", Code: code}); err == nil {
			t.Errorf("код %q должен отклоняться", code)
		}
	}
	if err := checkNomenclatureCode(map[string]string{"8201100000": ""}, "8201300000"); err == nil {
		t.Error("кода нет в номенклатуре — ошибка")
	}
	if err := checkNomenclatureCode(map[string]string{}, "8201300000"); err != nil {
		t.Errorf("номенклатура не загружена — код не проверяется: %v", err)
	}
}
