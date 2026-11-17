package model

import (
	"strings"
	"testing"
)

func TestTheErrorForABadIDSaysAgentIDAndNotAgentName(t *testing.T) {
	t.Parallel()
	text := ErrBadName.Error()
	if !strings.HasPrefix(text, "agent id ") || strings.Contains(text, "name") {
		t.Fatalf("got %q", text)
	}
	for _, rule := range []string{"1 to 32", "lowercase letters, digits, - or _", `"all"`} {
		if !strings.Contains(text, rule) {
			t.Errorf("the error lacks %q: %q", rule, text)
		}
	}
}
