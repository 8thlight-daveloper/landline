---
category: architecture
status: accepted
approval: person
affects: [hub/server, mcp/server, cli/command, agent/viewer]
builds-on: ['0060', '0070', '0090', '0100']
partly-supersedes: ['0010', '0060', '0090', '0100']
note: Drafted on 2026-11-14. It replaces the one-program layout of 0010, the command names of 0060, and the TUI binary of 0090 and 0100.
---

# 0110. Three binaries: `landline`, `landline-switchboard`, and `landline-mcp`

Builds on [0060](0060-agents-register-themselves-with-a-handle-and-attributes.md),
[0070](0070-delivery-is-a-push-to-a-connected-agent-with-no-inbox.md),
[0090](0090-observers-read-the-agent-list-and-the-tui-is-a-separate-binary.md), and
[0100](0100-a-build-names-itself-by-version-commit-and-date.md). It partly supersedes
[0010](0010-one-go-program-over-http.md), which chose one program. It partly supersedes
[0060](0060-agents-register-themselves-with-a-handle-and-attributes.md), where `landline server run` and `landline mcp`
are the foreground hub and the MCP process. It partly supersedes
[0090](0090-observers-read-the-agent-list-and-the-tui-is-a-separate-binary.md), which named the TUI `landline-tui` and
made `landline tui` a launcher, and [0100](0100-a-build-names-itself-by-version-commit-and-date.md), which says the TUI
binary is not stamped and names `landline-tui agents --json`. In each, only the command and binary names change.

## Context

`landline` is one binary with three jobs. It runs the hub, it is the MCP process that every harness session starts, and
it is the command line a person types. Since 0090 there is a fourth piece, `landline-tui`, that `landline tui` starts. A
person who wants to see their agents runs one binary to start a second, and a harness config names a binary that also
holds the hub. The TUI is a better place for setup and control than a list of commands, and the project means to grow it
into the place where a person sees and directs every agent.

The word "hub" names many unrelated tools. It does not say what this program does, which is connect one agent to another
and route the calls.

The project has no users outside its authors, so this record keeps no compatibility with the old names.

## Decision

**There are three binaries, each with one job.**

| Binary                 | Job                                                                                                |
| ---------------------- | -------------------------------------------------------------------------------------------------- |
| `landline`             | What a person runs: the TUI, setup, launchers, and the scripting commands                          |
| `landline-switchboard` | The program that holds the agent list and routes messages. Earlier records call it the hub         |
| `landline-mcp`         | The MCP process that a harness starts for one session. It registers the session and delivers to it |

**`landline-switchboard` is the old hub.** Run with no arguments it serves in the foreground, which is what launchd and
systemd run. It takes the flags the hub takes today. It has no third-party dependencies. Prose from this record on says
"the switchboard" for the program and "the secret" for its one credential. Records before this one keep "hub" as
history. The component id `hub/server` becomes `switchboard/server`, and the other ids keep their names.

**`landline-mcp` is the old `landline mcp`.** It takes the flags that `landline mcp` takes, such as `--channel`, and is
the only binary that a harness configuration names. It contains no switchboard code and no third-party dependencies. The
MCP server is still named `landline` in a harness, so the channel flag `server:landline` does not change. It still
registers its own build as [0100](0100-a-build-names-itself-by-version-commit-and-date.md) says.

**`landline` is the old `landline-tui` and the human-facing commands.** Run in a terminal with no arguments it opens the
TUI, and `--interval D` still sets the polling interval. When standard input or standard output is not a terminal it
prints the usage on standard error and exits `2`. `-h` and `--help` print the usage on standard output and exit `0`.

- Every command of today's `landline` that this record does not change keeps its name, flags, and exit codes and moves
  into `landline`. These are `setup`, `claude`, `register`, `send`, `log`, `join`, and `server up`, `down`, `status`,
  `restart`, `install`, `log`, and `join`. `help` prints the usage on standard output and exits `0`, as `-h` does.
  `agents` and `version` change as stated below. When standard input or standard output is not a terminal, a call that
  has flags and no command, such as `landline --interval 5s`, prints the usage on standard error and exits `2`.
- `landline hub`, `landline mcp`, `landline server run`, and `landline tui` no longer exist. The first three are the
  other two binaries. The last is `landline` itself.
- `landline setup claude|codex` writes the absolute path of `landline-mcp` into the harness, with the arguments that
  `landline mcp` had, and installs the `/landline` skill for Claude Code. It finds `landline-mcp` next to itself, then
  on the path.
- `landline agents` reads the agent list with an observer token, as
  [0090](0090-observers-read-the-agent-list-and-the-tui-is-a-separate-binary.md) sets, so it no longer takes `--as`. It
  uses the TUI module's own client and view model, so its rows come in the screen's order: connected first, then label,
  then handle. The text form is one tab-separated line per row with the handle, harness, host, state, and the attribute
  text of [0100](0100-a-build-names-itself-by-version-commit-and-date.md). That text is the root model's `Attributes()`
  output, passed through the same sanitising as today's line, and `tui/main.go` converts its rows to the root type to
  get it. `--json` prints the view model, as `landline-tui agents --json` did, and that command is gone. The `agents`
  case and its `--as` handling leave `cli/command`. The `landline` entry point handles `agents` itself and hands every
  other command to `cli/command`, which keeps using the root client for the commands that send, register, and read the
  log.
- `landline server up|restart` starts `landline-switchboard`, which it finds next to itself, then on the path, and
  passes it the flags it passes the hub today. `down`, `status`, and `restart` find the running switchboard by its pid
  file and by its command name. The launchd and systemd units run `landline-switchboard` directly. The launchd label is
  `dev.landline.switchboard`, the systemd unit is `landline-switchboard.service`, and the pid and output files in the
  data folder are `switchboard.pid` and `switchboard.out`. The switchboard's startup and listening lines say
  "switchboard" where they say "hub" today. An unknown command prints the usage on standard error and exits `2`.
