package pacing

import (
	"fmt"
	"time"
)

const (
	DefaultInterval = 2 * time.Second
	Floor           = 500 * time.Millisecond
)

func CheckInterval(interval time.Duration) error {
	if interval < Floor {
		return fmt.Errorf("the interval is at least %s, not %s", Floor, interval)
	}
	return nil
}

func NextWait(interval, took time.Duration) time.Duration {
	if took > interval {
		return 2 * took
	}
	return interval
}

func Measure(now func() time.Time, run func()) time.Duration {
	start := now()
	run()
	return now().Sub(start)
}
