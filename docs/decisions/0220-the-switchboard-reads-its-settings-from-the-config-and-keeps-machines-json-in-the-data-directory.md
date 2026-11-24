---
category: behavior
status: accepted
approval: person
approver: {kind: person, name: dnawara}
note: Drafted on 2026-11-23 from the person's choice of settings in config.toml over a file of the last run's flags. It adds a table to config.toml and moves machines.json, two stored formats, and needs the person's approval. Amended on 2026-11-24, so that `signature-window` is 5 seconds by default and from 1 to 5 seconds, as 0200 says.
affects: [cli/command, switchboard/server, agent/viewer, menubar/viewer]
builds-on: ['0040', '0110', '0140', '0170', '0190', '0200']
partly-supersedes: ['0190', '0200']
---

# 0220. The switchboard reads its settings from `config.toml` and keeps `machines.json` in the data directory

Builds on [0040](0040-hub-defaults-and-saved-state.md),
[0110](0110-three-binaries-landline-switchboard-and-landline-mcp.md),
[0140](0140-a-macos-menu-bar-item-shows-agents-runs-the-switchboard-and-sets-up-harnesses.md),
[0170](0170-a-person-rotates-the-switchboard-secret-with-one-command-or-one-menu-item.md),
[0190](0190-one-toml-config-names-the-switchboard-by-host-and-port.md), and
[0200](0200-a-machine-joins-with-an-invite-code-and-a-keypair-that-outlasts-secret-rotation.md). It replaces the reader
rule of 0190 that refuses every line other than a top-level key, and the rule of 0200 that puts `machines.json` next to
the log file. Follows C1 in [coding-principles.md](../reference/coding-principles.md), and P3, P5, and P8 in
[product-principles.md](../reference/product-principles.md).

## Context

`landline-switchboard` takes its settings as flags: `--addr`, `--log`, `--host`, and the timing flags `--keepalive`,
`--delivery-timeout`, `--grace`, `--invite-lifetime`, and `--signature-window`. Only the address also lives in
`~/.config/landline/config.toml`, as `host` and `port`
([0190](0190-one-toml-config-names-the-switchboard-by-host-and-port.md)). Every other setting a person chose lives only
on the command line of one process.

Several paths start the switchboard without that command line:

- `landline server restart` and `rotate-secret` stop the switchboard and start a new one with the flags given to them,
  and no others.
- The menu bar's `Start`, `Stop`, `Restart`, and `Rotate secret…` run `landline server up`, `down`, `restart`, and
  `rotate-secret` with no flags
  ([0140](0140-a-macos-menu-bar-item-shows-agents-runs-the-switchboard-and-sets-up-harnesses.md),
  [0170](0170-a-person-rotates-the-switchboard-secret-with-one-command-or-one-menu-item.md)).
- `landline server install` writes a launchd or systemd unit that runs the binary with no arguments
  (`internal/cli/command/service.go`), so the service ignores any `--log`.

The switchboard keeps `machines.json` next to its log file
([0200](0200-a-machine-joins-with-an-invite-code-and-a-keypair-that-outlasts-secret-rotation.md)), so `--log` also moves
the list of joined machines. The third review of 0200 found that rotating from the menu bar restarted a switchboard that
was started with `--log <custom>` on the default log folder. The new process read another `machines.json`, and every
joined machine was locked out. 0200 promises that a rotation keeps every machine's key working. The menu bar's `Stop`
then `Start` has the same gap, and so does the service.

The cause has two parts. Settings that a person chose lived only on one process's command line, and `machines.json`
followed the log path.

The interim build recovers the log path by parsing the `listening on` line that the switchboard writes to
`switchboard.out` (`runningLogPath` in `internal/cli/command/rotate_secret.go`). That line is written for people, and C1
says no program depends on it. It also recovers `--log` only, so the timing flags still return to their defaults.

## Decision

**The switchboard's settings live in a `[server]` table of `config.toml`, and `landline-switchboard` reads the table
itself.** Every path that starts it gets the same settings, and no command passes them on as flags.

### The `[server]` table

```toml
secret = "..."
host = "127.0.0.1"
port = 8787

[server]
log = "~/landline-logs/log.jsonl"
keepalive = "15s"
delivery-timeout = "20s"
grace = "2m"
invite-lifetime = "5m"
signature-window = "5s"
```

- The keys are the flag names without the leading `--`: `log`, `keepalive`, `delivery-timeout`, `grace`,
  `invite-lifetime`, and `signature-window`. One setting has one name in the flags and in the file.
- `log` is the path of the message log. It is an absolute path or starts with `~/`, which the reader replaces with the
  home directory. Any other value is an error.
- The timing keys are strings in the duration form the flags take, such as `200ms`, `15s`, or `5m`, and must be
  positive. Any other value is an error.
