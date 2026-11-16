# Writing from the TUI

A direction and not a decision. Nothing here is built, and no record depends on it. A part becomes an ADR when someone
takes it on. Set down on 2026-11-15, after the owner decided the TUI stays read-only for now. See
[a read-only TUI and events from harness hooks](read-only-tui-and-hook-events.md).

## The idea

An operator at the TUI sends a message to an agent, or moves to the terminal the agent runs in. The draft record
[0120](draft-decisions/operator-sends-and-attaches.md) worked out one version: an operator token,
`landline operator send`, and TUI keys to send and attach through tmux. It was never built.

## Why it waits

- **A send from Landline is not the person's input.** [0020](../decisions/0020-messages-carry-no-user-authority.md) says
  a delivered message is information and never an instruction from the user. A message the TUI sends would carry the
  same marker, so an agent treats it as a colleague's note. The person's own authority exists only when they type in the
  agent's own session. A TUI that sends therefore looks like a control surface and does not behave like one.
- **A send needs a new kind of credential.** The draft added an operator token. A leaked token can send to every
  connected agent until it expires. A new token kind is a person's decision under `AGENTS.md`.
- **Delivery differs by harness, and some does not show.** Claude Code has channels and Codex has `codex queue`, and
  both put the message in front of the session. Other harnesses have no clean path. The `agy` check in
  [other harnesses](other-harnesses.md) found no way to queue a message into an open window. A turn ran, and the window
  showed nothing until the person exited and resumed.
- **Attach depends on a self-reported fact.** The pane an agent registers is its own claim, and it can name another pane
  on the same socket. A socket in a custom location cannot be reached. A remote agent needs an `ssh` line that the
  person runs. tmux refuses to nest inside itself. The draft handled these, and each one adds rules.
- **The terminal layer is unsettled.** Reaching an input surface cleanly may need a multiplexer, or the pty layer a
  terminal product provides. The owner is in the beta of Rex (superlogical.com) and wants to look again when it leaves
  beta. Whether Rex builds on libghostty is a question the owner raised and nobody has checked.

## Where to start again

- Read the draft in git history at commit `961b8ff`. It holds the marker rules, the reserved sender name, the tmux
  patterns, and the attach steps.
- Check which harnesses can show a message in an open session before designing a send for them.
- Decide first whether a person's message through Landline should ever carry the person's authority. The draft said no.
  A path that says yes needs its own record and a person's approval.
