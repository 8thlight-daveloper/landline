# Architecture

One Go program with three parts: a hub, a command line, and an MCP server. They share one module and one set of
components. [architecture.yaml](../../architecture.yaml) holds the dependency rules.

## Components

[components.md](components.md) lists each component, its position, and its kind.

The model in `internal/model` holds the agent and envelope types, the message rendering, and the JSON shapes of the
hub's API, which the hub and every client share. Core components are pure: the caller passes the time and any identifier
in.

## Flow of one message

1. A client posts a send request. `hub/server` authenticates it and hands the authenticated name to `message/router`.
1. The router returns the stamped envelope and its recipients, or a refusal.
1. `log/store` appends the envelope.
1. If the recipient is not connected, the send fails and nothing is held. Otherwise the hub writes a delivery to the
   recipient's stream, which the recipient's `landline mcp` process opened with `GET /v1/stream`.
1. The MCP process delivers the text, as a channel event or, for Codex, with `codex queue` through `wake/codex`, and
   posts a delivery report. The hub answers the sender with that outcome.

The hub keeps no inbox and no wake queue. [../reference/delivery-protocol.md](../reference/delivery-protocol.md) defines
the stream and the report.
