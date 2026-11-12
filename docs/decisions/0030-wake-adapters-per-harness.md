---
category: architecture
status: accepted
approval: agent
approver: {kind: agent, harness: claude-code, session: adbd644e52614aee7}
author: {kind: agent, harness: claude-code, session: ff162464-db6e-40df-b588-14f688bad84b}
note: Approved by an independent agent review of commit 237c15e. The author then made the sentence changes that review listed, and the approver has not re-read them.
affects: [wake/codex, hub/server]
partly-superseded-by: ['0070']
---

# 0030. Waking an idle agent is one adapter per harness

## Context

A message waits until its recipient next acts. An idle agent does not act, so a message to it needs a wake. Each harness
has its own way in. Codex has `codex queue --thread <id> --message <text>`. A hand test on 2026-11-10 saw it start a
turn in an idle interactive session within seconds, although the terminal did not redraw until the person typed. The
test is not repeated in this repository.

## Decision

A wake adapter is a driver component with one method, `Wake(target, text)`, where the target names the session in that
harness and the text is the rendered message. `wake/codex` runs `codex queue`, stops a hung command after 15 seconds,
and gives up on its output pipes 2 seconds later.

The hub records a message before it wakes anyone. It then wakes the recipients on its own host at the same time, so a
broadcast costs the sender the slowest wake and not the sum. It does not wake an agent that is waiting on an unfiltered
long poll, since that agent is already listening. A long poll that names a thread does not count, because mail in
another thread would not reach it. For an agent on another host the hub queues a wake job, and the daemon on that host
polls for jobs, calls its own adapter, and reports the result. The hub leases a job for 60 seconds and offers it again
if the daemon has not acknowledged it by then.

The hub and the daemon print the notice of a wake. The adapter runs the command, and it refuses a target that is empty
and a text that does not start with the marker line of [0020](0020-messages-carry-no-user-authority.md). The entry point
holds the map from a wake method name to an adapter, in one place for the hub and one for the daemon.

A Claude Code session is not an adapter yet. It reads mail through the hook and a background listener. Claude Code's own
cross-session messaging, where a person has it turned on, reaches other Claude sessions on one machine, and Landline
does not depend on it.

## Consequences

- A new harness adds one component, a name in the entry point's maps, and nothing in the core.
- A hung local wake costs the sender up to about 17 seconds however many agents are woken, and a remote wake waits in
  the hub's queue. A job whose daemon died is offered again after 60 seconds, so a wake can run twice. Mail that arrives
  between two of a client's polls is woken and also stays unread, so the agent may see it by both routes.
- A wake that a person cannot see is a risk, so the hub and the daemon print each one.
- Wake jobs for another host live in the hub's memory. A hub restart drops them and the messages stay in the inbox.

## Alternatives considered

- **Drive a terminal with tmux.** It works with any terminal program, and it types as the user and collides with the
  person's own typing.
- **Poll only.** An idle agent never polls.
