package httpapi

import (
	"reflect"
	"testing"

	"github.com/east-trans-logistic/import-calculator/backend/internal/service/antidumping"
)

// Новая мера, снятая и мера с другой ставкой попадают каждая в свой список, неизменная — никуда.
func TestDiffMeasures(t *testing.T) {
	before := []antidumping.Measure{
		{Number: "AD-1", ShortName: "Трубы", RateText: "10%"},
		{Number: "AD-2", ShortName: "Гвозди", RateText: "5%"},
		{Number: "AD-3", ShortName: "Посуда", RateText: "7%"},
	}
	after := []antidumping.Measure{
		{Number: "AD-1", ShortName: "Трубы", RateText: "10%"},
		{Number: "AD-3", ShortName: "Посуда", RateText: "9%"},
		{Number: "AD-4", ShortName: "Шины", RateText: "12%"},
	}
	got := diffMeasures(before, after)
	want := measureChanges{
		MeasureCount: 3,
		Added:        []measureRef{{"AD-4", "Шины"}},
		Removed:      []measureRef{{"AD-2", "Гвозди"}},
		Changed:      []measureRef{{"AD-3", "Посуда"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, want %+v", got, want)
	}
}
