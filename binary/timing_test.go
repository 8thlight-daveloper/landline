package main

import (
	"testing"
	"time"
)

func TestTimingFlagsMapToTheirDurations(t *testing.T) {
	t.Parallel()
	got, err := timingFrom(map[string]string{"--keepalive": "200ms", "--delivery-timeout": "3s", "--retention": "1m"})
	if err != nil || got.keepalive != 200*time.Millisecond || got.deliveryTimeout != 3*time.Second || got.retention != time.Minute {
		t.Fatalf("%+v %v", got, err)
	}
	if got, err := timingFrom(map[string]string{}); err != nil || got != (hubTiming{}) {
		t.Fatalf("%+v %v", got, err)
	}
}

func TestTimingFlagsRefuseNonPositiveAndUnparsableDurations(t *testing.T) {
	t.Parallel()
	for _, flag := range []string{"--keepalive", "--delivery-timeout", "--retention"} {
		for _, bad := range []string{"0", "0s", "-1s", "soon", "10", ""} {
			if _, err := timingFrom(map[string]string{flag: bad}); err == nil {
				t.Errorf("%s accepted %q", flag, bad)
			}
		}
	}
}
