---
category: behavior
status: accepted
approval: person
approver: {kind: person, name: dnawara}
affects: [machine/registry, cli/command, switchboard/server, client/http, mcp/server, agent/viewer, menubar/viewer]
builds-on: ['0040', '0090', '0170', '0190']
note: Drafted on 2026-11-19 and amended the same day after the acceptance review. It adds a credential kind, a stored format on both machines, a route that takes no secret, and a core component. It has security consequences and needs a person's approval of the amendment.
---

# 0200. A machine joins with an invite code and a keypair that outlasts secret rotation

Builds on [0040](0040-hub-defaults-and-saved-state.md),
[0090](0090-observers-read-the-agent-list-and-the-tui-is-a-separate-binary.md),
[0170](0170-a-person-rotates-the-switchboard-secret-with-one-command-or-one-menu-item.md), and
[0190](0190-one-toml-config-names-the-switchboard-by-host-and-port.md).

## Context

One shared secret opens the register, observer, log, and build routes. A second machine joins by receiving that secret
out of band and saving it with `landline server join`. A person has to copy it by hand, and a copy sits in shell
history, a clipboard, or a chat.

`landline server rotate-secret`
([0170](0170-a-person-rotates-the-switchboard-secret-with-one-command-or-one-menu-item.md)) replaces the secret and ends
every agent token. Every machine that joined then holds a dead secret and must be joined again by hand. A secret that is
rotated because one machine leaked it also locks out the machines that did not.

## Decision

**A machine proves who it is with an Ed25519 key that never leaves it.** The switchboard keeps only the public half.
Rotating the shared secret does not touch a machine's key. The key is valid until a person revokes it.

### One resource, `machine`

A machine is pending after an invite and active after its code is redeemed. The routes are:

| Command                   | Route                                                                  | Credential          |
| ------------------------- | ---------------------------------------------------------------------- | ------------------- |
| `invite --name N`         | `POST /v1/machines` with `{"name"}`, answers `{"name", "code"}`        | the shared secret   |
| `join <host:port> --code` | `PATCH /v1/machines` with `{"code", "public_key"}`, answers `{"name"}` | the code, no header |
| `machines`                | `GET /v1/machines`                                                     | the shared secret   |
| `revoke N`                | `DELETE /v1/machines/N`, `404` for an unknown name                     | the shared secret   |
| `revoke --all`            | `DELETE /v1/machines`                                                  | the shared secret   |

The four routes that take the shared secret take it as a bearer token and nothing else. A machine signature on one of
them gets `401` or `403`. `GET /v1/machines` lists active machines only.

### Joining

1. On the switchboard's machine, `landline server invite --name <name>` asks the switchboard for an invite code and
   prints one line, the command to run on the other machine: `landline server join <host:port> --code <code>`. `<name>`
   is 1 to 32 lower case letters, digits, and hyphens, and starts with a letter.
1. A name is held until it is revoked, whether the machine is pending or active. `POST /v1/machines` for a held name
   answers `409`, and the command says to run `landline server revoke <name>`. `revoke <name>` also deletes a pending
   machine, and `revoke --all` also clears the pending ones. A pending machine whose code expires frees its name.
1. A code is eight characters from `23456789ABCDEFGHJKMNPQRSTUVWXYZ`, printed as two groups of four with a hyphen. A
   reader ignores the hyphen and the case. It is single use and valid for 5 minutes.
1. On the new machine, `landline server join <host:port> --code <code>` makes an Ed25519 key, sends the code and the
   public key, and saves the key, the address, and `machine = "<name>"` from the reply when the switchboard accepts. It
   removes the `secret` line from `config.toml`, so that the machine holds one credential. A join that fails changes no
   file, and a working key and `config.toml` stay as they were.
1. The switchboard keeps the invites in memory. A restart cancels every invite that is waiting.

`PATCH /v1/machines` takes no `Authorization` header. The public key is the standard base64 of the 32 key bytes.

- Every request that does not name a valid pending code gets `401` and the message `invalid or expired invite code`.
  That covers a missing code, `null`, an empty or non-text code, a body that is not JSON, a code of the wrong length or
  with a character outside the alphabet, and a wrong, used, unknown, or expired code. The switchboard says nothing that
  tells those cases apart. `landline server join` prints the switchboard's message when it is refused, exits non-zero,
  and says nothing else about why.
