---
category: architecture
status: accepted
approval: person
affects: [hub/server, mcp/server, wake/codex, daemon/agentd, message/inbox, message/router, client/http, cli/command]
builds-on: ['0010', '0020', '0060']
partly-supersedes: ['0030', '0050']
note: Drafted on 2026-11-11 after two hand tests of `codex queue` from inside an MCP process. It reverses the inbox decision of 0050.
---

# 0070. Delivery is a push to a connected agent, with no inbox

Builds on [0010](0010-one-go-program-over-http.md), [0020](0020-messages-carry-no-user-authority.md), and
[0060](0060-agents-register-themselves-with-a-handle-and-attributes.md). It partly supersedes
[0030](0030-wake-adapters-per-harness.md), which describes the host daemon and the hub's wake queue, and
[0050](0050-delivery-is-an-ordered-list-of-strategies.md), which keeps the inbox as the last delivery strategy.

## Context

Messages reach an agent through three paths today.

- The hub stores every message in the recipient's inbox. The agent reads it through the `landline_inbox` tool, the hook,
  or `landline listen`.
- A Claude Code session started with channels gets a push. That push is the `landline mcp` process long-polling the
  hub's inbox and writing a channel event for each message (`pushMail` in `internal/mcp/server/server.go`). The inbox is
  the transport, not a requirement of the channel.
- An idle Codex session is woken by `codex queue`. On the hub's own host the hub runs it. For another host the hub
  queues a wake job, and `landline agentd` on that host polls for jobs, runs `codex queue`, and acknowledges. The hub
  leases a job for 60 seconds and offers it again if it is not acknowledged.

The two harnesses Landline serves need none of this. A held connection from the agent's own MCP process to the hub is
enough, and it needs no inbound port on the agent's machine.

Two hand tests on 2026-11-11 used Codex 0.162.1 in an interactive session, with a throwaway MCP server that Codex
started. The server was not Landline.

- **The MCP process stays alive while the session is idle.** The server logged a heartbeat every second across an idle
  period of about 20 seconds with no turn running. It was not tested for longer, and the decision below does not depend
  on it.
- **The MCP process can run `codex queue` itself.** The server ran `codex queue --thread <id> --message <text>` and it
  returned 0. About 6 seconds later the session started a turn, with the text as a user message, and replied.
- **A message queued during a running turn is held, then run next.** The server queued a message 14 seconds into a
  60-second shell command. The running turn finished and replied. The queued message started a turn 32 milliseconds
  later, once, with nothing lost.
- **The MCP process cannot read the session id.** Its environment held no `CODEX_THREAD_ID`, as
  [0060](0060-agents-register-themselves-with-a-handle-and-attributes.md) records, so the proof step that gives it the
  id stays.
- **`codex queue` succeeds for a session that has exited.** It returned 0 and printed "Queued message" for a thread
  whose Codex process had been killed. The message was stored with the thread and ran as a turn when the session was
  resumed about ten minutes later. For an id Codex has no record of, it returned 1 with "no rollout found", and for an
  unknown name it returned 1 with "No active session found". A return of 0 therefore says that Codex stored the message,
  and not that a live session received it.
- **A resumed Codex session starts a new MCP process.** The resumed session kept its thread id, and the server started
  again with a new process id, so under [0060](0060-agents-register-themselves-with-a-handle-and-attributes.md) it
  registers again and gets a new handle.

## Decision

**The hub keeps no mail.** It records who is connected and how to reach each agent, and nothing else about a message
beyond the log. A message is either delivered to a connected agent or the sender is told that it failed.

**Delivery is done by the agent's own MCP process.**

1. Each `landline mcp` opens one outbound stream to the hub and keeps it open. The hub sends on that stream, and the MCP
   process answers on a separate request, so the stream can be a server-sent event stream over plain HTTP.
1. The sender calls `landline_send` with one recipient. The hub finds the recipient and sends the rendered message down
   that recipient's stream.
1. The MCP process delivers in the way its harness allows. Under Claude Code it writes a channel event. Under a Codex
   session it has proven (the rule of [0060](0060-agents-register-themselves-with-a-handle-and-attributes.md)), it runs
   the `wake/codex` adapter, which runs `codex queue`.
1. The MCP process reports success or the failure to the hub, and the hub reports it to the sender. "Delivered" means
   the channel event was written, or `codex queue` returned 0. It does not mean the agent has read or acted on it. A
   Codex message that `codex queue` stored for a session that is no longer running counts as delivered, because Codex
   holds it for that session.

**The connection is a stream, so there is no gap and no grace.** A stream has no moment between one request and the
next, which a long poll has. The hub knows an agent is gone when its stream closes, where it could only guess from a
missing poll. The hub never holds a message for later.

**What happens when a connection is lost.**

- The hub marks the agent disconnected as soon as its stream closes. A clean exit closes the stream, so no separate
  deregister call exists.
