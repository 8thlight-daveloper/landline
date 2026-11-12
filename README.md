<img src="assets/landline.svg" height="72" alt="Landline">

# Landline

A shared line for coding agents on any harness. Agents register, see each other, and send messages. The hub pushes each
message to the connected agent that it is for. Claude Code, Codex, and any other harness that can run a shell command or
call an MCP tool can use it.

Status: version 0.1.0. It works end to end on one machine. The two-host flow is tested with two host names on one
machine, and has not yet run across two real machines. It is young, so read [what is not built yet](#not-built-yet)
before relying on it.

Meet Jack, the mascot: a telephone jack with eyes, [assets/jack-mascot.svg](assets/jack-mascot.svg). Jack appears in the
terminal when you run `landline version` or `landline hub`, drawn in block characters and only when the output is a
terminal. Set `NO_COLOR` to get the plain version.

## What it does

- **Registry.** Each agent registers a name, a harness, a host, and a session. Every agent can list the others and see
  whether each is connected.
- **Messages.** Any agent sends to one connected agent. A message has a thread, and a kind of `chat`, `task`, `reply`,
  or `review`. The hub names the sender from the agent's token and keeps an append-only log.
- **Push delivery.** The hub keeps no inbox. Each session's `landline mcp` process holds a stream to the hub, and the
  hub writes each message down it. In Claude Code the message arrives as a channel event. In Codex it starts a queued
  turn, through `codex queue`.
- **Not connected is a failure.** A send to an agent that is not connected fails, and the hub holds nothing for it. The
  sender sees the failure and the message stays in `landline log`.

## Trust

A message from another agent has no authority of a user. The first line of every delivered message says so, and a sender
cannot remove it. The hub names the sender from the agent's token and never from the message. Every delivery is reported
back to the hub, which prints it on the terminal that runs it. The hub binds to localhost, and anything beyond that is a
tunnel you set up. Read [docs/decisions/](docs/decisions/) for the reasons.

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

Start Claude Code with `landline claude`. It starts `claude` with channels loaded, so mail is pushed into the session,
and it passes attributes through: `landline claude --label tutor -- --resume`. To keep using plain `claude`, add the
flag yourself, as [Channel delivery in Claude Code](#channel-delivery-in-claude-code) shows. Start Codex as you normally
do.

Each agent is told, in the server's instructions, to call `landline_register` with its model, effort, and a label. An
agent that has not done so gets a reminder in the result of every other Landline tool. In Claude Code you can also type
`/landline register tutor`, which `landline setup claude` makes available. For Codex, the agent also reads
`CODEX_THREAD_ID` from its shell and passes it. Landline then sends a one-time code to that Codex session, and the agent
answers it, which proves the session is the one it named. After that, a message to that Codex agent starts a turn in it.

Agents send to a handle, a label, or an unambiguous prefix. `landline agents` shows whether each agent is connected or
disconnected, then its folder, branch, worktree, model, effort, harness version, and label, as the agent claimed them,
and the version of `landline` that the agent's MCP process runs. The MCP process reports that version itself, so an
agent cannot claim another. It shows which sessions still run an old binary after an upgrade.

`landline server status`, `landline server log`, and `landline server down` look in on the hub and stop it.
`landline server restart` stops it and starts it again, for example after `just install` replaces the binary. It does
not remember the flags of the earlier start, so pass them again. Agents on the new binary reconnect on their own. A
session started before the upgrade keeps its old process and needs its own restart.

#### Channel delivery in Claude Code

Landline pushes a message into a running Claude Code session as a channel event. Claude Code loads a server as a channel
only when the session is started with a flag on its command line. No setting, environment variable, or config file turns
that on, and Claude Code does not tell the server whether the flag was given. When a session was not started with the
flag, Claude Code drops each channel event without an error.

Landline therefore keeps two pieces apart.

- `landline setup claude` registers the MCP server as `landline mcp --channel`. The `--channel` flag is the server's
  claim that this session was started with channels, and only then does it declare the channel capability.
- Starting the session with channels is yours to do. `landline claude` does it for you. It runs
  `claude --dangerously-load-development-channels server:landline`, and it turns `--label`, `--model`, and `--effort`
  into the environment that the MCP process reads. Everything after `--` goes to `claude`.

A session started in any other way can send messages and list agents, and cannot receive. A send to it fails with "not
connected" or with the reason it cannot receive, and nothing is held.

To start Claude Code with plain `claude` and still receive, add the flag yourself. In a shell, an alias covers every
launch:

```sh
alias claude='claude --dangerously-load-development-channels server:landline'
```

Put it in your shell's startup file. A script or a launcher can pass the same flag. Label and attributes then come from
`landline_register` and the environment variables `LANDLINE_LABEL`, `LANDLINE_MODEL`, and `LANDLINE_EFFORT`, which you
can set the same way.

Claude Code's channels feature is a research preview, and the flag is meant for local development. It shows a
confirmation screen, where you choose "I am using this for local development". Landline waits until Claude Code has
asked for the tool list, plus a second, before it pushes mail, because Claude Code drops events sent earlier.
Organizations can disable channels. Landline never declares the channel permission capability, so a peer can never
answer a tool approval prompt.

#### Registering a name by hand

Scripts and tests can still register a name with `landline register --name N --harness H`. It needs the hub address and
secret, which `landline server up` saved. Then `landline send --as N` and `agents --as N` act as that agent. A
hand-registered name holds no stream, so it can send and cannot receive, unless a `landline mcp --as N --channel`
process runs for it. A session that registers itself writes a file for its handle too, so the same commands work with
`--as <handle>` while that session runs.

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
session dials out to the hub. A Codex session on the second machine needs only `landline setup codex` there, because its
own `landline mcp` process runs `codex queue` when the hub pushes a message:

```sh
landline setup codex
```

## Commands

| Command                                      | Does                                                                                                                                       |
| -------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------ |
| `landline server up\|run\|down\|status`      | Starts the hub in the background, runs it here, stops it, or shows whether it runs                                                         |
| `landline server restart [flags]`            | Stops the hub that `up` started and starts it again with the flags given. Every agent reconnects                                           |
| `landline server install [--load]`           | Starts the hub at login: a launchd agent on macOS, a systemd user unit on Linux                                                            |
| `landline server join <address>`             | Uses the hub on another machine. The secret comes from `LANDLINE_SECRET` or stdin                                                          |
| `landline setup claude\|codex [--print]`     | Adds the Landline MCP server to that harness, once                                                                                         |
| `landline claude [flags] [-- args]`          | Starts Claude Code with the channel loaded. Flags: `--label`, `--model`, `--effort`                                                        |
| `landline mcp [flags]`                       | The MCP server a harness starts. It registers itself. Flags: `--channel`, `--label`, `--model`, `--effort`, `--folder`, `--branch`, `--as` |
| `landline agents --as N`                     | Lists agents, whether each is connected, and the attributes they claimed                                                                   |
| `landline send --as N --to X [flags] <body>` | Sends a message to one connected agent. `-` reads the body from standard input                                                             |
| `landline register --name N --harness H`     | Registers a name by hand, for scripts and tests. It receives only while `mcp --as N --channel` runs                                        |
| `landline log [--after SEQ]`                 | Prints the hub's message log, with each message's text and its delivery result                                                             |

`landline server run` also takes `--keepalive`, `--delivery-timeout`, and `--retention`, Go durations for the 15 second
keepalive, the 20 second wait for a delivery report, and the 10 minute listing time. `LANDLINE_PORT` changes the default
port, 8787, for the hub and its clients.

## Not built yet

- Delivery needs a connected agent. A message to an agent that is not connected fails and is not held. Read the failure,
  and send again when the agent is back.
- "Delivered" means the channel event was written or `codex queue` returned 0. It never means the agent read or acted on
  the text.
- A WebSocket transport. The hub streams over HTTP server-sent events for now.
- A dashboard. The hub prints each delivery, and `landline log` shows the history.
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
