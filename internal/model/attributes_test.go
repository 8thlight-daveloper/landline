package model

import (
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
