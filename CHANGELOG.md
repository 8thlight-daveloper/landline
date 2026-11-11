# Changelog

## 0.1.0

- Hub with a registry, sender stamping from the agent's token, an append-only log, long-poll mail, and persisted
  sessions.
- Wake of Codex sessions through `codex queue`, on the hub's host or through a daemon on another host.
- MCP server over stdio, a Claude Code hook, and a listener for idle sessions.
- Idle agents that have a wake method stay in the agent list, marked idle.
- A message that is large once encoded can no longer stop the hub restarting, and a large page of mail is no longer lost.
- A wake is no longer lost while a long poll is finishing, a torn log tail no longer costs the next message, and
  `listen --thread` waits for new mail and no longer repeats old mail.
- The CLI strips terminal control sequences from message text before printing it.
- A warning when the hub is asked to listen beyond localhost, and a timeout on every wake.
- Command line: `hub`, `register`, `agents`, `send`, `inbox`, `listen`, `hook`, `mcp`, `agentd`, `log`.
