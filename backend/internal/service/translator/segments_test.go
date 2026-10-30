package translator

import (
	"strings"
	"testing"
)

func TestValidateTargetRejectsBlankAndTooLong(t *testing.T) {
	for _, tc := range []struct {
		target string
		ok     bool
	}{
		{"Перевод", true},
		{"  отступ сохраняется  ", true},
		{"", false},
		{"   ", false},
		{"\t\n  ", false},
		{strings.Repeat("я", maxTargetRunes), true},
		{strings.Repeat("я", maxTargetRunes+1), false},
	} {
		if err := validateTarget(tc.target); (err == nil) != tc.ok {
			t.Errorf("%q: err=%v, want ok=%v", tc.target, err, tc.ok)
		}
	}
}
