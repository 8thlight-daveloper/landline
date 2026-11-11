package command

import (
	"reflect"
	"testing"
)

func TestSetupCommandAddsTheServerToEachHarnessWithTheBinaryPath(t *testing.T) {
	t.Parallel()
	claude, err := SetupCommand("claude", "/home/me/bin/landline")
	if err != nil || !reflect.DeepEqual(claude, []string{"claude", "mcp", "add", "--scope", "user", "landline", "--", "/home/me/bin/landline", "mcp", "--channel"}) {
		t.Fatalf("%v %v", claude, err)
	}
	codex, err := SetupCommand("codex", "/home/me/bin/landline")
	if err != nil || !reflect.DeepEqual(codex, []string{"codex", "mcp", "add", "landline", "--", "/home/me/bin/landline", "mcp"}) {
		t.Fatalf("%v %v", codex, err)
	}
	if _, err := SetupCommand("cursor", "x"); err == nil {
		t.Fatal("an unknown harness was accepted")
	}
}

func TestTheClaudeLauncherLoadsTheChannelAndPassesAttributesInTheEnvironment(t *testing.T) {
	t.Parallel()
	launch, err := ClaudeLaunch([]string{"--label", "tutor", "--model", "claude-opus-5-5", "--", "--resume", "abc"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(launch.Argv, []string{"claude", "--dangerously-load-development-channels", "server:landline", "--resume", "abc"}) {
		t.Fatalf("%v", launch.Argv)
	}
	if !reflect.DeepEqual(launch.Env, []string{"LANDLINE_LABEL=tutor", "LANDLINE_MODEL=claude-opus-5-5"}) {
		t.Fatalf("%v", launch.Env)
	}
	bare, err := ClaudeLaunch(nil)
	if err != nil || len(bare.Env) != 0 || len(bare.Argv) != 3 {
		t.Fatalf("%v %v", bare, err)
	}
}

func TestTheClaudeLauncherRefusesUnknownFlagsAndMissingValuesAndNeverPutsSecretsInTheEnvironment(t *testing.T) {
	t.Parallel()
	for _, bad := range [][]string{{"--resume"}, {"--label"}, {"--label", " "}, {"--model"}} {
		if _, err := ClaudeLaunch(bad); err == nil {
			t.Errorf("%v was accepted", bad)
		}
	}
	launch, _ := ClaudeLaunch([]string{"--label", "x"})
	for _, entry := range launch.Env {
		if len(entry) >= 17 && entry[:17] == "LANDLINE_SECRET=" {
			t.Fatalf("the secret was put in the environment: %v", launch.Env)
		}
	}
}
