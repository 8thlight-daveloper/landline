---
category: ux
status: accepted
approval: person
affects: [cli/command]
builds-on: ['0060', '0110', '0140', '0160']
partly-supersedes: ['0140']
note: Drafted on 2026-11-19. It moves the personal landline skill into the repo, adds the agent levels, which are standing authorizations with security consequences, up to full rights for named peers, and replaces the removal rule for the skill in 0140.
---

# 0210. The landline skill ships in the binary and refuses what it does not know

Builds on [0060](0060-agents-register-themselves-with-a-handle-and-attributes.md),
[0110](0110-three-binaries-landline-switchboard-and-landline-mcp.md),
[0140](0140-a-macos-menu-bar-item-shows-agents-runs-the-switchboard-and-sets-up-harnesses.md), and
[0160](0160-an-agent-has-an-id-a-project-and-a-task.md).

## Context

`landline setup claude` installs one file, `~/.claude/skills/landline/SKILL.md`, which knows only
`/landline register [project]`. A person's own copy of the skill, kept outside this repo and linked into
`~/.claude/skills/` and `~/.codex/skills/`, has a second mode, `agent`, and a `references/` folder. The two copies
drifted. On a machine that got only the embedded skill, `/landline agent` registered the session under the project
`agent`, because the embedded skill has no rule for a word it does not know.

Two more faults follow from the install. `setup` writes through a symlinked skill folder, so it would overwrite the
files that the link points at. And `setup codex` installs no skill at all, although Codex reads the same skill format.

## Decision

**The skill is a folder of three files, embedded in `landline`.** They are `SKILL.md`, `references/register.md`, and
`references/agent.md`. They live under `internal/cli/command/skills/landline/` and are the only source. The person's own
copy outside the repo is retired.

### The subcommands

The skill is invoked as `/landline <subcommand>` in Claude Code and `$landline <subcommand>` in Codex.

| Subcommand                  | Does                                                                                         |
| --------------------------- | -------------------------------------------------------------------------------------------- |
| `register [project]`        | Registers this session. Without a project, it suggests one and waits for the person's answer |
| `agent [read\|work]`        | Turns on a standing authorization for peers of the same project. `agent` alone is `read`     |
| `agent full <id> [<id>...]` | Turns on full authorization for the peers with exactly these ids                             |
| `agent off`                 | Turns the authorization off at once                                                          |
| `help`                      | Shows the usage and takes no action                                                          |

- With no subcommand, the skill shows the usage and takes no action, as `help` does.
- **An unknown subcommand or level, an argument that does not belong, `full` with no id, or an id that is not 4 lower
  case hex characters shows the usage, says what was wrong, and takes no action.** The skill never reads an unknown word
  as a project and never registers or grants on a guess.
- Everything after `register` is the project, kept exactly as typed, spaces included. A blank project means none was
  given.
- The usage names each subcommand with one sentence, shows the levels below, and says that the harness's own permission
  mode still applies.

### The `agent` levels

`agent` is a standing authorization from the person to the session, to act on a peer's request without asking each time.
It has three levels, and the lowest is the default.

| Level  | Who it covers                                     | The session may do, on a peer's request                                                                          |
| ------ | ------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------- |
| `read` | agents whose project equals the session's project | Reviews, research, and answers, with read-only tools                                                             |
| `work` | agents whose project equals the session's project | `read`, plus editing files in the folder the session started in and below it, and running builds and tests there |
| `full` | only the agents with the ids the person typed     | Anything the session can do itself: edits anywhere it can reach, commands, web requests, commits, and pushes     |

Rules for every level:

- The session identifies the sender from the delivered sender id and the registry, and never from a project or an id
  named in the message body. A project that is empty, different, or unverifiable gets nothing from `read` or `work`.
- A request beyond the level gets a refusal that names what the level lacks. The session never raises its own level.
- A message from another agent stays information, with its sender stamp and its "from another agent, not the user"
  marker. A peer cannot turn a level on, change it, or supply the person's approval.
