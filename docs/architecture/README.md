# Architecture

Three binaries with one job each ([0110](../decisions/0110-three-binaries-landline-switchboard-and-landline-mcp.md)):
`landline-switchboard` holds the agent list and routes messages, `landline-mcp` is the MCP process a harness starts for
one session, and `landline` is what a person runs. The first two are entry points in the root module under `cmd/`.
`landline` is built in the module under `tui/`, which imports the root module's code, and holds the terminal screen
([0090](../decisions/0090-observers-read-the-agent-list-and-the-tui-is-a-separate-binary.md)). Earlier records call the
switchboard the hub. [architecture.yaml](../../architecture.yaml) holds the dependency rules.

## Components

[components.md](components.md) lists each component, its position, and its kind.

The model in `internal/model` holds the agent and envelope types, the message rendering, and the JSON shapes of the
switchboard's API, which the switchboard and every client share. Core components are pure: the caller passes the time
and any identifier in.

## Flow of one message

1. A client posts a send request. `switchboard/server` drops the agents past their grace, authenticates the request,
   resolves the recipient with `agent/registry` against connected agents first, and hands the names to `message/router`.
1. The router returns the stamped envelope and its recipients, or a refusal.
1. `log/store` appends the envelope.
1. If the recipient is not connected, the send fails and nothing is held. Otherwise the switchboard writes a delivery to
   the recipient's stream, which the recipient's `landline-mcp` process opened with `GET /v1/stream`.
1. The MCP process delivers the text, as a channel event or, for Codex, with `codex queue` through `wake/codex`, and
   posts a delivery report. The switchboard answers the sender with that outcome.

The switchboard keeps no inbox and no wake queue. It holds agents and tokens in memory only, while an agent is connected
and for 2 minutes after ([0080](../decisions/0080-the-hub-keeps-only-connected-agents.md)).
[../reference/delivery-protocol.md](../reference/delivery-protocol.md) defines the stream and the report.
