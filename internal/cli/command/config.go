package command

import (
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	client "github.com/8thlight-daveloper/landline/internal/client/http"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
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
	Secret  string
	Host    string
	Port    int
	Machine string
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
	tableLine   = regexp.MustCompile(`^\[([A-Za-z0-9_-]+)\]$`)
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

const (
	wrongShape      = "this line is not a blank line, a comment that starts with #, a table name in brackets, or a key set to text in quotes or to a whole number"
	serverTableName = "server"
)

type tableKey struct {
	index      int
	key, value string
}

func parseConfig(path, text string) (Config, error) {
	config, _, err := parseConfigTables(path, text)
	return config, err
}

func parseConfigTables(path, text string) (Config, []tableKey, error) {
	var config Config
	var server []tableKey
	table := ""
	seenServer := false
	for number, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if opened := tableLine.FindStringSubmatch(line); opened != nil {
			table = opened[1]
			if table == serverTableName && seenServer {
				return Config{}, nil, lineError(path, number, "[server] is given a second time")
			}
			seenServer = seenServer || table == serverTableName
			continue
		}
		found := keyLine.FindStringSubmatch(line)
		if found == nil {
			return Config{}, nil, lineError(path, number, wrongShape)
		}
		key, value := found[1], found[2]
		quoted := stringValue.FindStringSubmatch(value)
		isNumber := wholeNumber.MatchString(value)
		if table == serverTableName {
			for _, earlier := range server {
				if earlier.key == key {
					return Config{}, nil, lineError(path, number, key+" is given a second time in [server]")
				}
			}
			server = append(server, tableKey{index: number, key: key, value: value})
		}
		switch {
		case quoted == nil && !isNumber:
			return Config{}, nil, lineError(path, number, wrongShape)
		case table != "":
		case key == "secret" || key == "host" || key == "machine":
			if quoted == nil {
				return Config{}, nil, lineError(path, number, key+" must be text in quotes")
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
				return Config{}, nil, lineError(path, number, "port must be a whole number from 1 to 65535")
			}
			config.Port = port
		}
	}
	return config, server, nil
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

var errAddressShape = errors.New("the switchboard address must look like host:port, as in alpha-host:8787")

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
	if change.Machine != "" {
		lines = setKey(lines, "machine", `"`+change.Machine+`"`)
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
	firstTable := len(lines)
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if tableLine.MatchString(trimmed) {
			firstTable = i
			break
		}
		if found := keyLine.FindStringSubmatch(trimmed); found != nil && found[1] == key {
			lines[i] = key + " = " + value
			replaced = true
		}
	}
	if replaced {
		return lines
	}
	return slices.Insert(lines, topLevelEnd(lines, firstTable), key+" = "+value)
}

func topLevelEnd(lines []string, firstTable int) int {
	end := firstTable
	for end > 0 && strings.TrimSpace(lines[end-1]) == "" {
		end--
	}
	return end
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
	env.keyProblem = nil
	if problem == nil && env.processGetenv("LANDLINE_SECRET") == "" {
		env.signer, env.keyProblem = savedSigner(env, saved)
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
	code, given := new(string), new(bool)
	rest := []string{}
	for i := 0; i < len(args); i++ {
		switch {
		case args[i] == "--code" && i+1 < len(args):
			*code = args[i+1]
			*given = true
			i++
		case strings.HasPrefix(args[i], "--code="):
			*code = strings.TrimPrefix(args[i], "--code=")
			*given = true
		default:
			rest = append(rest, args[i])
		}
	}
	if err := set.Parse(rest); err != nil {
		return err
	}
	if set.NArg() != 1 {
		return errors.New("usage: landline server join <host:port> [--code <code>]. Without a code, the switchboard secret comes from LANDLINE_SECRET or standard input")
	}
	host, port, err := splitSwitchboardAddress(set.Arg(0))
	if err != nil {
		return err
	}
	if *given && strings.TrimSpace(*code) == "" {
		return errors.New("the invite code is empty. Run landline server join <host:port> --code <code>")
	}
	if *code != "" {
		return joinWithCode(env, host, port, *code)
	}
	if _, statErr := os.Stat(machineKeyPath(env)); statErr == nil {
		return fmt.Errorf("this machine holds a machine key at %s. Joining with a secret would leave that key in use. Remove the key file and the machine line in %s first, or run landline server join <host:port> --code <code>", machineKeyPath(env), configPath(env))
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

func forgetSecretLine(env Env) error {
	raw, err := os.ReadFile(configPath(env))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var kept []string
	inTable := false
	for _, line := range splitLines(string(raw)) {
		trimmed := strings.TrimSpace(line)
		inTable = inTable || tableLine.MatchString(trimmed)
		if found := keyLine.FindStringSubmatch(trimmed); !inTable && found != nil && found[1] == "secret" {
			continue
		}
		kept = append(kept, line)
	}
	temporary := configPath(env) + ".tmp"
	os.Remove(temporary)
	if err := os.WriteFile(temporary, []byte(strings.Join(kept, "\n")+"\n"), 0o600); err != nil {
		return err
	}
	if err := os.Chmod(temporary, 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, configPath(env))
}

func saveJoin(env Env, private ed25519.PrivateKey, host string, port int, name string) ([]string, error) {
	if err := writeMachineKey(env, private); err != nil {
		return nil, err
	}
	changed := []string{machineKeyPath(env)}
	if err := SaveConfig(env, Config{Host: host, Port: port, Machine: name}); err != nil {
		return changed, err
	}
	changed = append(changed, configPath(env))
	return changed, forgetSecretLine(env)
}

func joinWithCode(env Env, host string, port int, code string) error {
	address := net.JoinHostPort(host, strconv.Itoa(port))
	restore, err := snapshotJoinFiles(env)
	if err != nil {
		return err
	}
	public, private, err := newMachineKey()
	if err != nil {
		return err
	}
	name, err := client.New("http://"+address, "").Redeem(code, public)
	if err != nil {
		var refused *client.StatusError
		if errors.As(err, &refused) && refused.Message != "" {
			return errors.New(refused.Message)
		}
		return err
	}
	if !machineNamePattern.MatchString(name) {
		return fmt.Errorf("the switchboard answered with a machine name %q that is not allowed. Nothing was saved", name)
	}
	if changed, err := saveJoin(env, private, host, port, name); err != nil {
		if restoreErr := restore(changed); restoreErr != nil {
			return fmt.Errorf("%w. %v. The new machine %s stays on the switchboard until you run landline server revoke %s", err, restoreErr, name, name)
		}
		return fmt.Errorf("%w. The earlier key and config.toml were restored, and the new machine %s stays on the switchboard until you run landline server revoke %s", err, name, name)
	}
	fmt.Fprintf(env.Stdout, "joined the switchboard at %s as %s. The key is saved in %s, readable only by you\n", address, name, machineKeyPath(env))
	return nil
}
