package commonxl

import "testing"

// Числа из .xls попадают в программу строками через эти форматы, поэтому они не должны терять
// знаки: General — до 11 значащих цифр (как показывает Excel), целочисленные форматы — округление.
func TestFormatsKeepDigitsLikeExcel(t *testing.T) {
	var f Formatter
	cases := []struct {
		fmtID uint16
		value interface{}
		want  string
	}{
		{0, 11568.42, "11568.42"},
		{0, 12345678.912, "12345678.912"}, // раньше 1.23457e+07
		{0, 102.00000000000001, "102"},    // шум double после формул
		{0, 844.99999999999989, "845"},    // шум double после формул
		{0, 6403911800.0, "6403911800"},   // код ТН ВЭД, записанный числом
		{0, int64(2737), "2737"},
		{1, 172.5, "173"},         // формат «0»: Excel округляет, а не отбрасывает
		{2, 11568.42, "11568.42"}, // формат «0.00»
	}
	for _, c := range cases {
		got, _ := f.Apply(c.fmtID, c.value)
		if got != c.want {
			t.Errorf("format %d of %v: got %q, want %q", c.fmtID, c.value, got, c.want)
		}
	}
}
