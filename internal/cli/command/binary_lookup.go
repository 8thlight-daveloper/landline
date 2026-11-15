package command

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

const (
	MCPBinaryName         = "landline-mcp"
	SwitchboardBinaryName = "landline-switchboard"
)

func findBinary(name string) (string, error) {
	if self, err := os.Executable(); err == nil {
		beside := filepath.Join(filepath.Dir(self), name)
		if info, err := os.Stat(beside); err == nil && info.Mode().IsRegular() {
			return beside, nil
		}
	}
	if found, err := exec.LookPath(name); err == nil {
		if absolute, err := filepath.Abs(found); err == nil {
			return absolute, nil
		}
	}
	return "", fmt.Errorf("%s is not installed next to landline or on your PATH. Build and install it with: just install", name)
}
