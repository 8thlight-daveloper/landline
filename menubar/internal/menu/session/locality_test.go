package session

import (
	"testing"
	"time"
)

func TestARememberedAnswerIsAskedOncePerLifetimeAndPerAddress(t *testing.T) {
	clock := time.Unix(0, 0)
	asked := map[string]int{}
	local := remembered(func(address string) bool { asked[address]++; return address == "http://a:1" }, func() time.Time { return clock }, time.Minute)
	for i := 0; i < 5; i++ {
		if !local("http://a:1") || local("http://b:1") {
			t.Fatal("wrong answer")
		}
	}
	if asked["http://a:1"] != 1 || asked["http://b:1"] != 1 {
		t.Fatalf("asked %v", asked)
	}
	clock = clock.Add(time.Minute)
	local("http://a:1")
	if asked["http://a:1"] != 2 {
		t.Fatalf("after the lifetime: %v", asked)
	}
}
