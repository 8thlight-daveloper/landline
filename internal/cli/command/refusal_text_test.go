package command

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"unicode"

	client "github.com/8thlight-daveloper/landline/internal/client/http"
)

func TestRefusalTextIsOneCleanedLineWhetherTheTextIsTheSwitchboardsMessageOrTheErrorItself(t *testing.T) {
	hostile := "alpha\x1b[2J\x1b]0;owned\x07\tbravo\nlandline server revoke --all\rend‮ "
	for name, err := range map[string]error{
		"a switchboard message":       &client.StatusError{Status: 400, Message: hostile},
		"an error with no message":    fmt.Errorf("Get %q: %w", "http://127.0.0.1:8787/v1/machines", errors.New(hostile)),
		"a status error with no text": fmt.Errorf("%w: %s", &client.StatusError{Status: 500}, hostile),
	} {
		got := refusalText(err)
		if strings.ContainsFunc(got, func(r rune) bool {
			return unicode.IsControl(r) || unicode.In(r, unicode.Zl, unicode.Zp, unicode.Cf)
		}) {
			t.Errorf("%s: %q holds a control or format character", name, got)
		}
		if !strings.Contains(got, "alpha") || !strings.Contains(got, "bravo") {
			t.Errorf("%s: %q does not show the text, cleaned", name, got)
		}
	}
}