- `signature-window` is from 1 to 5 seconds, and 5 seconds by default, as
  [0200](0200-a-machine-joins-with-an-invite-code-and-a-keypair-that-outlasts-secret-rotation.md) says. A value below 1
  second or above 5 seconds is an error, in the table and in the flag.
- A missing table, or a table that omits a key, means today's default for that key: the log at
  `~/.local/share/landline/log.jsonl` and the switchboard's default for each timing. Nothing writes the table. A person
  edits it.
- The table never holds a secret, a token, or a key.

**The address stays where 0190 puts it.** `host` and `port` stay top-level keys, and the table has no address key.
Clients read `host` and `port` to reach the switchboard, and 0190 makes one address name both where it listens and where
clients connect. A second copy in `[server]` could disagree with it. `--addr` keeps the rule of 0190: it overrides the
saved address for one run, and `up` saves `host` and `port` once the switchboard listens.

**The `--host` flag stays a flag only**, as 0190 says. It is the label that the switchboard shows for its agents, and
`host` in `config.toml` already names the address, so a `[server]` key for the label would give one word two meanings. A
later record can give the label a key under another name.

### Who reads it

`landline-switchboard` reads `config.toml` when it starts, as it already does for `host`, `port`, and `secret`, and
takes each setting from the table. So `landline server up`, `restart`, and `rotate-secret`, the menu bar's `Start`,
`Restart`, and `Rotate secret…`, and the launchd and systemd units all start it with the same settings. `up`, `restart`,
and `rotate-secret` pass on only the flags a person gave them, and never a value they read from the table. The units
keep running the binary with no arguments.

A line in the table that the reader reports as an error stops every one of these before it changes anything. The binary,
`up`, `restart`, and `rotate-secret` exit `1` with the error of 0190, which names the file and the line number and never
repeats the line. `rotate-secret` reads the file before it stops the switchboard, as it does today.

### A flag wins for one run

A flag given to `up`, `restart`, `rotate-secret`, or the binary overrides the table for that run only. No command writes
a flag's value to the table. `--addr` is the one flag whose value a command saves, under the rule of 0190 above.

When a flag's value differs from the table's value, or from the default when the table has no value, the command prints
one line for each such flag. The line names the flag and `config.toml`, and says that `landline server restart`,
`landline server rotate-secret`, and the menu bar will use the value in `config.toml`, and that a person sets the key in
`[server]` to keep the value. Two values differ when they mean different things: `5m` and `300s` are equal, and a `log`
path is compared after `~/` is replaced and a relative flag path is made absolute. A `--host` that differs from the
machine's host name gets the same line, which says those paths start the switchboard without it.

`up`, `restart`, and `rotate-secret` print the line on standard output, before their own result line. The binary prints
it on standard error, so it shows in a foreground run and in `switchboard.out`. Each works the line out from its own
flags and the file. The line is text for people, and by C1 no program reads it.

### Where `machines.json` lives

**`machines.json` lives at `~/.local/share/landline/machines.json`, in the data directory, whatever the log path.**
Moving the log never moves the list of joined machines. The file keeps the format, the mode `0600`, and the write
through a temporary file that 0200 gives it. Only its location changes.

The default log is in the same directory, so a switchboard that never had a custom log already keeps `machines.json` at
this path.

### Carrying over an old `machines.json`

A switchboard that ran with a custom log before this change has its machines in a `machines.json` next to that log. When
the switchboard starts, it applies this rule before it listens:

1. When `~/.local/share/landline/machines.json` exists, the switchboard reads it, and no other.
1. When it does not exist and a `machines.json` exists in the folder of the log path in use for this run, from the flag,
   the table, or the default, the switchboard reads that file and writes its entries to
   `~/.local/share/landline/machines.json`. It prints one line that names both files and says that it now reads the new
   one. If the write fails, the switchboard exits `1` with a message that names the new file and the error, and writes
   nothing else.
1. When neither exists, the switchboard starts with no machines, as it does today.
1. The switchboard never deletes, renames, or changes the old file.

When both files exist and the old one is in another folder, the new one wins, and the switchboard prints at each start
one line that names the old file, says that it is no longer read, and says that a person may delete it. A person who
used two custom logs, each with its own `machines.json`, copies the one to keep to the new path before the first start.
A file that cannot be read stops the start with today's error.

The switchboard writes the new file at start, and not at its next save, so that the next restart from the menu bar finds
the machines whatever log it uses.

### Rotation

`landline server rotate-secret` writes the new secret to `config.toml`, as it does today, and starts the switchboard
with only the flags given to it. The new switchboard reads its settings from the table and its machines from the fixed
path, so a rotation keeps every joined machine whatever log the old switchboard used. `runningLogPath` and its markers
are removed. No program reads the `listening on` line for a setting.

`up` also waits for the text `listening on` in `switchboard.out` to learn that the switchboard listens. That is the same
kind of dependency, and it is out of scope here. It needs its own record.

