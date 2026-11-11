package main

import (
	"bufio"
	"errors"
	"os"
	"strings"
)

func readAccepted(path string) (map[string]bool, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return map[string]bool{}, nil
	}
	if err != nil {
		return nil, err
	}
	defer file.Close()
	accepted := map[string]bool{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) == 0 || strings.HasPrefix(fields[0], "#") {
			continue
		}
		accepted[fields[0]] = true
	}
	return accepted, scanner.Err()
}
