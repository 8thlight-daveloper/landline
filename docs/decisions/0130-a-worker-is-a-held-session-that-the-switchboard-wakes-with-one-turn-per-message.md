---
category: behavior
status: proposed
affects: [switchboard/server, client/http, cli/command, agent/viewer, agent/registry]
builds-on: ['0020', '0070', '0080', '0110', '0120']
partly-supersedes: ['0080']
note: Drafted on 2026-11-15 to add a kind of agent that the switchboard remembers, which 0080 says it does not.
---

# 0130. A worker is a held session that the switchboard wakes with one turn per message

Builds on [0020](0020-messages-carry-no-user-authority.md),
[0070](0070-delivery-is-a-push-to-a-connected-agent-with-no-inbox.md),
[0080](0080-the-hub-keeps-only-connected-agents.md),
[0110](0110-three-binaries-landline-switchboard-and-landline-mcp.md), and
[0120](0120-an-operator-sends-and-attaches-and-its-messages-carry-no-user-authority.md). It partly supersedes
[0080](0080-the-hub-keeps-only-connected-agents.md), which says the switchboard keeps no state of its own: a worker is
the one thing it saves besides the log.

## Context

Every agent so far is a session that a person started in a terminal. It can receive only while its MCP process runs, so
a session that is closed is gone, and a message to it fails at once. A person who wants a reviewer or a researcher on
demand has to start and keep a session open.

Three harnesses run one turn of a stored session without a terminal: Claude Code with `claude -p --resume`, Codex with
`codex exec resume`, and opencode with `opencode run --session`. Their help text was read on 2026-11-15, for Claude Code
2.1.321, Codex 0.167.1, and opencode 2.2.6. A turn loads the session's history, runs, calls the session's MCP tools, and
exits. Landline can hold the session id and start a turn when a message arrives.

A turn runs unattended and with the owner's account. A worker is therefore a deliberate act of the person who holds the
secret. This record states what it can and cannot limit.

## Decision

**A worker is a record that the switchboard holds.** It names a stored session of one harness and a folder to run in.

- A worker has a `name`, a `harness` (`claude-code`, `codex`, or `opencode`), a `session` id, a `folder`, an optional
  `label` and `model`, and a `policy` that is `read-only` or `write`. The folder must exist on the switchboard's host
  and be the folder the session was started in, because the harnesses keep sessions per folder. The name matches
  `^[a-z0-9][a-z0-9-]{0,31}$`, is not `all` or `operator`, and does not change. It is the worker's identity, and no MCP
  handle is. The pair of harness and session is unique.
- `POST /v1/workers` creates one and `DELETE /v1/workers/<name>` removes one. Both take the secret and refuse an agent,
  observer, or operator token with `403`. `DELETE` of a running worker answers `409`, and removes the worker's log file
  otherwise. `landline worker add --name N --harness H --session S --folder F [--label L] [--model M] [--policy P]`,
  `landline worker list`, and `landline worker remove N` use them. `policy` defaults to `read-only`, and `write` must be
  named.
- A name that is taken, or that equals the handle or label of a present agent, is refused with `409`. After creation,
  the switchboard refuses a registration whose name, handle, or label equals a worker's name with `409`, and it makes a
  new handle again when a random one equals a worker's name.
- The switchboard saves workers in `workers.json` in its data folder, as a private file written through a temporary file
  and a rename. It holds the fields above and no secret or token. A missing file means no workers, and a file that
  cannot be read stops the start with a message. A later change to the format needs a record that says how the old file
  loads.

**What a turn runs is fixed by the adapter table below.** A worker record holds no command, flag, or environment.

