package pacing

import (
	"testing"
	"time"
)

func TestCheckIntervalEnforcesTheFloor(t *testing.T) {
	if CheckInterval(499*time.Millisecond) == nil {
		t.Fatal("below the floor must fail")
	}
	if err := CheckInterval(500 * time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if CheckInterval(0) == nil || CheckInterval(-time.Second) == nil {
		t.Fatal("zero and negative must fail")
	}
}

func TestNextWaitKeepsTheIntervalForAFastRequest(t *testing.T) {
	interval := 2 * time.Second
	for _, took := range []time.Duration{0, time.Second, interval} {
		if got := NextWait(interval, took); got != interval {
			t.Fatalf("took %s: %s", took, got)
		}
	}
}

func TestNextWaitDoublesTheRequestTimeForASlowRequest(t *testing.T) {
	if got := NextWait(2*time.Second, 3*time.Second); got != 6*time.Second {
		t.Fatalf("got %s", got)
	}
}

func TestMeasureUsesTheInjectedClock(t *testing.T) {
	clock := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := func() time.Time { return clock }
	took := Measure(now, func() { clock = clock.Add(3 * time.Second) })
	if took != 3*time.Second {
		t.Fatalf("got %s", took)
	}
	if NextWait(2*time.Second, took) != 6*time.Second {
		t.Fatal("the measured time feeds the back-off")
	}
}
