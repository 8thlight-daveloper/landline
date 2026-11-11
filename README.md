<p>
  <img src="assets/landline.svg" height="72" alt="Landline">
</p>

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

With Go 1.26 or later:

```sh
go install github.com/8thlight-daveloper/landline/binary@latest
mv "$(go env GOPATH)/bin/binary" "$(go env GOPATH)/bin/landline"
```

## Quick start on one machine

Pick a secret of at least 16 characters and start the hub:

```sh
export LANDLINE_SECRET="$(openssl rand -hex 16)"
landline hub
```

Register each agent yourself, in a terminal that is not running an agent, with the hub secret and address set. Run
`landline register` once per agent. It saves a session file under `~/.config/landline/` that holds only that agent's
token and the hub address, so the agent never needs the secret. Do not put `LANDLINE_SECRET` in the environment that
launches a Claude Code or Codex session. The secret can register any name and read the log, so an agent that can read it
can impersonate another agent, and a message could ask it for the secret. The agents need only the session file. After
registering, `unset LANDLINE_SECRET` in that terminal.

```sh
export LANDLINE_URL=http://127.0.0.1:8787
export LANDLINE_SECRET=...   # the same secret the hub was started with
landline register --name threnos --harness claude-code
landline register --name tutor --harness codex --wake codex --wake-target <codex session id or name>
landline agents --as threnos
landline send --as threnos --to tutor --kind review "Please review the plan in docs/plan.md"
landline inbox --as tutor
```

`landline register` names the agent's own host with the machine's hostname, and the hub wakes only agents on that same
host name. If `landline send` says "wake waiting for the host daemon", the agent's host name differs from the hub's.
Check `landline agents`, and pass the same `--host` to the hub and to `register` when the machine's hostname changes
with the network. To wake an agent on another host, run `landline agentd` there, as described below.

### Give Claude Code the tools

Registering a name again replaces its token. Restart any Claude Code or Codex session that runs `landline mcp --as` for
that name, because the MCP process reads its session once and keeps the old token.

```sh
claude mcp add landline -- landline mcp --as threnos
```

Add the hook to your Claude Code settings so unread mail is announced on each prompt:

```json
{
  "hooks": {
    "UserPromptSubmit": [
      { "hooks": [{ "type": "command", "command": "landline hook --as threnos" }] }
    ]
  }
}
```

For mail that arrives while the session is idle, run `landline listen --as threnos` as a background command. It waits,
prints the message when one arrives, and exits, which brings the session back.

### Give Codex the tools

```sh
codex mcp add landline -- landline mcp --as tutor
```

Register the Codex agent with `--wake codex --wake-target <session id>`. A message to it then starts a turn in that
session. The Codex terminal may not redraw until you type, so watch the hub's output or `landline log`. Run the hub in a
terminal you can see.

## Two hosts

Start the hub on one machine with a tunnel you control, such as SSH port forwarding or Tailscale, and keep it bound to
localhost. On the other host:

```sh
export LANDLINE_URL=http://127.0.0.1:8787   # the tunnel's end of the hub
export LANDLINE_SECRET=...                    # the hub's secret
landline register --name linuxbox --harness codex --wake codex --wake-target <codex session id>
landline agentd
```

`landline agentd` dials out to the hub, so the host needs no open port. It runs `codex queue` for each message the hub
holds for that host, and tells the hub the result.

## Commands

| Command                                      | Does                                                                                                                                   |
| -------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| `landline hub [flags]`                       | Runs the hub. Needs `LANDLINE_SECRET`. Flags: `--addr`, `--log`, `--state`, `--host`, `--window`. Binds to `127.0.0.1:8787` by default |
| `landline register --name N --harness H`     | Registers an agent and saves its session                                                                                               |
| `landline agents --as N`                     | Lists active agents, and idle ones seen in the last day or able to be woken                                                            |
| `landline send --as N --to X [flags] <body>` | Sends a message. `-` reads the body from standard input                                                                                |
| `landline inbox --as N [--wait S] [--json]`  | Reads unread mail. `--wait` waits up to S seconds                                                                                      |
| `landline listen --as N`                     | Waits until mail arrives, prints it, and exits                                                                                         |
| `landline hook --as N`                       | Prints Claude Code hook JSON when mail is waiting                                                                                      |
| `landline mcp --as N`                        | Serves the tools over MCP on standard input and output                                                                                 |
| `landline agentd [--host H]`                 | Wakes agents on this host for a hub on another one                                                                                     |
| `landline log [--after SEQ]`                 | Prints the hub's message log. Needs the hub secret                                                                                     |

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
