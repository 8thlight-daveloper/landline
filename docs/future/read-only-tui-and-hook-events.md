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

## Activity: what an agent is doing now

A direction and not a decision. It waits on the events above, because activity has to come from the harness and not from
the agent's own words.

An agent has three levels of "what am I doing", and each has one source.

| Level    | Example                                      | Changes              | Set by                       |
| -------- | -------------------------------------------- | -------------------- | ---------------------------- |
| Label    | `landline`                                   | Not within a session | The agent, as the project    |
| Task     | `menu bar app for ADR 0140`                  | Minutes to hours     | The agent, in a short phrase |
| Activity | running a tool, waiting for the person, idle | Seconds              | The harness, from hooks      |

- **Label and task are the agent's to say.** A task is the outcome the agent is pursuing, in words it would use to a
  teammate. It changes when the goal changes and not when a step ends: a phrase that would change when one test passes
  is too low. The record for label and task states this in the tool text, so an agent does not have to guess.
- **Activity is not agent prose.** An agent that updates a line every few seconds spends tool calls on it, forgets, and
  leaves a stale line that misleads more than no line. Free text also reaches every viewer, so it is the likeliest place
  for a secret or a client name. A hook reports a fixed set of states and tool names, which are metadata only.
- **A state set, not sentences.** Candidates are `working`, `waiting for the person`, `idle`, and `ended`, with the tool
  name while a tool runs. The shapes of [0150](../decisions/0150-agent-state-shows-as-an-icon.md) are a solid green
  circle for connected and a hollow gray circle for disconnected. The activity feature chooses its own, and a candidate
  set is a ring with a center dot in blue for working and a solid diamond in amber for waiting for the person. Green
  means available, blue means busy, amber means the person is needed, and a hollow shape means gone. The TUI and the
  menu bar item would show the shape beside the agent, and an agent's task stays beside it as text.
- **What it needs.** Everything under "Events from hooks" above: how a hook authenticates, metadata only, a bounded ring
  in memory, and which harnesses have hooks. The reporter token is a new kind of credential, so a person approves the
  record. Activity is built after the events work and not before it.

## Dropped by this direction

[0120](draft-decisions/operator-sends-and-attaches.md) proposed an operator token, `landline operator send`, and TUI
keys to send and attach. This direction removes those parts, and [writing from the TUI](writing-from-the-tui.md) records
why they wait. The record needs narrowing to the read-only parts before anything is built from it.
