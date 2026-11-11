package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const keyLength = 16

type survivor struct {
	key      string
	file     string
	line     int
	kind     string
	original string
	mutated  string
	source   string
}

func survivorsOf(results []result, root string) ([]survivor, error) {
	ordered := append([]result(nil), results...)
	sort.SliceStable(ordered, func(left, right int) bool {
		a, b := ordered[left].Mutant, ordered[right].Mutant
		if a.FilePath != b.FilePath {
			return a.FilePath < b.FilePath
		}
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		return a.Column < b.Column
	})
	lines := map[string][]string{}
	seen := map[string]int{}
	survivors := make([]survivor, 0, len(ordered))
	for _, each := range ordered {
		relative, err := filepath.Rel(root, each.Mutant.FilePath)
		if err != nil {
			return nil, err
		}
		fileLines, loaded := lines[each.Mutant.FilePath]
		if !loaded {
			data, err := os.ReadFile(each.Mutant.FilePath)
			if err != nil {
				return nil, err
			}
			fileLines = strings.Split(string(data), "\n")
			lines[each.Mutant.FilePath] = fileLines
		}
		source := ""
		if each.Mutant.Line >= 1 && each.Mutant.Line <= len(fileLines) {
			source = strings.TrimSpace(fileLines[each.Mutant.Line-1])
		}
		base := strings.Join([]string{filepath.ToSlash(relative), each.Mutant.Type, each.Mutant.Original, each.Mutant.Mutated, source}, "\x00")
		seen[base]++
		if each.Status != statusSurvived {
			continue
		}
		survivors = append(survivors, survivor{
			key:      keyOf(base, seen[base]),
			file:     filepath.ToSlash(relative),
			line:     each.Mutant.Line,
			kind:     each.Mutant.Type,
			original: each.Mutant.Original,
			mutated:  each.Mutant.Mutated,
			source:   source,
		})
	}
	return survivors, nil
}

func keyOf(base string, ordinal int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d", base, ordinal)))
	return hex.EncodeToString(sum[:])[:keyLength]
}
