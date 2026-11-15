# Components

The components in [architecture.yaml](../../architecture.yaml), with their position and kind. A core component is pure.
A driver does input and output.

| Component            | Position | Kind   | Does                                                                                                                              |
| -------------------- | -------- | ------ | --------------------------------------------------------------------------------------------------------------------------------- |
| `agent/registry`     | 1        | core   | Registers agents and resolves a name to one of them                                                                               |
| `message/router`     | 1        | core   | Checks a send request, stamps the sender, and chooses the recipients                                                              |
| `log/store`          | 2        | driver | Appends envelopes to a file and reads them back                                                                                   |
| `wake/codex`         | 2        | driver | Queues a turn into a Codex session with `codex queue`                                                                             |
| `client/http`        | 2        | driver | Calls the switchboard's HTTP API                                                                                                  |
| `switchboard/server` | 3        | driver | Serves the HTTP API, holds agents, tokens, and observers in memory, and joins the core components to the store                    |
| `cli/command`        | 3        | driver | The commands of `landline` except `agents`: setup, claude, server, register, send, log, join, and a session that holds the stream |
| `mcp/server`         | 3        | driver | Serves the register, agents, and send tools over MCP, holds the delivery stream, and hands each delivery on                       |
| `agent/viewer`       | 3        | driver | The screen packages of the `tui/` module. Show the agent list on a terminal, and only read                                        |

`agent/viewer` is the one component outside `internal/`. It is the set of screen packages under `tui/internal/`, in the
`tui/` Go module. They hold their own small copies of the switchboard's JSON types and import no root package and no
other component. They read the switchboard over HTTP with an observer token and read
`$HOME/.config/landline/config.json` for its address and secret. They write the terminal and standard output, and no
files.

The entry points are not components and are outside the dependency graph, so they may import any component.
`cmd/landline-switchboard` serves the switchboard, and `cmd/landline-mcp` serves the MCP process and holds no
switchboard code. `tui/main.go` builds `landline`. It handles `agents` itself with the client and view model of
`agent/viewer`, converts each row to the root model's type for the attribute text, and hands every other command to
`cli/command`. It is the only file of the `tui/` module that imports root packages. The switchboard takes its store as
an interface, and `mcp/server` takes its Codex waker as one, so the wiring in the entry points is what joins them.
