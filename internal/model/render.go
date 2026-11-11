package model

import "fmt"

func Render(e Envelope, from Agent) string {
	header := fmt.Sprintf("[landline] %s\nfrom: %s (%s on %s) thread: %s kind: %s id: %s", Marker, e.From, from.Harness, from.Host, e.Thread, e.Kind, e.ID)
	if e.InReplyTo != "" {
		header += " in-reply-to: " + e.InReplyTo
	}
	return header + "\n\n" + e.Body
}
