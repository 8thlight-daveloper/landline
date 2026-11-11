# Architecture

One Go program with four parts: a hub, a host daemon, a command line, and an MCP server. They share one module and one
set of components. [architecture.yaml](../../architecture.yaml) holds the dependency rules.

## Components

[components.md](components.md) lists each component, its position, and its kind.

The model in `internal/model` holds the agent and envelope types, the message rendering, and the JSON shapes of the
hub's API, which the hub and every client share. Core components are pure: the caller passes the time and any identifier
in.

## Flow of one message

1. A client posts a send request. `hub/server` authenticates it and hands the authenticated name to `message/router`.
1. The router returns the stamped envelope and its recipients, or a refusal.
1. `log/store` appends the envelope. The hub then wakes any recipient that has a wake method.
1. A recipient reads its mail through `message/inbox`, by long poll or by a prompt hook.
