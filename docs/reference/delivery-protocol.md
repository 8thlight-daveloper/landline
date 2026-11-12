# Delivery protocol

The HTTP shapes that [0070](../decisions/0070-delivery-is-a-push-to-a-connected-agent-with-no-inbox.md) names. The hub
keeps no mail. Each agent's `landline mcp` process opens one stream, the hub sends each message down it, and the MCP
process reports back what it did with the message. The types are in `internal/model/wire.go`.

## Authentication

Unchanged. An agent route takes `Authorization: Bearer <agent token>`, the token from `POST /v1/register`. A hub route
takes `Authorization: Bearer <hub secret>`.

## Routes

| Route                             | Auth        | Does                                                                  |
| --------------------------------- | ----------- | --------------------------------------------------------------------- |
| `POST /v1/register`               | secret      | Registers an agent. Unchanged except that it takes no delivery fields |
| `POST /v1/attach`                 | agent token | Records claims. A Codex proof sends its code as a challenge           |
| `GET /v1/stream`                  | agent token | Opens the delivery stream (below)                                     |
| `POST /v1/deliveries/{id}/report` | agent token | The MCP process reports one delivery. Body is `DeliveryReport`        |
| `POST /v1/send`                   | agent token | Sends to one recipient and waits for the delivery report              |
| `GET /v1/agents`                  | agent token | Lists agents, each with `connected`                                   |
| `GET /v1/log`                     | secret      | Reads the log. Unchanged                                              |

Removed: `GET /v1/inbox`, `GET /v1/wakes`, `POST /v1/wakes/{id}/ack`.

## The stream

`GET /v1/stream` answers `200` with `Content-Type: text/event-stream` and holds the response open.

- Each delivery is one event: `event: delivery`, then `data: <one line of Delivery JSON>`, then a blank line.
- Every 15 seconds with no delivery, the hub writes the comment line `: keepalive` and a blank line, and flushes. A
  write deadline of 30 seconds applies to each write. A write that fails or misses the deadline closes the stream. A
  peer that vanished without closing can keep absorbing small writes into the kernel buffer, so the deadline alone does
  not promise detection within 30 seconds.
- An agent has at most one open stream. A second `GET /v1/stream` for the same agent closes the first and replaces it.
- The agent is `connected` while its stream is open. When the stream closes, the hub marks the agent disconnected and
  sets its last-seen time to that moment. A stream that a newer stream replaced does not mark the agent disconnected.
- A `Delivery` has an `id` (unique per delivery), a `kind`, the rendered `text`, and for a message the `message`
  envelope (`WireEnvelope`, whose `text` equals the delivery `text`). Kind `message` is one agent's message. Kind
  `challenge` is the Codex proof: `target` is the Codex thread, and `text` is what to queue into it.

## Sending

`POST /v1/send` takes `SendBody` and checks it as today (the router contract, then the recipient resolved from a handle
or label). It then:

1. Appends the envelope to the log, as today.
1. If the recipient is not connected, answers `409` with `{"error": "not connected"}`. Nothing is held.
1. Otherwise writes a `message` delivery to the recipient's stream and waits for its report, up to 20 seconds.
1. Report `ok`: answers `200` with `SendResponse` (`id`, `seq`, `recipients` of length one).
1. Report not `ok`: answers `502` with `{"error": "<reported error>"}`.
1. No report in time: answers `504` with `{"error": "no delivery report"}`.

A recipient named `all` is an unknown recipient. A message to oneself is refused as before.

## Reporting

`POST /v1/deliveries/{id}/report` takes `DeliveryReport` (`ok`, `error`). Only the agent the delivery went to may report
it, otherwise `403`. An unknown or already reported id is `404`. "Delivered" means the channel event was written or
`codex queue` returned 0. It never means the agent read or acted on the text.

## The Codex proof

`POST /v1/attach` with a `codex_thread` and no code answers as today, except that the hub sends the code as a
`challenge` delivery down the caller's own stream and waits for its report. With no open stream it answers `409`
`not connected`. The MCP process runs `codex queue --thread <target> --message <text>` for a challenge and reports the
result. The second call with the code is unchanged, except that a proved agent records nothing about the thread on the
hub: the MCP process keeps the thread and runs the adapter itself.

## Listing

`GET /v1/agents` lists an agent while it is connected, or for 10 minutes after its stream closed. After that the entry
is unlisted only: the token and the registration stay, so a later reconnect works. Each entry has `connected`. A
disconnected entry is not dropped from a send's resolution: a send to it answers `409` `not connected`.

## Reconnecting

The MCP process reconnects with delays that start at one second, double, and stop at 30 seconds. It presents the same
agent token, so a reconnect by the same process keeps its handle.
