package main

import (
	"encoding/json"
	"fmt"
	"os"
)

const (
	statusKilled    = "KILLED"
	statusSurvived  = "SURVIVED"
	statusNotViable = "NOT_VIABLE"
	statusTimedOut  = "TIMED_OUT"
	statusError     = "ERROR"
)

type mutant struct {
	FilePath string `json:"filePath"`
	Line     int    `json:"line"`
	Column   int    `json:"column"`
	Type     string `json:"type"`
	Original string `json:"original"`
	Mutated  string `json:"mutated"`
}

type result struct {
	Mutant mutant `json:"mutant"`
	Status string `json:"status"`
}

type report struct {
	Results []result `json:"results"`
}

func readReport(path string) (report, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return report{}, err
	}
	var parsed report
	if err := json.Unmarshal(data, &parsed); err != nil {
		return report{}, fmt.Errorf("%s is not a gomu report: %w", path, err)
	}
	return parsed, nil
}
