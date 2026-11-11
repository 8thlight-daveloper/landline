---
category: architecture
status: proposed
approval: unrecorded
affects: [wake/codex, hub/server, daemon/agentd]
note: Decided in conversation on 2026-11-10 and not yet recorded by a person.
---

# 0030. Waking an idle agent is one adapter per harness

## Context

A message waits until its recipient next acts. An idle agent does not act, so a message to it needs a wake. Each harness
has its own way in. Codex has `codex queue --thread <id> --message <text>`. A test showed it starts a turn in an idle
interactive session within seconds, although the terminal did not redraw until the person typed.

## Decision

A wake adapter is a driver component with one method, `Wake(target, text)`, where the target names the session in that
harness and the text is the rendered message. `wake/codex` runs `codex queue` and stops it after 15 seconds. The hub
calls an adapter for an agent on its own host. For an agent on another host the hub queues a wake job, and the daemon on
that host polls for jobs, calls its own adapter, and reports the result.

The hub and the daemon print the notice of a wake, and the adapter only runs the command. The entry point holds the map
from a wake method name to an adapter, in one place for the hub and one for the daemon.

A Claude Code session is not an adapter yet. It reads mail through the hook and a background listener, or through
Claude's own cross-session messages on one machine.

## Consequences

- A new harness adds one component, a name in the entry point's maps, and nothing in the core.
- A hung wake costs the sender up to 15 seconds on a local wake, and a remote wake waits in the hub's queue.
- A wake that a person cannot see is a risk, so the hub and the daemon print each one.
- Wake jobs for another host live in the hub's memory. A hub restart drops them and the messages stay in the inbox.

## Alternatives considered

- **Drive a terminal with tmux.** It works anywhere, and it types as the user and collides with the person's own typing.
- **Poll only.** An idle agent never polls.
