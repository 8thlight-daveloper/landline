package model

import (
	"strings"
	"testing"
	"time"
)

const (
	testCommit = "0123456789abcdef0123456789abcdef01234567"
	testDate   = "2026-11-12T16:34:00Z"
)

func TestAttributesShowTheShortCommitAndTheDateAfterTheLandlineVersion(t *testing.T) {
	t.Parallel()
	info := AgentInfo{Project: "lead", LandlineVersion: "0.1.0", LandlineCommit: testCommit, LandlineDate: testDate}
	want := "project: lead  landline: 0.1.0  commit: 0123456  built: " + testDate
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

func TestAttributesShowTheProjectThenTheTaskThenTheFolderAndTheRest(t *testing.T) {
	t.Parallel()
	info := AgentInfo{
		ID: "a3f9", Project: "landline", Task: "menu bar app for ADR 0140", Folder: "landline-wt", Branch: "main", Worktree: "wt-2",
		Model: "opus", Effort: "high", HarnessVersion: "2.1.296", LandlineVersion: "0.1.0", LandlineCommit: testCommit, LandlineDate: testDate,
	}
	want := "project: landline  task: menu bar app for ADR 0140  folder: landline-wt  branch: main  worktree: wt-2  model: opus  effort: high  version: 2.1.296  landline: 0.1.0  commit: 0123456  built: " + testDate
	if got := info.Attributes(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestAttributesLeaveOutAnEmptyTaskAndAnEmptyProject(t *testing.T) {
	t.Parallel()
	if got := (AgentInfo{Task: "ship it"}).Attributes(); got != "task: ship it" {
		t.Fatalf("task alone: %q", got)
	}
	if got := (AgentInfo{Project: "landline", Folder: "f"}).Attributes(); got != "project: landline  folder: f" {
		t.Fatalf("project and folder: %q", got)
	}
	if got := (AgentInfo{ID: "a3f9", Harness: "codex"}).Attributes(); got != "" {
		t.Fatalf("neither: %q", got)
	}
}

func TestAttributesNeverSayLabelOrNameOrHandleAndNeverShowTheIDOrTheTaskTime(t *testing.T) {
	t.Parallel()
	got := AgentInfo{ID: "a3f9", Project: "p", Task: "t", TaskAt: time.Date(2026, 11, 16, 9, 30, 15, 0, time.UTC)}.Attributes()
	if got != "project: p  task: t" {
		t.Fatalf("got %q", got)
	}
}

func TestAttributesNeverLetAProjectOrATaskBreakTheLine(t *testing.T) {
	t.Parallel()
	got := AgentInfo{Project: "a\nb c", Task: "x\r\ny‮z\x00"}.Attributes()
	if strings.ContainsAny(got, "\n\r ‮\x00") || !strings.HasPrefix(got, "project: a b c  task: x  y z") {
		t.Fatalf("got %q", got)
	}
}
