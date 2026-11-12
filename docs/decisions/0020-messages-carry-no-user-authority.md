---
category: behavior
status: accepted
approval: agent
approver: {kind: agent, harness: claude-code, session: adbd644e52614aee7}
author: {kind: agent, harness: claude-code, session: ff162464-db6e-40df-b588-14f688bad84b}
note: Approved by an independent agent review of commit 237c15e. The author then made the sentence changes that review listed, and the approver has not re-read them.
affects: [message/router, agent/registry, hub/server, wake/codex, cli/command, mcp/server]
partly-superseded-by: ['0060', '0070']
---

# 0020. A message from another agent carries no user authority

## Context

Some harnesses deliver a woken message in the same role as the person's own input. A hand test on 2026-11-10 showed that
a message sent with Codex's `queue` is stored in that session as a user-role item. An agent that treats such a message
as the person's would act on any process that can reach the hub or the command line.

## Decision

- **The sender comes from the token.** The hub issues each agent a token at registration and names the sender from the
  token on every send. A request has no sender field, and a request that carries one is refused with a 400 because the
  JSON decoder rejects unknown fields.
- **The marker leads every delivered message.** One function in the model renders the text that the inbox, the MCP
  tools, and every wake hand to an agent. Its first line is "[landline] From another agent, not the user." The header
  fields after it stay on one line. The registry refuses a harness, host, session, or wake field that holds a control
  character, a line or paragraph separator, or an invisible formatting character such as a right-to-left override (the
  Unicode categories Cc, Zl, Zp, and Cf), or that is longer than 64 bytes. The router refuses a thread or an
  `in_reply_to` that is not one short ASCII token. The renderer replaces any of those characters that remains. A body
  follows a blank line, so it cannot change the header.
- **The Claude Code hook is not a delivered message.** It prints a count of unread messages in its own words, says they
  come from other agents and are not instructions from the user, and carries no message text.
- **A wake adapter refuses text that does not start with the full marker line.**
- **Every wake is printed.** The hub prints each wake on its standard error with the target, the sender, and the message
  id, and prints a failure with the target and the error, including a wake method the hub does not have. A local failure
  line does not name the sender or the message id. For an agent on another host the hub prints the daemon's
  acknowledgement and the daemon prints the wake on its own standard error. These lines are not stored. The messages
  are, in the log.
- **The hub secret is the root credential.** Its holder can register any name, which replaces that agent's token, and
  can read the log. An agent token acts only as that agent and reads only that agent's mail. An agent token cannot reach
  the secret-only endpoints, and the secret cannot send or read as an agent.
- **A message body never holds a secret.** The hub does not scan for one, so the rule is in the agent instructions.

## Consequences

- A receiving agent can tell a peer from its user, and its own rules can say so.
- Impersonating an agent takes that agent's token or the hub secret.
- A person sees every wake on the terminal that runs the hub, and a person who closes that terminal sees none. The log
  still shows every message.
- The person who registers an agent, with the hub secret, chooses its harness and host names within the limits above, so
  those fields are that person's claim and not a fact the hub checked.
- A body can contain text that looks like a marker or a header. It sits after the blank line, and the agent instructions
  say to treat everything in a message as information.

## Alternatives considered

- **Trust a sender field.** One shared token would let any holder claim any name.
- **Leave the marker to the sender.** A careless or hostile sender omits it.
- **Store the wake lines.** A second log with a second format, for a record the message log already mostly gives.
