package command

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	DefaultPort = 8787
	loopback    = "127.0.0.1"
)

const DefaultURL = "http://127.0.0.1:8787"

func portFrom(getenv func(string) string) (int, error) {
	raw := getenv("LANDLINE_PORT")
	if raw == "" {
		return DefaultPort, nil
	}
	port, err := strconv.Atoi(raw)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("LANDLINE_PORT must be an integer from 1 to 65535, not %q", raw)
	}
	return port, nil
}

func DefaultAddr(getenv func(string) string) (string, error) {
	port, err := portFrom(getenv)
	if err != nil {
		return "", err
	}
	return net.JoinHostPort(loopback, strconv.Itoa(port)), nil
}

type Config struct {
	Secret string
	Host   string
	Port   int
}

func (c Config) Listen(getenv func(string) string) (host string, port int, err error) {
	host = Or(c.Host, loopback)
	port = c.Port
	if port == 0 {
		port, err = portFrom(getenv)
	}
	return host, port, err
}

func configPath(env Env) string { return filepath.Join(env.ConfigDir, "config.toml") }

func legacyConfigPath(env Env) string { return filepath.Join(env.ConfigDir, "config.json") }

var (
	keyLine     = regexp.MustCompile(`^([A-Za-z0-9_-]+)\s*=\s*(.*)$`)
	stringValue = regexp.MustCompile(`^"([^"\\]*)"$`)
	wholeNumber = regexp.MustCompile(`^-?[0-9]+$`)
)

func ReadConfig(env Env) (Config, error) {
	raw, err := os.ReadFile(configPath(env))
	if err == nil {
		return parseConfig(configPath(env), string(raw))
	}
	if errors.Is(err, os.ErrNotExist) {
		return readLegacyConfig(env)
	}
	return Config{}, fmt.Errorf("%s cannot be read: %v", configPath(env), err)
}

func parseConfig(path, text string) (Config, error) {
	var config Config
	for number, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		found := keyLine.FindStringSubmatch(line)
		if found == nil {
			return Config{}, lineError(path, number, "this line is not a blank line, a comment that starts with #, or a key set to text in quotes or to a whole number")
		}
		key, value := found[1], found[2]
		quoted := stringValue.FindStringSubmatch(value)
		isNumber := wholeNumber.MatchString(value)
		switch {
		case key == "secret" || key == "host":
			if quoted == nil {
				return Config{}, lineError(path, number, key+" must be text in quotes")
			}
			if key == "secret" {
				config.Secret = quoted[1]
			} else {
				config.Host = quoted[1]
			}
		case key == "port":
			port, convertErr := strconv.Atoi(value)
			if !isNumber || convertErr != nil || port < 1 || port > 65535 {
				return Config{}, lineError(path, number, "port must be a whole number from 1 to 65535")
			}
			config.Port = port
		case quoted == nil && !isNumber:
			return Config{}, lineError(path, number, "this line is not a blank line, a comment that starts with #, or a key set to text in quotes or to a whole number")
		}
	}
	return config, nil
}

func lineError(path string, index int, problem string) error {
	return fmt.Errorf("%s line %d: %s", path, index+1, problem)
}

func readLegacyConfig(env Env) (Config, error) {
	raw, err := os.ReadFile(legacyConfigPath(env))
	if err != nil {
		return Config{}, nil
	}
	var saved struct {
		URL    string `json:"url"`
		Secret string `json:"secret"`
	}
	if json.Unmarshal(raw, &saved) != nil {
		return Config{}, nil
	}
	config := Config{Secret: saved.Secret}
	if saved.URL == "" {
		return config, nil
	}
	host, port, err := splitSwitchboardAddress(saved.URL)
	if err != nil {
		return Config{}, fmt.Errorf("%s holds a url that Landline cannot use: %v. Edit or remove it", legacyConfigPath(env), err)
	}
	config.Host, config.Port = host, port
	return config, nil
}

const httpOnly = "Landline speaks http only, so the address cannot start with https://"

var errAddressShape = errors.New("the switchboard address must look like host:port, as in davelight-mbp:8787")

func splitSwitchboardAddress(text string) (string, int, error) {
	text = strings.TrimRight(strings.TrimSpace(text), "/")
	if strings.HasPrefix(strings.ToLower(text), "https://") {
		return "", 0, errors.New(httpOnly)
	}
	text = strings.TrimPrefix(text, "http://")
	if text == "" || strings.Contains(text, "://") {
		return "", 0, errAddressShape
	}
	parsed, err := url.Parse("http://" + text)
	if err != nil || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.Path != "" || parsed.Hostname() == "" {
		return "", 0, errAddressShape
	}
	port, err := strconv.Atoi(parsed.Port())
	if err != nil || port < 1 || port > 65535 {
		return "", 0, errAddressShape
	}
	return parsed.Hostname(), port, nil
}

