package command

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAFailedJoinLeavesAnEarlierKeyFileItCannotReadInPlaceAndUnchanged(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads a file with mode 0000, so the key file cannot be made unreadable")
	}
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
	keyBefore, _ := os.ReadFile(machineKeyPath(env))
	configBefore, _ := os.ReadFile(configPath(env))
	if err := os.Chmod(machineKeyPath(env), 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(machineKeyPath(env), 0o600) })
	if err := os.MkdirAll(filepath.Join(dir, "config.toml.tmp", "blocker"), 0o700); err != nil {
		t.Fatal(err)
	}

	if code := Run([]string{"join", redeemServer(t, "beta"), "--code", "ABCD-EFGH"}, env); code == 0 {
		t.Fatalf("the join could not save config.toml and still exited 0: %s", out.String())
	}

	info, statErr := os.Stat(machineKeyPath(env))
	if statErr != nil {
		t.Fatalf("the failed join removed the earlier key file it could not read: %v. stderr: %s", statErr, errOut.String())
	}
	if info.Mode().Perm() != 0o000 {
		t.Errorf("the failed join changed the mode of the earlier key file from 0000 to %o", info.Mode().Perm())
	}
	os.Chmod(machineKeyPath(env), 0o600)
	if keyAfter, _ := os.ReadFile(machineKeyPath(env)); string(keyAfter) != string(keyBefore) {
		t.Errorf("the failed join replaced the earlier key file it could not read")
	}
	if configAfter, _ := os.ReadFile(configPath(env)); string(configAfter) != string(configBefore) {
		t.Errorf("the failed join changed config.toml")
	}
}
