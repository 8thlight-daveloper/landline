# Changelog

## Unreleased

- Agents register themselves: `landline mcp` registers on start with a handle that the hub assigns and attributes such
  as folder, branch, worktree, model, effort, harness version, and a label. The hub resolves a handle, a label, or a
  prefix.
- `landline_attach` records an agent's own claims, and proves a Codex session id by sending a one-time code to it.
- `landline server up|run|down|status|install|join|log`, `landline setup`, and `landline claude` replace the manual
  registration steps. The hub address and secret are saved in a private config file.
- Channel mail is pushed only after the handshake and the first tool list, with a one second grace period.

## 0.1.0

- Hub with a registry, sender stamping from the agent's token, an append-only log, long-poll mail, and persisted
  sessions.
- Wake of Codex sessions through `codex queue`, on the hub's host or through a daemon on another host.
- MCP server over stdio, a Claude Code hook, and a listener for idle sessions.
- Idle agents that have a wake method stay in the agent list, marked idle.
- A message that is large once encoded can no longer stop the hub restarting, and a large page of mail is no longer lost.
- A wake is no longer lost while a long poll is finishing, a torn log tail no longer costs the next message, and
  `listen --thread` waits for new mail and no longer repeats old mail.
- A wake is no longer lost when a send lands as a long poll times out or disconnects.
- A read position past the end of the log is moved back, and an unreadable state file is reported.
- The CLI strips terminal control sequences from message text before printing it.
- A warning when the hub is asked to listen beyond localhost, and a timeout on every wake.
- Command line: `hub`, `register`, `agents`, `send`, `inbox`, `listen`, `hook`, `mcp`, `agentd`, `log`.
