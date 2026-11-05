package tnvedagent

import (
	"testing"

	"github.com/east-trans-logistic/import-calculator/backend/internal/repository"
)

// Открыть доступ — автор или администратор и только к чату на главной; закрыть себе может и получатель.
func TestShareRules(t *testing.T) {
	estimateID := int64(5)
	chat := repository.TNVEDAgentSession{ID: 1, UserID: 10, Mode: modeReference}
	row := repository.TNVEDAgentSession{ID: 2, UserID: 10, Mode: "row", EstimateID: &estimateID}
	author := repository.User{ID: 10, Role: repository.RoleInvoicer}
	colleague := repository.User{ID: 11, Role: repository.RoleInvoicer}
	admin := repository.User{ID: 1, Role: repository.RoleAdmin}
	cases := []struct {
		name    string
		session repository.TNVEDAgentSession
		user    repository.User
		target  int64
		share   bool
		unshare bool
	}{
		{"автор открывает и закрывает", chat, author, 11, true, true},
		{"администратор открывает и закрывает", chat, admin, 11, true, true},
		{"получатель не открывает другим и не закрывает другим", chat, colleague, 12, false, false},
		{"получатель убирает вопрос у себя", chat, colleague, 11, false, true},
		{"подбор по строке сметы не делится даже автором", row, author, 11, false, false},
		{"и администратором", row, admin, 11, false, false},
		{"получатель строки сметы себе тоже не закрывает", row, colleague, 11, false, false},
	}
	for _, tc := range cases {
		if got := canShare(tc.session, tc.user); got != tc.share {
			t.Errorf("%s: открыть %v, ожидалось %v", tc.name, got, tc.share)
		}
		if got := canUnshare(tc.session, tc.user, tc.target); got != tc.unshare {
			t.Errorf("%s: закрыть %v, ожидалось %v", tc.name, got, tc.unshare)
		}
	}
}
