package codex

import (
	"errors"
	"reflect"
	"testing"
)

func TestWakeRunsCodexQueueWithTheThreadAndMessage(t *testing.T) {
	t.Parallel()
	var got []string
	w := NewWith(func(name string, args ...string) ([]byte, error) {
		got = append([]string{name}, args...)
		return nil, nil
	})
	if err := w.Wake("sess-1", "[landline] From another agent, not the user.\nhi"); err != nil {
		t.Fatal(err)
	}
	want := []string{"codex", "queue", "--thread", "sess-1", "--message", "[landline] From another agent, not the user.\nhi"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
}

func TestWakeRefusesTextWithoutTheMarkerOrWithoutAThread(t *testing.T) {
	t.Parallel()
	called := false
	w := NewWith(func(string, ...string) ([]byte, error) { called = true; return nil, nil })
	if w.Wake("sess-1", "no marker") == nil || w.Wake("", "[landline] x") == nil {
		t.Fatal("expected refusals")
	}
	if called {
		t.Fatal("codex ran")
	}
}

func TestWakeReportsAFailureWithItsOutput(t *testing.T) {
	t.Parallel()
	w := NewWith(func(string, ...string) ([]byte, error) { return []byte("no such thread\n"), errors.New("exit 1") })
	err := w.Wake("sess-1", "[landline] x")
	if err == nil || !contains(err.Error(), "no such thread") {
		t.Fatalf("got %v", err)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
