---
category: architecture
status: accepted
approval: person
approver: {kind: person, name: dnawara}
affects: [cli/command, agent/viewer, menubar/viewer]
builds-on: ['0040', '0140', '0170']
partly-superseded-by: ['0220']
note: Drafted on 2026-11-18. It replaces config.json with config.toml, a stored format, and removes the rule that a non-loopback address means this machine joined another switchboard.
---

# 0190. One TOML config names the switchboard by host and port

Builds on [0040](0040-hub-defaults-and-saved-state.md),
[0140](0140-a-macos-menu-bar-item-shows-agents-runs-the-switchboard-and-sets-up-harnesses.md), and
[0170](0170-a-person-rotates-the-switchboard-secret-with-one-command-or-one-menu-item.md).

## Context

`~/.config/landline/config.json` holds `url` and `secret`. The `url` serves two roles. `landline server up` writes it so
local clients find the switchboard it started, and `landline server join` writes it to point at a switchboard on another
machine. The file does not say which role applies, so `ensureLocalConfig` guesses from the address: loopback means this
machine hosts, anything else means it joined.

The guess is wrong for a person who hosts on a private network address. A switchboard started with
`--addr 100.114.49.17:8787` on a machine that owns that address saves `http://100.114.49.17:8787`. The next
`landline server up` then refuses with "this machine joined the switchboard", and the person must edit the file by hand.
The menu bar and `rotate-secret` use the same loopback test, so they treat that switchboard as remote and disable
themselves.

The file is also edited by hand, and JSON has no comments.

## Decision

**`~/.config/landline/config.toml` replaces `config.json`.** It has mode `0600` and these top-level keys:

```toml
secret = "..."
host = "alpha-host"
port = 8787
```

- `host` is a name or an IP address. `port` is an integer. Neither holds a scheme. Every client speaks `http`, and a
  client reaches the switchboard at `http://` plus `host:port`, with brackets around an IPv6 host.
- Without a file, or with a file that omits a key, `host` is `127.0.0.1` and `port` is `8787`, or the port in
  `LANDLINE_PORT`. Every reader applies this: the `landline` commands, `landline-mcp`, the screen, and the menu bar. A
  missing file means `landline server up` has not run here, because `up` makes the file and its secret. A person who
  runs the switchboard in the foreground with `LANDLINE_SECRET` set needs no file, and every client then reaches it at
  the default address. A missing secret stays an error with today's message. This replaces the error that `landline log`
  and the other commands give today when only the address is missing.
- `LANDLINE_URL` and `LANDLINE_SECRET` still override the file for every client.
- The reader accepts only blank lines, `#` comments, and top-level `key = "string"` or `key = integer` lines, which is a
  subset of TOML. It ignores a key it does not know. It reports any other line as an error that names the file and the
  line number and never repeats the line. A known key with the wrong type, such as `port = "8787"` or `host = 5`, is the
  same error.
- The writer changes the `secret`, `host`, and `port` lines in place and appends a missing one. It keeps every other
  line, so a person's comments survive `rotate-secret` and `server up`. It writes a new file with mode `0600` and
  renames it over the old one, as it does today.
- The root module takes no third-party dependency for this. Each of the three modules that read the file keeps its own
  copy of the reader, as they keep their own copies of the switchboard's types.

**One address names both where the switchboard listens and where clients connect, and no field names a role.**

- `landline server up` binds `host:port`. When the machine does not own the address, or the port is taken, the bind
  fails. The command then prints `cannot listen on <host>:<port>: <error>`, exits `1`, and saves nothing except a secret
  it had to make. A machine that joined another switchboard gets this message, and the message adds that `config.toml`
  names a switchboard elsewhere, so a person can edit it to host here.
- `--addr host:port` overrides the saved address for `up` and `restart`. `up` saves `host` and `port` after the
  switchboard reports `listening on`, and not before, so a failed bind leaves them unchanged.
