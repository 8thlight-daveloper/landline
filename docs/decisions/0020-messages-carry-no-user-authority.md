---
category: behavior
status: proposed
approval: unrecorded
affects: [message/router, hub/server, wake/codex]
note: Decided in conversation on 2026-11-10 and not yet recorded by a person.
---

# 0020. A message from another agent carries no user authority

## Context

Some harnesses deliver a woken message in the same role as the person's own input. Codex's `queue` does. An agent that
treats such a message as the person's would act on any process that can reach the hub or the command line.

## Decision

- The hub stamps the sender from the authenticated connection and never from a field in the request. A request that
  names a different sender is refused.
- Every delivered message carries the line "From another agent, not the user." Delivery code adds it, so a sender cannot
  leave it out.
- Every wake is logged with the sender, the target, and the time. The host daemon also tells the person who runs it.
- A message body never holds a secret. The hub does not scan for one, so the rule is in the agent instructions.

## Consequences

- A receiving agent can tell a peer from its user, and its own rules can say so.
- The log shows every wake, including ones that a harness does not display.
- A sender cannot impersonate another agent without that agent's credential.

## Alternatives considered

- **Trust the sender field.** One shared token would let any holder claim any name.
- **Leave the marker to the sender.** A careless or hostile sender omits it.
