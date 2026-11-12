package hubclient

import (
	"encoding/json"
	"errors"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/8thlight-daveloper/landline/tui/internal/clean"
)

const defaultPort = 8787

type saved struct {
	URL    string `json:"url"`
	Secret string `json:"secret"`
}

func Discover(getenv func(string) string, home string) (address, secret string, err error) {
	var config saved
	if raw, err := os.ReadFile(filepath.Join(home, ".config", "landline", "config.json")); err == nil {
		json.Unmarshal(raw, &config)
	}
	address = getenv("LANDLINE_URL")
	if address == "" {
		address = config.URL
	}
	if address == "" {
		address = "http://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(portFrom(getenv)))
	}
	secret = getenv("LANDLINE_SECRET")
	if secret == "" {
		secret = config.Secret
	}
	address = strings.TrimRight(address, "/")
	if err := CheckAddress(address); err != nil {
		return "", "", err
	}
	return address, secret, nil
}

var errAddress = errors.New("the hub address must look like http://host:port, with no credentials, query, or fragment")

func CheckAddress(address string) error {
	parsed, err := url.Parse(address)
	if err != nil || clean.Text(address) != address || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
		parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.ForceQuery ||
		parsed.Fragment != "" || strings.Contains(address, "#") || (parsed.Path != "" && parsed.Path != "/") {
		return errAddress
	}
	return nil
}

func portFrom(getenv func(string) string) int {
	port, err := strconv.Atoi(getenv("LANDLINE_PORT"))
	if err != nil || port < 1 || port > 65535 {
		return defaultPort
	}
	return port
}
