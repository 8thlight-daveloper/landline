package pacing

import (
	"testing"
	"time"
)

func TestCheckInterval(t *testing.T) {
	cases := map[time.Duration]bool{
		-time.Second: false, 0: false, 499 * time.Millisecond: false,
		Floor: true, 7 * time.Second: true,
	}
	for interval, ok := range cases {
		if err := CheckInterval(interval); (err == nil) != ok {
			t.Errorf("%s: %v", interval, err)
		}
	}
}

func TestNextWaitKeepsTheIntervalUnlessTheRequestWasSlower(t *testing.T) {
	if got := NextWait(5*time.Second, 5*time.Second); got != 5*time.Second {
		t.Errorf("an equal request: %s", got)
	}
	if got := NextWait(5*time.Second, 6*time.Second); got != 12*time.Second {
		t.Errorf("a slow request: %s", got)
	}
}

func TestTheDefaultIsFiveSeconds(t *testing.T) {
	if DefaultInterval != 5*time.Second {
		t.Fatal(DefaultInterval)
	}
}
