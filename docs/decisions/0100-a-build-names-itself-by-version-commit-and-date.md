---
category: behavior
status: accepted
approval: person
affects: [agent/registry, switchboard/server, mcp/server, client/http, cli/command, agent/viewer]
builds-on: ['0060', '0070', '0080', '0090']
partly-superseded-by: ['0110']
note: Drafted on 2026-11-12 to add to the attribute list of 0060, which stays as written.
---

# 0100. A build names itself by version, commit, and date

Builds on [0060](0060-agents-register-themselves-with-a-handle-and-attributes.md),
[0070](0070-delivery-is-a-push-to-a-connected-agent-with-no-inbox.md),
[0080](0080-the-hub-keeps-only-connected-agents.md), and
[0090](0090-observers-read-the-agent-list-and-the-tui-is-a-separate-binary.md).

## Context

[0060](0060-agents-register-themselves-with-a-handle-and-attributes.md) lists the attributes an agent registers. Its
list does not name the landline version, although registration carries it as `landline_version` and the agent list shows
it. The record is history and stays as written, so this record adds the attribute.

Every build reports `0.1.0`, because the number comes from the `VERSION` file and a person bumps that file by hand. Two
sessions started from different commits look the same in `landline agents`. When a fix lands in the hub or the MCP
process, nobody can tell which running session already has it.

## Decision

**A build carries three values, each its own field: version, commit, and date.**

- `version` is the content of `VERSION`, for example `0.1.0`. A person changes it for a release.
- `commit` is the full git commit hash that the build came from, 40 hex characters. When tracked files differ from that
  commit, the value ends in `-dirty`. Untracked files do not count. The stamping script and the nix expression treat a
  hash of any other length, such as a SHA-256 repository gives, as no commit, so a value they stamp never exceeds 46
  bytes. The program sends what the variable holds, and the hub enforces the field rules.
- `date` is the committer date of that commit in UTC, as RFC 3339 to the second, for example `2026-11-12T16:34:00Z`. It
  is not the time of the build, so two builds of one commit are identical. A build with a `-dirty` commit has no date. A
  date needs a clean commit, and the stamping script and the nix expression give both or neither.

**The build stamps the values with linker variables.** `main.version`, `main.commit`, and `main.date` are set with
`-ldflags -X`. The program reads no other source for them, and does not read the version control data that the Go tool
may add to a binary.

- `just build` and `just install` read them from git. The date is the committer date in UTC and not in the committer's
  offset. When git is missing, or the source is not a checkout, the commit and date are empty and the build still
  succeeds.
- The nix package reads them from the flake source: the commit is `rev`, else `dirtyRev`, else empty, and the date is
  `lastModifiedDate` (`yyyymmddhhmmss`) cut into the RFC 3339 form. By this rule it is empty for a dirty tree, although
  nix may offer a time for one.
- A plain `go build` sets none of them. The version reads `dev`, and the commit and date are empty.
- Nothing else sets them. For these two fields the source order of
  [0060](0060-agents-register-themselves-with-a-handle-and-attributes.md), with its flags and `LANDLINE_*` variables,
  does not apply. A flag, an environment variable, and a `landline_register` argument cannot override the commit or the
  date, so they stay a fact about the build and not a claim by a person.

**The MCP process registers all three.** `landline_version` stays as it is, and the registration gains `landline_commit`
and `landline_date`. A hand-registered name, from `landline register`, carries none of the three, as it carries no
landline version today.

- The process reports them, so a peer treats them as self-reported, like every other attribute. They follow the
  free-text rules of [0060](0060-agents-register-themselves-with-a-handle-and-attributes.md): at most 64 bytes, and no
  control character, line or paragraph separator, or invisible formatting character. A value that breaks the rules
  refuses the whole registration, as any other attribute does. An empty value is left out.
- The name form of `POST /v1/register` accepts the same two fields, and a registration by name replaces the earlier one,
  so registering again replaces the commit and the date, and an empty value clears the old one. A registration by seed,
  which an MCP process sends, gets a new handle each time and replaces nothing.
- `GET /v1/agents` returns `landline_commit` and `landline_date` for each agent, with the full values.
  `landline-tui agents --json` prints the same names.
- The attribute text of `landline agents` and of the `landline_agents` tool gains `commit: <short>` and `built: <date>`
  after `landline: <version>`. The short commit is its first 7 characters, followed by `-dirty` when the value ends in
  it. The TUI screen shows the same short commit and no date, and its export carries the full commit and the date.