`landline-switchboard` exits with status `3` when it cannot listen on its address, and with status `1` for every other
failure to start. `up` gives the advice about the saved address when the switchboard exits with status `3`, and never
decides it from the text in `switchboard.out`, by C1. The status is a format between programs, and a change to it needs
a new record.

### The reader and writer of `config.toml`

`config.toml` stays a stored format, and this record changes it. The hand-written reader of 0190, in each of its copies
in the root module, the `tui/` module, and the `menubar/` module, learns tables:

- A line that holds only `[name]`, where `name` is a bare key, opens a table. Every key line after it belongs to that
  table until the next such line. Top-level keys are the ones before the first table line.
- A reader takes the keys it knows from the tables it knows and ignores the rest, as it ignores an unknown top-level key
  today. The screen and the menu bar know no table, so they skip every key of `[server]`. A key in a table is never read
  as a top-level key.
- A second `[server]` line, or a key given twice in `[server]`, is an error. Any other line that is not blank, a `#`
  comment, a key line, or a table line stays an error.

The writer keeps every line it does not change, including the `[server]` table and the comments in it, as 0190 requires.
When it adds a missing top-level key such as `secret`, `host`, `port`, or `machine`, it inserts the key before the first
table line, so that the key stays top-level. It writes the file with mode `0600`, because the file holds the secret, and
renames it over the old one, as it does today.

A change to the names, values, or meaning of the `[server]` table, or to the location of `machines.json`, needs a new
record.

## Consequences

- `cli/command` changes its config reader and writer, `up`, `restart`, `rotate-secret`, the flag line, and the usage
  text, and loses `runningLogPath`. `switchboard/server` reads `machines.json` from the fixed path and carries over an
  old file, and `cmd/landline-switchboard` passes it both paths and reads the table. `agent/viewer` and `menubar/viewer`
  change their copies of the reader so that a `[server]` table is not an error. The menu bar runs the same commands as
  before. The contracts and acceptance scenarios of these components change, and a person authorizes those edits.
- `docs/architecture/components.md` says where `machines.json` lives and that the switchboard reads `config.toml`. The
  README sections on restart, rotation, and the service, and the command table, say that the switchboard takes its
  settings from `[server]` and that a flag lasts one run. The README text that says `rotate-secret` keeps the log of the
  switchboard it stops is removed.
- The menu bar's `Stop` then `Start`, `Restart`, `Rotate secret…`, and the launchd and systemd units all keep a custom
  log and the timings, and a rotation from any of them keeps every joined machine, as 0200 promises. The sentence of
  0140 that `Restart` does not remember the old flags stays true for flags, and a person who wants a setting in the menu
  bar puts it in `[server]`.
- An acceptance test or another caller that isolated a switchboard with `--log` alone now shares the real
  `~/.local/share/landline/machines.json`. It isolates with a throwaway `HOME`, or with a throwaway `config.toml` and
  data directory, as the test author decides. The tests of the reader gain table cases, and the shared fixture of
  accepted and rejected files that 0190 suggests gains them for all three copies.
- A switchboard that an older build started with a custom `--log`, and that is still running, keeps its machines next to
  that log. A rotation or a menu bar restart from the new build then starts on the table's log, finds no
  `machines.json`, and locks the machines out until the person starts the switchboard once with that `--log` or sets
  `log` in `[server]`. The old file is never changed, so nothing is lost.
- A setting in `[server]` lasts across restarts, where a flag lasted one run. A `signature-window` stays as set until a
  person edits the file, and 0200 bounds it at 5 seconds.
- C1 cites this record.

## Alternatives considered

- **Save the last run's flags in a state file beside `switchboard.pid`.** This was the earlier draft. The flags are a
  setting, not state. `down` removed the file, so the menu bar's `Stop` then `Start` still lost the settings. The
  launchd and systemd units do not run `up` and never read it. It also needed a refusal for a running switchboard with
  no file.
- **Parse the `listening on` line in `switchboard.out`.** The interim build does this. The line is written for people,
  and C1 rules it out. A reworded line would bring the lockout back, and it recovers only the log path.
- **Ask the running switchboard for its settings over a route.** `rotate-secret` and the menu bar's `Start` must work
  when the switchboard is down, and a route is a network change that needs its own record.
- **`up` writes its flags into `config.toml`.** It is surprising, and it changes a person's file without being asked,
  which P8 rules out. A flag given to try a value once would become permanent.
- **Refuse a flag that differs from the table unless `--save` or `--once` is given.** The person chose a printed line
  instead, which lets a one-off run go ahead and still says what the next restart will do.
- **Keep `machines.json` next to the log and carry the log path in the table only.** A flag that moves the log for one
  run would still move the machines for that run, and the service and the menu bar would see another list.
- **Move the old `machines.json` instead of copying it.** It loses the file if the write half succeeds, and it changes a
  file the switchboard no longer owns. A copy leaves the person to delete the old file.
