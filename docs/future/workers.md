# Workers: held sessions woken per message

A direction and not a decision. Nothing here is built. A draft record, `0130`, was written under a delegation and was
never approved. This note keeps the idea and the review findings, and the draft is a starting point if someone takes it
on. Set down on 2026-11-15.

## The idea

A worker is a stored session of one harness plus a folder, which the switchboard remembers by name. A message to the
worker starts one headless turn of that session, and the worker answers with `landline_send`. The harness commands are
`claude -p --resume <id>`, `codex exec resume <id>`, and `opencode run --session <id>`.

It lets a person have a reviewer or researcher on demand without keeping a terminal open. It is the only thing the
switchboard would save, so it partly supersedes [0080](../decisions/0080-the-hub-keeps-only-connected-agents.md).

In the TUI a worker would show as an indented row under its owner, with a state of idle or running. A worker cannot be
reached from a terminal until its session is opened in one.

## Compared with Claude Code subagents

The [subagent docs](https://code.claude.com/docs/en/sub-agents) describe the closest existing feature.

- A subagent is private to its parent session and runs on Claude only. A worker is addressable by any peer, works across
  harnesses, and outlives sessions.
- A subagent returns its final result to the parent. A worker has no parent waiting, so it replies by message.
- A subagent is a definition of a prompt, tools, and model. A worker, as drafted, is only a session id and a folder.
- Both start with no conversation history unless resumed, and no message from an agent counts as approval.

## Changes the review suggested

- A cap on turns running at once across all workers. The draft limits each worker to one turn and 20 turns an hour, but
  many workers can still run together.
- A maximum number of model turns in one wake, beside the wall clock timeout.
- A command to stop a running turn.
- A stated outcome for a headless turn that needs a permission prompt, which must fail and not stall.
- An alternative named in the record: use the harness's own subagents, rejected for the reach reasons above.

Two further ideas carry a stored-format or trust choice, so a person decides them: create a worker without a session id
and capture the id from the first turn, and let a worker name a subagent definition that lives in its folder.

## Consequences a person must approve

The switchboard would start processes and keep a stored file of session ids and folders. A worker runs as the owner, so
a policy limits accidents and not an attack. Both put the record in the person-approval list of `AGENTS.md`.
