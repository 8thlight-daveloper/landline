package command

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func skipAsRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root writes a file whatever its mode")
	}
}

func blockMachineKeySave(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "machine.key.tmp", "blocker"), 0o700); err != nil {
		t.Fatal(err)
	}
}

func blockConfigSave(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "config.toml.tmp", "blocker"), 0o700); err != nil {
		t.Fatal(err)
	}
}

func TestAFailedJoinLeavesTheModeOfAReadOnlyConfigFileItNeverReplaced(t *testing.T) {
	skipAsRoot(t)
	t.Parallel()
	dir := t.TempDir()
	var out, errOut strings.Builder
	env := joinEnv(dir, &out, &errOut)
	if err := SaveConfig(env, Config{Host: "old.example", Port: 9, Machine: "alpha"}); err != nil {
		t.Fatal(err)
	}
	if err := writeMachineKey(env, mustKey(t)); err != nil {
		t.Fatal(err)
	}
	configBefore, _ := os.ReadFile(configPath(env))
	if err := os.Chmod(configPath(env), 0o400); err != nil {
		t.Fatal(err)
	}
	blockMachineKeySave(t, dir)

	if code := Run([]string{"join", redeemServer(t, "beta"), "--code", "ABCD-EFGH"}, env); code == 0 {
		t.Fatalf("the join could not save the key and still exited 0: %s", out.String())
	}

	info, err := os.Stat(configPath(env))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o400 {
		t.Errorf("the failed join changed the mode of config.toml from 0400 to %o", info.Mode().Perm())
	}
	if configAfter, _ := os.ReadFile(configPath(env)); !bytes.Equal(configAfter, configBefore) {
		t.Errorf("the failed join changed config.toml")
	}
}

func TestAFailedJoinRestoresAnEmptyConfigFileAndAnEmptyKeyFileAsEmptyFiles(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	var out, errOut strings.Builder
	env := joinEnv(dir, &out, &errOut)
	writeFile(t, dir, "config.toml", "")
	writeFile(t, dir, "machine.key", "")
	blockConfigSave(t, dir)

	if code := Run([]string{"join", redeemServer(t, "beta"), "--code", "ABCD-EFGH"}, env); code == 0 {
		t.Fatalf("the join could not save config.toml and still exited 0: %s", out.String())
	}

	for _, path := range []string{configPath(env), machineKeyPath(env)} {
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("the failed join did not leave the empty file %s: %v", filepath.Base(path), err)
			continue
		}
		if len(raw) != 0 {
			t.Errorf("the failed join left %d bytes in the file %s that was empty", len(raw), filepath.Base(path))
		}
	}
}

func largeConfigComment() string {
	line := "# " + strings.Repeat("x", 1<<20) + "\n"
	return strings.Repeat(line, 48)
}

func makeReadOnlyOnceChanged(path string, before []byte, done <-chan struct{}) <-chan bool {
	changed := make(chan bool, 1)
	go func() {
		for {
			select {
			case <-done:
				changed <- false
				return
			default:
			}
			if now, err := os.ReadFile(path); err == nil && len(now) > 0 && !bytes.Equal(now, before) {
				os.Chmod(path, 0o400)
				changed <- true
				return
			}
			time.Sleep(20 * time.Microsecond)
		}
	}()
	return changed
}

func TestAFailedJoinThatCannotPutTheEarlierKeyBackDoesNotSayItWasRestored(t *testing.T) {
	skipAsRoot(t)
	t.Parallel()
	dir := t.TempDir()
	var out, errOut strings.Builder
	env := joinEnv(dir, &out, &errOut)
	if err := SaveConfig(env, Config{Host: "old.example", Port: 9, Machine: "alpha"}); err != nil {
		t.Fatal(err)
	}
	configFile, err := os.OpenFile(configPath(env), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	configFile.WriteString(largeConfigComment())
	configFile.Close()
	if err := writeMachineKey(env, mustKey(t)); err != nil {
		t.Fatal(err)
	}
	keyBefore, _ := os.ReadFile(machineKeyPath(env))
	blockConfigSave(t, dir)
	done := make(chan struct{})
	changed := makeReadOnlyOnceChanged(machineKeyPath(env), keyBefore, done)

	code := Run([]string{"join", redeemServer(t, "beta"), "--code", "ABCD-EFGH"}, env)
	close(done)
	caught := <-changed
	os.Chmod(machineKeyPath(env), 0o600)

	if code == 0 {
		t.Fatalf("the join could not save config.toml and still exited 0: %s", out.String())
	}
	if !caught {
		t.Skip("the join put the key back before the test could make the key file read-only, so the failed restore was not staged")
	}
	keyAfter, _ := os.ReadFile(machineKeyPath(env))
	if bytes.Equal(keyAfter, keyBefore) {
		return
	}
	message := errOut.String()
	if strings.Contains(message, "were restored") || strings.Contains(message, "was restored") {
		t.Errorf("machine.key still holds the new key after the failed join, and the join said: %s", message)
	}
	if !strings.Contains(message, "machine.key") {
		t.Errorf("the join did not name the key file it could not put back: %s", message)
	}
}
