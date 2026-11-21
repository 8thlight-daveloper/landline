package hubclient

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type saved struct {
	Secret  string
	Host    string
	Port    int
	Machine string
}

var (
	keyLine     = regexp.MustCompile(`^([A-Za-z0-9_-]+)\s*=\s*(.*)$`)
	stringValue = regexp.MustCompile(`^"([^"\\]*)"$`)
	wholeNumber = regexp.MustCompile(`^-?[0-9]+$`)
)

const wrongShape = "this line is not a blank line, a comment that starts with #, or a key set to text in quotes or to a whole number"

func readSaved(directory string) (saved, error) {
	path := filepath.Join(directory, "config.toml")
	raw, err := os.ReadFile(path)
	if err == nil {
		return parseSaved(path, string(raw))
	}
	if errors.Is(err, os.ErrNotExist) {
		return readLegacy(filepath.Join(directory, "config.json"))
	}
	return saved{}, fmt.Errorf("%s cannot be read: %v", path, err)
}

func parseSaved(path, text string) (saved, error) {
	var config saved
	for number, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		found := keyLine.FindStringSubmatch(line)
		if found == nil {
			return saved{}, fmt.Errorf("%s line %d: %s", path, number+1, wrongShape)
		}
		key, value := found[1], found[2]
		quoted := stringValue.FindStringSubmatch(value)
		isNumber := wholeNumber.MatchString(value)
		switch {
		case key == "secret" || key == "host" || key == "machine":
			if quoted == nil {
				return saved{}, fmt.Errorf("%s line %d: %s must be text in quotes", path, number+1, key)
			}
			switch key {
			case "secret":
				config.Secret = quoted[1]
			case "host":
				config.Host = quoted[1]
			default:
				config.Machine = quoted[1]
			}
		case key == "port":
			port, convertErr := strconv.Atoi(value)
			if !isNumber || convertErr != nil || port < 1 || port > 65535 {
				return saved{}, fmt.Errorf("%s line %d: port must be a whole number from 1 to 65535", path, number+1)
			}
			config.Port = port
		case quoted == nil && !isNumber:
			return saved{}, fmt.Errorf("%s line %d: %s", path, number+1, wrongShape)
		}
	}
	return config, nil
}

func readLegacy(path string) (saved, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return saved{}, nil
	}
	var old struct {
		URL    string `json:"url"`
		Secret string `json:"secret"`
	}
	if json.Unmarshal(raw, &old) != nil {
		return saved{}, nil
	}
	config := saved{Secret: old.Secret}
	if old.URL == "" {
		return config, nil
	}
	host, port, ok := splitLegacyAddress(old.URL)
	if !ok {
		return saved{}, fmt.Errorf("%s holds a url that Landline cannot use. Landline speaks http only, and the url must look like host:port or http://host:port. Edit or remove the file", path)
	}
	config.Host, config.Port = host, port
	return config, nil
}

func splitLegacyAddress(text string) (string, int, bool) {
	text = strings.TrimPrefix(strings.TrimRight(strings.TrimSpace(text), "/"), "http://")
	if text == "" || strings.Contains(text, "://") {
		return "", 0, false
	}
	parsed, err := url.Parse("http://" + text)
	if err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != "" || parsed.Hostname() == "" {
		return "", 0, false
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil || port < 1 || port > 65535 {
		return "", 0, false
	}
	return parsed.Hostname(), port, true
}
