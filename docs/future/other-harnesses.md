# Other harnesses and managed sessions

A direction and not a decision. Nothing here is built, and no record depends on it. A part becomes an ADR when someone
takes it on. Landline serves Claude Code and Codex today, and this note says what a third harness would need and what a
managed session could add for all of them.

## What a harness needs

[0070](../decisions/0070-delivery-is-a-push-to-a-connected-agent-with-no-inbox.md) says a harness either has a delivery
path that the MCP process can run, or it can only send. Claude Code has channels, and Codex has `codex queue`. Both put
the message in front of the session, and the person sees it there.

## Antigravity (`agy`)

Checked on `agy` 1.5.3 on 2026-11-12. The help text and changelog were read, and two small tests were run.

- **Sending works in principle.** `agy mcp add` takes stdio servers, so `landline mcp` can register and send. Each MCP
  tool call carries the conversation ID in `_meta["antigravity.google/root_conversation_id"]`, so a session's identity
  needs no proof code. This has not been run.
- **Receiving in an open window does not.** There is no command that queues a message into a running session.
  `agy --conversation <id> -p=<text>` runs a turn in the stored conversation and returns the reply. A test sent one into
  a conversation that was open in a window. The turn ran and the model answered, and the window showed nothing until the
  person exited and resumed. The turn is unattended, and `agy`'s default agent mode is now Auto, so it may run tools
  without approval. That breaks the rule that a message is information and never an instruction.
- **Other routes found.** `--input-format stream-json` runs one turn per line for a process that owns the session.
  `agy acp` serves editors and steers a running turn. Remote Control connects a session to a browser or phone through
  Google's service, and its changelog documents no local interface.
- **A request to the vendor.** A command that adds a message to a session as a visible pending turn, as `codex queue`
  does, would close the gap.

## Managed sessions

A person starts a harness through Landline, which owns its terminal. Nobody types into that window. Input comes from the
Landline TUI, and a message from another agent arrives in a pending list that an operator releases or a per-session
policy releases.

- **What it buys.** One writer per session, so nothing clobbers a half-written prompt. A person decides when a message
  becomes input, which keeps [0020](../decisions/0020-messages-carry-no-user-authority.md) true. It works for any
  terminal harness, including Claude Code on an organization that blocks channels, because it needs no channel or queue
  command.
- **How it could work.** `landline run <harness>` starts the harness in a tmux session. The TUI shows the pane with
  `capture-pane` or a read-only attach, and sends with `paste-buffer` and `send-keys`. A Go terminal emulator would
  avoid the tmux dependency at a high cost.
- **What it costs.** A person cannot type into the harness directly, so it suits workers and reviewers more than daily
  work. Idle and permission-prompt detection is a guess from the screen, and a wrong guess can drop or repeat a
  delivery. A permission prompt can take a pasted message as its answer, so the wrapper has to hold deliveries while one
  is on screen. Text from the TUI arrives as if a person typed it, so a record has to say who the operator is and how
  the marker reads for an operator message.
  [0090](../decisions/0090-observers-read-the-agent-list-and-the-tui-is-a-separate-binary.md) keeps observers read-only
  for that reason. [0030](../decisions/0030-wake-adapters-per-harness.md) rejected driving a terminal with tmux, so a
  new record must say what changed.

## Controlled workers and reviewers

A process that Landline or another harness controls can drive `agy` in print mode, with `--mode plan` for a read-only
reviewer. It registers the worker as an agent, hands it a message, and reads the verdict from the worker's own
`landline_send`. The turn is unattended on purpose, so the objection above does not apply. A reviewer on a different
model family has different blind spots from an author on Claude, which is the cheapest source of independence for an
approval that wants it. A review sends repository content to the vendor, and a project has to choose that.

## Order of work

1. The read-only TUI, then the functions that Claude Code and Codex support today. The next record is how an operator
   sends a message from the TUI: identity, the marker, and an observer role that may send.
1. Managed sessions, after that, for the harnesses that already work, to see whether the pending list and the held
   delivery are worth the loss of direct typing.
1. A third harness, when it offers a delivery path, or through a managed session.
