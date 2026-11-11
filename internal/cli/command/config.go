package command

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

const DefaultURL = "http://127.0.0.1:8787"

type Config struct {
	URL    string `json:"url"`
	Secret string `json:"secret"`
}

func configPath(env Env) string { return filepath.Join(env.ConfigDir, "config.json") }

func LoadConfig(env Env) Config {
	var saved Config
	if raw, err := os.ReadFile(configPath(env)); err == nil {
		json.Unmarshal(raw, &saved)
	}
	return saved
}

func SaveConfig(env Env, config Config) error {
	if err := os.MkdirAll(env.ConfigDir, 0o700); err != nil {
		return err
	}
	raw, err := json.Marshal(config)
	if err != nil {
		return err
	}
	path := configPath(env)
	temp := path + ".tmp"
	os.Remove(temp)
	if err := os.WriteFile(temp, raw, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(temp, 0o600); err != nil {
		return err
	}
	return os.Rename(temp, path)
}

func withConfig(env Env) Env {
	original := env.Getenv
	if original == nil {
		original = func(string) string { return "" }
	}
	saved := LoadConfig(env)
	env.Getenv = func(key string) string {
		if value := original(key); value != "" {
			return value
		}
		switch key {
		case "LANDLINE_URL":
			if saved.URL != "" {
				return saved.URL
			}
		case "LANDLINE_SECRET":
			return saved.Secret
		}
		return ""
	}
	return env
}

func HubURL(env Env) string {
	if value := withConfig(env).Getenv("LANDLINE_URL"); value != "" {
		return value
	}
	return DefaultURL
}

func validHubURL(text string) error {
	parsed, err := url.Parse(text)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || (parsed.Path != "" && parsed.Path != "/") {
		return errors.New("the hub address must look like http://host:port")
	}
	return nil
}

func join(args []string, env Env) error {
	set := flags("join", env)
	if err := set.Parse(args); err != nil {
		return err
	}
	if set.NArg() != 1 {
		return errors.New("usage: landline server join <hub address>. The hub secret comes from LANDLINE_SECRET or standard input")
	}
	address := strings.TrimRight(set.Arg(0), "/")
	if err := validHubURL(address); err != nil {
		return err
	}
	secret := env.Getenv("LANDLINE_SECRET")
	if secret == "" {
		fmt.Fprint(env.Stderr, "Hub secret: ")
		raw := make([]byte, 256)
		n, _ := env.Stdin.Read(raw)
		secret = strings.TrimSpace(string(raw[:n]))
	}
	if len(secret) < 16 {
		return errors.New("the hub secret is at least 16 characters")
	}
	if err := SaveConfig(env, Config{URL: address, Secret: secret}); err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "joined the hub at %s. The address and secret are saved in %s, readable only by you\n", address, configPath(env))
	return nil
}
