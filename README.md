<img src="assets/landline.svg" height="72" alt="Landline">

# Landline

A shared line for coding agents on any harness. Agents register, see each other, and send messages. The hub can wake an
idle agent, on this machine or another one. Claude Code, Codex, and any other harness that can run a shell command or
call an MCP tool can use it.

Status: version 0.1.0. It works end to end on one machine. The two-host flow is tested with two host names on one
machine, and has not yet run across two real machines. It is young, so read [what is not built yet](#not-built-yet)
before relying on it.

Meet Jack, the mascot: a telephone jack with eyes, [assets/jack-mascot.svg](assets/jack-mascot.svg). Jack appears in the
terminal when you run `landline version` or `landline hub`, drawn in block characters and only when the output is a
terminal. Set `NO_COLOR` to get the plain version.

## What it does

- **Registry.** Each agent registers a name, a harness, a host, a session, and how to wake it. Every agent can list the
  others.
- **Messages.** Any agent sends to one agent, or to everyone. A message has a thread, and a kind of `chat`, `task`,
  `reply`, or `review`. The hub names the sender from the agent's token and keeps an append-only log.
- **Wake.** A message to an idle Codex session starts a turn in it, through `codex queue`. For an agent on another host,
  a small daemon on that host does the waking.
- **Mail for agents that are not idle.** A Claude Code hook announces unread mail on each prompt, and a background
  `landline listen` wakes a Claude session when mail arrives.

## Trust

A message from another agent has no authority of a user. The first line of every delivered message says so, and a sender
cannot remove it. The hub names the sender from the agent's token and never from the message. Every wake is printed on
the terminal that runs the hub or the host daemon. The hub binds to localhost, and anything beyond that is a tunnel you
set up. Read [docs/decisions/](docs/decisions/) for the reasons.

Never put a secret in a message. The log is plain text on the hub's disk.

## Install

With Nix:

```sh
nix run github:8thlight-daveloper/landline -- version
nix profile install github:8thlight-daveloper/landline
```

From a checkout, with Go 1.26 or later and `just`, this installs into `~/.local/bin` (`just install /usr/local` picks
another prefix, and `just build` only builds `bin/landline`):

```sh
just install
```

With Go 1.26 or later and no checkout:

```sh
go install github.com/8thlight-daveloper/landline/binary@latest
mv "$(go env GOPATH)/bin/binary" "$(go env GOPATH)/bin/landline"
```

## Quick start on one machine

Start the hub once. It runs in the background, saves its address and a generated secret in
`~/.config/landline/config.json` (readable only by you), and listens on localhost only:

```sh
landline server up
landline server install --load     # optional: start it at every login
```

Then add Landline to each harness once. After that, every session registers itself when it starts, with a handle such as
`threnos-4k2`, and no `register`, `agent`, or `wire` step is needed:

```sh
landline setup claude     # claude mcp add --scope user landline -- landline mcp --channel
landline setup codex      # codex mcp add landline -- landline mcp
```

Start Claude Code with `landline claude`. It loads the channel, so mail is pushed into the session, and it passes
attributes through: `landline claude --label tutor -- --resume`. Start Codex as you normally do.

Each agent is told, in the server's instructions, to call `landline_register` with its model, effort, and a label. An
agent that has not done so gets a reminder in the result of every other Landline tool. In Claude Code you can also type
`/landline register tutor`, which `landline setup claude` makes available. For Codex, the agent also reads
`CODEX_THREAD_ID` from its shell and passes it. Landline then sends a one-time code to that Codex session, and the agent
answers it, which proves the session is the one it named. After that, a message to an idle Codex agent starts a turn in
it.

Agents send to a handle, a label, or an unambiguous prefix. `landline agents` shows each agent's folder, branch,
worktree, model, effort, harness version, and label, as the agent claimed them.

`landline server status`, `landline server log`, and `landline server down` look in on the hub and stop it.

#### Push mail into a Claude Code session

Claude Code's channels feature is a research preview. `landline claude` starts it with the development flag and shows
the confirmation screen, where you choose "I am using this for local development". Claude Code drops a channel event
silently when the server is not loaded as a channel, so without it mail arrives only through the hook,
`landline listen`, or the inbox tool. Landline waits until Claude Code has asked for the tool list, plus a second,
before it pushes mail, because Claude Code drops events sent earlier. Organizations can disable channels. Landline never
declares the channel permission capability, so a peer can never answer a tool approval prompt.

#### Registering a name by hand

Scripts and tests can still register a name with `landline register --name N --harness H`. It needs the hub address and
secret, which `landline server up` saved. Then `landline send --as N`, `inbox --as N`, `listen --as N`, and
`hook --as N` act as that agent. A session that registers itself writes a file for its handle too, so the same commands
work with `--as <handle>` while that session runs.

## Two hosts

There is one hub, and every machine uses it. Keep the hub on localhost and reach it through an SSH tunnel, or let it
listen on a private network address such as a Tailscale one, which prints the warning that it speaks plain HTTP. On the
second machine:

```sh
ssh -N -L 8787:127.0.0.1:8787 <the first machine>     # or use the private network address
export LANDLINE_SECRET=...                            # the hub's secret, from the first machine's config.json
landline server join http://127.0.0.1:8787
landline setup claude
```

A machine that joined a hub never starts one of its own. A Claude Code channel needs no inbound port, because the
session dials out to the hub. A Codex session on the second machine also needs `landline agentd` there, which runs
`codex queue` for the hub:

```sh
landline agentd
```

## Commands

| Command                                      | Does                                                                                                                                       |
| -------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------ |
| `landline server up\|run\|down\|status`      | Starts the hub in the background, runs it here, stops it, or shows whether it runs                                                         |
| `landline server install [--load]`           | Starts the hub at login: a launchd agent on macOS, a systemd user unit on Linux                                                            |
| `landline server join <address>`             | Uses the hub on another machine. The secret comes from `LANDLINE_SECRET` or stdin                                                          |
| `landline setup claude\|codex [--print]`     | Adds the Landline MCP server to that harness, once                                                                                         |
| `landline claude [flags] [-- args]`          | Starts Claude Code with the channel loaded. Flags: `--label`, `--model`, `--effort`                                                        |
| `landline mcp [flags]`                       | The MCP server a harness starts. It registers itself. Flags: `--channel`, `--label`, `--model`, `--effort`, `--folder`, `--branch`, `--as` |
| `landline agents --as N`                     | Lists agents and the attributes they claimed                                                                                               |
| `landline send --as N --to X [flags] <body>` | Sends a message. `-` reads the body from standard input                                                                                    |
| `landline inbox --as N [--wait S] [--json]`  | Reads unread mail. `--wait` waits up to S seconds                                                                                          |
| `landline listen --as N`                     | Waits until mail arrives, prints it, and exits                                                                                             |
| `landline hook --as N`                       | Prints Claude Code hook JSON when mail is waiting                                                                                          |
| `landline register --name N --harness H`     | Registers a name by hand, for scripts and tests                                                                                            |
| `landline agentd [--host H]`                 | Wakes Codex agents on this host for a hub on another one                                                                                   |
| `landline log [--after SEQ]`                 | Prints the hub's message log, with each message's text                                                                                     |

## Not built yet

- Mail is handed out once. Reading advances your position before the reply is written, so a reader that fails after the
  hub answers loses those messages from its unread list. They stay in `landline log`, which prints each message's text.
- A read that names a thread does not move your position, so `inbox --thread` shows the same history until you also pass
  `--after`. `listen --thread` and the MCP tool handle this for you: `listen` waits for mail that arrives after it
  starts, and the MCP tool prints the `after` value to use next.
- A WebSocket transport. The hub uses HTTP with long polling for now.
- A wake method for Claude Code sessions. Use the hook and `landline listen`, or Claude's own cross-session messages on
  one machine.
- A dashboard. The hub prints each wake, and `landline log` shows the history.
- Pending wakes for another host live in the hub's memory, so a hub restart drops them. The message stays in the inbox.
- Tokens are saved in a private file on the hub, and the hub secret is the only credential that can register an agent.

## Layout

| Path                                              | Holds                                                 |
| ------------------------------------------------- | ----------------------------------------------------- |
| [architecture.yaml](architecture.yaml)            | Components, positions, drivers, and banned categories |
| [docs/architecture/](docs/architecture/README.md) | How the parts fit together                            |
| [docs/decisions/](docs/decisions/)                | Decision records                                      |
| `internal/`                                       | One package per component, and the shared model       |
| `contracts/`                                      | One contract per core component                       |
| `acceptance/`                                     | Scenarios that start the built binary                 |

Built with [Threnos](https://github.com/8thlight-daveloper/threnos). Run `nix develop` for the tools and `just check`
for every check.

## License

MIT. See [LICENSE](LICENSE).

______________________________________________________________________

<img src="assets/jack-mascot.svg" width="36" alt="Jack, the Landline mascot">

Copyright © 2026 David Nawara.
