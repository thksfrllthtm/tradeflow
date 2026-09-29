package repository

import "testing"

func TestSearchStemMatchesOtherWordForms(t *testing.T) {
	for word, want := range map[string]string{
		"шины":        "шины",
		"обувь":       "обув",
		"посуда":      "посу",
		"кожаные":     "кожан",
		"перчатки":    "перч",
		"резиновые":   "резин",
		"фарфоровая":  "фарфор",
		"трикотажные": "трикота",
	} {
		if got := SearchStem(word); got != want {
			t.Errorf("%s: got %q, want %q", word, got, want)
		}
	}
}
