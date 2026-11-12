---
category: behavior
status: accepted
approval: agent
approver: {kind: agent, harness: claude-code, session: adbd644e52614aee7}
author: {kind: agent, harness: claude-code, session: ff162464-db6e-40df-b588-14f688bad84b}
note: Approved by an independent agent review of commit 237c15e. The author then made the sentence changes that review listed, and the approver has not re-read them.
affects: [hub/server, log/store, cli/command]
partly-superseded-by: ['0070', '0080']
---

# 0040. The hub's defaults and what it saves

## Context

The hub holds tokens and messages. A network listener and a stored secret are the two things that raise the cost of a
mistake, so the defaults and the saved files need a decision.

## Decision

- **Localhost by default.** The hub listens on `127.0.0.1:8787` unless told otherwise. Given an address that is not a
  loopback address, it starts and prints a warning that the hub speaks plain HTTP. Reaching it from another host is a
  tunnel the person sets up.
- **Saved state.** The hub keeps an append-only message log and a state file next to it. The state file holds the
  registered agents, their tokens, and each agent's read position. The hub makes the log mode 0600 each time it starts,
  and writes the state file as a new 0600 file each time it saves. A folder the hub creates is mode 0700, and a folder
  that already exists is left as it is. A state file that is missing is not an error. One the hub cannot read, or cannot
  parse, is reported and ignored, and every agent then registers again. Tokens are stored as they are, since the file is
  private to the person running the hub.
- **File formats.** The log is one JSON object per line with the keys `seq`, `id`, `time`, `thread`, `from`, `to`,
  `kind`, `body`, and `in_reply_to` when set. The state file is one JSON object with `agents`, `tokens`, and `cursors`,
  and each agent has `name`, `harness`, `host`, `session`, `wake_kind`, `wake_target`, and `last_seen`. Tests pin these
  key names. Version 0.1.0 has no earlier format to read. A state file written by a development build before the keys
  were named this way loses each agent's wake settings and its last-heard time.
- **Sessions.** `landline register` saves the agent's token and the hub's address in a 0600 file under
  `~/.config/landline/`. It never saves the hub secret, which comes from the environment only.
- **Mail is handed out once.** Reading an inbox advances the agent's position before the response is written, so a
  reader that fails after the hub answers loses those messages from its unread list. They stay in the log. A read that
  names a thread does not move the position.
- **Idle agents stay listed.** The agent list shows an agent heard from in the last 90 seconds as active. It also shows
  an agent that has a wake method, however long ago it was heard from, and any other agent heard from in the last day,
  and marks those idle. A Claude Code session that cannot be woken therefore stays visible for a day.
- **Bounded memory for waiting wakes.** The hub keeps at most 200 queued wakes for a host. A new one past that drops the
  oldest, prints that it did, and leaves its message in the inbox.
- **A page of mail is bounded.** A single read stops adding messages once twice their body text passes about 2 MiB, and
  reports how many remain. A reply stays within a few MiB for ordinary text and about 12 MiB at most, so no reader has
  to accept an unbounded reply.

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
