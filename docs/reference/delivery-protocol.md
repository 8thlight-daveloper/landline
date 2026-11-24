# Delivery protocol

The HTTP shapes that [0070](../decisions/0070-delivery-is-a-push-to-a-connected-agent-with-no-inbox.md) names, with the
words of [0160](../decisions/0160-an-agent-has-an-id-a-project-and-a-task.md): an agent has an `id`, a `project`, and a
`task`. Older records say handle, name, and label. The switchboard keeps no mail. Each agent's `landline-mcp` process
opens one stream, the switchboard sends each message down it, and the MCP process reports back what it did with the
message. The types are in `internal/model/wire.go`.

## Authentication

An agent route takes `Authorization: Bearer <agent token>`, the token from `POST /v1/register`. A switchboard route
takes `Authorization: Bearer <switchboard secret>`. The four routes that open a session, `POST /v1/register`,
`POST /v1/observers`, `GET /v1/log`, and `GET /v1/build`, also take a machine signature in place of the secret
([0200](../decisions/0200-a-machine-joins-with-an-invite-code-and-a-keypair-that-outlasts-secret-rotation.md)). The
machine routes take the secret and never a signature, except the redeem route, which takes no header. The section
[Machines](#machines) has the header, the signed text, and the refusal.

## Routes

| Route                             | Auth                                 | Does                                                                                    |
| --------------------------------- | ------------------------------------ | --------------------------------------------------------------------------------------- |
| `POST /v1/register`               | secret or signature                  | Registers an agent. It takes no delivery fields and carries the build values (below)    |
| `POST /v1/observers`              | secret or signature                  | Registers an observer. Body is `{"kind": "tui"}`, answer is `{"token": "..."}`          |
| `POST /v1/attach`                 | agent token                          | Records claims and answers `project_peers`. A Codex proof sends its code as a challenge |
| `POST /v1/status`                 | agent token                          | Sets or clears the agent's task (below)                                                 |
| `GET /v1/stream`                  | agent token                          | Opens the delivery stream (below)                                                       |
| `POST /v1/deliveries/{id}/report` | agent token                          | The MCP process reports one delivery. Body is `DeliveryReport`                          |
| `POST /v1/send`                   | agent token                          | Sends to one recipient and waits for the delivery report                                |
| `GET /v1/agents`                  | agent or observer token              | Lists agents, each with `connected`, and `task` and `task_at` when set                  |
| `GET /v1/log`                     | secret or signature                  | Reads the log. Each entry has `result`                                                  |
| `GET /v1/build`                   | secret, signature, or observer token | Answers the switchboard's own build (below)                                             |
| `POST /v1/machines`               | secret                               | Makes an invite code for a named machine (below)                                        |
| `PATCH /v1/machines`              | none, the invite code is in the body | Redeems an invite code with the machine's public key (below)                            |
| `GET /v1/machines`                | secret                               | Lists active machines (below)                                                           |
| `DELETE /v1/machines/{name}`      | secret                               | Revokes one machine, pending or active, and ends its tokens and streams (below)         |
| `DELETE /v1/machines`             | secret                               | Revokes every machine (below)                                                           |

Removed: `GET /v1/inbox`, `GET /v1/wakes`, `POST /v1/wakes/{id}/ack`.

## Identity

An agent has three words, and each key carries one of them.

| Word      | Key       | What it is                                                                                      |
| --------- | --------- | ----------------------------------------------------------------------------------------------- |
| `id`      | `id`      | The address of one registration. It names nothing else                                          |
| `project` | `project` | The group of agents that work on one project. It is the agent's own claim, so it proves nothing |
| `task`    | `task`    | The outcome the agent is working toward now. It is the agent's own words                        |

There is no alias for `name`, `handle`, or `label`. The body of every route rejects an unknown field with `400`, so a
client that sends `name` or `label` fails loudly.

### Registering

`POST /v1/register` takes the switchboard secret or a machine signature and a body with `harness`, `host`, `session`,
the optional attributes (`folder`, `branch`, `worktree`, `harness_version`, `model`, `effort`, `project`, and the build
values below), and exactly one of `id` and `seed`. Giving both or neither is `400`. It answers
`{"id": "...", "token": "..."}`.

- A registration by `id` takes that id if it is valid: 1 to 32 lowercase letters, digits, `-`, or `_`, starting with a
  letter or digit, and not `all`. It replaces any earlier registration of that id, so the token changes and the task is
  cleared. An invalid id is `400`.
- A registration by `seed` asks the switchboard for an id. The switchboard answers 4 lowercase hexadecimal characters,
  such as `a3f9`, drawn at random, and draws again until the value is not the id of any listed agent, including one in
  its grace. The text of the seed does not shape the id.
- `project`, like every text field, is at most 64 bytes with no control character, line or paragraph separator, or
  invisible formatting character. A value that breaks the rules answers `400` and registers nothing.

```json
{"seed": "landline", "harness": "claude-code", "host": "mac", "session": "s-1", "project": "landline"}
```

```json
{"id": "a3f9", "token": "<agent token>"}
```

### Attaching

`POST /v1/attach` takes an agent token and the claims `model`, `effort`, `project`, `codex_thread`, and `code`. It
answers `id`, `updated`, `challenge`, `attached`, and `project_peers`.

`project_peers` is the number of other agents that are connected and hold the caller's exact project. The caller is not
counted, and an agent that is not connected, including one in its grace, is not counted. A caller without a project gets
`0`. The switchboard writes `project_peers` in every answer, including `0` and the challenge calls for Codex.

```json
{"id": "a3f9", "updated": ["project"], "challenge": false, "attached": false, "project_peers": 2}
```

An empty claim means no change, so attach cannot clear anything.

### The task

`POST /v1/status` takes an agent token and a body with one field, `task`, which must be present and a string.

```json
{"task": "menu bar app for ADR 0140"}
```

- An empty string clears the task. A body without `task`, with `"task": null`, with a value that is not a string, or
  with an unknown field is `400`. An observer token is `403`. A missing or unknown token is `401`.
- The task follows the free-text rules of the registry: at most 64 bytes, and no control character, line or paragraph
  separator, or invisible formatting character. A value that breaks them is `400` and changes nothing.
- It answers `200` with `{"task": "<text>"}` and, when the task is not empty, `"task_at": "<RFC 3339 time>"`. After a
  clear the answer is `{"task": ""}` with no `task_at`.
- The switchboard stamps `task_at` itself, when the task changes. Setting the text the agent already has is not a
  change: it answers `200` with the existing time and stamps nothing. The agent never supplies a time.
- The switchboard keeps the task with the agent in memory. A registration by id clears it, the switchboard forgets it
  when it forgets the agent, and a restarted switchboard holds none.
- A task is for display. Nothing routes by it, and a delivered message's header and sender stamp do not carry it.

## The stream

`GET /v1/stream` answers `200` with `Content-Type: text/event-stream` and holds the response open.

- Each delivery is one event: `event: delivery`, then `data: <one line of Delivery JSON>`, then a blank line.
- Every 15 seconds with no delivery, the switchboard writes the comment line `: keepalive` and a blank line, and
  flushes. A write deadline of 30 seconds applies to each write. A write that fails or misses the deadline closes the
  stream. A peer that vanished without closing can keep absorbing small writes into the kernel buffer, so the deadline
  alone does not promise detection within 30 seconds.
- An agent has at most one open stream. A second `GET /v1/stream` for the same agent closes the first and replaces it.
- The agent is `connected` while its stream is open. When the stream closes, the switchboard marks the agent
  disconnected and sets its last-seen time to that moment. A stream that a newer stream replaced does not mark the agent
  disconnected.
- A `Delivery` has an `id` (unique per delivery), a `kind`, the rendered `text`, and for a message the `message`
  envelope (`WireEnvelope`, whose `text` equals the delivery `text`). Kind `message` is one agent's message. Kind
  `challenge` is the Codex proof: `target` is the Codex thread, and `text` is what to queue into it.

## Sending

`POST /v1/send` takes `SendBody` (`to`, `thread`, `kind`, `body`, `in_reply_to`) and checks it as today (the router
contract, then the recipient looked up by exact id). `to` and the sender's `from` hold ids, and keep their names. It
then:

1. Appends the envelope to the log, as today.
1. If the recipient is not connected, answers `409` with `{"error": "not connected"}`. Nothing is held. When no
   `landline_version` is on record for the recipient, the text is
   `not connected: no landline version is on record, so it is a hand-registered id, which can send and cannot receive, or an older landline. Restart its session if it is a landline session`.
1. Otherwise writes a `message` delivery to the recipient's stream and waits for its report, up to 20 seconds.
1. Report `ok`: answers `200` with `SendResponse` (`id`, `seq`, `recipients` of length one).
1. Report not `ok`: answers `502` with `{"error": "<reported error>"}`.
1. No report in time: answers `504` with `{"error": "no delivery report"}`.

After the delivery ends, the switchboard appends a result line to the log for the envelope:
`{"seq": N, "result": "..."}`, where the result is `delivered` or the error the sender saw. `GET /v1/log` returns each
envelope with its `result`, empty when none was recorded, as in a log from before this change.

### The recipient

A send takes an exact id and nothing else. The switchboard looks the id up among every present agent.

- An id of a connected agent is delivered to. An id of an agent in its grace answers `409` `not connected`. An id the
  switchboard has forgotten, or never issued, answers `404` with `{"error": "the recipient is not a registered agent"}`,
  the existing text.
- No prefix resolves, and no project resolves, whether one agent or ten hold it. Each is an unknown recipient with the
  plain `404`. There is no `409` for several matches. `all` is an unknown recipient with the plain `404`, and it never
  gets the hint below, even when an agent has the project `all`. An unknown sender is `422`, without a hint.
- **The hint.** When the value of `to` equals the `project` of at least one connected agent, the `404` text of the
  unknown recipient gains a sentence: `<value> is a project, not an agent. Send to one of: <ids>`. The ids are the
  connected holders of that project other than the sender, sorted, at most 10, with `, and more` after the tenth when
  there are others. The hint is added only to that `404`. A project held only by agents that are not connected, or only
  by the sender, gets the plain `404`. The status is `404` in every case, so a command does not change behavior when a
  second agent starts.

A message to oneself is refused as before.

## Reporting

`POST /v1/deliveries/{id}/report` takes `DeliveryReport` (`ok`, `error`). Only the agent the delivery went to may report
it, otherwise `403`. An unknown or already reported id is `404`. "Delivered" means the channel event was written or
`codex queue` returned 0. It never means the agent read or acted on the text.

## The Codex proof

`POST /v1/attach` with a `codex_thread` and no code answers as today, except that the switchboard sends the code as a
`challenge` delivery down the caller's own stream and waits for its report. With no open stream it answers `409`
`not connected`. The MCP process runs `codex queue --thread <target> --message <text>` for a challenge and reports the
result. The switchboard sends at most 5 challenges per agent in any 5 minutes. Past that, `POST /v1/attach` answers
`429` with `{"error": "too many proof challenges, try again later"}` and sends nothing. The second call with the code is
unchanged, except that a proved agent records nothing about the thread on the switchboard: the MCP process keeps the
thread and runs the adapter itself.

## Listing

[0080](../decisions/0080-the-hub-keeps-only-connected-agents.md) holds agents in memory only, and only while they are
present. An agent is present while its stream is open, and for 2 minutes after its stream closes. A hand-registered
agent has no stream, and it is present for 2 minutes after its last authenticated request. When that time runs out the
switchboard deletes the registration and the token, and the token is answered with `401` from then on. The switchboard
checks this before every request, against its own clock.

`GET /v1/agents` lists every present agent. Each entry has `id`, `harness`, `host`, `session`, `connected` (`false` for
an agent in its grace), and `last_seen`. It has `project`, `task`, and `task_at` when the agent has them, and leaves
each out when it is empty. `task_at` is a string in RFC 3339 and is present only with a task. An entry also has
`landline_version`, `landline_commit`, and `landline_date` when the agent registered them, with the full values and with
the key left out when its value is empty.

## Build values

[0100](../decisions/0100-a-build-names-itself-by-version-commit-and-date.md) adds `landline_commit` and `landline_date`
to `landline_version`. `POST /v1/register` takes all three in both forms, the id form and the seed form. Each is an
optional string of at most 64 bytes with no control character, line or paragraph separator, or invisible formatting
character. A value that breaks the rules answers `400` and registers nothing. An empty value is left out.

A registration by id replaces the earlier one, so registering again replaces the commit and the date, and an empty value
clears the old one. A registration by seed gets a new id and replaces nothing. A hand-registered id sends none of the
three.

| Field              | Example                                          | Meaning                                                                           |
| ------------------ | ------------------------------------------------ | --------------------------------------------------------------------------------- |
| `landline_version` | `0.1.0`                                          | The content of `VERSION`                                                          |
| `landline_commit`  | `0123456789abcdef0123456789abcdef01234567-dirty` | The 40 hex character commit of the build, with `-dirty` when tracked files differ |
| `landline_date`    | `2026-11-12T16:34:00Z`                           | The committer date of that commit in UTC. Absent for a `-dirty` commit            |

```json
{"id": "alpha", "harness": "claude-code", "host": "mac", "session": "s-1",
 "landline_version": "0.1.0",
 "landline_commit": "0123456789abcdef0123456789abcdef01234567",
 "landline_date": "2026-11-12T16:34:00Z"}
```

The switchboard enforces the field rules only. The shapes in the table come from the build, which no flag, environment
variable, or `landline_register` argument can override.

The switchboard keeps no agent state on disk. Its only state file is `machines.json`, the list of joined machines
([Machines](#machines)). A switchboard restart forgets every agent and token, and each agent registers again and gets a
new id. The switchboard does not issue an id that a present agent holds, including one in its grace, and a forgotten id
may be issued again.

## Observers

[0090](../decisions/0090-observers-read-the-agent-list-and-the-tui-is-a-separate-binary.md) adds a client that is not an
agent, such as a terminal viewer.

- `POST /v1/observers` takes the switchboard secret or a machine signature and a body `{"kind": "tui"}`, and answers
  `200` with `{"token": "<observer token>"}`. `kind` is required and follows the free-text rules of the registry: at
  most 64 bytes, no control, line or paragraph separator, or format characters, and not empty after trimming white
  space. Otherwise the switchboard answers `400`. A missing or wrong secret is `401`.
- An observer is not an agent. It has no id and no stream, and it never appears in `GET /v1/agents`.
- An observer token is read-only. `GET /v1/agents` answers it with the list an agent gets. Every other route, including
  the secret routes, refuses it with `403` and `{"error": "..."}` that says the token is read-only. An unknown token is
  `401`.
- The switchboard holds observers in memory only. An observer is forgotten after 2 minutes (`--grace`) without a
  request, and the boundary is inclusive as for agents. Every request with its token, refused or not, resets that time.
  The switchboard checks this before every request, against its own clock, and the token then answers `401`. A
  switchboard restart forgets every observer. A client registers again with the switchboard secret or a machine
  signature on a `401`.

## The build

[0140](../decisions/0140-a-macos-menu-bar-item-shows-agents-runs-the-switchboard-and-sets-up-harnesses.md) adds a route
that reports the build of the running switchboard.

- `GET /v1/build` takes the switchboard secret, a machine signature, or an observer token and answers `200` with
  `{"version": "0.2.0", "commit": "<40 hex>", "date": "<RFC 3339>"}`. The values are the ones
  [0100](../decisions/0100-a-build-names-itself-by-version-commit-and-date.md) stamps. A value the build lacks is an
  empty string, and an unstamped build answers `dev` as its version.
- An agent token gets `403`. A missing, malformed, or unknown credential gets `401`. Only `GET` answers.
- The answer carries no secret and no token. A switchboard from before this route answers `404`.

## Timing flags

`landline-switchboard` and `landline server up` take `--keepalive`, `--delivery-timeout`, `--grace`,
`--invite-lifetime`, and `--signature-window`, each a Go duration such as `200ms`, for the 15 second keepalive, the 20
second report wait, the 2 minute grace, the 5 minute invite lifetime, and the 5 second signature window. A zero,
negative, or unparsable value is refused, and so is a signature window below 1 second or above 5 seconds. The same
settings live in the `[server]` table of `config.toml`, and a flag overrides the table for one run
([0220](../decisions/0220-the-switchboard-reads-its-settings-from-the-config-and-keeps-machines-json-in-the-data-directory.md),
[config-toml.md](config-toml.md)). They exist so a test can run the timing rules fast. `LANDLINE_PORT` sets the default
port, 8787, for a switchboard with no `--addr` and for a client with no saved switchboard.

## Machines

[0200](../decisions/0200-a-machine-joins-with-an-invite-code-and-a-keypair-that-outlasts-secret-rotation.md) lets a
machine join with an invite code and prove who it is with an Ed25519 key. The switchboard keeps only the public half. A
machine is pending after an invite and active after its code is redeemed. A machine name is 1 to 32 lower case letters,
digits, and hyphens, and starts with a letter.

### Invite

`POST /v1/machines` takes the secret and `{"name": "alpha"}`, and answers `200` with `{"name": "alpha", "code": "..."}`.
The code is eight characters from `23456789ABCDEFGHJKMNPQRSTUVWXYZ`, single use, and valid for 5 minutes
(`--invite-lifetime`). The switchboard keeps invites in memory, so a restart cancels them.

- A name that breaks the rule is `400`. A name that a pending or active machine holds is `409`.
- A machine signature on this route, or on any machine route below except the redeem, gets `401` with
  `{"error": "missing or wrong switchboard secret"}`. An observer token gets `403`.

### Redeem

`PATCH /v1/machines` takes no `Authorization` header and the body `{"code": "...", "public_key": "<base64>"}`, where the
key is the standard base64 of the 32 public key bytes. It answers `200` with `{"name": "alpha"}`, and the machine is
active.

- Every request that does not name a valid pending code gets `401` and `{"error": "invalid or expired invite code"}`,
  whatever the reason: a missing, malformed, wrong, used, or expired code, or a body that is not JSON.
- A valid code with a missing or malformed public key gets `400`. It does not use up the code.
- After 5 failed redeems since the last good redeem or the last cancel, the switchboard cancels every waiting invite.

### List and revoke

`GET /v1/machines` takes the secret and answers `200` with a list of the active machines, never the pending ones:

```json
[{"name": "alpha", "fingerprint": "0123456789ab", "created": "2026-11-19"}]
```

`fingerprint` is the first 12 lower case hex characters of the SHA-256 of the 32 public key bytes, and `created` is the
date the machine joined, in UTC. The answer never holds a key.

`DELETE /v1/machines/{name}` takes the secret, deletes the pending or active machine of that exact name, and answers
`200` with `{}`. It ends at once every agent token and observer token that registered through the machine, with their
open streams. An unknown name is `404`. `DELETE /v1/machines` does the same for every machine, pending or active.

### Signed requests

A machine signs `POST /v1/register`, `POST /v1/observers`, `GET /v1/log`, and `GET /v1/build` with this header:

```
Authorization: Landline-Machine name=<name>, time=<unix seconds>, nonce=<base64>, signature=<base64>
```

The nonce is 16 random bytes in standard base64, new for each request. The signature is the standard base64 Ed25519
signature of these lines, joined by line feeds:

```
landline-machine-v1
<name>
<time>
<nonce>
<HTTP method>
<path with its query>
<lower case hex SHA-256 of the body>
```

The switchboard accepts the request when the time is within the signature window of its own clock, the time is later
than the start period, the machine is active, the signature verifies, and it has not seen the signature before. The
window is 5 seconds by default and from 1 to 5 seconds. The start period is every time up to 5 seconds after the second
the switchboard started, so the switchboard refuses every signed request for one window after it starts. A request that
registers records which machine it came from, and a revoke of that machine ends its token.

### Signed refusals

Every `401` that answers a signed request carries `error` and `reason`:

- A time outside the window gets `"reason": "clock"` and `"clock_difference": N`, the time in the request minus the
  switchboard's time in signed whole seconds, positive when the machine's clock runs ahead. `error` names the
  difference.
- Every other refusal, including one in the start period and a header that cannot be read, gets `"reason": "refused"`
  and one message, whichever check failed:

```json
{"error": "missing or wrong switchboard secret or machine signature", "reason": "refused"}
```

```json
{"error": "the clock of this machine differs from the switchboard's by 12 seconds. Set the clock", "reason": "clock", "clock_difference": 12}
```

A client reads `reason` and `clock_difference` and never the text of `error`. It treats an unknown or missing `reason`,
or `clock` without an integer `clock_difference`, as `refused`. On `refused` it retries about once a second, signing
each retry anew, until 6 seconds have passed since the first refusal, and then gives the generic advice. It never
retries `clock`.

### `machines.json`

The switchboard keeps the active machines in `~/.local/share/landline/machines.json`, whatever the log path
([0220](../decisions/0220-the-switchboard-reads-its-settings-from-the-config-and-keeps-machines-json-in-the-data-directory.md)).
It writes the file with mode `0600` through a temporary file renamed over the old one, after each redeem and revoke, and
reads it at start. The file is a JSON list. Each entry has `name`, `public_key` as the standard base64 of the 32 key
bytes, and `created` as an RFC 3339 UTC time. It never holds an invite code or a seen signature.

## Reconnecting

The MCP process reconnects with delays that start at one second, double, and stop at 30 seconds. It presents the same
agent token, so a reconnect inside the 2 minute grace keeps its id. After the grace the switchboard answers `401`. The
MCP process then registers again once, gets a new id, and removes the session file of the id it replaced. A CLI command
that gets `401` prints that the switchboard does not know the agent any more and tells the person to run
`landline register` again.
