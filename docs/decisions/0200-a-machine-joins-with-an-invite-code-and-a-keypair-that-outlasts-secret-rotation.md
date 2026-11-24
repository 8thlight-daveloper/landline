---
category: behavior
status: accepted
approval: person
approver: {kind: person, name: dnawara}
affects: [machine/registry, cli/command, switchboard/server, client/http, mcp/server, agent/viewer, menubar/viewer]
builds-on: ['0040', '0090', '0170', '0190']
partly-superseded-by: ['0220']
note: Drafted on 2026-11-19 and amended the same day after the acceptance review. Amended on 2026-11-21 after the third review, so that a time in the start second is refused and an unknown name costs the same Ed25519 verify as a known one. Amended on 2026-11-23, so that a client names the possible causes of the generic refusal. Amended on 2026-11-23 again, so that a refusal carries a reason that clients read instead of its text. Amended on 2026-11-24, so that the window is 5 seconds at most and the switchboard refuses signed requests for one window after it starts. It adds a credential kind, a stored format on both machines, a route that takes no secret, and a core component. The second amendment of 2026-11-23 changes the wire format of four routes. Amended on 2026-11-24 again, so that the menu bar shows a short line for no credential.
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
1. `landline server join <host:port>` without a code refuses while a `machine.key` is saved, and changes nothing,
   because the key would stay first in the credential order. The person removes the key file and the `machine` line, or
   joins with a code. An empty `--code` is an error and never falls back to the secret.
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

The switchboard accepts a request when the time is within the signature window of its own clock, the time is later than
the start period below, the machine is active, the signature verifies, and it has not seen that signature while it could
still pass the window check.

The signature window is 5 seconds by default. A person may set it from 1 to 5 seconds, with `--signature-window` or the
`signature-window` key of the `[server]` table
([0220](0220-the-switchboard-reads-its-settings-from-the-config-and-keeps-machines-json-in-the-data-directory.md)). A
value below 1 second or above 5 seconds is an error that stops the switchboard before it listens. The switchboard counts
the window in whole seconds.

Clocks must agree within the window. The switchboard's machine and every joined machine need working time sync, such as
the network time service that macOS and most Linux systems turn on by default. A machine whose clock differs from the
switchboard's by more than the window gets the clock refusal below, which names the difference.

The window check comes first and does not look at the name. A time outside the window gets `401` and a message that
names the difference in seconds and tells the person to set the clock, whether or not the name belongs to a machine.

Inside the window, the switchboard runs one Ed25519 verify before it answers, whatever the answer is. For an active
machine it verifies against the machine's public key. For a name that is unknown, pending, or revoked, it verifies
against a fixed dummy key and refuses the request whatever the result. Every refusal inside the window gets `401` and
the message `missing or wrong switchboard secret or machine signature`, whichever check failed. Neither the answer nor
the work done before it depends on whether the name exists, so a caller who holds no credential learns nothing about
which names exist, from the text or from the time it takes.

The switchboard remembers a signature until one second after the time in it plus the window, which is the last moment it
could pass the window check, because a time has whole seconds. A signature is therefore never forgotten while a copy of
it could still be accepted. Two requests that differ only in their nonce are both accepted, and an exact copy of one is
refused.

The switchboard keeps the signatures it has seen in memory, and a restart empties them. A copy of a request that the old
process accepted would then pass the new one, which does not know it.

**For one window after every start, the switchboard refuses every signed request.** It refuses a request whose time is
not later than the second it started plus 5 seconds, the largest window, whatever window this run uses. This is the
start period. The old process accepted a time at most one window past its own clock, its window was at most 5 seconds,
and it stopped before the new process started. So every copy that the old process could have accepted has a time no
later than the end of the start period, and is refused. That holds for a copy from a machine whose clock runs ahead, and
for a copy accepted under a wider window before a person narrowed it. A time later than the start period is one the old
process never accepted, and the memory of the new process covers it from then on.

The rule reads the time in the request, and not the moment it arrives. A copy can arrive up to one window after the time
it carries, so a rule on arrival alone would let a copy signed near the end of the period pass just after it.

A machine whose clock agrees with the switchboard's is refused for about the first 5 seconds after every start,
including the restart that `rotate-secret` makes. One whose clock runs `d` seconds behind, with `d` inside the window,
is refused for about `5 + d` seconds, and one whose clock runs ahead for less. A refusal in the start period gets `401`,
`"reason": "refused"`, and the generic message, like every refusal inside the window, so it does not tell a caller that
the switchboard just started.

The start period replaces the rule of the amendment of 2026-11-21, which refused only a time in the second the
switchboard started. That rule is a start period of no length, and the start period refuses everything it refused, so it
is removed. It let a copy from a machine whose clock runs ahead pass, as the alternatives below say.

Every `401` that answers a request signed with a machine key carries a JSON body with the message in `error` and a field
`reason`:

- A time outside the window gets `"reason": "clock"` and `"clock_difference": N`, besides the message that names the
  difference. `N` is the time in the request minus the switchboard's time, in signed whole seconds, so it is positive
  when the machine's clock runs ahead.
- Every refusal inside the window gets `"reason": "refused"` and the one message above. A `Landline-Machine` header that
  cannot be read gets the same answer.

```json
{"error": "missing or wrong switchboard secret or machine signature", "reason": "refused"}
```

