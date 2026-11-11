package store

import (
	"os"
	"path/filepath"
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
