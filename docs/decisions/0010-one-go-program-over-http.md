---
category: architecture
status: proposed
approval: unrecorded
affects: [hub/server, client/http, cli/command, daemon/agentd, mcp/server]
note: Decided in conversation on 2026-11-10 and not yet recorded by a person.
---

# 0010. One Go program, HTTP first

## Context

Agents on Claude Code, Codex, and other harnesses need a shared place to register and exchange messages, across hosts.
The harnesses have no common channel. Most can run a shell command, and many can call an MCP tool.

## Decision

Build one Go program with a hub, a host daemon, a command line, and an MCP server, in one module with no third-party
dependency. The hub serves JSON over HTTP. A client waits for mail with a long poll: the hub holds the request open, up
to 60 seconds, and answers as soon as mail arrives. A host daemon dials out to the hub, so a host behind a firewall
needs no inbound port. A tunnel between hosts is enough for now.

WebSocket is the likely next transport, because it pushes without polling. It needs a library outside the standard
library, so it waits until the core works. The core components know nothing of the transport.

The JSON shapes of the hub's API live in the domain model, because the hub and every client share them.

## Consequences

- The command line works on every harness that can run a shell command, with no MCP support required.
- Moving to WebSocket changes `hub/server` and the clients. The JSON shapes in the model may gain fields, which is a
  model change a person approves.
- Keeping the transport shapes in the model puts them under the model's protection. That is deliberate for now, and
  moving them to their own folder is the alternative when a second transport arrives.

## Alternatives considered

- **An MCP server as the first interface.** Not every harness has MCP, and a server cannot wake an idle agent either.
- **WebSocket first.** More moving parts before there is anything to send.
- **Fork an existing relay.** The ones we read hold a mailbox and lack wake-up, a harness field, and a sender stamp.
