package command

import (
	"fmt"
	"time"
)

var SwitchboardFlags = []string{"--addr", "--log", "--host", "--keepalive", "--delivery-timeout", "--grace", "--invite-lifetime", "--signature-window"}

func FlagValues(args []string, allowed ...string) (map[string]string, error) {
	values := map[string]string{}
	for i := 0; i < len(args); i += 2 {
		known := false
		for _, name := range allowed {
			known = known || args[i] == name
		}
		if !known || i+1 >= len(args) {
			return nil, fmt.Errorf("unknown or incomplete flag %s", args[i])
		}
		values[args[i]] = args[i+1]
	}
	return values, nil
}

type Timing struct {
	Keepalive, DeliveryTimeout, Grace, InviteLifetime, SignatureWindow time.Duration
}

const (
	shortestSignatureWindow = time.Second
	longestSignatureWindow  = 5 * time.Second
)

func signatureWindowInRange(window time.Duration) bool {
	return shortestSignatureWindow <= window && window <= longestSignatureWindow
}

func TimingFrom(values map[string]string) (Timing, error) {
	var out Timing
	for flag, target := range map[string]*time.Duration{"--keepalive": &out.Keepalive, "--delivery-timeout": &out.DeliveryTimeout, "--grace": &out.Grace, "--invite-lifetime": &out.InviteLifetime, "--signature-window": &out.SignatureWindow} {
		raw, given := values[flag]
		if !given {
			continue
		}
		d, err := time.ParseDuration(raw)
		if err != nil || (d <= 0 && flag != "--signature-window") {
			return Timing{}, fmt.Errorf("%s needs a positive duration such as 200ms or 15s, not %q", flag, raw)
		}
		*target = d
	}
	return out, nil
}

func Or(value, fallback string) string {
	if value != "" {
		return value
	}
	return fallback
}
