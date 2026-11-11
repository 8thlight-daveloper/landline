package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCacheKeyChangesWithTheSourceTheSuiteAndShortRuns(t *testing.T) {
	base := cacheKey([]byte("source"), "suite", "v1", false)
	for name, other := range map[string]string{
		"source": cacheKey([]byte("source2"), "suite", "v1", false),
		"suite":  cacheKey([]byte("source"), "suite2", "v1", false),
		"short":  cacheKey([]byte("source"), "suite", "v1", true),
	} {
		if other == base {
			t.Errorf("changing the %s kept the key", name)
		}
	}
	if cacheKey([]byte("source"), "suite", "v1", false) != base {
		t.Error("the same inputs gave another key")
	}
}

func TestSuiteDigestChangesWhenAnyContractFileChanges(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "contracts", "noun", "role", "testdata")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name string, text string) {
		if err := os.WriteFile(filepath.Join(root, "contracts", "noun", "role", name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("a_test.go", "one")
	write("testdata/fixture.yaml", "x")
	first, err := suiteDigest(root, defaultLayout(), "noun/role")
	if err != nil {
		t.Fatal(err)
	}
	write("testdata/fixture.yaml", "y")
	second, err := suiteDigest(root, defaultLayout(), "noun/role")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("a changed fixture kept the digest")
	}
}

func TestSourceFilesLeavesOutTestsAndFolders(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "internal", "noun", "role")
	if err := os.MkdirAll(filepath.Join(folder, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.go", "b_test.go", "notes.md"} {
		if err := os.WriteFile(filepath.Join(folder, name), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	names, err := sourceFiles(root, defaultLayout(), "noun/role")
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "a.go" {
		t.Fatalf("got %v", names)
	}
}

func TestCachedResultsMutateEveryFileWhenAnySourceFileChanges(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "internal", "noun", "role")
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "contracts", "noun", "role"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, text := range map[string]string{"a.go": "package role", "b.go": "package role"} {
		if err := os.WriteFile(filepath.Join(folder, name), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	var mutated []string
	mutate := func(target string) error {
		mutated = append(mutated, filepath.Base(target))
		return os.WriteFile(filepath.Join(root, reportFile), []byte(`{"results":[{"mutant":{"filePath":"`+filepath.Base(target)+`"},"status":"KILLED"}]}`), 0o644)
	}
	first, err := cachedResults(root, defaultLayout(), "noun/role", mutate, false)
	if err != nil || len(first) != 2 || len(mutated) != 2 {
		t.Fatalf("first run: %v %v %v", first, mutated, err)
	}
	mutated = nil
	if _, err := cachedResults(root, defaultLayout(), "noun/role", mutate, false); err != nil || len(mutated) != 0 {
		t.Fatalf("second run mutated %v: %v", mutated, err)
	}
	if err := os.WriteFile(filepath.Join(folder, "b.go"), []byte("package role // edited"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := cachedResults(root, defaultLayout(), "noun/role", mutate, false); err != nil || len(mutated) != 2 {
		t.Fatalf("third run mutated %v: %v", mutated, err)
	}
	for _, name := range []string{"a.go", "b.go"} {
		matches, _ := filepath.Glob(filepath.Join(root, cacheFolder, "noun_role", name+".*.json"))
		if len(matches) != 1 {
			t.Fatalf("stale cache files remain for %s: %v", name, matches)
		}
	}
}
