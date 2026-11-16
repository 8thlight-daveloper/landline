package pasteboard

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyWritesExactlyTheTextToTheCommandsStandardInput(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "got")
	bin := filepath.Join(dir, "pbcopy")
	os.WriteFile(bin, []byte("#!/bin/sh\n/bin/cat > "+out+"\n"), 0o755)
	if err := Copy(bin, "alpha-1"); err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(out); string(raw) != "alpha-1" {
		t.Fatalf("%q", raw)
	}
}

func TestCopyReportsAMissingCommand(t *testing.T) {
	if err := Copy(filepath.Join(t.TempDir(), "none"), "x"); err == nil {
		t.Fatal("want an error")
	}
}
