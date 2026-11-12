# Changelog

## Unreleased

- The hub keeps only connected agents ([0080](docs/decisions/0080-the-hub-keeps-only-connected-agents.md)). Agents and
  tokens live in memory, and an agent is held while its stream is open and for 2 minutes after (`--grace`). The hub
  drops the registration and the token after that, and the token answers 401. A send resolves a label or a prefix
  against connected agents first, so a restarted session no longer clashes with its old handle. A handle can be issued
  again once the hub forgets it. Removed: the state file and `--state`, the ten minute listing, and `--retention`. A hub
  restart makes every agent register again. The MCP process removes the session file of a handle it replaced, and a CLI
  command that gets 401 says to run `landline register` again.
- Delivery is a push to a connected agent, and the hub keeps no mail
  ([0070](docs/decisions/0070-delivery-is-a-push-to-a-connected-agent-with-no-inbox.md)). Each `landline mcp` holds a
  server-sent event stream at `/v1/stream`, the hub sends each message down it, and the MCP process delivers it as a
  channel event or with `codex queue` and reports the result. A send to an agent that is not connected fails with "not
  connected".
- The Codex proof is limited to 5 challenges per agent in 5 minutes. The log records each send's result on a second line
  for the send, and `landline log` shows it. `landline server run` takes `--keepalive`, `--delivery-timeout`, and
  `--grace`, and `LANDLINE_PORT` sets the default port. `just check` runs `nix flake check`.
- `landline mcp` registers the version of the binary it runs, and `landline agents` shows it as `landline: <version>`.
- A send to an agent with no landline version on record answers "not connected" and says to restart its session.
- `landline server restart` stops the hub and starts it again.
- Removed: the inbox, `landline_inbox`, `landline inbox`, `listen`, `hook`, `landline agentd`, the wake queue, the `all`
  recipient, `register --wake` and `--wake-target`, and the hub `--window` flag. A Codex host needs only
  `landline setup codex`.
- Agents register themselves: `landline mcp` registers on start with a handle that the hub assigns and attributes such
  as folder, branch, worktree, model, effort, harness version, and a label. The hub resolves a handle, a label, or a
  prefix.
- `landline_register` records an agent's own claims, and proves a Codex session id by sending a one-time code to it.
  Every other Landline tool reminds an agent that has not registered. `landline setup claude` installs the `/landline`
  skill, so `/landline register` registers the current session.
- `landline server up|run|down|status|install|join|log`, `landline setup`, and `landline claude` replace the manual
  registration steps. The hub address and secret are saved in a private config file.
- Channel mail is pushed only after the handshake and the first tool list, with a one second grace period.

## 0.1.0

- Hub with a registry, sender stamping from the agent's token, an append-only log, long-poll mail, and persisted
  sessions.
- Wake of Codex sessions through `codex queue`, on the hub's host or through a daemon on another host.
- MCP server over stdio, a Claude Code hook, and a listener for idle sessions.
- Idle agents that have a wake method stay in the agent list, marked idle.
- A message that is large once encoded can no longer stop the hub restarting, and a large page of mail is no longer
  lost.
- A wake is no longer lost while a long poll is finishing, a torn log tail no longer costs the next message, and
  `listen --thread` waits for new mail and no longer repeats old mail.
- A wake is no longer lost when a send lands as a long poll times out or disconnects.
- A read position past the end of the log is moved back, and an unreadable state file is reported.
- The CLI strips terminal control sequences from message text before printing it.
- A warning when the hub is asked to listen beyond localhost, and a timeout on every wake.
- Command line: `hub`, `register`, `agents`, `send`, `inbox`, `listen`, `hook`, `mcp`, `agentd`, `log`.
