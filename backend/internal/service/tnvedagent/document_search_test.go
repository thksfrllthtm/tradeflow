package tnvedagent

import (
	"strings"
	"testing"
)

func TestSearchDocumentReturnsFragments(t *testing.T) {
	text := testActText()
	fragments, matches := searchDocument(text, "перчатки трикотажные")
	if matches != 1 || len(fragments) != 1 || !strings.Contains(fragments[0], "перчатки трикотажные с покрытием") || len(fragments[0]) > len(text)/10 {
		t.Fatalf("matches %d fragments %v", matches, fragments)
	}
	if fragments, matches := searchDocument(text, "раздела"); matches < 3000 || len(fragments) != agentSearchFragments {
		t.Fatalf("common word: %d fragments, %d matches", len(fragments), matches)
	}
	long := strings.Repeat("слово ", 2000) + "искомая перчатка " + strings.Repeat("хвост ", 2000)
	if fragment := aroundMatch(long, []string{"перчат"}, 300); !strings.Contains(fragment, "искомая перчатка") || len([]rune(fragment)) > 302 {
		t.Fatalf("around match: %q", fragment)
	}
}