- When `landline-switchboard` is missing, `server up`, `restart`, and `install` exit `1` with a message that names it
  and says `just install`. When `landline-mcp` is missing, `setup` does the same. The Nix store warning that `setup` and
  `server install` give today applies to the path of the binary they write, which is `landline-mcp` for `setup` and
  `landline-switchboard` for `server install`.
- `landline version` prints the banner and its build as [0100](0100-a-build-names-itself-by-version-commit-and-date.md)
  sets. `landline-mcp version` and `landline-switchboard version` print one line with their own name in place of
  `landline`, with no banner.

**The TUI becomes the surface for setup and control.** This record fixes the binaries and the command surface. Screens
for setup, joining another machine, server control, and sending are decided in later records. Until a screen exists, the
command that does the job stays.

**`landline` reuses the root module's code.** It is built in the `tui` module, which holds the terminal framework. That
module requires the root module with a `replace` to the folder above it, and its entry point, `tui/main.go`, imports the
packages that the commands need, including `internal` ones, which Go allows because the import paths share the root. The
screen packages under `tui/internal/`, which are the `agent/viewer` component, import no root package and keep their own
types. This corrects the statement in 0090 that one module cannot import the other's `internal` packages. The root
module and the other two binaries stay free of third-party dependencies. The flake builds `landline` from the whole
source with the `tui` folder as the module root. Its `vendorHash` covers third-party dependencies only, so it changes
when those change and not when the root's code changes. The flake sets `proxyVendor` for the build, which keeps this
true. A stale hash already fails the vendor build, so no extra check is needed. The stamp of
[0100](0100-a-build-names-itself-by-version-commit-and-date.md) reaches `landline` through the same `-X` flags as the
others, so 0100's statement that the TUI binary is not stamped no longer holds.

**The code layout follows the binaries.** The switchboard's entry point is `cmd/landline-switchboard` and the MCP
process's is `cmd/landline-mcp`, both in the root module. `landline` has its own entry point in the `tui` module.
`internal/hub/server` moves to `internal/switchboard/server`. The code of `binary/` that `setup`, `claude`, and
`server up|restart` need, and the flag parsing and pid file code that the switchboard shares with them, moves into
`cli/command`. No new component is added. The entry points are not components, so they may import any of them.

**The three are built, versioned, and installed together.** One `VERSION` and the stamp of
[0100](0100-a-build-names-itself-by-version-commit-and-date.md) apply to all three. `just build` builds all three into
`bin/` and `just install` installs all three. The flake has a package for each, and `default` is a bundle that holds all
three. The flake check of 0100 runs against the bundle's `bin/landline`, and the `landline-tui` package is removed. A
`landline-mcp` and a switchboard from different builds are not supported, as the `400` for unknown fields in 0100
already shows.

## Consequences

- A harness configuration holds one small binary, and a person runs one binary. The documents can say what each does in
  a sentence.
- A running installation breaks until its owner runs `landline setup claude|codex` and `landline server install` again
  and removes any old launchd or systemd unit by hand. The README and CHANGELOG say so. A `landline-tui` that is already
  installed is stale and the owner deletes it.
- `architecture.yaml` renames `hub/server` to `switchboard/server` and lists the entry points of three binaries. It is
  protected, so a person approves it. The `affects` line in the front matter of every record that names `hub/server`,
  this record included, changes to the new id in the same commit as the `architecture.yaml` change, because the records
  check fails on an id that does not exist. That is the only edit to those records apart from the `partly-superseded-by`
  lines in 0010 and 0090, and a person approves it.
- The acceptance suite builds three binaries and drives them by their new names. It is protected, so a person approves
  the changes. The `landline tui` scenarios become scenarios of `landline`, and the agent list scenarios use
  `landline agents --json` and drop `--as` for that command.
- `tui/go.mod` gains a `require` and a `replace`, and `docs/stack/` says the TUI module imports the root. The `tui`
  module has no third-party code in the root's graph, and a dependency scan covers each module.
- `landline agents` registers an observer with the secret, as the TUI does, so `landline` needs the secret on the
  machine where it runs.
- The nix flake gains packages for `landline-switchboard` and `landline-mcp` and a bundle, and `just install-tui` is
  removed because `just install` builds everything.
- Records before this one use "hub" and `landline mcp`. They are history, and this record says which names replaced
  them.

## Alternatives considered

- **Two binaries, the switchboard with the MCP process inside it.** Fewer files. Every harness configuration would then
  name the program that also holds the switchboard, and a session's MCP process would carry code it never runs.
- **Keep one program and make the TUI a subcommand.** It keeps the old layout. It puts the terminal framework in the
  binary that every harness session starts, and 0090 kept it out for that reason.
- **Keep `landline-hub`.** It needs no renaming. The word names many unrelated tools and says nothing about routing.
- **Forward the old commands for a release.** Existing configurations would keep working. The project has no outside
  users, so it would add a path to remove later for no one.
- **Copy the commands into the `tui` module.** The modules would not depend on each other. Two implementations of the
  same commands would have to match their flags and exit codes, and a test would have to pin them.
- **One module for everything.** One `go.mod` and one vendor hash. The root module would no longer be free of
  third-party dependencies, and the other two binaries would build against a vendor hash for code they never import.
- **Name the switchboard `landlined`.** It follows the daemon convention. It breaks the `landline-<role>` pattern of the
  other two names.
