# Changelog

## 0.3.0

- An agent has an id, a project, and a task ([0160](docs/decisions/0160-an-agent-has-an-id-a-project-and-a-task.md)).
  The id replaces the handle and the name: the switchboard assigns 4 hexadecimal characters, such as `a3f9`. The project
  replaces the label and names the group of agents on one project. It defaults to the repository's folder name, also
  from a linked worktree or a subfolder. The task is what an agent is working toward, set through the new
  `landline_status` tool and `POST /v1/status`. The JSON keys `name` and `label` are now `id` and `project`, `--name` is
  `--id`, `--label` is `--project`, and `LANDLINE_LABEL` is `LANDLINE_PROJECT`, with no aliases. `send` takes an exact
  id and nothing else: a prefix, a project, or `all` answers `404`, and a project gets a hint that lists the connected
  ids. The register tool says what it recorded and how many other agents share the project.
- Agent state shows as a shape ([0150](docs/decisions/0150-agent-state-shows-as-an-icon.md)). The terminal screen has a
  `●` or `○` before each id, no `STATE` column, and a legend below the table. `PROJECT` and `TASK` columns follow the
  id.
- `landline-menubar` and `Landline Menubar.app`
  ([0140](docs/decisions/0140-a-macos-menu-bar-item-shows-agents-runs-the-switchboard-and-sets-up-harnesses.md)). A
  macOS menu bar item shows the agents, starts, stops, and restarts the switchboard through `landline server`, runs
  `landline setup` for Claude Code and Codex, and shows its own build and the switchboard's. `just install` also
  installs the app into `~/Applications`. `GET /v1/build` reports the switchboard's version, commit, and date.
  `landline setup claude|codex --remove` takes Landline out of a harness. Upgrade: stop any old switchboard, including a
  `landline server run` from before 0.2.0, then run `just install` and `landline server up`. Restart every agent session
  and `landline setup claude|codex` again so each loads the new tool text. A skill or script that uses the old words,
  such as one that matches agents by label, changes to `project`.

## 0.2.0

- Three binaries ([0110](docs/decisions/0110-three-binaries-landline-switchboard-and-landline-mcp.md)).
  `landline-switchboard` is the old hub and serves in the foreground with no arguments. `landline-mcp` is the old
  `landline mcp`. `landline` is the terminal screen and the commands a person runs, built in the `tui/` module, which
  now imports the root module. `landline agents` reads the list with an observer token, takes no `--as`, and has a text
  form and `--json`. `just build` and `just install` build all three, and the flake has a package for each and a bundle
  as the default. The names are `dev.landline.switchboard`, `landline-switchboard.service`, `switchboard.pid`, and
  `switchboard.out`. Removed: `landline hub`, `landline mcp`, `landline server run`, `landline tui`, the `landline-tui`
  binary and package, and `just install-tui`. A running install breaks until you run `landline setup claude|codex` and
  `landline server install` again. Remove an old launchd unit `dev.landline.hub` or an old systemd unit
  `landline-hub.service` by hand, and delete a `landline-tui` that is still installed.
- `landline tui` opens a terminal screen of the connected agents, and `landline-tui agents --json` prints a snapshot as
  JSON ([0090](docs/decisions/0090-observers-read-the-agent-list-and-the-tui-is-a-separate-binary.md)). `landline-tui`
  is its own binary and Go module in `tui/`, built on Bubble Tea. `just install-tui` installs it, `landline tui` runs
  it, and `nix build .#landline-tui` builds it. The `landline` module still has no third-party dependencies.
  `just check` also tests and vets `tui/`.
- `POST /v1/observers` registers an observer, a read-only client that is not an agent
  ([0090](docs/decisions/0090-observers-read-the-agent-list-and-the-tui-is-a-separate-binary.md)). Its token may call
  `GET /v1/agents` and gets 403 on every other route. The hub holds it in memory and forgets it after 2 minutes without
  a request, and the token then answers 401. An observer never appears in the agent list.
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
- A send to an agent with no landline version on record answers "not connected" and says it is a hand-registered name
  that cannot receive, or an older landline to restart.
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
