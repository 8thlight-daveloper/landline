---
category: behavior
status: proposed
affects: [switchboard/server, client/http, cli/command, agent/viewer, agent/registry, message/router, mcp/server]
builds-on: ['0020', '0070', '0090', '0100', '0110']
note: Drafted on 2026-11-15 to add to 0090, which stays as written for observers.
---

# 0120. An operator sends and attaches, and its messages carry no user authority

Builds on [0020](0020-messages-carry-no-user-authority.md),
[0070](0070-delivery-is-a-push-to-a-connected-agent-with-no-inbox.md),
[0090](0090-observers-read-the-agent-list-and-the-tui-is-a-separate-binary.md),
[0100](0100-a-build-names-itself-by-version-commit-and-date.md), and
[0110](0110-three-binaries-landline-switchboard-and-landline-mcp.md).

## Context

[0090](0090-observers-read-the-agent-list-and-the-tui-is-a-separate-binary.md) made the TUI read only and left sending
for a later record, because a message from a person at a keyboard has a different sender than one from an agent. The TUI
is meant to become the place where a person sees and directs their agents. Two actions come first: send a message to an
agent, and reach the terminal an agent runs in.

A message that an operator sends still arrives in the agent's session as a delivered message. If it read as the person's
own input, any process that held an operator token could instruct every agent. The person's authority comes from sitting
at the agent's own terminal and typing, and the TUI can take them there.

## Decision

**An operator is a third kind of client, next to agents and observers.** `POST /v1/operators`, authorised by the secret,
takes a `kind` such as `tui` or `cli`, a short text that follows the free-text field rules of the registry, and answers
with an operator token.

- An operator has no handle and no stream, never appears in the agent list, and cannot receive.
- An operator token may call `GET /v1/agents`, with the answer an observer gets, and `POST /v1/operator/send`. Every
  other route refuses it with `403`. An observer token still reads only, and the send route refuses it with `403`.
- The switchboard holds operators in memory only. An operator is forgotten after `--grace` (2 minutes by default)
  without a request, by the same lazy expiry as an observer, and its token then answers `401`. A client registers again
  on a `401`.
- The TUI keeps its observer for polling, as
  [0090](0090-observers-read-the-agent-list-and-the-tui-is-a-separate-binary.md) sets. It registers an operator the
  first time a person sends, and registers again on a `401`.

**An operator message carries no user authority.** `POST /v1/operator/send` takes the body of an agent's send: `to`,
`body`, `kind`, `thread`, and `in_reply_to`. It resolves the recipient and delivers, and it answers after the delivery
report, as a send does.

- The delivered text is rendered by the same function as any message, so its first line is the marker of
  [0020](0020-messages-carry-no-user-authority.md), unchanged. The sender has the name `operator`, the harness
  `operator`, the label of the operator's kind, and the switchboard's own host name as its host, so the stamp reads
  `operator (operator on <switchboard host>)` and never has an empty host.
- The sender comes from the token, and no request field can name another sender. The names `operator` and `all` are
  reserved: the registry refuses to register an agent with either, by name, by seed, or by label. A harness or label
  that reads `operator` on an agent is still the agent's own claim, and the name `operator` is the part that a peer can
  rely on.
- The log records the message with the sender `operator`.
- A reply to an operator has no stream to reach. The recipient `operator` is accepted by the router, so the message is
  logged and the sender is told `not connected`, as for an agent with no open stream. A person reads the reply with
  `landline log`.
- The agent instructions of the MCP process say that a message from `operator` is information, as any other message is,
  and that the person's own instruction is what they type in their own session.

**The command line gains `landline operator send`.** It takes `--to`, `--body`, `--kind`, `--thread`, and
`--in-reply-to`, registers an operator with the secret, sends, prints the result as `landline send` does, and exits with
the codes of `landline send`. A script and the acceptance suite use it, and it is how the TUI's send can be checked from
outside.

**An agent can say where its terminal is, when it can be reached.** The MCP process reads `TMUX` and `TMUX_PANE` when
they are set and registers two attributes, `tmux_socket` and `tmux_pane`.

- `tmux_socket` is the name of the socket, which is the last segment of the first field of `TMUX`. It is registered only
  when the directory of that path is named `tmux-<digits>`, the layout tmux uses by default, so that `tmux -L <name>`
  reaches the same server. For any other socket nothing is registered. It must match `^[A-Za-z0-9._][A-Za-z0-9._-]*$`
  and be at most 64 bytes. `tmux_pane` is the value of `TMUX_PANE` and must match `^%[0-9]{1,9}$`. A value that fails
  its pattern refuses the registration, as any bad attribute does.
