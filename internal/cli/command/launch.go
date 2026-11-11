package command

import (
	"fmt"
	"strings"
)

const ChannelServerName = "landline"

func SetupCommand(harness, binary string) ([]string, error) {
	switch harness {
	case "claude":
		return []string{"claude", "mcp", "add", "--scope", "user", ChannelServerName, "--", binary, "mcp", "--channel"}, nil
	case "codex":
		return []string{"codex", "mcp", "add", ChannelServerName, "--", binary, "mcp"}, nil
	}
	return nil, fmt.Errorf("landline setup knows claude and codex, and not %q", harness)
}

type Launch struct {
	Argv []string
	Env  []string
}

func ClaudeLaunch(args []string) (Launch, error) {
	launch := Launch{Argv: []string{"claude", "--dangerously-load-development-channels", "server:" + ChannelServerName}}
	var pass []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--label", "--model", "--effort":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return Launch{}, fmt.Errorf("%s needs a value", args[i])
			}
			launch.Env = append(launch.Env, "LANDLINE_"+strings.ToUpper(strings.TrimPrefix(args[i], "--"))+"="+args[i+1])
			i++
		case "--":
			pass = append(pass, args[i+1:]...)
			i = len(args)
		default:
			return Launch{}, fmt.Errorf("unknown flag %s. Put arguments for claude after --", args[i])
		}
	}
	launch.Argv = append(launch.Argv, pass...)
	return launch, nil
}
