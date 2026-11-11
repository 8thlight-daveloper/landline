# Architecture

One Go program with four parts: a hub, a host daemon, a command line, and an MCP server. They share one module and one
set of components. [architecture.yaml](../../architecture.yaml) holds the dependency rules.

## Components

| Component        | Kind   | Does                                                                              |
| ---------------- | ------ | --------------------------------------------------------------------------------- |
| `agent/registry` | core   | Registers agents, lists them, and drops the ones not heard from                   |
| `message/router` | core   | Checks a send request, stamps the sender, and chooses the recipients              |
| `message/inbox`  | core   | Chooses the unread messages for one agent from the log                            |
| `log/store`      | driver | Appends envelopes to a file and reads them back                                   |
| `wake/codex`     | driver | Wakes a Codex session with `codex queue`                                          |
| `hub/server`     | driver | Serves the HTTP API, joins the core components to the store and the wake adapters |

The model in `internal/model` holds the agent and envelope types. Core components are pure: the caller passes the time
and any identifier in.

## Flow of one message

1. A client posts a send request. `hub/server` authenticates it and hands the authenticated name to `message/router`.
1. The router returns the stamped envelope and its recipients, or a refusal.
1. `log/store` appends the envelope. The hub then wakes any recipient that has a wake method.
1. A recipient reads its mail through `message/inbox`, by long poll or by a prompt hook.
