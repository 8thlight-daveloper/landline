# Components

The components in [architecture.yaml](../../architecture.yaml), with their position and kind. A core component is pure.
A driver does input and output.

| Component        | Position | Kind   | Does                                                                                                           |
| ---------------- | -------- | ------ | -------------------------------------------------------------------------------------------------------------- |
| `agent/registry` | 1        | core   | Registers agents and resolves a name to one of them                                                            |
| `message/router` | 1        | core   | Checks a send request, stamps the sender, and chooses the recipients                                           |
| `log/store`      | 2        | driver | Appends envelopes to a file and reads them back                                                                |
| `wake/codex`     | 2        | driver | Queues a turn into a Codex session with `codex queue`                                                          |
| `client/http`    | 2        | driver | Calls the hub's HTTP API                                                                                       |
| `hub/server`     | 3        | driver | Serves the HTTP API, holds agents, tokens, and observers in memory, and joins the core components to the store |
| `cli/command`    | 3        | driver | The command line: register, send, agents, log, tui, and a session that holds the stream                        |
| `mcp/server`     | 3        | driver | Serves the register, agents, and send tools over MCP, holds the delivery stream, and hands each delivery on    |
| `agent/viewer`   | 3        | driver | The `tui/` module, built as `landline-tui`. Shows the agent list on a terminal and as JSON, and only reads     |

`agent/viewer` is the one component outside `internal/`. It is its own Go module, so it holds its own small copies of
the hub's JSON types and imports no other component. It reads the hub over HTTP with an observer token and reads
`$HOME/.config/landline/config.json` for the hub address and secret. It writes the terminal and standard output, and no
files. `landline tui` finds `landline-tui` on the path or beside the `landline` executable and replaces itself with it.

`binary/` is the entry point. It wires the components together and is outside the dependency graph. The hub takes its
store as an interface, and `mcp/server` takes its Codex waker as one, so the wiring in `binary/main.go` is what joins
them.