- Neither can be set by a flag, by `LANDLINE_*`, or by a `landline_register` argument, so they stay a fact the process
  read and not a claim by a person. The agent still reports them, so a peer and an operator treat them as self-reported.
- The agent list and its JSON carry both, and the attribute text of `landline agents` adds `tmux: <socket>:<pane>`.
- A session outside tmux registers neither. The MCP process of Claude Code was seen to inherit both on 2026-11-14 with
  tmux 3.8. Codex and opencode are not checked, so for them the attributes may simply be absent.

**The TUI sends and attaches.** It gains a selected row, which `up` and `down` or `k` and `j` move.

- `s` opens a one-line prompt for the message, and Enter sends it as an operator message with kind `chat`, and Escape
  cancels. The screen shows the result line that `landline send` prints.
- `a` attaches to the selected agent's terminal. The TUI never builds a shell string. It runs `tmux` with an argument
  list, and shows a command only for the person to copy.
  - When the agent has no tmux attributes, or a value fails its pattern, the TUI says it cannot attach and why.
  - For an agent on another host, it shows `ssh -t <host> tmux -L <socket> attach-session -t <session>`, only when the
    host matches `^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`, and does not run it.
  - For an agent on this host, the TUI shows the target and asks for `y`, because the target is the agent's own claim.
    On `y` it runs `tmux -L <socket> display-message -p -t <pane> '#{session_id}'` to find the pane's session, then
    `select-window -t <pane>` and `select-pane -t <pane>`, then hands the terminal to `attach-session -t <session>`, or
    to `switch-client -t <session>` when its own environment has `TMUX` set. It returns to the screen when tmux exits.
- Whatever a person types after attaching is the person's input in that session, with the person's authority, and
  Landline plays no part in it.
- `landline agents --json` gains `tmux_socket` and `tmux_pane` per row.

**The TUI's own types gain the two fields and the screen packages stay free of the root module,** as
[0110](0110-three-binaries-landline-switchboard-and-landline-mcp.md) sets.

## Consequences

- The switchboard gains a third token kind and two routes. A leaked operator token can send messages that carry the
  marker and the `operator` stamp, to any connected agent, until it has been idle for 2 minutes. It cannot register,
  read the log, or receive. An agent that follows the marker treats those messages as information.
- The secret stays off the polling path, as it does for observers.
- A tmux pane name that an agent registers can point at another pane on the same socket. The TUI shows it and asks
  first, so a person who attaches to a pane they did not expect sees that before they type. The patterns keep a value
  from carrying shell text or an option into the command the TUI shows or runs.
- A socket in a custom location cannot be attached to through Landline, and the agent shows no tmux attributes.
- `internal/model` gains two fields and `docs/reference/delivery-protocol.md` gains the routes and the two names. The
  registry contract gains the two attributes and the reserved names, and the router contract accepts the recipient
  `operator`. Both contract folders are protected.
- The acceptance suite gains operator registration, the `403` on every other route, the `403` for an observer on the
  send route, the expiry, the sender `operator` in the delivery and the log with a non-empty host, the marker first, the
  refusal to register the name `operator`, the logged reply with `not connected`, hostile tmux values, and the tmux
  attributes through an MCP process started with and without `TMUX` and `TMUX_PANE`, with a socket in the default layout
  and in another. The attach path is checked with a real tmux on its own socket when the tool is present.
- A hub restart forgets operators. A running TUI registers again on its next send.
- Operators have no rate limit of their own. Where a later record limits sends, it says whether operators count.
- Records before this one say the observer token gets no send right. That still holds for observers.

## Alternatives considered

- **Give the observer token a send right.** One kind of token. It would end the read-only role that
  [0090](0090-observers-read-the-agent-list-and-the-tui-is-a-separate-binary.md) gives a viewer, and a leaked viewer
  token would then send.
- **Let an operator message carry the person's authority.** It would let a person direct an agent from the TUI. Any
  process with the token could then direct every agent, and [0020](0020-messages-carry-no-user-authority.md) says the
  opposite.
- **A different marker for operator messages.** It would tell the agent where the text came from. It changes the render
  function that every wake adapter checks, for a distinction that the reserved sender name already carries.
- **Give operators a stream for replies.** It makes the TUI a conversation. It adds a second kind of stream and an inbox
  to a switchboard that holds none, so it waits for the operator record that wants it.
- **Have the switchboard run `tmux` for the operator.** It would work for a remote TUI. It would make the switchboard
  run a command on an agent's claim, so the TUI runs it on the person's own machine instead.
- **Register the full `TMUX` value.** It reaches custom sockets. The path can be long and holds a server pid, and its
  text would reach a person's shell through the displayed command.