| Harness       | `read-only` turn                                                                                                 | `write` turn                                      |
| ------------- | ---------------------------------------------------------------------------------------------------------------- | ------------------------------------------------- |
| `claude-code` | `claude -p --resume S --permission-mode plan --strict-mcp-config --mcp-config F --allowedTools <landline tools>` | The same with `--permission-mode acceptEdits`     |
| `codex`       | `codex exec resume S -c sandbox_mode="read-only" -c approval_policy="never" -`                                   | The same with `sandbox_mode="workspace-write"`    |
| `opencode`    | `opencode run --standalone --session S --agent plan --format json MESSAGE`                                       | The same with `--auto` and without `--agent plan` |

- `S` is the session id, and the prompt goes to standard input for Claude Code and Codex and as the last argument for
  opencode, where other users of the machine can see it in the process list. `--model` follows when the worker has a
  model. The turn's working folder is the worker's folder. For Claude Code, `F` is a file that the runner writes for the
  turn, in the data folder with mode `0600`, which names only `landline-mcp` as the one MCP server, and removes after
  the turn. `<landline tools>` are `mcp__landline__landline_send` and `mcp__landline__landline_agents`.
- Only Claude Code limits the turn to the Landline tools. Codex and opencode load every MCP server in their own
  configuration, so a turn can use their side effects. A `read-only` policy limits file edits and commands that change
  state, and does not limit other tools or the network.
- The `opencode` row is not checked. The adapter accepts `read-only` for opencode only after a hand check against a
  throwaway session shows that `--agent plan` refuses an edit. Until then the creation of a `read-only` opencode worker
  is refused with `400`.
- The runner looks up the harness binary on the switchboard's path when a turn starts. A binary that is missing fails
  the send, and the message names it.
- The environment of the process is built from a list and not from the switchboard's own. It holds `PATH`, `HOME`,
  `USER`, `LOGNAME`, `SHELL`, `LANG`, `LC_ALL`, `TMPDIR`, and for Claude Code `CLAUDE_CONFIG_DIR`, `ANTHROPIC_API_KEY`,
  and `ANTHROPIC_BASE_URL`, for Codex `CODEX_HOME` and `OPENAI_API_KEY`, and for opencode `OPENCODE_CONFIG` and the
  `XDG_*` folders, when the switchboard has them. It sets `LANDLINE_LABEL` to `worker <name>`, which no worker name can
  equal. It never passes `LANDLINE_SECRET`.
- The runner refuses to start a turn when the prompt does not start with the full marker line of
  [0020](0020-messages-carry-no-user-authority.md), as a wake adapter does.

**A worker is an agent that is idle or running.** The agent list gains a row for each worker, with `worker: true`,
`connected: true`, and a state of `idle` or `running`. The row has the worker's `name`, `harness`, `label`, `model`, and
the switchboard's host. It has no `session` and no `folder`. The screen and the JSON export sort workers with the
connected agents.

- While a turn runs, the harness's MCP process registers as an ordinary agent with the label `worker <name>`. After the
  turn it exits and is forgotten by the grace of [0080](0080-the-hub-keeps-only-connected-agents.md).
- A send resolves in this order: an exact handle, an exact worker name, a connected label or prefix, then a present
  agent in its grace. A worker name matches only exactly.

**A message to an idle worker starts one turn.** The switchboard starts the adapter's command in its own process group
with the rendered message as the prompt, so the prompt starts with the marker.

- The send waits up to 3 seconds. A process that exits with a failure in that time answers `502` with the exit status,
  and a message that names the log file. Otherwise the send answers `200` with `delivery: "started"` and the turn goes
  on. The message is in the log for both, and the log's result is `started` or `failed`.
- A worker runs one turn at a time. The check and the start happen under one lock, so concurrent sends start one turn,
  and the others answer `409` with `busy`. The switchboard holds no queue, as
  [0070](0070-delivery-is-a-push-to-a-connected-agent-with-no-inbox.md) says.
- A worker starts at most 20 turns in a sliding hour, counted in memory for each worker. Every start counts, and a send
  that is refused for `busy` or the limit does not. The 21st answers `429`. A message from any agent or operator can
  start a turn, so the limit keeps a loop from spending the owner's account.