func SaveConfig(env Env, change Config) error {
	if err := os.MkdirAll(env.ConfigDir, 0o700); err != nil {
		return err
	}
	var lines []string
	migrating := false
	raw, readErr := os.ReadFile(configPath(env))
	switch {
	case readErr == nil:
		lines = splitLines(string(raw))
	case !errors.Is(readErr, os.ErrNotExist):
		return readErr
	default:
		migrating = fileExists(legacyConfigPath(env))
		if legacy, legacyErr := readLegacyConfig(env); legacyErr == nil {
			lines = renderConfig(legacy)
		}
	}
	if change.Secret != "" {
		lines = setKey(lines, "secret", `"`+change.Secret+`"`)
	}
	if change.Host != "" {
		lines = setKey(lines, "host", `"`+change.Host+`"`)
	}
	if change.Port != 0 {
		lines = setKey(lines, "port", strconv.Itoa(change.Port))
	}
	path := configPath(env)
	temp := path + ".tmp"
	os.Remove(temp)
	if err := os.WriteFile(temp, []byte(strings.Join(lines, "\n")+"\n"), 0o600); err != nil {
		return err
	}
	if err := os.Chmod(temp, 0o600); err != nil {
		return err
	}
	if err := os.Rename(temp, path); err != nil {
		return err
	}
	if migrating {
		os.Remove(legacyConfigPath(env))
	}
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func splitLines(text string) []string {
	if text == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(text, "\n"), "\n")
}

func renderConfig(config Config) []string {
	var lines []string
	if config.Secret != "" {
		lines = append(lines, `secret = "`+config.Secret+`"`)
	}
	if config.Host != "" {
		lines = append(lines, `host = "`+config.Host+`"`)
	}
	if config.Port != 0 {
		lines = append(lines, "port = "+strconv.Itoa(config.Port))
	}
	return lines
}

func setKey(lines []string, key, value string) []string {
	replaced := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if found := keyLine.FindStringSubmatch(trimmed); found != nil && found[1] == key && !strings.HasPrefix(trimmed, "#") {
			lines[i] = key + " = " + value
			replaced = true
		}
	}
	if replaced {
		return lines
	}
	return append(lines, key+" = "+value)
}

func withConfig(env Env) Env {
	original := env.Getenv
	if original == nil {
		original = func(string) string { return "" }
	}
	saved, problem := ReadConfig(env)
	if env.processGetenv == nil {
		env.processGetenv = original
	}
	env.configProblem = problem
	env.Getenv = func(key string) string {
		if value := original(key); value != "" {
			return value
		}
		switch key {
		case "LANDLINE_URL":
			return savedURL(saved, original)
		case "LANDLINE_SECRET":
			return saved.Secret
		}
		return ""
	}
	return env
}

func savedURL(saved Config, getenv func(string) string) string {
	host, port, err := saved.Listen(getenv)
	if err != nil {
		return DefaultURL
	}
	return "http://" + net.JoinHostPort(host, strconv.Itoa(port))
}

func HubURL(env Env) string {
	return withConfig(env).Getenv("LANDLINE_URL")
}

func join(args []string, env Env) error {
	set := flags("join", env)
	if err := set.Parse(args); err != nil {
		return err
	}
	if set.NArg() != 1 {
		return errors.New("usage: landline server join <host:port>. The switchboard secret comes from LANDLINE_SECRET or standard input")
	}
	host, port, err := splitSwitchboardAddress(set.Arg(0))
	if err != nil {
		return err
	}
	secret := env.processEnvironment("LANDLINE_SECRET")
	if secret == "" {
		fmt.Fprint(env.Stderr, "Switchboard secret: ")
		raw := make([]byte, 256)
		n, _ := env.Stdin.Read(raw)
		secret = strings.TrimSpace(string(raw[:n]))
	}
	if len(secret) < 16 {
		return errors.New("the switchboard secret is at least 16 characters")
	}
	if strings.ContainsAny(secret, "\"\\") || strings.IndexFunc(secret, func(r rune) bool { return r < ' ' || r == 0x7f }) >= 0 {
		return errors.New("the switchboard secret cannot hold a quote, a backslash, or a control character")
	}
	if err := SaveConfig(env, Config{Host: host, Port: port, Secret: secret}); err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "joined the switchboard at %s. The address and secret are saved in %s, readable only by you\n", net.JoinHostPort(host, strconv.Itoa(port)), configPath(env))
	return nil
}
