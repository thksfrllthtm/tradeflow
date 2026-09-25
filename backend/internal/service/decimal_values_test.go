package service

import "testing"

func TestParseDecimalSeparators(t *testing.T) {
	for input, want := range map[string]string{
		"4.125":    "4.125",
		"4,125":    "4.125",
		"1580.125": "1580.125",
		"0.125":    "0.125",
		"1 500":    "1500",
		"1 500,50": "1500.5",
		"1 500":    "1500", // неразрывный пробел
		"12":       "12",
		"-2.5":     "-2.5",
	} {
		got, err := parseDecimal(input)
		if err != nil || got.String() != want {
			t.Fatalf("%q: want %s, got %s (%v)", input, want, got.String(), err)
		}
	}
	for _, input := range []string{"1,500.50", "1.500.000", "1,2,3", "4.", ".5", "abc", ""} {
		if _, err := parseDecimal(input); err == nil {
			t.Fatalf("%q must be rejected", input)
		}
	}
}