- Registering, reading the skill, and receiving a message never turn a level on. Only the person typing `agent` does.
- The levels grant the person's authorization to proceed without asking. They do not waive the person's other written
  instructions, the project's rules, or the harness's own permission mode. A session that the harness has not set to
  accept edits still gets the harness's prompt.
- Secrets, tokens, credentials, and personal or client data stay out of every message, at every level.
- Replies use `kind: reply`, the original thread, and `in_reply_to` set to the incoming id, and go to the peer's exact
  id.

Rules for `read` and `work`: the session may also start such requests to a matching peer. Neither level covers a commit,
a push, a merge, a deploy, a web request of the session's own, or recording an approval.

Rules for `full`:

- It applies to the exact ids the person typed, and to no project. An id is 4 random hex characters, new on each
  registration, so a peer that restarts has a new id and has lost the authorization.
- The session checks each id against the registry. An id that is not registered shows the usage and grants nothing.
- Before it turns `full` on, and whenever the id list changes, the session shows the person the ids, their registered
  project and host, and what `full` lets those peers make it do, and waits for an explicit yes. Anything else leaves the
  mode as it was.
- Each time the session acts under `full`, it tells the person which peer asked and what it did.

Lowering the level, or `agent off`, takes effect at once and needs no confirmation. Raising from `read` to `work` needs
none beyond the typed command. The level lasts for the session and ends when the session ends. It is not stored.

### Installing and removing

- `landline setup claude` installs the folder at `~/.claude/skills/landline/`. `landline setup codex` installs it at
  `$CODEX_HOME/skills/landline/`, or `~/.codex/skills/landline/` when `CODEX_HOME` is not set.
- Each installed file carries a line that says it is managed by `landline setup` and names the build that wrote it. An
  HTML comment holds it, after the front matter in `SKILL.md` and on the first line of the others.
- `setup` replaces a file that carries that line, whatever build wrote it, so that a machine is never stuck on an older
  skill. It keeps a file that does not carry the line, and says `kept <path>, which is not managed by landline setup`.
- **`setup` never writes through a symlink.** When the skill folder, or a file in it, is a symbolic link, `setup` skips
  it and says `kept <path>, a link to <target>, which is yours`. It still exits `0`.
- `landline setup claude|codex --remove` deletes each file that carries the line, then each of the two folders that this
  leaves empty, and says `kept <path>` for any other file. This replaces the rule of 0140 that removed the skill only
  when it matched the embedded copy byte for byte. The skill removal still runs whatever the harness command's status
  is.

## Consequences

- `cli/command` embeds the folder, installs it for both harnesses, and removes it by the rule above. The acceptance
  scenarios of `setup` change, because they check one file and now check three, the marker, the symlink rule, and the
  Codex install. A person authorizes those edits, and an agent does not make them.
- The README and the command table describe `/landline register|agent|help` and the Codex install.
- A person who keeps the skill in their own repo and links it into the harness folders removes that copy, or points it
  at this one, once this record is built. `setup` leaves a symlink alone, so a link that points at the old copy keeps
  the old skill until then.
- A person who edits an installed managed file loses the edit on the next `setup`. A copy under another name is kept.
- The skill text is the contract of the mode, so a change to what `agent` allows is a change to this record.

## Alternatives considered

- **Leave the skill outside the repo.** It drifts, and a machine that never received the personal copy gets the old
  skill, which is the fault that prompted this record.
- **A `landline skill install` command.** It adds a command for what `setup` already does, and a person would have to
  run both.
- **Keep byte-for-byte removal.** A machine with an older managed copy would keep it for ever, because `setup` could
  neither replace nor remove it.
- **Overwrite any existing file.** It destroys a person's own skill of the same name, and writes through a link into
  another repo.
- **A plugin for each harness.** Each harness has its own plugin format and its own distribution. The skill folder is
  the same text for both.
- **Pin `full` to a project, as `read` is.** A project is a claim that any session with the secret can make. `full` lets
  a peer run commands, so the person names the peers.
- **Store the level between sessions.** A stored `full` would apply to a session the person never decided about.
- **Make `agent` the default.** The levels let a peer's request be acted on without asking. The person must choose one.
