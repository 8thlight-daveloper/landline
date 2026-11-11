package command

import (
	"strings"
	"testing"
)

func TestTheLaunchdFileRunsServerRunAtLoadAndKeepsItAlive(t *testing.T) {
	t.Parallel()
	plist := LaunchdPlist("/Users/me/.nix-profile/bin/landline", "/Users/me/.local/share/landline")
	for _, want := range []string{
		"<string>dev.landline.hub</string>", "<string>/Users/me/.nix-profile/bin/landline</string>", "<string>server</string>", "<string>run</string>",
		"<key>RunAtLoad</key>\n  <true/>", "<key>KeepAlive</key>\n  <true/>", "/Users/me/.local/share/landline/hub.out",
	} {
		if !strings.Contains(plist, want) {
			t.Errorf("no %q in\n%s", want, plist)
		}
	}
	if strings.Contains(strings.ToLower(plist), "secret") {
		t.Fatalf("the service file mentions a secret:\n%s", plist)
	}
}

func TestPathsAreEscapedInTheLaunchdFileAndQuotedInTheSystemdFile(t *testing.T) {
	t.Parallel()
	if plist := LaunchdPlist("/Users/a&b/<x>/landline", "/d"); !strings.Contains(plist, "/Users/a&amp;b/&lt;x&gt;/landline") || strings.Contains(plist, "a&b") {
		t.Fatalf("%s", plist)
	}
	unit := SystemdUnitFile(`/home/me/my "tools"/landline`)
	if !strings.Contains(unit, `ExecStart="/home/me/my \"tools\"/landline" server run`) || !strings.Contains(unit, "WantedBy=default.target") || !strings.Contains(unit, "Restart=on-failure") {
		t.Fatalf("%s", unit)
	}
}

func TestPlanServiceInstallPicksThePathAndTheLoadCommandPerSystem(t *testing.T) {
	t.Parallel()
	mac, err := PlanServiceInstall("darwin", "/Users/me", "/bin/landline", "/Users/me/.local/share/landline", 501)
	if err != nil || mac.Path != "/Users/me/Library/LaunchAgents/dev.landline.hub.plist" || strings.Join(mac.Load, " ") != "launchctl bootstrap gui/501 /Users/me/Library/LaunchAgents/dev.landline.hub.plist" || !strings.Contains(mac.Content, "<plist") {
		t.Fatalf("%+v %v", mac, err)
	}
	linux, err := PlanServiceInstall("linux", "/home/me", "/bin/landline", "/home/me/.local/share/landline", 1000)
	if err != nil || linux.Path != "/home/me/.config/systemd/user/landline-hub.service" || strings.Join(linux.Load, " ") != "systemctl --user enable --now landline-hub.service" || !strings.Contains(linux.Content, "[Service]") {
		t.Fatalf("%+v %v", linux, err)
	}
	if _, err := PlanServiceInstall("windows", "C:\\Users\\me", "landline.exe", "d", 0); err == nil {
		t.Fatal("windows was accepted")
	}
}