- A turn ends when the process exits, or after 15 minutes. `--worker-timeout` changes the time. At the limit the
  switchboard sends `SIGTERM` to the process group and `SIGKILL` after 10 seconds. A stopping switchboard does the same
  to every running turn. The state returns to `idle` and the switchboard prints one line with the worker, the sender,
  the message id, and the exit.
- The process's output goes to `worker-<name>.log` in the data folder, a private file that is cut at the start of each
  turn. It holds whatever the harness prints. The switchboard never prints or sends it.
- A switchboard that crashes can leave a turn running, which the owner stops. The restarted switchboard shows the worker
  as `idle`.

**The reply is the worker's own send.** A worker answers with `landline_send`, as an agent does. A sender that wants the
answer reads it from its own mail. A message that started a turn from an operator has no reply path, as
[0120](0120-an-operator-sends-and-attaches-and-its-messages-carry-no-user-authority.md) says, so the reply is in the
log.

## Consequences

- The switchboard now starts processes. Only the secret holder creates a worker, and only a fixed table of commands
  runs. A worker's `write` policy runs unattended with the owner's account, and any agent that can send can start a
  turn, so a person creates a `write` worker on purpose and understands that a peer's message can drive it.
- The policy limits accident and not attack. A turn runs as the owner, so it can read any file the owner can, including
  the saved configuration that holds the secret. A message that persuades a worker to leak it is a prompt injection, and
  the marker and the agent instructions are the defence. A person who does not trust the agents on their machine to send
  a worker anything keeps no workers.
- `workers.json` and the worker rows are new. The rows carry no session id or folder, but the file does, and it is
  private.
- `internal/model` gains the `worker` field, the `state` values, and the `delivery` answer. `docs/reference/` gains the
  routes, the answers `started`, `busy`, and `429`, and the adapter table. A new component, `worker/runner`, joins
  `architecture.yaml`, which is protected, as a driver at the position before `switchboard/server`.
- A worker has an identity apart from any process, so a trusted-peer list can name it later.
- The acceptance suite gains worker creation, the refusal for each token kind, the name pattern and reserved names, a
  collision with a later registration, the saved file and its mode, a restart that keeps the worker, a send that starts
  a stand-in harness, the exact environment and arguments the stand-in receives, `busy` under concurrent sends, the 21st
  turn, a fast failure, the timeout with a process that ignores `SIGTERM`, `DELETE` of a running worker, and the log
  file. The suite replaces each harness with a stand-in program, since the real ones need accounts.
- The owner checks the real harnesses by hand, including that a closed or missing session fails cleanly and that
  `claude -p --resume` keeps the session id, as the earlier work for Claude Code and Codex was.
- A worker's host is the switchboard's. A worker on another machine needs a switchboard there.

## Alternatives considered

- **Run the turn from the MCP process or the TUI.** The switchboard would start nothing. A closed TUI would then stop
  every worker, and the MCP process of a closed session does not exist.
- **Keep sessions open in tmux and not hold ids.** It is closer to a person's own session. It needs startup dialogs
  answered, loses an initial prompt, and guesses idle from the screen. A later record takes it for attachable sessions.
- **A queue for a running worker.** A message is never lost. It makes the switchboard an inbox, which
  [0070](0070-delivery-is-a-push-to-a-connected-agent-with-no-inbox.md) rejected.
- **Let a worker record carry its own command.** It fits a new harness without code. It would let a stolen secret run
  any program, and a record is a poor place for that.
- **A scoped token for the turn's MCP process, and a different configuration folder.** It would keep the secret out of
  the turn's reach. It adds a fourth kind of token, and the turn can still read the owner's other files, so it limits
  one file and not the account.
- **Use the MCP handle as the worker's identity.** No new name. A handle changes each turn, so nothing could hold or
  trust it.
