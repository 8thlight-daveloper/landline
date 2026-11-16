# A read-only TUI and events from harness hooks

A direction and not a decision. Nothing here is built, and no record depends on it. A part becomes an ADR when someone
takes it on. Set by the owner on 2026-11-15.

## Direction

The TUI is for observability. It shows an operator which sessions are registered and, later, what they are doing. It
stays read-only. It does not send messages, and it does not attach to a session's terminal.

Switching a terminal multiplexer to an agent's input surface is a possible later step. Nothing here plans it.

## What the TUI shows today

The TUI lists the agents that the switchboard holds, as
[0090](../decisions/0090-observers-read-the-agent-list-and-the-tui-is-a-separate-binary.md) sets. Candidates that need
no new switchboard state:

- A selected row, with a details pane for the fields an agent registered: folder, host, harness, model, label.
- The tmux socket and pane that the MCP process can read from its environment, shown as text. A person copies the attach
  command and runs it. Landline runs nothing.

## Events from hooks

Harnesses can run a command at points in a session, such as session start, a tool call, or the end of a turn. A hook
could report each point to the switchboard, and the TUI could show what each agent is doing now.

What a record for this must settle:

- **Authentication.** A hook process is not the MCP process, so it holds no agent token. Either the MCP process writes a
  per-session token to a private file that the hook reads, or the switchboard gains a new kind of token for reporters. A
  new token kind needs a person's approval.
- **Content.** Metadata only: the event type, the tool name, a time, and the agent's handle. No prompt text, file
  content, or tool output, because those can hold secrets.
- **Retention.** A bounded ring in memory, so the switchboard stays free of saved state as
  [0080](../decisions/0080-the-hub-keeps-only-connected-agents.md) says.
- **Harness support.** Claude Code has hooks. Codex and opencode are not checked.

## Dropped by this direction

[0120](draft-decisions/operator-sends-and-attaches.md) proposed an operator token, `landline operator send`, and TUI
keys to send and attach. This direction removes those parts, and [writing from the TUI](writing-from-the-tui.md) records
why they wait. The record needs narrowing to the read-only parts before anything is built from it.
