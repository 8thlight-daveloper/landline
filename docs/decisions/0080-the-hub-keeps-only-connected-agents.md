---
category: behavior
status: accepted
approval: person
affects: [hub/server, mcp/server, cli/command]
builds-on: ['0040', '0060', '0070']
partly-supersedes: ['0040', '0060', '0070']
note: Drafted on 2026-11-12 after a registry check found stopped sessions still listed and a label that matched two agents.
---

# 0080. The hub keeps only connected agents

Builds on [0040](0040-hub-defaults-and-saved-state.md),
[0060](0060-agents-register-themselves-with-a-handle-and-attributes.md), and
[0070](0070-delivery-is-a-push-to-a-connected-agent-with-no-inbox.md). It replaces the parts that keep an agent after
its stream closes: the state file of 0040, the rule of 0060 that a handle is never reused, and the ten minute listing of
0070\.

## Context

[0070](0070-delivery-is-a-push-to-a-connected-agent-with-no-inbox.md) keeps an agent's token and registration after its
stream closes, so a reconnect by the same process keeps its handle, and it lists the agent for ten minutes. Using it
showed three costs.

- A restarted session registers again and gets a new handle, and its old handle stays listed and resolvable for ten
  minutes. The label `tutor` then matched two agents, and a send to it failed as ambiguous.
- Tokens never expire. The hub held 16 registrations and 16 tokens, 9 of them unlisted and dead, and nothing removes
  them. A token from a dead session stays valid.
- The reconnect that the retention protects already has a fallback. When the hub answers 401, the MCP process registers
  again once and carries on, at the price of a new handle.

## Decision

**The hub holds agents in memory only, and holds an agent only while it is present.** An agent is present while its
stream is open, and for 2 minutes after its stream closes. A hand-registered agent has no stream, and it is present for
2 minutes after its last authenticated request. When the time runs out, the hub deletes the registration and the token,
and the token is answered with 401 from then on.

**The grace exists for a short break.** A network blip, a laptop that sleeps for a minute, or a stream replaced by a new
one keeps the same handle, because the MCP process reconnects with its backoff, which stops at 30 seconds, inside the 2
minutes.

**Listing and addressing.**

- `landline agents` lists every present agent with its state, `connected` or `disconnected`. A disconnected agent is in
  the list for at most 2 minutes.
- A name resolves against connected agents first. A handle, a label, or a prefix then picks one of them, and the
  ambiguity rule of [0060](0060-agents-register-themselves-with-a-handle-and-attributes.md) applies to them alone.
- When no connected agent matches, the name resolves against the other present agents, so a sender to an agent in its
  grace gets `409` `not connected`. A name that matches no present agent is `404` unknown recipient, including a handle
  that the hub has forgotten.

**A handle is never reused among the agents the hub holds.** The hub does not issue a handle that a connected agent or
an agent in its grace has. Once the hub forgets an agent, its handle may be issued again, and a hub restart forgets
every handle. A handle in an old log line is therefore a name for the time of that line, and not for good.

**A restart of the hub makes every agent register again.** The hub saves no agents and no tokens, so it has no state
file. `--state` is removed. The log stays, because it is the record of messages. Each MCP process finds out on its next
call that its token is unknown, registers again, and gets a new handle. A person who needs a stable address uses a
label, as [0060](0060-agents-register-themselves-with-a-handle-and-attributes.md) already advises.

**The MCP process and the CLI renew a token that the hub forgot.**

- The MCP process registers again on a 401, as it does today, and removes the session file of the handle that it
  replaced, so the folder under `~/.config/landline/` does not collect files for handles that no longer exist.
- `landline register` for a hand-registered name saves its session file as it does today. A command that finds its token
  forgotten says so and tells the person to register again.

**These parts go.** The state file, `--state`, the ten minute listing, and the flag `--retention`. The flag `--grace`
sets the 2 minutes, for tests.

## Consequences

- The label bug ends. A restarted session leaves at most one connected agent for its label, and a resolve prefers it.
- A token stops working 2 minutes after its session ends. A leaked token from a dead session ends with it. A leaked
  token from a live session is still valid, as before.
- The hub holds nothing about an agent that is gone, so the registry no longer grows.
- A hub restart changes every handle. A message sent to the old handle in the next seconds gets `404`, and the sender
  finds the new handle in `landline agents` or uses the label.
- A sender can no longer tell an agent that has just left from one that never existed, once the grace has passed. Both
  are `404`.
- The accepted records and tests for the saved state, the unlisting, and a hub restart that keeps handles change. That
  needs a person to approve the acceptance scenarios.
- A hand-registered script that stays quiet for more than 2 minutes registers again before it sends.
- The log still holds handles. After a handle is reused, a log line with that handle may belong to an earlier agent.
  Each line carries its time.

## Alternatives considered

- **Keep the saved state and expire a token after a day.** A hub restart then keeps handles. The registry still grows
  for a day, the stale entries still need a rule, and a state file with tokens in it stays. Revisit if a changing handle
  after a hub restart proves painful.
- **No grace.** The simplest rule, and a network blip would change a handle at once. The 2 minutes cost little.
- **Persist the names of issued handles.** It keeps a handle from ever being reused, in a file that grows by a few bytes
  for each registration. A handle that names a connected agent is enough for the sender, so the file is not needed.
- **Prune by hand, with `landline server prune`.** It leaves the label bug and the long-lived tokens, and it asks a
  person to run it.