- The hub writes a keepalive comment on each stream every 15 seconds. A write that fails, or two keepalives in a row
  that the connection does not take, marks the agent disconnected. This catches a peer that vanished without closing,
  such as a laptop that went to sleep or a tunnel that fell.
- A send to a disconnected agent fails at once with "not connected", and the sender is told.
- `landline_agents` lists the agent with `disconnected` in place of its connection state, until it reconnects or the hub
  forgets it 10 minutes after the stream closed. A recipient that is not connected is never silently skipped.
- The MCP process reconnects with growing delays. It presents the token it got at registration, so a reconnect by the
  same process keeps the same handle. A restarted process registers again and gets a new handle, as in
  [0060](0060-agents-register-themselves-with-a-handle-and-attributes.md).
- This covers every cause the same way: a dropped network, a laptop asleep, a tunnel that fell, an MCP process that
  exited, and a Codex session that was closed. The hub does not tell them apart.

**A send names one recipient.** The `all` recipient goes away. Landline has no use for it yet, and a result for each
recipient is not worth designing before one does.

**These parts are removed.**

- The inbox, its cursors, and read marks.
- The `all` recipient of `landline_send`.
- The `landline_inbox` tool, `landline listen`, and the hook command.
- `landline agentd`, the wake job queue, the 60-second lease, and the wake acknowledgement call.
- The hub-side wake of [0030](0030-wake-adapters-per-harness.md). The `wake/codex` adapter stays and moves into the MCP
  process.

**These parts are kept.**

- The marker and the sender stamp of [0020](0020-messages-carry-no-user-authority.md) lead every delivered text, on
  every path.
- The log, which records each send and its result, so a failure is visible after the fact.
- Handles, attributes, and the Codex proof of [0060](0060-agents-register-themselves-with-a-handle-and-attributes.md).
- Only `landline_register`, `landline_agents`, and `landline_send` remain as tools. Mail arrives as a channel event or a
  queued turn, and never through a tool the agent calls.

**The harness supports delivery or it is not supported.** A harness joins Landline by adding one delivery adapter that
the MCP process can run. A session without one, such as Claude Code started without channels, can send and list agents
and cannot receive.

## Consequences

- One delivery path replaces three. The hub loses its inbox store, the long-poll for readers, and the wake job queue
  with its lease. The CLI loses `listen`, `hook`, `agentd`, and the inbox command.
- Setting up a second machine drops a step. A Codex host needs `landline setup codex` and nothing else, where today it
  also needs `landline agentd` running.
- A sender learns the outcome at once. Today a message to an offline agent is accepted and may never be read.
- Mail is lost when the recipient is not connected. An MCP restart, a dropped network, or a session that has exited
  means the sender sees a failure and retries. Nothing catches the agent up afterward.
- A channel event that Claude Code drops silently, as in the findings of
  [0050](0050-delivery-is-an-ordered-list-of-strategies.md), is now a lost message with no inbox to recover it. The
  one-second wait before the first push stays, and the loss is seen only by the person.
- The acceptance scenarios gain a case for a recipient that is gone: register an agent, stop its MCP process, send to
  it, and expect "not connected" and a `disconnected` entry in the agent list. It is not specific to one harness.
- An agent can no longer send and then block on a reply with `wait_seconds`. A reply arrives as a new delivery.
- Registration loses the `delivery` list and `wake_kind` that 0050 and 0030 add, because a session's delivery method is
  fixed by its harness.
- The contracts and acceptance scenarios for the inbox, the router's read marks, and `agentd` change or go. That needs a
  person to approve each protected unit.
- Landline still needs a held outbound connection per agent, and the hub still listens on a port. Nothing here opens a
  listener on an agent's machine.

## Alternatives considered

- **Keep the inbox and drop only `agentd`.** The MCP process would run `codex queue` for each message it reads from its
  inbox poll. It removes the daemon and the wake job queue and keeps catch-up and offline mail. It leaves the cursor and
  read-mark rules, and the "woken and also unread" overlap that 0030 records. Revisit if losing mail turns out to matter
  in use.
- **The hub calls an endpoint on the MCP process.** The MCP process speaks standard input and output and has no
  listener. Adding one needs an inbound port on every agent's machine, which a firewall or a tunnel must allow. The
  hub's default of localhost only exists to avoid that.
- **Keep `agentd` as it is.** It also covers a Codex session whose MCP process has exited. The tests show no need for
  that case, and it costs a second daemon, a job queue, and a lease. Revisit if a Codex release stops its MCP processes
  while a session idles.
- **Drive a terminal with tmux.** [0030](0030-wake-adapters-per-harness.md) rejected it. Nothing here changes that.

## Open questions

- None. The wire form of the stream and of the delivery report is left to the contracts for `client/http` and
  `hub/server`.
