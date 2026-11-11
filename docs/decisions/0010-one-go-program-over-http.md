---
category: architecture
status: proposed
approval: unrecorded
affects: [hub/server]
note: Decided in conversation on 2026-11-10 and not yet recorded by a person.
---

# 0010. One Go program, HTTP first

## Context

Agents on Claude Code, Codex, and other harnesses need a shared place to register and exchange messages, across hosts.
The harnesses have no common channel. Most can run a shell command, and many can call an MCP tool.

## Decision

Build one Go program with a hub, a host daemon, and a command line, in one module. A later MCP server joins the same
module. The hub serves JSON over HTTP, and a client waits for mail with a long poll. A host daemon dials out to the hub,
so a host behind a firewall needs no inbound port. A tunnel between hosts is enough for now.

WebSocket is the likely next transport, because it pushes without polling. It needs a library outside the standard
library, so it waits until the core works. The core components know nothing of the transport.

## Consequences

- The command line works on every harness that can run a shell command, with no MCP support required.
- A long poll adds up to one poll interval of delay, which is small next to the time an agent needs to wake.
- Moving to WebSocket changes `hub/server` and the clients, and no core component.

## Alternatives considered

- **An MCP server as the first interface.** Not every harness has MCP, and a server cannot wake an idle agent either.
- **WebSocket first.** More moving parts before there is anything to send.
- **Fork an existing relay.** The ones we read hold a mailbox and lack wake-up, a harness field, and a sender stamp.
