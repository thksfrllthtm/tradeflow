package xls

import "testing"

// RK-числа из настоящего инвойса (.xls): Excel сам выбирает, как хранить число с двумя знаками —
// как double или как целое ×100. Второй вид раньше читался нулём.
func TestRKNumberDecodesAllFourKinds(t *testing.T) {
	cases := []struct {
		rk   RKNumber
		want float64
		name string
	}{
		{0x41307911, 10795.68, "double ×1/100"},
		{0x00469bab, 11568.42, "integer ×1/100"},
		{0x0000003e, 15, "integer"},
		{0x40590000, 100, "double"},
		{0xfffffff6, -3, "negative integer"},
	}
	for _, c := range cases {
		got := c.rk.Float64()
		if c.rk.IsInteger() {
			got = float64(c.rk.Int())
		}
		if got != c.want {
			t.Errorf("%s (%08x): got %v, want %v", c.name, uint32(c.rk), got, c.want)
		}
	}
}
