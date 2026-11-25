# Components

The components in [architecture.yaml](../../architecture.yaml), with their position and kind. A core component is pure.
A driver does input and output.

| Component            | Position | Kind   | Does                                                                                                                              |
| -------------------- | -------- | ------ | --------------------------------------------------------------------------------------------------------------------------------- |
| `agent/registry`     | 1        | core   | Registers agents and looks one up by exact id                                                                                     |
| `message/router`     | 1        | core   | Checks a send request, stamps the sender, and chooses the recipients                                                              |
| `machine/registry`   | 1        | core   | Holds invites and machines as values, counts failed redeems, and checks a machine's signed request against a given time           |
| `log/store`          | 2        | driver | Appends envelopes to a file and reads them back                                                                                   |
| `wake/codex`         | 2        | driver | Queues a turn into a Codex session with `codex queue`                                                                             |
| `client/http`        | 2        | driver | Calls the switchboard's HTTP API                                                                                                  |
| `switchboard/server` | 3        | driver | Serves the HTTP API, holds agents, tokens, and observers in memory, and joins the core components to the store                    |
| `cli/command`        | 3        | driver | The commands of `landline` except `agents`: setup, claude, server, register, send, log, join, and a session that holds the stream |
| `mcp/server`         | 3        | driver | Serves the register, agents, and send tools over MCP, holds the delivery stream, and hands each delivery on                       |
| `agent/viewer`       | 3        | driver | The screen packages of the `tui/` module. Show the agent list on a terminal, and only read                                        |
| `menubar/viewer`     | 3        | driver | The packages of the `menubar/` module. Show the agents in a macOS menu bar item, and run `landline` for server and setup          |

`agent/viewer` is the one component outside `internal/`. It is the set of screen packages under `tui/internal/`, in the
`tui/` Go module. They hold their own small copies of the switchboard's JSON types and import no root package and no
other component. They read the switchboard over HTTP with an observer token and read
`$HOME/.config/landline/config.toml` for its host, port, secret, and machine name, and skip every key of its `[server]`
table. They read the old `config.json` when there is no `config.toml`. They read `$HOME/.config/landline/machine.key` to
sign a registration, as
[0200](../decisions/0200-a-machine-joins-with-an-invite-code-and-a-keypair-that-outlasts-secret-rotation.md) says. They
write the terminal and standard output, and no files.

`menubar/viewer` is the set of packages of the `menubar/` Go module, outside `internal/`, built on macOS only
([0140](../decisions/0140-a-macos-menu-bar-item-shows-agents-runs-the-switchboard-and-sets-up-harnesses.md)). Like
`agent/viewer` it holds its own copies of the switchboard's JSON types and imports no root package and no other
component. It reads the switchboard over HTTP with an observer token, including `GET /v1/build`, and reads
`$HOME/.config/landline/config.toml` for the host, port, secret, and machine name, and skips every key of its `[server]`
table. It reads the old `config.json` when there is no `config.toml`. It reads `$HOME/.config/landline/machine.key` to
sign a registration. It tests whether the host is this machine's, and runs
`landline server up|down|restart|rotate-secret` and `landline setup claude|codex [--remove]` as child processes, and
asks the login shell once for its `PATH`. It writes the menu bar, the About panel, and the clipboard, and no files.

Two files hold the machine keys of
[0200](../decisions/0200-a-machine-joins-with-an-invite-code-and-a-keypair-that-outlasts-secret-rotation.md).
`switchboard/server` keeps `machines.json` at `$HOME/.local/share/landline/machines.json`, in the data directory,
whatever the log path
([0220](../decisions/0220-the-switchboard-reads-its-settings-from-the-config-and-keeps-machines-json-in-the-data-directory.md)).
It reads the file when it starts and writes it after a redeem or a revoke. When that file does not exist at start and a
`machines.json` exists in the folder of the log path in use, it reads the old file and writes its entries to the new
path before it listens. It never writes, renames, or deletes the old file. Before it reads `machines.json`, it creates
`$HOME/.local/share/landline/switchboard.lock` and holds an exclusive lock on it until it exits.
`cmd/landline-switchboard` passes both paths in. `cli/command` writes `$HOME/.config/landline/machine.key` on
`landline server join --code`, and reads it to sign a registration and to refuse `join` without a code while a key is
saved. `mcp/server` reads no file. `cmd/landline-mcp` signs through `cli/command`, which reads the key. `agent/viewer`
and `menubar/viewer` read the key with their own copies of the reader. `client/http` and `machine/registry` read and
write no file.

`$HOME/.config/landline/config.toml` holds the switchboard's settings in a `[server]` table
([0220](../decisions/0220-the-switchboard-reads-its-settings-from-the-config-and-keeps-machines-json-in-the-data-directory.md)).
`cmd/landline-switchboard` reads the file when it starts, with the reader of `cli/command`: the top-level keys for the
address and the secret, and `[server]` for the log path and the timings. On `SIGHUP` it reads the top-level `secret`
again, and nothing else
([0240](../decisions/0240-rotate-secret-reloads-the-running-switchboard-instead-of-restarting-it.md)). It writes no part
of the file. `cli/command` reads `[server]` for `landline server up`, `restart`, and `rotate-secret`, so that an error
in the table stops them before they change anything and so that they print a line for each flag that differs from the
table. They pass on the flags a person gave and the saved address, and never a value from the table. Its writer keeps
the `[server]` table and adds a missing top-level key before the first table line. `cli/command` reads `switchboard.out`
to wait for `listening on` after `up` starts the switchboard, and to show the output when the start fails, but not to
find the log path. That wait depends on text written for people, which C1 in
[coding-principles.md](../reference/coding-principles.md) rules out, and it is older code to fix in its own change.

The entry points are not components and are outside the dependency graph, so they may import any component.
`cmd/landline-switchboard` serves the switchboard, and `cmd/landline-mcp` serves the MCP process and holds no
switchboard code. `tui/main.go` builds `landline`. It handles `agents` itself with the client and view model of
`agent/viewer`, converts each row to the root model's type for the attribute text, and hands every other command to
`cli/command`. It is the only file of the `tui/` module that imports root packages. The switchboard takes its store as
an interface, and `mcp/server` takes its Codex waker as one, so the wiring in the entry points is what joins them.
