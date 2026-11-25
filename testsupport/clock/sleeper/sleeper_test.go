package sleeper

import (
	"testing"
	"time"
)

func TestSleeperNeverReturnsBeforeTheWallClockReachesTheTargetEvenWhenASleepEndsEarly(t *testing.T) {
	t.Parallel()
	target := time.Now().Add(50 * time.Millisecond)
	UntilUsing(target, func(d time.Duration) { time.Sleep(d / 2) })
	if now, wall := time.Now().Round(0), target.Round(0); now.Before(wall) {
		t.Fatalf("returned %s before the target", wall.Sub(now))
	}
}