- The delivered message and its sender stamp do not change.
- `landline version` prints the banner and then one line. It reports the installed binary and not a running hub.
  - Clean: `landline 0.1.0 (commit 0123456789abcdef0123456789abcdef01234567, built 2026-11-12T16:34:00Z)`
  - Dirty: `landline 0.1.0 (commit 0123456789abcdef0123456789abcdef01234567-dirty)`
  - No commit: `landline 0.1.0`, and a date without a commit is not printed
  - Plain `go build`: `landline dev`
- A session that still runs a binary from before this record registers no commit and no date, and shows neither.

**The hub reports no build of its own, and the TUI binary is not stamped.** Only an MCP process registers a build. A
person finds the build of an installed hub with `landline version` on its machine, and runs `landline server restart` so
the running hub matches the installed binary.

## Consequences

- Two sessions from different commits differ in `landline agents`, so a person can see which sessions to restart after
  an install.
- `internal/model` gains two fields, and the protocol document gains two names on the register and agent routes.
  `internal/model` is protected, so a person authorizes that change. The registry contract gains two attributes, and the
  contract folder is protected too. The TUI keeps its own types and gains the same two fields.
- The hub decodes a request with unknown fields refused. An MCP process of this version that registers with a hub of the
  last version gets a `400`. A person restarts the hub with `landline server restart` before the sessions that carry
  this change.
- The hub keeps no saved agents, as [0080](0080-the-hub-keeps-only-connected-agents.md) says, so no stored format
  changes.
- The stamping reads git in one script that `just build` and `just install` both run. A check runs it in a temporary git
  repository and asserts the values for a clean commit, a tracked change, only an untracked file, and no repository. The
  nix package gets a check that compares `landline version` of its build with `self.rev` and `self.lastModifiedDate`,
  the inputs it reads. When `self.rev` is absent, the check expects `dirtyRev` and no date. Only the script check
  compares against git, because a nix sandbox has none.
- The acceptance scenarios gain these checks. The scenario build sets known values with the linker variables.
  - A session registers. `GET /v1/agents` and `landline-tui agents --json` show the exact full commit and date, and
    `landline agents` shows `commit: <first 7 characters>[-dirty]` and `built: <date>`.
  - The commit matches `^[0-9a-f]{40}(-dirty)?$`. The date matches the RFC 3339 form with `Z` when the commit has no
    `-dirty`, and is absent when it has.
  - `landline version` prints each of the forms above for the matching linker values, and a dirty build prints no date.
  - The `landline_agents` tool text and the TUI screen show `commit:` before `built:` and the first 7 characters of the
    commit with `-dirty` after them, and the screen shows no date.
  - A bad commit or date, with a control character or longer than 64 bytes, is refused. Over raw HTTP the hub answers
    `400` and the agent list is unchanged. Through an MCP process built with that value, no agent appears in the list
    and the tool call returns the hub's error.
  - A registration by name with the same fields, sent over raw HTTP, replaces both, and an empty value clears them.
  - A hand-registered name shows neither.
  - A build with no values shows `dev` and no commit or date.
  - A person approves the scenarios.
- A build from a dirty tree is marked, so a session that runs unreviewed code is visible. The mark says nothing about
  what the changes were.
- A private build sends its commit hash and its dirty mark to the hub, and the hub shows them to every agent that lists
  agents. For a public build they say which version of a public project runs. The hub is the person's own, as the other
  attributes are.

## Alternatives considered

- **Stamp `0.1.0+abc1234` into the version.** One field and no model change. A script that wants the commit has to parse
  it, and the version is no longer the release number alone.
- **Bump `VERSION` by hand on each change.** No tooling change. It depends on someone remembering, and the number says
  nothing about a commit.
- **Read the commit and date from the version control data that the Go tool adds.** No justfile or flake change. The nix
  build runs on a source without `.git`, so it would report nothing, and the two paths would differ.
- **The time of the build as the date.** It says when a person built. It makes two builds of one commit differ, and the
  nix build stops being reproducible.
- **A hub build field in `GET /v1/agents`.** It would show a hub and agent mismatch in one place. It adds a field and a
  contract change for a case that `landline version` on the hub's machine answers, so it can come when that proves slow.
- **Edit the attribute list in 0060.** The record is history. A later reader should see what was decided in 0060 and
  what this record added, and an edit would hide the difference.
