package command

import (
	"testing"
	"time"
)

func TestTimingFlagsMapToTheirDurations(t *testing.T) {
	t.Parallel()
	got, err := TimingFrom(map[string]string{"--keepalive": "200ms", "--delivery-timeout": "3s", "--grace": "1m"})
	if err != nil || got.Keepalive != 200*time.Millisecond || got.DeliveryTimeout != 3*time.Second || got.Grace != time.Minute {
		t.Fatalf("%+v %v", got, err)
	}
	if got, err := TimingFrom(map[string]string{}); err != nil || got != (Timing{}) {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestTimingFlagsRefuseNonPositiveAndUnparsableDurations(t *testing.T) {
	t.Parallel()
	for _, flag := range []string{"--keepalive", "--delivery-timeout", "--grace"} {
		for _, bad := range []string{"0", "0s", "-1s", "soon", "10", ""} {
			if _, err := TimingFrom(map[string]string{flag: bad}); err == nil {
				t.Errorf("%s accepted %q", flag, bad)
			}
		}
	}
}