- A valid code with a missing or malformed public key gets `400`. It does not use up the code and it does not count as a
  failed redeem.
- Every `401` counts as a failed redeem. After 5 failed redeems since the last good redeem or the last cancel, the
  switchboard cancels every waiting invite, and the count starts again at 0.

### What the switchboard stores

The switchboard writes `machines.json` next to its log file with mode `0600`, through a temporary file renamed over the
old one, and rewrites it with the same mode after a revoke. It reads the file when it starts. An entry holds `name`,
`public_key`, and `created` as an RFC 3339 UTC time. It never stores or logs a code.

### What the new machine stores

`~/.config/landline/machine.key` holds one line, the standard base64 of the 32-byte seed, with mode `0600`, written the
way `config.toml` is. The reader takes the first line and ignores anything after it. A first line that is not a 32-byte
seed is an error that names the file and never repeats its content. `join --code` replaces an existing key file when it
succeeds. The old switchboard keeps its stale entry until a person revokes it. `config.toml` gains the key `machine`,
which the reader of 0190 already ignores when it does not know it.

### Proving it on a request

Every route that the shared secret opens today also accepts a machine signature: `POST /v1/register`,
`POST /v1/observers`, `GET /v1/log`, and `GET /v1/build`. The request carries:

```
Authorization: Landline-Machine name=<name>, time=<unix seconds>, nonce=<base64>, signature=<base64>
```

The nonce is 16 random bytes in standard base64, new for each request. The signature covers these lines, joined by line
feeds: `landline-machine-v1`, the name, the time, the nonce, the HTTP method, the path with its query, and the lower
case hex SHA-256 of the body.

The switchboard accepts a request when the machine is active, the time is within the signature window of its own clock,
the time is not earlier than the second in which the switchboard started, the signature verifies, and it has not seen
that signature while it could still pass the window check. The window is 60 seconds. A time in the second of the start
passes, because a time has whole seconds and a legitimate request in the first second after a restart must work. The
switchboard remembers a signature until one second after the time in it plus the window, which is the last moment it
could pass the window check, because a time has whole seconds. A signature is therefore never forgotten while a copy of
it could still be accepted. The start rule stops a copy from being replayed after a restart empties the cache. A time
outside the window gets `401` and a message that names the difference in seconds and tells the person to set the clock.
Two requests that differ only in their nonce are both accepted, and an exact copy of one is refused.

After a request registers, the agent or observer token works as it does today, and the switchboard records which machine
it came from.

A machine key opens no route beyond those four.

### Which credential a client sends

The order is `LANDLINE_SECRET`, then `machine.key`, then the `secret` in `config.toml`. The key comes before the file's
secret, so a stale secret left in the file after a rotation cannot override a working key. Every client that registers
or reads follows it: the `landline` commands, `landline-mcp`, the screen, and the menu bar.

`invite`, `machines`, and `revoke` are administration commands. They send the shared secret from `LANDLINE_SECRET` or
`config.toml` and never sign. Run on a machine that holds only a key, they fail with the message for no credential.

A client with no credential prints one message, whichever command it was running:
`there is no switchboard credential. Run landline server join <host:port> --code <code>, or set LANDLINE_SECRET`. A
machine with a key has a credential.

### Managing machines

- `landline server machines` prints one line for each active machine: the name, the first 12 hex characters of the
  SHA-256 of its 32 public key bytes, and the created date as `YYYY-MM-DD` in UTC. It never prints a key.
- `landline server revoke <name>` deletes the machine and ends at once every observer token and agent token that came
  through it, with their open streams. It exits `1` when the name is unknown.
- `landline server revoke --all` does the same for every machine.
- **An administration command refuses when the saved host is not this machine's address.** That covers `rotate-secret`,
  `invite`, `machines`, and both forms of `revoke`, and a later record that adds an administration command keeps the
  rule. The menu bar adds no item.

### Rotation

`landline server rotate-secret` is unchanged, and so is its output line. It replaces the shared secret and ends every
agent token. A machine keeps its key, and its `landline-mcp` and screen register again on their own with the next `401`,
so the person does not join the machine again. A person who wants to cut machines off as well runs
`landline server revoke --all`.

### Time and the component

The rules of invites, machines, failed redeems, and signatures live in a core component, `machine/registry`. Its
functions take the current time, the second the switchboard started, the durations, and any random bytes as arguments
and return new values, as `agent/registry` does. They read no clock. `switchboard/server` reads its clock once for each
request, through the `Now` function of its config, and passes the value in. Nothing in the machine code calls
`time.Now`. The component's contract tests set the time exactly. `machines.json` is read and written by a driver.

