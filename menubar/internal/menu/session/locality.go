package session

import (
	"sync"
	"time"

	"github.com/8thlight-daveloper/landline/menubar/internal/hubclient"
)

const localityLifetime = time.Minute

type answer struct {
	local bool
	at    time.Time
}

func remembered(ask func(string) bool, now func() time.Time, lifetime time.Duration) func(string) bool {
	var mu sync.Mutex
	answers := map[string]answer{}
	return func(address string) bool {
		mu.Lock()
		known, ok := answers[address]
		mu.Unlock()
		if ok && now().Sub(known.at) < lifetime {
			return known.local
		}
		local := ask(address)
		mu.Lock()
		answers[address] = answer{local: local, at: now()}
		mu.Unlock()
		return local
	}
}

func thisMachinesRemembered() func(string) bool {
	return remembered(hubclient.ThisMachines, time.Now, localityLifetime)
}
