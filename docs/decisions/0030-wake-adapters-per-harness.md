---
category: architecture
status: proposed
approval: unrecorded
affects: [wake/codex]
note: Decided in conversation on 2026-11-10 and not yet recorded by a person.
---

# 0030. Waking an idle agent is one adapter per harness

## Context

A message waits until its recipient next acts. An idle agent does not act, so a message to it needs a wake. Each harness
has its own way in. Codex has `codex queue --thread <id> --message <text>`. A test showed it starts a turn in an idle
interactive session within seconds, although the terminal did not redraw until the person typed.

## Decision

Waking is a driver component per harness, behind one function that takes an agent and a message. `wake/codex` runs
`codex queue`. A Claude session on the same host uses its own cross-session messaging, and a Claude session on another
host runs a background wait that the hub releases when mail arrives. Each adapter is added when it has been tried.

## Consequences

- A new harness adds one component and a wake method name. The hub and the core do not change.
- A wake that a person cannot see is a risk, so the log and a notification are part of the adapter's work.
- Adapters that are not yet tried are not in the code.

## Alternatives considered

- **Drive a terminal with tmux.** It works anywhere, and it types as the user and collides with the person's own typing.
- **Poll only.** An idle agent never polls.
