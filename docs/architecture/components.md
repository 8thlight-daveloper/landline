# Components

The components in [architecture.yaml](../../architecture.yaml), with their position and kind. A core component is pure.
A driver does input and output.

| Component        | Position | Kind   | Does                                                                           |
| ---------------- | -------- | ------ | ------------------------------------------------------------------------------ |
| `agent/registry` | 1        | core   | Registers agents, lists them, and drops the ones not heard from                |
| `message/router` | 1        | core   | Checks a send request, stamps the sender, and chooses the recipients           |
| `message/inbox`  | 1        | core   | Chooses the unread messages for one agent from the log                         |
| `log/store`      | 2        | driver | Appends envelopes to a file and reads them back                                |
| `wake/codex`     | 2        | driver | Wakes a Codex session with `codex queue`                                       |
| `client/http`    | 2        | driver | Calls the hub's HTTP API                                                       |
| `hub/server`     | 3        | driver | Serves the HTTP API and joins the core components to the store                 |
| `cli/command`    | 3        | driver | The command line: register, send, inbox, listen, hook, and the rest            |
| `daemon/agentd`  | 3        | driver | Polls the hub for wake jobs on this host and runs them                         |
| `mcp/server`     | 3        | driver | Serves the send, inbox, and agents tools over MCP on standard input and output |

`binary/` is the entry point. It wires the components together and is outside the dependency graph. The hub and the
daemon take their store and their wake adapters as interfaces, so the wiring in `binary/main.go` is what joins them.
