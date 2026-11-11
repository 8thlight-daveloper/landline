package command

import (
	"fmt"
	"html"
	"path/filepath"
	"strings"
)

const (
	LaunchdLabel = "dev.landline.hub"
	SystemdUnit  = "landline-hub.service"
)

func LaunchdPlist(binary, dataDir string) string {
	esc := html.EscapeString
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>Label</key>
  <string>%s</string>
  <key>ProgramArguments</key>
  <array>
    <string>%s</string>
    <string>server</string>
    <string>run</string>
  </array>
  <key>RunAtLoad</key>
  <true/>
  <key>KeepAlive</key>
  <true/>
  <key>StandardOutPath</key>
  <string>%s</string>
  <key>StandardErrorPath</key>
  <string>%s</string>
</dict>
</plist>
`, LaunchdLabel, esc(binary), esc(filepath.Join(dataDir, "hub.out")), esc(filepath.Join(dataDir, "hub.out")))
}

func SystemdUnitFile(binary string) string {
	quoted := strings.ReplaceAll(strings.ReplaceAll(binary, `\`, `\\`), `"`, `\"`)
	return fmt.Sprintf(`[Unit]
Description=Landline hub, a shared line for coding agents

[Service]
ExecStart="%s" server run
Restart=on-failure

[Install]
WantedBy=default.target
`, quoted)
}

type ServiceInstall struct {
	Path    string
	Content string
	Load    []string
	Note    string
}

func PlanServiceInstall(goos, home, binary, dataDir string, uid int) (ServiceInstall, error) {
	switch goos {
	case "darwin":
		path := filepath.Join(home, "Library", "LaunchAgents", LaunchdLabel+".plist")
		return ServiceInstall{
			Path: path, Content: LaunchdPlist(binary, dataDir),
			Load: []string{"launchctl", "bootstrap", fmt.Sprintf("gui/%d", uid), path},
			Note: "To stop it for good: launchctl bootout gui/" + fmt.Sprint(uid) + " " + path,
		}, nil
	case "linux":
		path := filepath.Join(home, ".config", "systemd", "user", SystemdUnit)
		return ServiceInstall{
			Path: path, Content: SystemdUnitFile(binary),
			Load: []string{"systemctl", "--user", "enable", "--now", SystemdUnit},
			Note: "To stop it for good: systemctl --user disable --now " + SystemdUnit,
		}, nil
	}
	return ServiceInstall{}, fmt.Errorf("landline server install supports macOS and Linux, and not %s", goos)
}
