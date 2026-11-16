package model

import (
	"encoding/json"
	"strings"
	"testing"
)

const (
	testCommit = "0123456789abcdef0123456789abcdef01234567"
	testDate   = "2026-11-12T16:34:00Z"
)

func TestAttributesShowTheShortCommitAndTheDateAfterTheLandlineVersion(t *testing.T) {
	t.Parallel()
	info := AgentInfo{Label: "lead", LandlineVersion: "0.1.0", LandlineCommit: testCommit, LandlineDate: testDate}
	want := "label: lead  landline: 0.1.0  commit: 0123456  built: " + testDate
	if got := info.Attributes(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestAttributesKeepTheDirtyMarkAfterTheShortCommitAndShowNoDate(t *testing.T) {
	t.Parallel()
	info := AgentInfo{LandlineVersion: "0.1.0", LandlineCommit: testCommit + "-dirty"}
	want := "landline: 0.1.0  commit: 0123456-dirty"
	if got := info.Attributes(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestAttributesLeaveOutAnEmptyCommitAndAnEmptyDate(t *testing.T) {
	t.Parallel()
	got := AgentInfo{LandlineVersion: "0.1.0"}.Attributes()
	if got != "landline: 0.1.0" {
		t.Fatalf("got %q", got)
	}
	if got := (AgentInfo{LandlineCommit: testCommit}).Attributes(); got != "commit: 0123456" {
		t.Fatalf("commit alone: %q", got)
	}
	if got := (AgentInfo{LandlineDate: testDate}).Attributes(); got != "built: "+testDate {
		t.Fatalf("date alone: %q", got)
	}
}

func TestAttributesShowAShortCommitWhole(t *testing.T) {
	t.Parallel()
	for _, commit := range []string{"abc", "abcdefg"} {
		if got := (AgentInfo{LandlineCommit: commit}).Attributes(); got != "commit: "+commit {
			t.Errorf("%q: got %q", commit, got)
		}
	}
}

func TestAttributesCutTheCommitAfterSevenCharactersAndNotSevenBytes(t *testing.T) {
	t.Parallel()
	for commit, want := range map[string]string{
		"éàüöíóúñç":          "commit: éàüöíóú",
		"éàüöíóúñç-dirty":    "commit: éàüöíóú-dirty",
		"日本語のコミットの値です":       "commit: 日本語のコミッ",
		"éàüöíóú":            "commit: éàüöíóú",
		"éàüöíó":             "commit: éàüöíó",
		"日本語のコミットの値です-dirty": "commit: 日本語のコミッ-dirty",
	} {
		if got := (AgentInfo{LandlineCommit: commit}).Attributes(); got != want {
			t.Errorf("%q: got %q, want %q", commit, got, want)
		}
	}
}

func TestAttributesShowAMultibyteDateWhole(t *testing.T) {
	t.Parallel()
	date := "２０２６年１１月１２日"
	if got := (AgentInfo{LandlineDate: date}).Attributes(); got != "built: "+date {
		t.Fatalf("got %q", got)
	}
}

func TestAttributesNeverLetACommitOrADateBreakTheLine(t *testing.T) {
	t.Parallel()
	got := AgentInfo{LandlineCommit: "ab\ncdefgh", LandlineDate: "x\u2028y"}.Attributes()
	if strings.ContainsAny(got, "\n\r\u2028") {
		t.Fatalf("got %q", got)
	}
}

func TestAttributesShowTheTmuxSocketAndPaneAsOneItem(t *testing.T) {
	t.Parallel()
	if got := (AgentInfo{TmuxSocket: "default", TmuxPane: "%3"}).Attributes(); got != "tmux: default:%3" {
		t.Fatalf("got %q", got)
	}
}

func TestAttributesShowTheTmuxItemAmongTheOthersSeparatedByTwoSpaces(t *testing.T) {
	t.Parallel()
	got := AgentInfo{Label: "lead", LandlineVersion: "0.1.0", TmuxSocket: "work", TmuxPane: "%12"}.Attributes()
	for _, want := range []string{"label: lead", "landline: 0.1.0", "  tmux: work:%12"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	if strings.Count(got, "tmux:") != 1 {
		t.Fatalf("got %q", got)
	}
}

func TestAttributesLeaveOutTheTmuxItemWhenThereIsNoSocketAndNoPane(t *testing.T) {
	t.Parallel()
	if got := (AgentInfo{Label: "lead"}).Attributes(); strings.Contains(got, "tmux") {
		t.Fatalf("got %q", got)
	}
}

func TestAttributesNeverLetATmuxValueBreakTheLine(t *testing.T) {
	t.Parallel()
	got := AgentInfo{TmuxSocket: "a\nb", TmuxPane: "%1 "}.Attributes()
	if strings.ContainsAny(got, "\n\r ") {
		t.Fatalf("got %q", got)
	}
}

func TestTheTmuxValuesTravelInTheAgentAndAgentInfoJSONUnderTheirOwnKeysAndAreLeftOutWhenEmpty(t *testing.T) {
	t.Parallel()
	for name, value := range map[string]any{
		"agent":      Agent{Name: "alpha", Harness: "h", TmuxSocket: "default", TmuxPane: "%3"},
		"agent info": AgentInfo{Name: "alpha", Harness: "h", TmuxSocket: "default", TmuxPane: "%3"},
	} {
		raw, err := json.Marshal(value)
		if err != nil || !strings.Contains(string(raw), `"tmux_socket":"default"`) || !strings.Contains(string(raw), `"tmux_pane":"%3"`) {
			t.Errorf("%s: %s %v", name, raw, err)
		}
	}
	for name, value := range map[string]any{"agent": Agent{Name: "alpha", Harness: "h"}, "agent info": AgentInfo{Name: "alpha", Harness: "h"}} {
		raw, _ := json.Marshal(value)
		if strings.Contains(string(raw), "tmux") {
			t.Errorf("%s: an empty value is present: %s", name, raw)
		}
	}
}

func TestARegisterRequestCarriesTheTmuxValuesUnderTheirOwnKeys(t *testing.T) {
	t.Parallel()
	var got RegisterRequest
	if err := json.Unmarshal([]byte(`{"name":"alpha","harness":"h","tmux_socket":"default","tmux_pane":"%3"}`), &got); err != nil {
		t.Fatal(err)
	}
	if got.TmuxSocket != "default" || got.TmuxPane != "%3" {
		t.Fatalf("got %+v", got)
	}
}
