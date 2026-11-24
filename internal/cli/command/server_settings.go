package command

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type ServerSettings struct {
	Log    string
	Host   string
	Timing Timing
}

var defaultTiming = Timing{
	Keepalive:       15 * time.Second,
	DeliveryTimeout: 20 * time.Second,
	Grace:           2 * time.Minute,
	InviteLifetime:  5 * time.Minute,
	SignatureWindow: 5 * time.Second,
}

type timingSetting struct {
	key    string
	target func(*Timing) *time.Duration
}

var timingSettings = []timingSetting{
	{"keepalive", func(t *Timing) *time.Duration { return &t.Keepalive }},
	{"delivery-timeout", func(t *Timing) *time.Duration { return &t.DeliveryTimeout }},
	{"grace", func(t *Timing) *time.Duration { return &t.Grace }},
	{"invite-lifetime", func(t *Timing) *time.Duration { return &t.InviteLifetime }},
	{"signature-window", func(t *Timing) *time.Duration { return &t.SignatureWindow }},
}

func DefaultLogPath(env Env) string { return filepath.Join(dataDirectory(env), "log.jsonl") }

func readServerTable(env Env) (ServerSettings, error) {
	settings := ServerSettings{Log: DefaultLogPath(env), Timing: defaultTiming}
	path := configPath(env)
	raw, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return settings, nil
	}
	if err != nil {
		return ServerSettings{}, fmt.Errorf("%s cannot be read: %v", path, err)
	}
	_, table, err := parseConfigTables(path, string(raw))
	if err != nil {
		return ServerSettings{}, err
	}
	for _, line := range table {
		quoted := stringValue.FindStringSubmatch(line.value)
		if line.key == "log" {
			if quoted == nil || !(filepath.IsAbs(quoted[1]) || strings.HasPrefix(quoted[1], "~/")) {
				return ServerSettings{}, lineError(path, line.index, "log must be a path in quotes that is absolute or starts with ~/")
			}
			settings.Log = homeExpanded(env, quoted[1])
			continue
		}
		for _, timing := range timingSettings {
			if line.key != timing.key {
				continue
			}
			duration, parseErr := time.Duration(0), errors.New("not text in quotes")
			if quoted != nil {
				duration, parseErr = time.ParseDuration(quoted[1])
			}
			if parseErr != nil || duration <= 0 {
				return ServerSettings{}, lineError(path, line.index, timing.key+" must be a positive duration in quotes with a unit, such as 200ms, 15s, or 5m")
			}
			if timing.key == "signature-window" && !signatureWindowInRange(duration) {
				return ServerSettings{}, lineError(path, line.index, signatureWindowRange("signature-window"))
			}
			*timing.target(&settings.Timing) = duration
		}
	}
	return settings, nil
}

func signatureWindowRange(name string) string {
	return name + " must be from " + shortestSignatureWindow.String() + " to " + longestSignatureWindow.String() + ", such as 5s"
}

func homeExpanded(env Env, path string) string {
	if rest, found := strings.CutPrefix(path, "~/"); found {
		return filepath.Join(env.Home, rest)
	}
	return path
}

func comparableLog(env Env, path string) string {
	path = homeExpanded(env, path)
	if absolute, err := filepath.Abs(path); err == nil {
		return absolute
	}
	return filepath.Clean(path)
}

func SwitchboardSettings(env Env, values map[string]string) (ServerSettings, []string, error) {
	table, err := readServerTable(env)
	if err != nil {
		return ServerSettings{}, nil, err
	}
	given, err := TimingFrom(values)
	if err != nil {
		return ServerSettings{}, nil, err
	}
	if _, set := values["--signature-window"]; set && !signatureWindowInRange(given.SignatureWindow) {
		return ServerSettings{}, nil, errors.New(signatureWindowRange("--signature-window"))
	}
	settings := table
	settings.Host = Or(values["--host"], env.Host)
	var lines []string
	if flag, set := values["--log"]; set {
		settings.Log = homeExpanded(env, flag)
		if comparableLog(env, flag) != filepath.Clean(table.Log) {
			lines = append(lines, flagLine("--log", "log"))
		}
	}
	if flag, set := values["--host"]; set && flag != env.Host {
		lines = append(lines, "--host "+flag+" is not this machine's host name, and config.toml has no key for it. landline server restart, landline server rotate-secret, and the menu bar start the switchboard without it")
	}
	for _, timing := range timingSettings {
		if _, set := values["--"+timing.key]; !set {
			continue
		}
		chosen := *timing.target(&given)
		if chosen != *timing.target(&table.Timing) {
			lines = append(lines, flagLine("--"+timing.key, timing.key))
		}
		*timing.target(&settings.Timing) = chosen
	}
	return settings, lines, nil
}

func flagLine(flag, key string) string {
	return flag + " differs from config.toml for this run only. landline server restart, landline server rotate-secret, and the menu bar will use the value in config.toml, or the default when it has none. Set " + key + " in [server] of config.toml to keep it"
}
