package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	cacheFolder    = ".direnv/mutation-cache"
	cacheSeparator = "\x00"
)

func sourceFiles(root string, place layout, component string) ([]string, error) {
	entries, err := os.ReadDir(filepath.Join(root, place.components, component))
	if err != nil {
		return nil, err
	}
	var names []string
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() && strings.HasSuffix(name, ".go") && !strings.HasSuffix(name, "_test.go") {
			names = append(names, name)
		}
	}
	return names, nil
}

func sourceDigest(root string, place layout, component string, names []string) (string, error) {
	hash := sha256.New()
	for _, name := range names {
		data, err := os.ReadFile(filepath.Join(root, place.components, component, name))
		if err != nil {
			return "", err
		}
		hash.Write([]byte(name + cacheSeparator))
		hash.Write(data)
		hash.Write([]byte(cacheSeparator))
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func suiteDigest(root string, place layout, component string) (string, error) {
	base := filepath.Join(root, place.contracts, component)
	var paths []string
	err := filepath.WalkDir(base, func(path string, entry fs.DirEntry, err error) error {
		if err == nil && !entry.IsDir() {
			paths = append(paths, path)
		}
		return err
	})
	if err != nil {
		return "", err
	}
	sort.Strings(paths)
	hash := sha256.New()
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			return "", err
		}
		hash.Write([]byte(strings.TrimPrefix(path, base) + cacheSeparator))
		hash.Write(data)
		hash.Write([]byte(cacheSeparator))
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func cacheKey(source []byte, suite string, version string, short bool) string {
	hash := sha256.New()
	hash.Write(source)
	hash.Write([]byte(cacheSeparator + suite + cacheSeparator + version))
	if short {
		hash.Write([]byte(cacheSeparator + "short"))
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func cachePath(root string, component string, name string, key string) string {
	folder := strings.ReplaceAll(component, "/", "_")
	return filepath.Join(root, cacheFolder, folder, name+"."+key+".json")
}

func readCached(path string) ([]result, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, false
	}
	var results []result
	if json.Unmarshal(data, &results) != nil {
		return nil, false
	}
	return results, true
}

func writeCached(path string, results []result) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.Marshal(results)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

func forgetStale(root string, component string, name string, keep string) {
	pattern := cachePath(root, component, name, "*")
	matches, err := filepath.Glob(pattern)
	if err != nil && !errors.Is(err, filepath.ErrBadPattern) {
		return
	}
	for _, match := range matches {
		if match != keep {
			os.Remove(match)
		}
	}
}
