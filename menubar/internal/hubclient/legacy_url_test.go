package hubclient

import (
	"fmt"
	"strings"
	"testing"
)

func discoverRecovered(home string) (address, secret string, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("Discover panicked: %v", recovered)
			address, secret = "", ""
		}
	}()
	return Discover(env(nil), home)
}

func TestAnOldConfigJSONWithAURLWithoutASchemeIsReadAsJoinReadsAnAddress(t *testing.T) {
	for url, want := range map[string]string{
		"localhost:8787":  "http://localhost:8787",
		"alpha-host:8787": "http://alpha-host:8787",
		"127.0.0.1:9":     "http://127.0.0.1:9",
		"[::1]:9":         "http://[::1]:9",
	} {
		old := `{"url":"` + url + `","secret":"old-secret-0000000"}`
		address, secret, err := discoverRecovered(homeWith(t, map[string]string{"config.json": old}))
		if err != nil || address != want || secret != "old-secret-0000000" {
			t.Errorf("config.json url %q: got %q, secret kept %v, %v; want %q as landline server join reads it", url, address, secret == "old-secret-0000000", err, want)
		}
	}
}

func TestAnOldConfigJSONWithAURLThatDoesNotParseIsAnErrorThatNamesTheFileAndNotAPanic(t *testing.T) {
	for _, bad := range []string{"http://host:abc", "http://[::1", "http://a b:1", "host:abc", "http://host:99999"} {
		_, _, err := discoverRecovered(homeWith(t, map[string]string{"config.json": `{"url":"` + bad + `","secret":"old-secret-0000000"}`}))
		if err == nil || strings.Contains(err.Error(), "panicked") || !strings.Contains(err.Error(), "config.json") || strings.Contains(err.Error(), "old-secret") {
			t.Errorf("config.json url %q: %v, want an error that names config.json", bad, err)
		}
	}
}
