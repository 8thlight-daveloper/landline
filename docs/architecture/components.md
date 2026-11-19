# Components

The components in [architecture.yaml](../../architecture.yaml), with their position and kind. A core component is pure.
A driver does input and output.

| Component            | Position | Kind   | Does                                                                                                                              |
| -------------------- | -------- | ------ | --------------------------------------------------------------------------------------------------------------------------------- |
| `agent/registry`     | 1        | core   | Registers agents and looks one up by exact id                                                                                     |
| `message/router`     | 1        | core   | Checks a send request, stamps the sender, and chooses the recipients                                                              |
| `machine/registry`   | 1        | core   | Holds invites and machines as values, counts failed redeems, and checks a machine's signed request against a given time           |
| `log/store`          | 2        | driver | Appends envelopes to a file and reads them back                                                                                   |
| `wake/codex`         | 2        | driver | Queues a turn into a Codex session with `codex queue`                                                                             |
| `client/http`        | 2        | driver | Calls the switchboard's HTTP API                                                                                                  |
| `switchboard/server` | 3        | driver | Serves the HTTP API, holds agents, tokens, and observers in memory, and joins the core components to the store                    |
| `cli/command`        | 3        | driver | The commands of `landline` except `agents`: setup, claude, server, register, send, log, join, and a session that holds the stream |
| `mcp/server`         | 3        | driver | Serves the register, agents, and send tools over MCP, holds the delivery stream, and hands each delivery on                       |
| `agent/viewer`       | 3        | driver | The screen packages of the `tui/` module. Show the agent list on a terminal, and only read                                        |
| `menubar/viewer`     | 3        | driver | The packages of the `menubar/` module. Show the agents in a macOS menu bar item, and run `landline` for server and setup          |

`agent/viewer` is the one component outside `internal/`. It is the set of screen packages under `tui/internal/`, in the
`tui/` Go module. They hold their own small copies of the switchboard's JSON types and import no root package and no
other component. They read the switchboard over HTTP with an observer token and read
`$HOME/.config/landline/config.toml` for its host, port, and secret, and the old `config.json` when there is no
`config.toml`. They write the terminal and standard output, and no files.

`menubar/viewer` is the set of packages of the `menubar/` Go module, outside `internal/`, built on macOS only
([0140](../decisions/0140-a-macos-menu-bar-item-shows-agents-runs-the-switchboard-and-sets-up-harnesses.md)). Like
`agent/viewer` it holds its own copies of the switchboard's JSON types and imports no root package and no other
component. It reads the switchboard over HTTP with an observer token, including `GET /v1/build`, and reads
`$HOME/.config/landline/config.toml` for the host, port, and secret, and the old `config.json` when there is no
`config.toml`. It tests whether the host is this machine's, and runs `landline server up|down|restart|rotate-secret` and
`landline setup claude|codex [--remove]` as child processes, and asks the login shell once for its `PATH`. It writes the
menu bar, the About panel, and the clipboard, and no files.

The entry points are not components and are outside the dependency graph, so they may import any component.
`cmd/landline-switchboard` serves the switchboard, and `cmd/landline-mcp` serves the MCP process and holds no
switchboard code. `tui/main.go` builds `landline`. It handles `agents` itself with the client and view model of
`agent/viewer`, converts each row to the root model's type for the attribute text, and hands every other command to
`cli/command`. It is the only file of the `tui/` module that imports root packages. The switchboard takes its store as
an interface, and `mcp/server` takes its Codex waker as one, so the wiring in the entry points is what joins them.
