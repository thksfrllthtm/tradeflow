package tnvedagent

import (
	"testing"
	"time"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
)

func flagText(value *bool) string {
	switch {
	case value == nil:
		return "-"
	case *value:
		return "да"
	}
	return "нет"
}

// Согласие — только когда у обеих моделей есть код; правота — только по проверенной записи.
func TestCompareCodes(t *testing.T) {
	for _, c := range []struct {
		main, shadow, verifiedCode string
		verified                   bool
		want                       string
	}{
		{"6116102000", "6116102000", "6116102000", true, "да да да"},
		{"6116102000", "4015190000", "4015190000", true, "нет нет да"},
		{"6116102000", "", "6116102000", true, "- да -"},
		{"", "4015190000", "4015190000", true, "- - да"},
		{"6116102000", "4015190000", "4015190000", false, "нет - -"},
		{"", "", "", false, "- - -"},
	} {
		agree, mainCorrect, shadowCorrect := compareCodes(c.main, c.shadow, c.verifiedCode, c.verified)
		if got := flagText(agree) + " " + flagText(mainCorrect) + " " + flagText(shadowCorrect); got != c.want {
			t.Fatalf("%+v: %s", c, got)
		}
	}
}

// Строка журнала: тень и решение только когда они есть; имя — из наименования, затем поставщика, затем заголовка.
func TestLogRowView(t *testing.T) {
	started := time.Date(2026, 11, 3, 10, 0, 0, 0, time.UTC)
	finished := started.Add(90 * time.Second)
	shadowID, decisionID := int64(3), int64(9)
	view := logRowView(repository.AgentLogRow{ID: 1, UserEmail: "a@b.c", Title: "вопрос", BestCode: "6116102000", StartedAt: &started, FinishedAt: &finished,
		ShadowID: &shadowID, ShadowStatus: "done", ShadowBestCode: "6116102000", ShadowStartedAt: &started,
		DecisionID: &decisionID, DecisionCode: "6116102000", DecisionStatus: "pending"})
	if view.User != "a@b.c" || view.Name != "вопрос" || view.Main.Seconds == nil || *view.Main.Seconds != 90 || view.Shadow == nil || view.Shadow.Seconds != nil ||
		view.Decision == nil || view.Decision.Status != "pending" || flagText(view.Agree) != "да" || view.MainCorrect != nil {
		t.Fatalf("строка: %+v", view)
	}
	bare := logRowView(repository.AgentLogRow{ID: 2, UserName: "Айгерим", ItemName: "Перчатки"})
	if bare.User != "Айгерим" || bare.Name != "Перчатки" || bare.Shadow != nil || bare.Decision != nil || bare.Agree != nil || bare.Main.Seconds != nil {
		t.Fatalf("без тени и решения: %+v", bare)
	}
}
