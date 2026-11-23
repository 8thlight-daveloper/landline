# config.toml

The exact format of `~/.config/landline/config.toml`. The file names the switchboard, holds this machine's credential,
and holds the switchboard's settings. The rules come from
[0190](../decisions/0190-one-toml-config-names-the-switchboard-by-host-and-port.md),
[0200](../decisions/0200-a-machine-joins-with-an-invite-code-and-a-keypair-that-outlasts-secret-rotation.md), and
[0220](../decisions/0220-the-switchboard-reads-its-settings-from-the-config-and-keeps-machines-json-in-the-data-directory.md).
A change to the names, values, or meaning of a key, or to the line forms below, needs a new record (0200, 0220).

## A full file

```toml
# Written by landline server up. Comments survive every save.
secret = "..."
host = "alpha-host"
port = 8787

[server]
log = "~/landline-logs/log.jsonl"
keepalive = "15s"
delivery-timeout = "20s"
grace = "2m"
invite-lifetime = "5m"
signature-window = "60s"
```

## Lines

The reader accepts a subset of TOML (0190, 0220). Each line, after leading and trailing spaces are dropped, is one of:

| Line          | Form                                                              | Example                |
| ------------- | ----------------------------------------------------------------- | ---------------------- |
| Blank         | nothing                                                           |                        |
| Comment       | starts with `#`                                                   | `# hosts on a tailnet` |
| Text key      | `key = "text"`, with no `"` and no `\` inside the quotes          | `host = "alpha-host"`  |
| Number key    | `key = 123`, a whole number                                       | `port = 8787`          |
| Table heading | `[name]`, where `name` is a bare key of letters, digits, `-`, `_` | `[server]`             |

- A comment takes a whole line. A `#` after a value makes the line an error (0190).
- Keys before the first table heading are top-level. Every key after a heading belongs to that table until the next
  heading. A key in a table is never read as a top-level key (0220).
- A reader ignores a key it does not know, at the top level or in a table, and every key of a table it does not know
  (0190, 0220). The line must still have one of the forms above.

## Top-level keys

| Key       | Value                                                                                              | Default                      | Record |
| --------- | -------------------------------------------------------------------------------------------------- | ---------------------------- | ------ |
| `secret`  | Text. The switchboard's shared secret                                                              | none                         | 0190   |
| `host`    | Text. A name or an IP address, with no scheme, port, or brackets                                   | `127.0.0.1`                  | 0190   |
| `port`    | A whole number from 1 to 65535                                                                     | `LANDLINE_PORT`, else `8787` | 0190   |
| `machine` | Text. This machine's name: 1 to 32 lower case letters, digits, and hyphens, starting with a letter | none                         | 0200   |

- A client reaches the switchboard at `http://` plus `host:port`, with brackets around an IPv6 host. Every client speaks
  `http` (0190).
- One address names both where the switchboard listens and where clients connect. No key names a role (0190).
- A missing file, or a file that omits `host` or `port`, means the default. A missing secret is an error only when a
  program needs it and has no other credential (0190, 0200).
- A machine that joined with an invite code has `machine` and no `secret`, so it holds one credential, its `machine.key`
  (0200).

## The `[server]` table

Only `landline-switchboard` takes its settings from the table. The keys are the switchboard's flag names without the
leading `--` (0220).

| Key                | Value                                                     | Default                             |
| ------------------ | --------------------------------------------------------- | ----------------------------------- |
| `log`              | Text. An absolute path, or a path that starts with `~/`   | `~/.local/share/landline/log.jsonl` |
| `keepalive`        | Text. A positive duration such as `200ms`, `15s`, or `5m` | `15s`                               |
| `delivery-timeout` | Text. A positive duration                                 | `20s`                               |
| `grace`            | Text. A positive duration                                 | `2m`                                |
| `invite-lifetime`  | Text. A positive duration                                 | `5m`                                |
| `signature-window` | Text. A positive duration                                 | `60s`                               |

- The reader replaces a leading `~/` in `log` with the home directory (0220).
- A missing table, or a table that omits a key, means the default for that key (0220).
- The table has no address key. The address stays in `host` and `port` (0220).
- The table never holds a secret, a token, or a key (0220).
- Nothing writes the table. A person edits it (0220).
- `machines.json` stays at `~/.local/share/landline/machines.json` whatever `log` says (0220).

## Errors

Each of these is an error. The message names the file and the line number and never repeats the line (0190, 0220).

- A line that is not blank, a comment, a key line, or a table heading.
- A known key with the wrong type, such as `port = "8787"` or `host = 5` (0190).
- A `port` outside 1 to 65535.
- A `log` that is neither absolute nor starts with `~/`, or a timing that is not a positive duration (0220).
- A second `[server]` heading, or a key given twice in `[server]` (0220).

