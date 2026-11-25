package sleeper

import "time"

func UntilUsing(target time.Time, sleep func(time.Duration)) {
	target = target.Round(0)
	for {
		remaining := time.Until(target)
		if remaining <= 0 {
			return
		}
		sleep(remaining)
	}
}

func Until(target time.Time) { UntilUsing(target, time.Sleep) }
