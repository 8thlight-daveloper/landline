package main

import (
	"bytes"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const noCredentialMessage = "there is no switchboard credential. Run landline server join <host:port> --code <code>, or set LANDLINE_SECRET"

func writeConfigWithoutSecret(t *testing.T, home, address, extraLines string) {
	t.Helper()
	host, port, err := net.SplitHostPort(strings.TrimPrefix(address, "http://"))
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(configDirectory(home), 0o700); err != nil {
		t.Fatal(err)
	}
	text := fmt.Sprintf("host = %q\nport = %s\n%s", host, port, extraLines)
	if err := os.WriteFile(filepath.Join(configDirectory(home), "config.toml"), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func shortHomeOnlyFromConfig(t *testing.T) string {
	t.Helper()
	home, err := os.MkdirTemp("/tmp", "lm")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(home) })
	homeOnlyFromConfig(t)
	t.Setenv("HOME", home)
	return home
}

func TestTheMenuNamesTheKeyFileWhenItsFirstLineIsNotAMachineKeyAndDoesNotCallTheSwitchboardUnreachable(t *testing.T) {
	home := shortHomeOnlyFromConfig(t)
	var seen seenAuthorizations
	server := newSwitchboardAnswering(t, &seen, func(*http.Request) bool { return true })
	writeConfigWithoutSecret(t, home, server.URL, "machine = \"linux-box\"\n")
	keyPath := filepath.Join(configDirectory(home), "machine.key")
	if err := os.WriteFile(keyPath, []byte("not a key\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out, errs bytes.Buffer
	run([]string{"menu", "--text"}, &out, &errs)

	menu := out.String() + errs.String()
	if !strings.Contains(menu, keyPath) {
		t.Errorf("the menu does not name the key file %s:\n%s", keyPath, menu)
	}
	if strings.Contains(strings.ToLower(menu), "unreachable") {
		t.Errorf("the menu calls the switchboard unreachable when the key file is the problem:\n%s", menu)
	}
	if strings.Contains(menu, "not a key") {
		t.Errorf("the menu repeats the content of the key file:\n%s", menu)
	}
}

func TestTheMenuShowsTheOneNoCredentialMessageWhenThereIsNoSecretAndNoKey(t *testing.T) {
	home := homeOnlyFromConfig(t)
	var seen seenAuthorizations
	server := newSwitchboardAnswering(t, &seen, func(*http.Request) bool { return true })
	writeConfigWithoutSecret(t, home, server.URL, "")

	var out, errs bytes.Buffer
	run([]string{"menu", "--text"}, &out, &errs)

	if menu := out.String() + errs.String(); !strings.Contains(menu, noCredentialMessage) {
		t.Errorf("the menu does not show the message for no credential, %q:\n%s", noCredentialMessage, menu)
	}
}
