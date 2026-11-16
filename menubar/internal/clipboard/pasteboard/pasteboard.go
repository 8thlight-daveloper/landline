package pasteboard

import (
	"os"
	"os/exec"
	"strings"
)

const Binary = "/usr/bin/pbcopy"

func Copy(binary, text string) error {
	cmd := exec.Command(binary)
	cmd.Env = append(os.Environ(), "LANG=en_US.UTF-8")
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}
