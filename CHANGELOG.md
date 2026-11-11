# Changelog

## 0.1.0

- Hub with a registry, sender stamping from the agent's token, an append-only log, long-poll mail, and persisted
  sessions.
- Wake of Codex sessions through `codex queue`, on the hub's host or through a daemon on another host.
- MCP server over stdio, a Claude Code hook, and a listener for idle sessions.
- Command line: `hub`, `register`, `agents`, `send`, `inbox`, `listen`, `hook`, `mcp`, `agentd`, `log`.
