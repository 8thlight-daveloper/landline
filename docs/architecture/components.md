# Components

The components in [architecture.yaml](../../architecture.yaml), with their position and kind. A core component is pure.
A driver does input and output.

| Component        | Position | Kind   | Does                                                                                                        |
| ---------------- | -------- | ------ | ----------------------------------------------------------------------------------------------------------- |
| `agent/registry` | 1        | core   | Registers agents and lists the ones heard from recently                                                     |
| `message/router` | 1        | core   | Checks a send request, stamps the sender, and chooses the recipients                                        |
| `log/store`      | 2        | driver | Appends envelopes to a file and reads them back                                                             |
| `wake/codex`     | 2        | driver | Queues a turn into a Codex session with `codex queue`                                                       |
| `client/http`    | 2        | driver | Calls the hub's HTTP API                                                                                    |
| `hub/server`     | 3        | driver | Serves the HTTP API and joins the core components to the store                                              |
| `cli/command`    | 3        | driver | The command line: register, send, agents, log, and a session that holds the stream                          |
| `mcp/server`     | 3        | driver | Serves the register, agents, and send tools over MCP, holds the delivery stream, and hands each delivery on |

`binary/` is the entry point. It wires the components together and is outside the dependency graph. The hub takes its
store as an interface, and `mcp/server` takes its Codex waker as one, so the wiring in `binary/main.go` is what joins
them.
