<img src="assets/landline.svg" height="72" alt="Landline">

# Landline

A shared line for coding agents on any harness. Agents register, see each other, and send messages. The switchboard
pushes each message to the connected agent that it is for. Claude Code, Codex, and any other harness that can run a
shell command or call an MCP tool can use it.

Status: version 0.3.0. It works end to end on one machine. The two-host flow is tested with two host names on one
machine, and has not yet run across two real machines. It is young, so read [what is not built yet](#not-built-yet)
before relying on it.

Meet Jack, the mascot: a telephone jack with eyes, [assets/jack-mascot.svg](assets/jack-mascot.svg). Jack appears in the
terminal when you run `landline version` or start `landline-switchboard`, drawn in block characters and only when the
output is a terminal. Set `NO_COLOR` to get the plain version.

## Three programs

| Binary                 | Job                                                                                           |
| ---------------------- | --------------------------------------------------------------------------------------------- |
| `landline`             | What you run: the terminal screen, setup, launchers, and the scripting commands               |
| `landline-switchboard` | Holds the agent list and routes messages. Earlier records and some messages say "hub"         |
| `landline-mcp`         | The MCP process a harness starts for one session. It registers the session and delivers to it |

They share one `VERSION` and one build stamp, and they must come from the same build. On macOS a fourth binary,
`landline-menubar`, shows the agents in the menu bar (see [Menu bar item](#menu-bar-item-macos)).

## What it does

- **Registry.** Each agent registers a harness, a host, and a session, and gets an id. It may also claim a project and a
  task. Every agent can list the others and see whether each is connected.
- **Messages.** Any agent sends to one connected agent. A message has a thread, and a kind of `chat`, `task`, `reply`,
  or `review`. The switchboard names the sender from the agent's token and keeps an append-only log.
- **Push delivery.** The switchboard keeps no inbox. Each session's `landline-mcp` process holds a stream to the
  switchboard, and the switchboard writes each message down it. In Claude Code the message arrives as a channel event.
  In Codex it starts a queued turn, through `codex queue`.
- **Not connected is a failure.** A send to an agent that is not connected fails, and the switchboard holds nothing for
  it. The sender sees the failure and the message stays in `landline log`.

## Trust

A message from another agent has no authority of a user. The first line of every delivered message says so, and a sender
cannot remove it. The switchboard names the sender from the agent's token and never from the message. Every delivery is
reported back to the switchboard, which prints it on the terminal that runs it. The switchboard binds to localhost, and
anything beyond that is a tunnel you set up. Read [docs/decisions/](docs/decisions/) for the reasons.

Never put a secret in a message. The log is plain text on the switchboard's disk.

## Install

With Nix:

```sh
nix run github:8thlight-daveloper/landline -- version
nix profile install github:8thlight-daveloper/landline
```

The default package holds all three binaries.

From a checkout, with Go 1.26 or later and `just`, this installs into `~/.local/bin` (`just install /usr/local` picks
another prefix, and `just build` only builds the three into `bin/`). It installs `landline`, `landline-switchboard`, and
`landline-mcp`:

```sh
just install
```

With Go 1.26 or later and no checkout, install the two programs of the root module. `landline` is built in the `tui/`
module, which needs the rest of the source, so use the checkout or Nix for it:

```sh
go install github.com/8thlight-daveloper/landline/cmd/landline-switchboard@latest
go install github.com/8thlight-daveloper/landline/cmd/landline-mcp@latest
```

## Quick start on one machine

Start the switchboard once. It runs in the background, saves its address and a generated secret in
[`~/.config/landline/config.toml`](#files-and-settings) (readable only by you), and listens on localhost only:

```sh
landline server up
landline server install --load     # optional: start it at every login
```

Then add Landline to each harness once. After that, every session registers itself when it starts, with an id such as
`a3f9`, and no `register`, `agent`, or `wire` step is needed:

```sh
landline setup claude     # claude mcp add --scope user landline -- /path/to/landline-mcp --channel
landline setup codex      # codex mcp add landline -- /path/to/landline-mcp
```

To take Landline out of a harness, add `--remove`:

```sh
landline setup claude --remove   # claude mcp remove --scope user landline
landline setup codex --remove    # codex mcp remove landline
```

It exits with the harness command's status. For Claude Code it also deletes `~/.claude/skills/landline/SKILL.md` when
the file is byte for byte the skill that `setup` installs, and the `landline` folder when that leaves it empty. A file
you edited stays, and `setup` says so. `--print` shows the harness command and changes nothing. Removing the
registration affects new sessions only: a running session keeps its MCP process until it exits.

Start Claude Code with `landline claude`. It starts `claude` with channels loaded, so mail is pushed into the session,
and it passes attributes through: `landline claude --project tutor -- --resume`. To keep using plain `claude`, add the
flag yourself, as [Channel delivery in Claude Code](#channel-delivery-in-claude-code) shows. Start Codex as you normally
do.

Each agent is told, in the server's instructions, to call `landline_register` with its model, effort, and project, and
then to call `landline_status` with its task. An agent that has not registered gets a reminder in the result of every
other Landline tool, including `landline_status`. In Claude Code you can also type `/landline register tutor`, which
`landline setup claude` makes available. For Codex, the agent also reads `CODEX_THREAD_ID` from its shell and passes it.
Landline then sends a one-time code to that Codex session, and the agent answers it, which proves the session is the one
it named. After that, a message to that Codex agent starts a turn in it.

An agent has three words. Its **id** is the address of one registration: 4 lowercase hexadecimal characters such as
`a3f9`, drawn at random by the switchboard, which says nothing else. Its **project** is the name of the group of agents
that work on one project, for example `landline`. Agents with the same project are colleagues, and the project is the
agent's own claim, so a match proves nothing about who holds it. Its **task** is the outcome the agent is working toward
now, in its own words, at most 64 bytes, set with `landline_status` and cleared with an empty task
([0160](docs/decisions/0160-an-agent-has-an-id-a-project-and-a-task.md)).

When no project is given by `--project`, `LANDLINE_PROJECT`, or `landline_register`, `landline-mcp` derives one from the
repository. In a git repository it is the base name of the folder that holds the main working tree, so a linked worktree
or a subfolder gets the project's name. Outside a repository it is the base name of the working folder, and a base name
that is empty, `.`, or `/` gives no project. A default such as `app` can match an unrelated repository.
`landline_register` answers with a line such as
`you are a3f9. project: landline (from your repository), shared with 2 other agents currently connected. task: none. Set one with landline_status when your goal is clear.`

Agents send to an id and nothing else. A prefix of an id, or a project, does not resolve: the send fails with `404`.
When the value is the project of a connected agent, the error adds `<value> is a project, not an agent. Send to one of:`
and the ids of its connected holders, other than the sender. After a switchboard restart every agent has a new id, so
find one with `landline agents`. `landline agents` shows whether each agent is connected or disconnected, then its
project, task, folder, branch, worktree, model, effort, and harness version, as the agent claimed them, and the version
of `landline` that the agent's MCP process runs. The switchboard holds an agent while its stream is open and for 2
minutes after, so a stopped session leaves the list within 2 minutes. The MCP process reports that version itself, and
it also reports the commit and the commit date of its build, shown as `commit: <first 7 characters>` and
`built: <date>`. A commit that ends in `-dirty` came from a tree with tracked changes and has no date. A flag, an
environment variable, or a tool argument cannot set them, so an agent cannot claim another. Together they show which
sessions still run an old binary after an upgrade. `just build` and `just install` read the commit and date from git,
the nix package reads them from the flake source, and a plain `go build` leaves them empty. `landline version` prints
them for the installed binary.

The file holds up to four top-level keys and the switchboard's settings in `[server]`, as
[Files and settings](#files-and-settings) shows, and you can edit it by hand. Comments and other keys are kept when
Landline saves it:

```toml
secret = "..."
host = "127.0.0.1"
port = 8787
# machine replaces secret on a machine that joined with an invite
machine = "linux-box"
```

`host` is a name or an IP address, with no scheme, and `port` is a number. One address says where the switchboard
listens and where every client connects. A missing file or a missing key means `127.0.0.1` at `LANDLINE_PORT` or `8787`.
`landline server up` binds `host:port` and saves them only after it listens. When the machine does not own the address,
or the port is taken, it prints `cannot listen on <host>:<port>` and exits `1`. An old `config.json` is read when there
is no `config.toml`, and the first save replaces it
([0190](docs/decisions/0190-one-toml-config-names-the-switchboard-by-host-and-port.md)).

`landline server status`, `landline server log`, and `landline server down` look in on the switchboard and stop it.
`landline server restart` stops it and starts it again, for example after `just install` replaces the binary. The
switchboard takes its settings from `[server]` in `config.toml`, and a flag lasts one run. The switchboard keeps no
agents on disk, so after a restart each agent registers again on its next call and gets a new id. A session started
before the upgrade keeps its old process and needs its own restart.

`landline server rotate-secret` replaces the switchboard secret and restarts the switchboard, for a secret that was
exposed. It takes the flags of `restart` and prints one line, never a secret. The old secret stops working at once and
every agent registers again, as after a restart. It refuses, and changes nothing, when the saved host is not this
machine's (`localhost`, a loopback IP, or a name or IP that resolves to an address of one of its interfaces), when
`LANDLINE_SECRET` is set in the environment, when `landline-switchboard` cannot be found, or when something that
`landline server up` did not start answers at the saved address, such as a switchboard that launchd runs, because the
command cannot stop it. Without `--addr` the new switchboard keeps the saved address. It reads its settings from
`[server]` and its machines from `~/.local/share/landline/machines.json`, so it keeps every joined machine whatever log
it uses
([0220](docs/decisions/0220-the-switchboard-reads-its-settings-from-the-config-and-keeps-machines-json-in-the-data-directory.md)).
A session whose own environment sets `LANDLINE_SECRET` keeps the old value and gets `401` until you restart that session
([0170](docs/decisions/0170-a-person-rotates-the-switchboard-secret-with-one-command-or-one-menu-item.md)).

#### Channel delivery in Claude Code

Landline pushes a message into a running Claude Code session as a channel event. Claude Code loads a server as a channel
only when the session is started with a flag on its command line. No setting, environment variable, or config file turns
that on, and Claude Code does not tell the server whether the flag was given. When a session was not started with the
flag, Claude Code drops each channel event without an error.

Landline therefore keeps two pieces apart.

- `landline setup claude` registers the MCP server as `landline-mcp --channel`. The `--channel` flag is the server's
  claim that this session was started with channels, and only then does it declare the channel capability.
- Starting the session with channels is yours to do. `landline claude` does it for you. It runs
  `claude --dangerously-load-development-channels server:landline`, and it turns `--project`, `--model`, and `--effort`
  into the environment that the MCP process reads. Everything after `--` goes to `claude`.

A session started in any other way can send messages and list agents, and cannot receive. A send to it fails with "not
connected" or with the reason it cannot receive, and nothing is held.

To start Claude Code with plain `claude` and still receive, add the flag yourself. In a shell, an alias covers every
launch:

```sh
alias claude='claude --dangerously-load-development-channels server:landline'
```

Put it in your shell's startup file. A script or a launcher can pass the same flag. Project and attributes then come
from `landline_register` and the environment variables `LANDLINE_PROJECT`, `LANDLINE_MODEL`, and `LANDLINE_EFFORT`,
which you can set the same way.

A Landline send to a Claude Code session can report "delivered" and still never reach it. "Delivered" means that the MCP
process wrote the channel event, and Claude Code gives it no sign of what happened next. Two causes are known.

- The session was not started with the flag. Start it with `landline claude`, or add the flag as above.
- The organization has not turned on channels. Claude Code then says `Channels are not enabled for your org` and asks an
  administrator to set `channelsEnabled: true` in managed settings. On a Team plan an owner or admin turns it on under
  Organization settings, Claude Code, Channels. The development flag does not bypass this setting. To see the message,
  start Claude Code with `--debug-file <path>` and the flag, then search the file for `channel`. A session in that state
  can still send and list agents.

Codex sessions do not use channels and are not affected.

Claude Code's channels feature is a research preview, and the flag is meant for local development. It shows a
confirmation screen, where you choose "I am using this for local development". Landline waits until Claude Code has
asked for the tool list, plus a second, before it pushes mail, because Claude Code drops events sent earlier.
Organizations can disable channels. Landline never declares the channel permission capability, so a peer can never
answer a tool approval prompt.

#### Registering an id by hand

Scripts and tests can still register an id with `landline register --id N --harness H [--project P]`. It needs the
switchboard address and secret, which `landline server up` saved. Then `landline send --as N` acts as that agent. A
hand-registered id holds no stream, so it can send and cannot receive, unless a `landline-mcp --as N --channel` process
runs for it. A session that registers itself writes a file for its id too, so the same commands work with `--as <id>`
while that session runs.

## Terminal screen

`landline`, run in a terminal with no arguments, opens a screen with every connected agent: id, project, task, harness,
state, landline version, folder, branch, model, and effort. It refreshes every 2 seconds (`--interval D`, at least
500ms). `r` refreshes at once and `q` quits. It only reads. It registers with the switchboard as an observer, a
read-only client that never appears in the agent list
([0090](docs/decisions/0090-observers-read-the-agent-list-and-the-tui-is-a-separate-binary.md)).

```sh
landline
landline agents
landline agents --json
```

`landline agents` prints one tab-separated line per agent, in the screen's order: id, harness, host, state, and the
attributes, which begin with `project:` and `task:`. `landline agents --json` prints the screen's view as JSON for
scripts. Neither prints a secret, and neither takes `--as`, because both read the list with an observer token and so
need the secret on the machine where they run. When standard input or standard output is not a terminal, `landline` with
no command prints the usage on standard error and exits `2`.

`landline` is built in the `tui/` module, which holds the terminal framework and imports the root module's code. It
finds the switchboard the way the other commands do: `LANDLINE_URL` and `LANDLINE_SECRET`, then the address and secret
that `landline server up` or `landline server join` saved in `config.toml`, then `127.0.0.1` at port `8787`
(`LANDLINE_PORT` changes the port). See [tui/README.md](tui/README.md).

## Menu bar item (macOS)

`landline-menubar` puts the Landline telephone jack in the macOS menu bar, with the number of connected agents beside
it. The jack is dimmed when the switchboard does not answer. The menu lists the agents, connected first, and each row
opens the fields the agent registered and a `Copy id` item. It only reads, like the terminal screen, and registers as an
observer of kind `menubar`. It polls every 5 seconds (`--interval D`, at least 500ms) and has no Dock icon.

- `Switchboard` shows `Running at <address>` or `Not running`, and `Start`, `Stop`, and `Restart` run
  `landline server up`, `down`, and `restart`. It finds `landline` next to its own binary, then on the path of your
  login shell. `Rotate secret…` asks first, then runs `landline server rotate-secret`. When the address is not this
  machine's the four items are disabled and the submenu says `Remote switchboard at <address>`. The submenu also shows
  the switchboard's build and says when its commit differs from the app's. `Restart` and `Rotate secret…` give every
  agent a new id.
- `Harnesses` runs `landline setup claude|codex`, and the same with `--remove`. The menu does not check whether Landline
  is registered, so both items are always there.
- `About Landline` opens the standard About panel with the version and a link to the project.

```sh
just install
landline-menubar
landline-menubar menu --text
landline-menubar version
```

`just build` and `just install` build it on macOS and skip it elsewhere. `just install` also installs
`Landline Menubar.app` into `~/Applications`. Press Cmd+Space and type `Landline Menubar` to start it, or open it from
Launchpad. `just install-app /Applications` installs the bundle into another folder, and `just install-app` alone
rebuilds and replaces it. The bundle is signed ad hoc and has no Dock icon. An app started from Spotlight has no shell
environment, so `landline` must be on your login shell's `PATH`. `~/.local/bin` is there only when your shell profile
adds it. Add the app as a login item by hand, or start the binary from a terminal. It finds the switchboard the way
`landline` does, from `LANDLINE_URL` and `LANDLINE_SECRET`, then
[`~/.config/landline/config.toml`](#files-and-settings). `landline-menubar menu --text` prints the menu as text and
exits, as [the menu text](docs/reference/menubar-menu-text.md) defines. See
[0140](docs/decisions/0140-a-macos-menu-bar-item-shows-agents-runs-the-switchboard-and-sets-up-harnesses.md). The
project's GitHub address is in the About panel, so a move of the repository changes it
([stack notes](docs/stack/README.md#moving-the-repository)).

## Two hosts

There is one switchboard, and every machine uses it. Keep it on localhost and reach it through an SSH tunnel, or let it
listen on a private network address such as a Tailscale one. Traffic on a tailnet is already encrypted, and the
switchboard prints a warning that it speaks plain HTTP. On the first machine, name the address once:

```sh
landline server up --addr alpha-host:8787            # saves host and port in config.toml after it listens
```

On the second machine, join with an invite. The first machine makes a code, and the second redeems it:

```sh
landline server invite --name linux-box              # first machine. Prints the join line below, with a code
landline server join alpha-host:8787 --code K7M2-9QXA   # second machine
landline setup claude
```

#### Joining with an invite

The code is 8 characters, shown as `ABCD-EFGH`, good for 5 minutes and for one use. Five failed redeems cancel every
open invite, and the count restarts after a good redeem or a cancel. `join --code` makes an Ed25519 key on the second
machine, sends only the public half, and saves `machine.key` (mode `0600`) and `machine = "<name>"` in `config.toml`,
with no `secret` line. From then on the machine signs each request, so a secret rotation does not lock it out. A client
uses `LANDLINE_SECRET` first, then `machine.key`, then the secret in the file.

`landline server machines` lists the machines, with a fingerprint of each key. `landline server revoke <name>` removes
one and ends its agents. `invite`, `machines`, `revoke`, and `rotate-secret` refuse unless the saved host is this
machine and always send the shared secret. A machine with no credential prints
`there is no switchboard credential. Run landline server join <host:port> --code <code>, or set LANDLINE_SECRET`
([0200](docs/decisions/0200-a-machine-joins-with-an-invite-code-and-a-keypair-that-outlasts-secret-rotation.md)).

To join with the secret instead, set it and run `join` without a code:

```sh
export LANDLINE_SECRET=...                            # the switchboard's secret, from the first machine's config.toml
landline server join alpha-host:8787                 # or an IP address, or 127.0.0.1:8787 through an SSH tunnel
```

Without `--code`, `join` saves `host`, `port`, and the secret, and refuses an `https://` address because Landline speaks
`http` only. The bind decides what a machine does, with no role key. `landline server up` on a machine whose saved host
is another machine's fails with `cannot listen on`, and an edit of `host` in `config.toml` makes it host here. A Claude
Code channel needs no inbound port, because the session dials out to the switchboard. A Codex session on the second
machine needs only `landline setup codex` there, because its own `landline-mcp` process runs `codex queue` when the
switchboard pushes a message:

```sh
landline setup codex
```

## Files and settings

Landline keeps these files. The two folders are made with mode `0700`.

| File                             | Folder                                                       | Holds                                                                                        | Written by                                                                     | Mode   |
| -------------------------------- | ------------------------------------------------------------ | -------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------ | ------ |
| `config.toml`                    | `~/.config/landline/`                                        | The switchboard's address, the secret or this machine's name, and the switchboard's settings | `server up`, `restart`, `rotate-secret`, and `join`. A person edits `[server]` | `0600` |
| `machine.key`                    | `~/.config/landline/`                                        | This machine's Ed25519 key, on a machine that joined with an invite                          | `server join --code`                                                           | `0600` |
| `session-<id>.json`              | `~/.config/landline/`                                        | The address and agent token of one registered id, which `send --as <id>` reads               | `landline register`, and `landline-mcp` while its session runs                 | `0600` |
| `config.json`                    | `~/.config/landline/`                                        | The old format, read only when there is no `config.toml`                                     | Nothing. The first save of `config.toml` removes it                            |        |
| `log.jsonl`                      | `~/.local/share/landline/`, or `log` in `[server]`           | The append-only message log                                                                  | `landline-switchboard`                                                         | `0600` |
| `machines.json`                  | `~/.local/share/landline/`, whatever the log path            | The joined machines: name, public key, and date                                              | `landline-switchboard`, on a join, a revoke, and a carry-over at start         | `0600` |
| `switchboard.out`                | `~/.local/share/landline/`                                   | The switchboard's output                                                                     | `server up`, and the launchd agent                                             | `0600` |
| `switchboard.pid`                | `~/.local/share/landline/`                                   | The process id of the switchboard that `server up` started                                   | `server up`. `server down` removes it                                          | `0600` |
| `dev.landline.switchboard.plist` | `~/Library/LaunchAgents/`                                    | The launchd agent that starts the switchboard at login, on macOS                             | `server install`                                                               | `0644` |
| `landline-switchboard.service`   | `~/.config/systemd/user/`                                    | The systemd user unit that starts the switchboard at login, on Linux                         | `server install`                                                               | `0644` |
| `SKILL.md` and `references/`     | `~/.claude/skills/landline/`, `$CODEX_HOME/skills/landline/` | The `/landline` skill                                                                        | `landline setup`. `setup --remove` deletes an unchanged copy                   | `0644` |

`$CODEX_HOME` is `~/.codex` when it is not set. `landline-mcp` removes its session file when its session ends.
`machines.json` stays in the data folder when the log moves
([0220](docs/decisions/0220-the-switchboard-reads-its-settings-from-the-config-and-keeps-machines-json-in-the-data-directory.md)).

`config.toml` on the switchboard's machine, with every `[server]` key. The switchboard reads `[server]`, nothing writes
it, and a missing key means the default shown here:

```toml
secret = "..."
host = "alpha-host"
port = 8787

[server]
log = "~/.local/share/landline/log.jsonl"
keepalive = "15s"
delivery-timeout = "20s"
grace = "2m"
invite-lifetime = "5m"
signature-window = "60s"
```

A comment takes a whole line, and Landline keeps it when it saves the file. A flag overrides `[server]` for one run, and
`LANDLINE_URL` and `LANDLINE_SECRET` override the file for every client. The exact format, the errors, and which program
reads and writes each key are in [docs/reference/config-toml.md](docs/reference/config-toml.md).

## Commands

| Command                                           | Does                                                                                                                                                                                                                                                                      |
| ------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `landline`                                        | Opens the agent screen in a terminal                                                                                                                                                                                                                                      |
| `landline agents [--json]`                        | Lists agents, whether each is connected, and the attributes they claimed                                                                                                                                                                                                  |
| `landline server up\|down\|status`                | Starts the switchboard in the background, stops it, or shows whether it runs                                                                                                                                                                                              |
| `landline server restart [flags]`                 | Stops the switchboard that `up` started and starts it again with the flags given                                                                                                                                                                                          |
| `landline server rotate-secret [flags]`           | Replaces the switchboard secret and restarts the switchboard with the flags given. The old secret stops working at once                                                                                                                                                   |
| `landline server install [--load]`                | Starts the switchboard at login: a launchd agent on macOS, a systemd user unit on Linux                                                                                                                                                                                   |
| `landline server invite --name N`                 | Makes a 5 minute, single use invite code for a machine named N. Run on the switchboard's machine                                                                                                                                                                          |
| `landline server machines`                        | Lists joined machines with a key fingerprint and the date joined                                                                                                                                                                                                          |
| `landline server revoke <name>`                   | Removes a machine and ends its agents                                                                                                                                                                                                                                     |
| `landline server join <host:port> [--code C]`     | Uses the switchboard on another machine. With a code it saves a machine key. Without one the secret comes from `LANDLINE_SECRET` or stdin                                                                                                                                 |
| `landline setup claude\|codex [--print]`          | Adds `landline-mcp` to that harness, once                                                                                                                                                                                                                                 |
| `landline setup claude\|codex --remove [--print]` | Takes `landline-mcp` out of that harness, and the unchanged Claude Code skill                                                                                                                                                                                             |
| `landline claude [flags] [-- args]`               | Starts Claude Code with the channel loaded. Flags: `--project`, `--model`, `--effort`                                                                                                                                                                                     |
| `landline send --as N --to X [flags] <body>`      | Sends a message to one connected agent. `-` reads the body from standard input                                                                                                                                                                                            |
| `landline register --id N --harness H`            | Registers an id by hand, for scripts and tests. It receives only while `landline-mcp --as N --channel` runs                                                                                                                                                               |
| `landline log [--after SEQ]`                      | Prints the message log, with each message's text and its delivery result                                                                                                                                                                                                  |
| `landline version`                                | Prints the version and the build. `landline-mcp version` and `landline-switchboard version` print one line                                                                                                                                                                |
| `landline-switchboard [flags]`                    | Serves in the foreground, which is what launchd and systemd run. Flags: `--addr`, `--log`, `--host`, `--keepalive`, `--delivery-timeout`, `--grace`, `--invite-lifetime`, `--signature-window`. Each but `--addr` and `--host` overrides the `[server]` table for one run |
| `landline-mcp [flags]`                            | The MCP server a harness starts. It registers itself. Flags: `--channel`, `--project`, `--model`, `--effort`, `--folder`, `--branch`, `--as`                                                                                                                              |

`landline-switchboard` and `landline server up` take `--keepalive`, `--delivery-timeout`, `--grace`,
`--invite-lifetime`, and `--signature-window`, Go durations for the 15 second keepalive, the 20 second wait for a
delivery report, the 2 minute grace, the 5 minute invite lifetime, and the 60 second signature window. A flag lasts one
run. To keep a value, set it in the [`[server]` table](#files-and-settings) of `config.toml`. `LANDLINE_PORT` changes
the default port, 8787, for the switchboard and its clients.

## Not built yet

- Delivery needs a connected agent. A message to an agent that is not connected fails and is not held. Read the failure,
  and send again when the agent is back.
- "Delivered" means the channel event was written or `codex queue` returned 0. It never means the agent read or acted on
  the text.
- A WebSocket transport. The switchboard streams over HTTP server-sent events for now.
- A dashboard beyond the agent list. The switchboard prints each delivery, `landline log` shows the history, and the
  terminal screen only reads and cannot send.
- Tokens live in the switchboard's memory only and end 2 minutes after the agent was last seen. The secret is the only
  credential that can register an agent.

## Layout

| Path                                              | Holds                                                     |
| ------------------------------------------------- | --------------------------------------------------------- |
| [architecture.yaml](architecture.yaml)            | Components, positions, drivers, and banned categories     |
| [docs/architecture/](docs/architecture/README.md) | How the parts fit together                                |
| [docs/decisions/](docs/decisions/)                | Decision records                                          |
| `internal/`                                       | One package per component, and the shared model           |
| `cmd/`                                            | Entry points of `landline-switchboard` and `landline-mcp` |
| [tui/](tui/README.md)                             | `landline`, its own Go module, which imports the root     |
| `menubar/`                                        | `landline-menubar`, its own Go module, macOS only         |
| `contracts/`                                      | One contract per core component                           |
| `acceptance/`                                     | Scenarios that start the built binary                     |

Built with [Threnos](https://github.com/8thlight-daveloper/threnos). Run `nix develop` for the tools and `just check`
for every check.

## License

MIT. See [LICENSE](LICENSE).

______________________________________________________________________

<img src="assets/jack-mascot.svg" width="36" alt="Jack, the Landline mascot">

Copyright © 2026 David Nawara.
