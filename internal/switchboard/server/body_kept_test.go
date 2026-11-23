package server

import (
	"testing"

	"github.com/8thlight-daveloper/landline/internal/model"
)

const bodyWithControlCharacters = "bad\x1b]0;PWNED\x07\x1b[2J code\r\x00\x7f‮ end"

func TestTheSwitchboardStoresAndDeliversAMessageBodyWithControlCharactersExactlyAsItCame(t *testing.T) {
	t.Parallel()
	h := start(t)
	alpha := h.register("alpha")
	bravo := h.register("bravo")
	stream := h.openStream(bravo)

	delivery, result := h.deliveredSend(alpha, stream, bravo, model.SendBody{To: "bravo", Body: bodyWithControlCharacters})

	if result.status != 200 {
		t.Fatalf("send: %d %s", result.status, result.raw)
	}
	if delivery.Message == nil || delivery.Message.Body != bodyWithControlCharacters {
		t.Errorf("the delivered body is not the body that was sent: %+v", delivery.Message)
	}
	logged := h.logged()
	if len(logged) != 1 || logged[0].Body != bodyWithControlCharacters {
		t.Errorf("the logged body is not the body that was sent: %+v", logged)
	}
}
