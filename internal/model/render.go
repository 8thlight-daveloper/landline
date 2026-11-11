package model

import (
	"fmt"
	"strings"
	"unicode"
)

func oneLine(text string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.In(r, unicode.Zl, unicode.Zp, unicode.Cf) {
			return ' '
		}
		return r
	}, text)
}

func Render(e Envelope, from Agent) string {
	header := fmt.Sprintf("[landline] %s\nfrom: %s (%s on %s) thread: %s kind: %s id: %s",
		Marker, oneLine(e.From), oneLine(from.Harness), oneLine(from.Host), oneLine(e.Thread), oneLine(string(e.Kind)), oneLine(e.ID))
	if e.InReplyTo != "" {
		header += " in-reply-to: " + oneLine(e.InReplyTo)
	}
	return header + "\n\n" + e.Body
}
