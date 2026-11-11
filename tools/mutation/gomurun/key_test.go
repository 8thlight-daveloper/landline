package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func writeSource(t *testing.T, root string, name string, text string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func survivorAt(path string, line int, original string, mutated string) result {
	return result{Status: statusSurvived, Mutant: mutant{FilePath: path, Line: line, Column: 5, Type: "conditional_binary", Original: original, Mutated: mutated}}
}

func TestAKeyDoesNotChangeWhenTheLineMoves(t *testing.T) {
	root := t.TempDir()
	before := writeSource(t, root, "a.go", "package a\n\nfunc f(x int) bool {\n\treturn x == 1\n}\n")
	first, err := survivorsOf([]result{survivorAt(before, 4, "==", "<=")}, root)
	if err != nil {
		t.Fatal(err)
	}
	moved := writeSource(t, root, "a.go", "package a\n\n\n\nfunc f(x int) bool {\n\treturn x == 1\n}\n")
	second, err := survivorsOf([]result{survivorAt(moved, 6, "==", "<=")}, root)
	if err != nil {
		t.Fatal(err)
	}
	if first[0].key != second[0].key {
		t.Fatalf("the key changed when only the line number did: %s and %s", first[0].key, second[0].key)
	}
}

func TestAKeyChangesWhenTheLineChanges(t *testing.T) {
	root := t.TempDir()
	path := writeSource(t, root, "a.go", "package a\n\nfunc f(x int) bool {\n\treturn x == 1\n}\n")
	first, _ := survivorsOf([]result{survivorAt(path, 4, "==", "<=")}, root)
	path = writeSource(t, root, "a.go", "package a\n\nfunc f(x int) bool {\n\treturn x == 2\n}\n")
	second, _ := survivorsOf([]result{survivorAt(path, 4, "==", "<=")}, root)
	if first[0].key == second[0].key {
		t.Fatal("the key stayed the same after the line changed")
	}
}

func TestAKeyChangesWithTheFileTheMutationAndTheKind(t *testing.T) {
	root := t.TempDir()
	text := "package a\n\nfunc f(x int) bool {\n\treturn x == 1\n}\n"
	one := writeSource(t, root, "a.go", text)
	other := writeSource(t, root, "b.go", text)
	keys := map[string]bool{}
	for _, each := range []result{
		survivorAt(one, 4, "==", "<="),
		survivorAt(other, 4, "==", "<="),
		survivorAt(one, 4, "==", ">="),
		survivorAt(one, 4, "!=", "<="),
	} {
		found, err := survivorsOf([]result{each}, root)
		if err != nil {
			t.Fatal(err)
		}
		keys[found[0].key] = true
	}
	if len(keys) != 4 {
		t.Fatalf("keys collided: %v", keys)
	}
}

func TestTwoIdenticalLinesGetTwoKeysInFileOrder(t *testing.T) {
	root := t.TempDir()
	path := writeSource(t, root, "a.go", "package a\n\nfunc f(x int) bool {\n\tif x == 1 {\n\t\treturn true\n\t}\n\tif x == 1 {\n\t\treturn true\n\t}\n\treturn false\n}\n")
	found, err := survivorsOf([]result{survivorAt(path, 7, "==", "<="), survivorAt(path, 4, "==", "<=")}, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 2 || found[0].line != 4 || found[1].line != 7 {
		t.Fatalf("survivors are not in file order: %+v", found)
	}
	if found[0].key == found[1].key {
		t.Fatal("two identical lines share a key")
	}
}

func TestOnlySurvivorsGetAKey(t *testing.T) {
	root := t.TempDir()
	path := writeSource(t, root, "a.go", "package a\n\nfunc f(x int) bool {\n\treturn x == 1\n}\n")
	killed := survivorAt(path, 4, "==", "<=")
	killed.Status = statusKilled
	found, err := survivorsOf([]result{killed}, root)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 0 {
		t.Fatalf("a killed mutant got a key: %+v", found)
	}
}

func TestAKeyDoesNotChangeWhenAnEarlierDuplicateStopsSurviving(t *testing.T) {
	for _, sameLine := range []bool{false, true} {
		t.Run(fmt.Sprint(sameLine), func(t *testing.T) {
			root := t.TempDir()
			path := writeSource(t, root, "a.go", "package a\nfunc f(x int) bool {\n\tif x == 1 || x == 1 {\n\t}\n\tif x == 1 || x == 1 {\n\t}\n\treturn false\n}\n")
			first := survivorAt(path, 3, "==", "<=")
			second := survivorAt(path, 5, "==", "<=")
			if sameLine {
				second.Mutant.Line = first.Mutant.Line
				second.Mutant.Column = first.Mutant.Column + 10
			}
			both, err := survivorsOf([]result{second, first}, root)
			if err != nil || len(both) != 2 {
				t.Fatalf("both survivors: %+v, %v", both, err)
			}
			for _, status := range []string{statusKilled, statusNotViable, statusTimedOut, statusError} {
				first.Status = status
				remaining, err := survivorsOf([]result{first, second}, root)
				if err != nil || len(remaining) != 1 {
					t.Fatalf("remaining survivor: %+v, %v", remaining, err)
				}
				if remaining[0].key != both[1].key {
					t.Errorf("after earlier duplicate became %s, key changed from %s to %s", status, both[1].key, remaining[0].key)
				}
				_, unreviewed := tallyOf([]result{first, second}, remaining, map[string]bool{both[0].key: true})
				if len(unreviewed) != 1 {
					t.Errorf("an exception for the earlier duplicate hid the remaining survivor after %s", status)
				}
			}
		})
	}
}

func TestTheFileIsNamedRelativeToTheRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "internal", "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	path := writeSource(t, root, "internal/x/a.go", "package x\n\nvar y = 1 + 2\n")
	found, err := survivorsOf([]result{survivorAt(path, 3, "+", "-")}, root)
	if err != nil {
		t.Fatal(err)
	}
	if found[0].file != "internal/x/a.go" {
		t.Fatalf("file is %q", found[0].file)
	}
}
