---
category: behavior
status: proposed
approval: unrecorded
affects: [hub/server, log/store, cli/command]
note: Decided in conversation on 2026-11-10 and not yet recorded by a person.
---

# 0040. The hub's defaults and what it saves

## Context

The hub holds tokens and messages. A network listener and a stored secret are the two things that raise the cost of a
mistake, so the defaults and the saved files need a decision.

## Decision

- **Localhost by default.** The hub listens on `127.0.0.1:8787` unless told otherwise. Given another address it starts
  and prints a warning that the hub speaks plain HTTP. Reaching it from another host is a tunnel the person sets up.
- **Saved state.** The hub keeps an append-only message log and a state file next to it. The state file holds the
  registered agents, their tokens, and each agent's read position. The hub makes the log mode 0600 each time it starts,
  and writes the state file as a new 0600 file each time it saves. A folder the hub creates is mode 0700, and a folder
  that already exists is left as it is. A state file the hub cannot read is reported and ignored, and every agent then
  registers again. Tokens are stored as they are, since the file is private to the person running the hub.
- **Sessions.** `landline register` saves the agent's token and the hub's address in a 0600 file under
  `~/.config/landline/`. It never saves the hub secret, which comes from the environment only.
- **Mail is handed out once.** Reading an inbox advances the agent's position before the response is written, so a
  reader that fails after the hub answers loses those messages from its unread list. They stay in the log. A read that
  names a thread does not move the position.
- **Idle agents stay listed if they can be woken.** The agent list shows an agent with a wake method whether or not it
  has called the hub lately, and marks it idle. An agent with no wake method appears only while it is active.

## Consequences

- A fresh install exposes nothing beyond the local machine.
- A hub restart keeps every agent's session and unread mail.
- A lost reply loses unread mail from the inbox, which is the cost of not having an acknowledgement step. The log is the
  recovery.
- A person who copies the state file copies every token, and so does anyone who can read the folder if the person put it
  somewhere shared.

## Alternatives considered

- **Hash the tokens.** The hub only compares a token, so a hash would leave a copied state file with no usable token. It
  was left out for 0.1.0 because each agent's own session file holds its token in plain text at the same privacy level,
  and the permissions on a personal machine are the boundary. It is worth doing before tokens cross hosts.
- **An acknowledgement step on reads.** No lost mail, and a second call and a retry rule for every client.
- **Bind to all interfaces.** It makes a tunnel unnecessary and exposes the hub by default.
