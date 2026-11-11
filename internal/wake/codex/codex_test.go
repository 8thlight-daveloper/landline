package codex

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
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
	if w.Wake("sess-1", "no marker") == nil || w.Wake("", "[landline] From another agent, not the user.\nx") == nil {
		t.Fatal("expected refusals")
	}
	if called {
		t.Fatal("codex ran")
	}
}

func TestWakeReportsAFailureWithItsOutput(t *testing.T) {
	t.Parallel()
	w := NewWith(func(string, ...string) ([]byte, error) { return []byte("no such thread\n"), errors.New("exit 1") })
	err := w.Wake("sess-1", "[landline] From another agent, not the user.\nx")
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

func TestAHungCodexIsStoppedAtTheTimeout(t *testing.T) {
	script := t.TempDir() + "/codex"
	if err := os.WriteFile(script, []byte("#!/bin/sh\nsleep 60\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", filepath.Dir(script)+string(os.PathListSeparator)+os.Getenv("PATH"))
	started := time.Now()
	err := withTimeout(300*time.Millisecond).Wake("thread-1", "[landline] From another agent, not the user.\nx")
	if err == nil || time.Since(started) > 10*time.Second {
		t.Fatalf("got %v after %v", err, time.Since(started))
	}
}
