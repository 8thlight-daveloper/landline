package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/8thlight-daveloper/landline/internal/model"
)

func envelope(seq int64) model.Envelope {
	return model.Envelope{Seq: seq, ID: "m", Time: time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC), Thread: "main", From: "alpha", To: "bravo", Kind: model.KindChat, Body: "hi"}
}

func TestAppendedEnvelopesSurviveAReopen(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "log.jsonl")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, seq := range []int64{1, 2} {
		if err := s.Append(envelope(seq)); err != nil {
			t.Fatal(err)
		}
	}
	s.Close()
	again, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if got := again.All(); len(got) != 2 || got[1].Seq != 2 || !got[0].Time.Equal(envelope(1).Time) || again.LastSeq() != 2 {
		t.Fatalf("got %+v", got)
	}
}

func TestATornLastLineIsSkipped(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "log.jsonl")
	s, _ := Open(path)
	s.Append(envelope(1))
	s.Close()
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	f.WriteString(`{"Seq":2,"Bo`)
	f.Close()
	again, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if got := again.All(); len(got) != 1 {
		t.Fatalf("got %+v", got)
	}
}

func TestAnEmptyStoreHasNoLastSeq(t *testing.T) {
	t.Parallel()
	s, _ := Open(filepath.Join(t.TempDir(), "sub", "log.jsonl"))
	defer s.Close()
	if s.LastSeq() != 0 || len(s.All()) != 0 {
		t.Fatal("not empty")
	}
}

func TestTheLogFileIsPrivate(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "log.jsonl")
	s, _ := Open(path)
	defer s.Close()
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", info.Mode().Perm())
	}
}

func TestAnExistingLooseLogFileIsMadePrivate(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "log.jsonl")
	if err := os.WriteFile(path, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	info, _ := os.Stat(path)
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode %v", info.Mode().Perm())
	}
}

func TestAnEnvelopeWhoseEncodedLineIsHugeStillLoads(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "log.jsonl")
	s, _ := Open(path)
	big := envelope(1)
	big.Body = strings.Repeat("<", model.MaxBodyBytes)
	if err := s.Append(big); err != nil {
		t.Fatal(err)
	}
	control := envelope(2)
	control.Body = strings.Repeat("\x01", model.MaxBodyBytes)
	if err := s.Append(control); err != nil {
		t.Fatal(err)
	}
	s.Close()
	again, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer again.Close()
	if got := again.All(); len(got) != 2 || got[1].Seq != 2 || len(got[0].Body) != model.MaxBodyBytes {
		t.Fatalf("got %d envelopes", len(got))
	}
}

func TestALogThatEndsWithoutANewlineDoesNotGlueTheNextEntryToIt(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "log.jsonl")
	first, _ := Open(path)
	first.Append(envelope(1))
	first.Close()
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	line, _ := json.Marshal(envelope(2))
	f.Write(line)
	f.Close()
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Append(envelope(3)); err != nil {
		t.Fatal(err)
	}
	s.Close()
	again, _ := Open(path)
	defer again.Close()
	got := again.All()
	if len(got) != 3 || got[0].Seq != 1 || got[1].Seq != 2 || got[2].Seq != 3 {
		t.Fatalf("got %+v", got)
	}
}

func TestATornLastLineDoesNotCostTheNextEntry(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "log.jsonl")
	first, _ := Open(path)
	first.Append(envelope(1))
	first.Close()
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	f.WriteString(`{"seq":2,"bo`)
	f.Close()
	s, _ := Open(path)
	s.Append(envelope(3))
	s.Close()
	again, _ := Open(path)
	defer again.Close()
	got := again.All()
	if len(got) != 2 || got[0].Seq != 1 || got[1].Seq != 3 {
		t.Fatalf("got %+v", got)
	}
}

func TestTheLogUsesSnakeCaseKeys(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "log.jsonl")
	s, _ := Open(path)
	e := envelope(1)
	e.InReplyTo = "m-0"
	s.Append(e)
	s.Close()
	raw, _ := os.ReadFile(path)
	for _, key := range []string{`"seq":1`, `"id":"m"`, `"thread":"main"`, `"from":"alpha"`, `"to":"bravo"`, `"kind":"chat"`, `"body":"hi"`, `"in_reply_to":"m-0"`, `"time":`} {
		if !strings.Contains(string(raw), key) {
			t.Errorf("the log line lacks %s: %s", key, raw)
		}
	}
}