An error stops the program before it changes anything. `landline-switchboard`, `landline server up`, `restart`, and
`rotate-secret` exit `1`, and `rotate-secret` reads the file before it stops the switchboard (0220).

Rejected lines:

| Line                       | Why                                           |
| -------------------------- | --------------------------------------------- |
| `port = "8787"`            | A number in quotes                            |
| `host = 5`                 | A host that is not text                       |
| `machine = "bravo" # note` | A comment after a value                       |
| `secret = '...'`           | Single quotes, a TOML form outside the subset |
| `[server.timing]`          | A dotted table name                           |
| `[server]`, a second time  | A second `[server]` heading                   |

## Who reads which keys

| Program                                 | Reads                                                                                                                                      |
| --------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------ |
| `landline-switchboard`                  | `secret`, `host`, `port`, and `[server]` (0190, 0220)                                                                                      |
| `landline` commands                     | `secret`, `host`, `port`, `machine`. `up`, `restart`, and `rotate-secret` also read `[server]` to compare it with their flags (0200, 0220) |
| `landline-mcp`                          | `secret`, `host`, `port`, `machine` (0190, 0200)                                                                                           |
| The screen (`landline` with no command) | `secret`, `host`, `port`, `machine`. It skips `[server]` (0190, 0200, 0220)                                                                |
| The menu bar (`landline-menubar`)       | `secret`, `host`, `port`, `machine`. It skips `[server]` (0190, 0200, 0220)                                                                |

The screen and the menu bar keep their own copies of the reader, and every copy follows this format (0190, 0220).

## Who writes which keys

| Command                                     | Writes                                                                                                        |
| ------------------------------------------- | ------------------------------------------------------------------------------------------------------------- |
| `landline server up`                        | `secret` when there is none. `host` and `port`, as the person gave them, after the switchboard listens (0190) |
| `landline server restart`                   | `host` and `port` from `--addr`, after the switchboard listens (0190)                                         |
| `landline server rotate-secret`             | `secret` (0170, 0220)                                                                                         |
| `landline server join <host:port>`          | `host`, `port`, and `secret` from `LANDLINE_SECRET` or standard input (0190)                                  |
| `landline server join <host:port> --code C` | `host`, `port`, and `machine`, and removes `secret` (0200)                                                    |

- The writer changes the lines of the keys it saves and keeps every other line, including comments and the `[server]`
  table (0190, 0220).
- It inserts a missing top-level key before the first table heading, so the key stays top-level (0220).
- It writes a new file with mode `0600`, because the file holds the secret, and renames it over the old one (0190,
  0220).
- A failed bind saves no address. A failed join changes no file (0190, 0200).
- No command writes a flag's value to `[server]` (0220).

## Overrides

- A flag given to `landline-switchboard`, `up`, `restart`, or `rotate-secret` overrides `[server]` for that run only.
  When it differs from the table's value, or from the default, the command prints one line that says so and names
  `config.toml` (0220).
- `--addr host:port` overrides `host` and `port` for `up` and `restart`, and is the one flag whose value a command saves
  (0190, 0220).
- `LANDLINE_URL` overrides `host` and `port`, and `LANDLINE_SECRET` overrides `secret`, for every client (0190).
- `LANDLINE_PORT` sets the default port when the file has no `port` (0190).
- A client sends the first credential it finds in this order: `LANDLINE_SECRET`, then `machine.key`, then `secret`.
  `invite`, `machines`, and `revoke` send only the shared secret, from `LANDLINE_SECRET` or `secret` (0200).

## The old `config.json`

When `config.toml` is missing and `config.json` exists, the reader takes `secret` from it and reads its `url` as `join`
reads an address. An `https` URL is an error. The first save writes `config.toml` and removes `config.json`. When both
exist, `config.toml` wins (0190).

## Examples

A machine that hosts on localhost, after `landline server up`:

```toml
secret = "..."
host = "127.0.0.1"
port = 8787
```

A machine that hosts on a tailnet, with a custom log and a shorter invite lifetime:

```toml
# Reached by the other machines over Tailscale.
secret = "..."
host = "alpha-host"
port = 8787

[server]
log = "~/landline-logs/log.jsonl"
invite-lifetime = "2m"
```

A second machine after `landline server join alpha-host:8787 --code K7M2-9QXA`. Its credential is
`~/.config/landline/machine.key`, so the file has no `secret`:

```toml
host = "alpha-host"
port = 8787
machine = "bravo"
```

A second machine that joined with the secret instead, after `landline server join alpha-host:8787`:

```toml
secret = "..."
host = "alpha-host"
port = 8787
```