The switchboard takes two timing flags, like `--grace`: `--invite-lifetime D`, 5 minutes by default, and
`--signature-window D`, 60 seconds by default. `landline server up`, `restart`, and `rotate-secret` pass them on. They
let an acceptance test run the real binary with short times.

### What crosses the network

A machine's private key and the shared secret do not cross the network when a machine joins or registers. The code and
the public key cross once. Agent tokens, observer tokens, message bodies, and signed requests still cross in plain
`http`, as they do today, and the switchboard keeps its warning for a non-loopback address. On a tailnet, the transport
is encrypted by Tailscale. Off a tailnet, anyone on the path can read a token and use it until it ends, and a token
stays alive while its holder keeps calling. This record does not make that use safe. A later record that adds TLS does,
and Landline is not used off a tailnet before it.

## Consequences

- `machine/registry` is new, at position 1 with no dependencies. `switchboard/server` depends on it, gains the routes of
  the table, the signature check, the `machines.json` store, a record of the machine behind each token, and the two
  timing flags. `client/http` gains a signer and the redeem call. `cli/command` gains `invite`, `machines`, `revoke`,
  and `join --code`, and one message for no credential. `mcp/server`, `agent/viewer`, and `menubar/viewer` sign their
  registrations. The `tui/` and `menubar/` modules keep their own copies of the signer and of the key reader, as 0190
  says for the config reader.
- The domain model gains the types of the component, and `docs/architecture/components.md` names the component. The
  contracts and acceptance scenarios of these components change. A person authorizes those edits, and an agent does not
  make them. The README and the command table name the key file and the new commands.
- `machine.key`, `machines.json`, and the `machine` key of `config.toml` are stored formats. A change to any of them
  needs a new record.
- A machine that loses its key file or its config directory joins again with a new invite. Its old name needs a revoke
  first.
- The standard library covers the work: `crypto/ed25519`, `crypto/sha256`, and `crypto/rand`. No module gains a
  dependency.
- Tests of the command run with a throwaway config directory and a throwaway data directory, so that no test touches a
  real key or a real machine list.

## Alternatives considered

- **Bind the credential to the machine's source IP.** The switchboard sees only an IP address, and an `ssh -L` tunnel
  makes every remote client look like loopback. Outside a tailnet, NAT and spoofing make it unreliable, and a new
  address locks the machine out. A key stays correct when the address changes.
- **Bind it to a MAC address.** A MAC address does not cross a routed network, so the switchboard never sees one. A
  client can report any value, and many systems use a random MAC for each network.
- **Ask Tailscale which node sent the request.** It is stable across address changes and needs no client code. It ties
  Landline to Tailscale. Revisit it if Landline commits to Tailscale.
- **A per-machine bearer token in place of a keypair.** A copied config file would be a working credential, and the
  token would cross the network in each registration. The key does neither.
- **A reusable invite code.** One leaked code would admit any number of machines.
- **Replace a pending invite when the name is invited again.** It hides that a code already given out is now dead. A
  held name that needs a `revoke` is one rule for pending and active machines.
- **The machine's own host name as its name.** A host name can hold capitals and dots, can change, and can collide with
  another machine's. The person picks the name at invite time.
- **Put the name in the redeem route.** `PATCH /v1/machines/<name>` makes the person type or copy the name, and makes
  the route answer differently for a name with no pending machine. The code is the lookup key instead.
- **A clock that an outside caller can move.** A route or variable that moves time would defeat the window and the start
  rule. The timing flags give a test short real times without it.
- **`join --via ssh`, which fetches the shared secret over ssh.** It needs ssh from the new machine, and the machine
  still holds a secret that a rotation kills.
- **TLS on the switchboard's listener.** It would encrypt tokens and message bodies off a tailnet as well. It needs a
  certificate source and a way for each client to trust it, and no client speaks `https` today. It is a separate
  decision, and [0190](0190-one-toml-config-names-the-switchboard-by-host-and-port.md) left room for a `tls` key. The
  keypair here does not depend on it.
- **Make `rotate-secret` revoke every machine.** It would restore today's cost of re-joining after every rotation, which
  is the cost this record removes. `revoke --all` does it on purpose.