The two values reveal nothing the message does not already say. The clock message already names the difference, and
`refused` is one value for every refusal inside the window, whichever check failed. The promise above holds: a caller
who holds no credential learns nothing about which names exist.

A client (the `landline` commands through `client/http`, `landline-mcp`, the screen, and the menu bar) decides from
`reason` and `clock_difference` whether to retry and whether to give the clock advice or the generic advice. It never
compares the message text, by C1 in [coding-principles.md](../reference/coding-principles.md). A client that gets a
`reason` it does not know, or no `reason`, as from a switchboard built before this amendment, treats the answer as
`refused` and shows the message as given. So does a client that gets `clock` without an integer `clock_difference`.

This changes the wire format of `POST /v1/register`, `POST /v1/observers`, `GET /v1/log`, and `GET /v1/build`: the `401`
body for a signed request gains `reason`, and `clock_difference` for a time outside the window. A change to either field
needs a new record. [0230](0230-the-person-learns-why-a-signed-request-was-refused.md) proposed the same field, and this
record now defines it.

On an answer whose `reason` is `refused`, a client retries the request about once a second, signing each retry anew,
until 6 seconds have passed since the first refusal. That is the largest window plus one second. A client does not know
the switchboard's window, so it waits out the largest, and a retry signed after that by a machine whose clock agrees
with the switchboard's falls after the start period. A retry that succeeds shows the person nothing. When the last retry
is refused, the client names the possible causes, among them a rotated secret, a revoked machine, and a clock that runs
behind the switchboard's, and tells the person to join again if the refusal persists. A client never retries an answer
whose `reason` is `clock`, and gives the clock advice at once.

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

The menu bar is the exception. When it finds no credential, its state line reads `No switchboard credential`, and a menu
action that fails while it has none shows that same line as its notice in place of the text the command printed. The
menu bar decides this from its own reading of the credential order above, never from the text of a command's output, by
C1. A notification with the full message belongs to [0230](0230-the-person-learns-why-a-signed-request-was-refused.md).

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
`--signature-window D`, 5 seconds by default and from 1 to 5 seconds. `landline server up`, `restart`, and
`rotate-secret` pass them on. They let an acceptance test run the real binary with short times.

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
- `docs/reference/delivery-protocol.md` gains the machine routes, the signed header, the `reason` and `clock_difference`
  fields, the start period, and `machines.json`.
- The amendment of 2026-11-24 changes the signature check of `machine/registry`, the setting check of
  `switchboard/server` and `cli/command`, and the retry of every client. Their contracts and acceptance scenarios
  change, and a person authorizes those edits.
- Every machine's clock must agree with the switchboard's within the window, 5 seconds at most, so every machine needs
  working time sync. A clock that is off by more gets the clock message on every signed request until it is set.
- A switchboard refuses the signed requests of a machine whose clock agrees with its own for about 5 seconds after every
  start. A client covers that with its retries, so a person sees a refusal only when it lasts longer.
- A `config.toml` that sets `signature-window` above 5 seconds, such as the `60s` that the documentation showed before
  this amendment, stops the switchboard with an error until the person changes or removes the key.
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
- **Refuse only a time in the start second.** The amendment of 2026-11-21 did this, with a 60 second window. It covers a
  machine whose clock agrees with the switchboard's. A machine whose clock runs ahead signs a time later than the next
  start, and after a restart the memory of signatures is empty, so a copy of its request passed the new process. The
  start period covers every time the old process could have accepted.
- **Refuse by arrival for one window after the start.** A copy signed near the end of the period could arrive after it
  and still pass the window check. A rule on the time in the request has no such gap.
- **Save the seen signatures on every request.** It removes the cause: a new process would know every signature the old
  one accepted, and needs no start period. It costs a disk write on every signed request. A later record may revisit it
  with SQLite, as [the note on switchboard state](../future/switchboard-state-in-sqlite.md) says.
- **Save the seen signatures at a clean stop, and wait out a window after a crash.** A clean restart would refuse
  nothing. It adds a stored file with its own format and mode, and the switchboard must tell a clean stop from a crash.
- **Keep the 60 second window and narrow the promise.** The record would say that a copy may pass soon after a restart.
  That weakens the guarantee that a copy is never accepted twice, and a 60 second start period would close the
  switchboard for a minute after every rotation.
- **Answer an unknown name before the signature check.** It is cheaper, and the text of the answer is the same. The
  answer comes back in nanoseconds for an unknown name and in microseconds for a known one, so its time tells a caller
  which names exist.
- **A clock that an outside caller can move.** A route or variable that moves time would defeat the window and the start
  period. The timing flags give a test short real times without it.
- **`join --via ssh`, which fetches the shared secret over ssh.** It needs ssh from the new machine, and the machine
  still holds a secret that a rotation kills.
- **TLS on the switchboard's listener.** It would encrypt tokens and message bodies off a tailnet as well. It needs a
  certificate source and a way for each client to trust it, and no client speaks `https` today. It is a separate
  decision, and [0190](0190-one-toml-config-names-the-switchboard-by-host-and-port.md) left room for a `tls` key. The
  keypair here does not depend on it.
- **Make `rotate-secret` revoke every machine.** It would restore today's cost of re-joining after every rotation, which
  is the cost this record removes. `revoke --all` does it on purpose.
