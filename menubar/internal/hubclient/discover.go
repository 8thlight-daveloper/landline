package hubclient

import (
	"errors"
	"net"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/8thlight-daveloper/landline/menubar/internal/clean"
)

const defaultPort = 8787

func Discover(getenv func(string) string, home string) (address, secret string, err error) {
	address, credential, err := DiscoverCredential(getenv, home)
	return address, credential.Secret, err
}

func DiscoverCredential(getenv func(string) string, home string) (address string, credential Credential, err error) {
	directory := filepath.Join(home, ".config", "landline")
	config, err := readSaved(directory)
	if err != nil {
		return "", Credential{}, err
	}
	address = getenv("LANDLINE_URL")
	if address == "" {
		host, port := config.Host, config.Port
		if host == "" {
			host = "127.0.0.1"
		}
		if port == 0 {
			port = portFrom(getenv)
		}
		address = "http://" + net.JoinHostPort(host, strconv.Itoa(port))
	}
	credential, err = readCredential(directory, getenv, config)
	if err != nil {
		return "", Credential{}, err
	}
	address = strings.TrimRight(address, "/")
	if err := CheckAddress(address); err != nil {
		return "", Credential{}, err
	}
	return address, credential, nil
}

var errAddress = errors.New("the switchboard address must look like http://host:port, with no credentials, query, or fragment")

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