- `up` saves `host` as the person gave it, a name or an IP, and not the address it resolved to. The menu bar shows the
  address as given. The switchboard's `listening on` line shows the host and port as given, the address it bound, and
  every other address the name resolves to. A name that resolves to several addresses binds the first one the resolver
  returns, and the line says which that is.
- The default stays `127.0.0.1`. A switchboard listens on a network address only when a person sets `host`, and the
  warning that it speaks plain HTTP stays.
- `landline server join <address>` takes `host:port` or `http://host:port`. It refuses `https://` and says Landline
  speaks `http` only. It saves `host`, `port`, and the secret from `LANDLINE_SECRET` or standard input.

**One test, "this machine's address", guards the commands that change something before they can fail.** An address is
this machine's when its host is `localhost`, is a loopback IP, or resolves to an address of one of the machine's network
interfaces. A host that does not resolve is not this machine's.

- `landline server rotate-secret` refuses when the saved address is not this machine's. This replaces the loopback test
  in 0170. The reason is that rotation stops the switchboard and replaces the secret before it starts one, so it must
  not begin on a machine that cannot host.
- The menu bar enables `Start`, `Stop`, `Restart`, and `Rotate secret…` when the address is this machine's, and shows
  `Remote switchboard at <address>` with those items disabled when it is not. This replaces the loopback test in 0140
  and 0170.
- `landline server up` does not use the test. The bind decides.

**An existing `config.json` carries over.** When `config.toml` is missing and `config.json` exists, the reader takes
`secret` from it and reads `url` as `join` reads an address. The first write saves `config.toml` and then removes
`config.json`. A `url` with an `https` scheme is reported as an error. When both files exist, `config.toml` wins.

**The `--host` flag is unchanged.** On the switchboard it is the label shown for its agents, and on `landline register`
it is the agent's machine. Neither is a config key.

## Consequences

- `cli/command` changes its config reader and writer, `up`, `join`, `rotate-secret`, and the usage text. `agent/viewer`
  and `menubar/viewer` change their address and secret discovery, and the menu bar gains the local address test. Their
  contracts and acceptance scenarios change, and a person authorizes those edits. An agent does not.
- The menu text reference gains the `Rotate secret…` line that 0170 defines, after `Restart`.
  `landline-menubar menu --text` prints it, enabled and disabled as `Restart` is.
- The README sections on two hosts and rotation, `docs/reference/menubar-menu-text.md`, and the component descriptions
  in `docs/architecture/components.md` name `config.json` and the loopback rule, and change with the code.
- A person can host on a Tailscale address and keep `landline server up`, `restart`, and the menu bar working. The same
  file can point a second machine at it with `join`.
- A person who deletes `config.toml` by hand loses the secret, as with `config.json` today.
- A machine whose `host` is `127.0.0.1` through an SSH tunnel starts a local switchboard if it runs `up` while the
  tunnel is down. This is true today and does not change.
- Three copies of a small reader need three sets of tests. A shared fixture of accepted and rejected files would keep
  them equal.

## Alternatives considered

- **Keep JSON.** It needs no new reader. It has no comments, and the writer would still rewrite the whole file.
- **A TOML library in the root module.** It handles all of TOML. The root module has no third-party dependency today,
  and the flake builds it with `vendorHash = null`. Adding one changes the build for two binaries to read four keys.
  Revisit when the file needs tables or arrays, such as the `peers` of a cluster.
- **YAML.** Unquoted values change type, such as `no`, and indentation carries meaning. The file does not need either.
- **A `serve = true` key, or separate `listen` and `url` keys.** Both record a role the bind already decides. A wrong
  `host` fails at startup, so the key would add a way for the file to disagree with the machine.
- **Keep the loopback test for `up`.** It is the bug this record removes.
- **A `scheme` key.** There is one scheme. Traffic between machines on a tailnet is already encrypted by the tunnel, and
  a person with another network should use an SSH tunnel, as the README says. HTTPS needs certificates, and a later
  record can add a `tls` key without breaking this file.
- **Clustering.** Several switchboards relaying for each other need their own record. It would add a `peers` key, which
  a reader that ignores unknown keys already tolerates.
