package directory

import "testing"

func TestDirectoryValidationKeepsZeroAndParsesComma(t *testing.T) {
	entry, err := validateDirectoryInput(TNVEDDirectoryInput{
		Category: "Перчатки", Name: "Перчатки рабочие", Code: "0012345678",
		Duty: RateInput{Type: "percentage", Value: "12,5"}, VAT: RateInput{Type: "percentage", Value: "0"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry.DutyPercent == nil || entry.DutyPercent.String() != "12.5" {
		t.Fatalf("unexpected duty value: %v", entry.DutyPercent)
	}
	if entry.VATPercent == nil || !entry.VATPercent.IsZero() {
		t.Fatalf("explicit zero VAT was not preserved: %v", entry.VATPercent)
	}
}

func TestDirectoryValidationDistinguishesUnknown(t *testing.T) {
	entry, err := validateDirectoryInput(TNVEDDirectoryInput{Category: "Перчатки", Name: "Перчатки", Code: "0012345678", Duty: RateInput{Type: "unknown"}, VAT: RateInput{Type: "unknown"}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry.DutyPercent != nil || entry.VATPercent != nil {
		t.Fatal("unknown rates must remain nil")
	}
}

func TestDirectoryValidationRequiresTenDigitCode(t *testing.T) {
	_, err := validateDirectoryInput(TNVEDDirectoryInput{Category: "Перчатки", Name: "Перчатки", Code: "123", Duty: RateInput{Type: "unknown"}, VAT: RateInput{Type: "unknown"}})
	if err == nil {
		t.Fatal("expected code validation error")
	}
}

func TestDirectoryValidationSupportsStructuredRatesAndCoefficient(t *testing.T) {
	entry, err := validateDirectoryInput(TNVEDDirectoryInput{
		Category: "Перчатки", Name: "Перчатки резиновые", NameZH: "橡胶手套", Code: "4015190000",
		CompanyCoefficient: "0,65",
		Duty:               RateInput{Type: "combined_minimum", Percent: "10", Amount: "2,00", Currency: "eur", Unit: "пара", ValidFrom: "2026-01-01", CheckedAt: "2026-02-01", Source: "введено оператором"},
		VAT:                RateInput{Type: "exempt", Basis: "основание проверит оператор", CheckedAt: "2026-02-02"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if entry.CompanyCoefficient == nil || entry.CompanyCoefficient.String() != "0.65" {
		t.Fatalf("unexpected coefficient: %v", entry.CompanyCoefficient)
	}
	if entry.DutyPercent == nil || entry.DutyPercent.String() != "10" || entry.DutyAmount == nil || entry.DutyAmount.String() != "2" || entry.DutyCurrency != "EUR" || entry.DutyUnit != "пара" {
		t.Fatalf("structured duty was not preserved: %#v", entry)
	}
	if entry.DutyCheckedAt == nil || entry.DutyCheckedAt.Format("2006-01-02") != "2026-02-01" || entry.VATCheckedAt == nil || entry.VATCheckedAt.Format("2006-01-02") != "2026-02-02" {
		t.Fatalf("rate check dates were not preserved: %#v", entry)
	}
	if entry.VATType != "exempt" || entry.VATBasis == "" {
		t.Fatalf("VAT exemption was not preserved: %#v", entry)
	}
}

func TestDirectoryValidationRequiresExemptionBasis(t *testing.T) {
	_, err := validateDirectoryInput(TNVEDDirectoryInput{Category: "Перчатки", Name: "Перчатки", Code: "4015190000", Duty: RateInput{Type: "unknown"}, VAT: RateInput{Type: "exempt"}})
	if err == nil {
		t.Fatal("expected exemption basis validation error")
	}
}
