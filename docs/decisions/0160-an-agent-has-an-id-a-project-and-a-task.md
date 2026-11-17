---
category: behavior
status: accepted
approval: person
affects: [agent/registry, message/router, switchboard/server, mcp/server, client/http, cli/command, agent/viewer, menubar/viewer]
builds-on: ['0060', '0080', '0090', '0140']
partly-supersedes: ['0060', '0080']
note: Drafted on 2026-11-16. It renames handle to id and label to project everywhere, changes how a send resolves a name, adds the task attribute and a route for it, and rewrites what the register tool tells an agent. It has privacy, security, and behavior consequences, and its owner approved it.
---

# 0160. An agent has an id, a project, and a task

Builds on [0060](0060-agents-register-themselves-with-a-handle-and-attributes.md),
[0080](0080-the-hub-keeps-only-connected-agents.md),
[0090](0090-observers-read-the-agent-list-and-the-tui-is-a-separate-binary.md), and
[0140](0140-a-macos-menu-bar-item-shows-agents-runs-the-switchboard-and-sets-up-harnesses.md). It partly supersedes
0060, where `send` accepts a label or a prefix and a label is "the stable name a person remembers", and 0080, where a
send prefers the one connected agent that holds a label. Records before this one say handle, name, and label. They are
history, and this record says which words replaced them, as
[0110](0110-three-binaries-landline-switchboard-and-landline-mcp.md) did for "hub".

## Context

A session that registered itself on 2026-11-16 chose a label of `landline-menubar-coordinator`. The register tool said a
label is "a short name your user gave you". Nobody had given it one, so it invented a name that put the task in the
label. The owner's rule is different: the label is the project, shared by the agents that work on it, and an agent
approves a read-only request from another only when their labels match. The tool never said so, and nothing told the
session afterward that its label matched no one.

Four facts follow. The label already is not unique, and 0060 says so. An agent's `folder` already names the project in
most sessions. `send` takes a label or a prefix as a name, so a shared label fails as ambiguous and a unique one works
until a second agent starts, and the same command then behaves differently from one day to the next. And the handle
blurs the line further. The switchboard seeds it from the folder name, so it looks like `landline-f0d` and says the
project a second time, and a default label from the folder would be a prefix of every handle.

The words are also inconsistent. The wire calls an agent's address `name`, the screens call it `handle`, and the label
means a project while its name suggests a nickname. The agent list says nothing about what an agent is doing, so two
agents of one project look alike.

## Decision

**Three words name three things.** An agent has an id, a project, and a task.

| Old words    | New word  | What it is                                   |
| ------------ | --------- | -------------------------------------------- |
| handle, name | `id`      | The address of one registration              |
| label        | `project` | The group of agents that work on one project |
| (none)       | `task`    | The outcome the agent is working toward now  |

- The rename covers every place that carries a word, with no alias for an old one, because a word in two places is the
  failure that caused this record. The project has no users outside its authors, as 0110 recorded.
- The JSON keys `name` and `label` become `id` and `project` in `POST /v1/register`, `POST /v1/attach` and its answer,
  `GET /v1/agents`, and the TUI's export. `landline agents --json` rows and the menu text use `id` and `project`. The
  `to` and `from` of a send, and the same keys in the log, keep their names, and their values are ids.
- The flag `landline register --name` becomes `--id`, `--label` becomes `--project` on `landline claude`,
  `landline-mcp`, and `landline register`, and `LANDLINE_LABEL` becomes `LANDLINE_PROJECT`. `--as <id>` keeps its name
  and takes an id. The arguments of `landline_register` are `model`, `effort`, `project`, and the Codex pair, the
  argument of `landline_send` that names the recipient stays `to`, and the new tool takes `task`.
- The attribute text of `landline agents` and `landline_agents` says `project:` where it said `label:`. The TUI's
  columns are `ID`, `PROJECT`, and `TASK`, and the menu bar's `Handle`, `Copy handle`, and `Label` items become `ID`,
  `Copy id`, and `Project`.
- The domain model's `Agent.Name` becomes `ID` and `Agent.Label` becomes `Project`, and the registry's error for a bad
  name says `agent id`. An unknown field in a request already answers `400`, and an unknown flag already fails, so an
  old client fails loudly.

**A project is the name of the group, and agents that share it are colleagues.** The register tool's description of
`project` is:

> The name of the project you are working on, shared with the other agents on that project, for example `landline`.
> Agents with the same project are colleagues: they may exchange read-only requests, and other agents and your user see
> the project in the agent list. Do not put a task or a role here. If you give none, Landline uses the name of your
> project's repository folder.

- When no project is given by a flag, an environment variable, or the register tool, the MCP process derives one from
  the repository, so a session started in a linked worktree or in a subfolder gets the project's name and not the name
  of the folder it started in. In a git repository the project is the base name of the folder that holds the main
  working tree, which is the parent of the directory that `git rev-parse --git-common-dir` names when that directory is
  called `.git`, and otherwise that directory's base name without a `.git` suffix. Outside a repository it is the base
  name of the working folder. A base name that is empty, `.`, or `/` gives an empty project. The switchboard does not
  know where a project came from.
- A project is an agent's own claim. A match between two projects is a convenience for grouping and proves nothing about
  who holds them. A skill that approves work on a matching project keeps its own rule for that, and a default project
  from a common folder name such as `app` can match an unrelated repository.
- The server's instructions and the reminder an unregistered agent gets say the same in a sentence: the project names
  the group. The `/landline register [project]` skill and the README describe it this way.

**An id is an id and says nothing else.** The switchboard assigns each registration an id of 4 lowercase hexadecimal
characters, such as `a3f9`.

- The switchboard draws the id at random and draws again until the value is not the id of any agent it lists, including
  one in its grace period. The registry's rules for an id are the old rules for a name: 1 to 32 lowercase letters,
  digits, `-`, or `_`, and not `all`. It still refuses a duplicate, and an id that a person registers by hand with
  `landline register --id` is still valid.
- A registration by seed still asks for an id. The text of the seed no longer shapes it, so the MCP process may stop
  sending the folder as one. The request and its `400` for giving both or neither do not change.
- Everything else about an id holds as it did for a handle: it is never given to another registration while the
  switchboard lists the agent, a restarted process gets a new one, and once the switchboard forgets an agent the id may
  be issued again.
- The project is in `project`, the place is in `folder`, and the work is in the task. The id names the registration and
  nothing more.

**A send takes an id and nothing else.** The registry resolves a name only by exact id.

- The registry's `Resolve` is removed from its contract, because `Find` already looks up an exact id among all agents.
  The switchboard uses `Find`. An exact id of an agent in its grace period answers `409` as today, and an id the
  switchboard has forgotten answers `404`. The `409` for a name that matches more than one agent has no case left and is
  removed with its text.
- No prefix resolves, and no project resolves, whether one agent or ten hold it. A send to any other value answers `404`
  with the existing text for an unknown recipient. A person finds an id with `landline agents`. The descriptions of
  `landline_send` and of its `to` argument say `id` where they say `name` today.
- When the value equals the project of at least one agent that is connected, the switchboard adds a hint to the `404`
  text of an unknown recipient: `<value> is a project, not an agent. Send to one of: <ids>`. The ids are the connected
  holders of the project other than the sender, sorted, at most 10, with `, and more` after the tenth when there are
  others. The hint is added only to that `404`, and not to the `422` for an unknown sender. It comes from the
  switchboard's own lookup and not from the registry. A project held only by agents that are not connected, or only by
  the sender, gets the plain `404`. The status is `404` in every case, so a command does not change behavior when a
  second agent starts.
- `all` stays an unknown recipient with the plain `404`, as
  [0070](0070-delivery-is-a-push-to-a-connected-agent-with-no-inbox.md) left it, and the hint is never added for it,
  even when an agent has the project `all`.

**A task is what an agent is working toward.** It is an attribute like the others, in the agent's own words. The new
`landline_status` tool takes one argument, `task`, and its description is:

> The outcome you are working toward, in a short phrase you would say to a teammate, for example
> `menu bar app for ADR 0140`. Set it when you start work and change it when your goal changes, not when a step ends: if
> it would change when one test passes, it is too low. It is not the project, which is your `project`, and not a plan.
> Everyone connected and your user can read it, so put in no secrets, no client names, and no personal data. At most 64
> bytes. An empty task clears it.

- **A new route carries it.** `POST /v1/status` takes an agent token and a JSON body with one field, `task`, which must
  be present and a string. An empty string clears the task. A body without `task`, with `"task": null`, or with a value
  that is not a string answers `400`, and so does an unknown field. It answers `200` with `{"task": "<text>"}` and, when
  the task is not empty, `"task_at": "<RFC 3339 time>"`. After a clear the answer has `"task": ""` and no `task_at`.
  `POST /v1/attach` is not used, because an empty claim means no change there, so it cannot say a clear.
- The task follows the free-text rules of the registry: at most 64 bytes, and no control character, line or paragraph
  separator, or invisible formatting character, and empty means none. The registry's check of an agent's text fields
  covers `Task`, as it covers `Project`, so every registration of the entry validates it. A value that breaks the rules
  is refused with `400`.
- The switchboard keeps it with the agent in memory, as it keeps every attribute, and stamps the time of a change
  itself. Setting the text the agent already has is not a change, answers `200` with the existing time, and stamps
  nothing. On the wire `task_at` is a string in RFC 3339 that is left out when there is no task, and inside the
  switchboard it is a time that the wire form omits when zero. The agent does not supply it. A registration by id
  replaces the agent's entry, so it clears the task. The switchboard forgets the task when it forgets the agent, and a
  restarted switchboard holds none.
- The domain model's `Agent` gains `Task` and `TaskAt`. The delivered message's header and its sender stamp do not
  change and do not carry them.
- Nothing routes by a task, and no agent acts on another's task. It is for display, as every attribute is.
- Activity, which is what the agent is doing right now, is not a task and waits for the hook events work in
  [the future note](../future/read-only-tui-and-hook-events.md).

**The register tool says what it recorded and what that means.** `POST /v1/attach` answers with one more field,
`project_peers`, the number of other agents that are connected and hold the caller's exact project. The caller is not
counted, and an agent that is not connected, including one in its grace period, is not counted. A caller without a
project gets `0`, and the MCP process says there is no group. The MCP process composes the result of `landline_register`
from that number, the project it sent, and where it got the project. The result is one line such as:

`you are a3f9. project: landline (from your repository), shared with 2 other agents currently connected. task: none. Set one with landline_status when your goal is clear.`

- The origin is `(from your repository)` or `(as you gave it)`.
- The count reads `shared with N other agents currently connected`, `shared with 1 other agent currently connected`, or
  `shared with no other agent currently connected`, because a session that starts at the same moment may not have opened
  its stream yet. The switchboard computes `project_peers` for every answer of `POST /v1/attach`, including the
  challenge calls for Codex. When a call to `landline_register` sends no project, the MCP process reports the project it
  already holds and remembers, in its own memory, whether that project came from the repository or from the person. With
  no project the line says `no project, so you are in no group`.
- `landline_status` answers `task: <text>` or `task cleared`. A successful call does not count as the introduction that
  the unregistered reminder waits for. A session that renews its token or follows a switchboard restart registers again
  with no task, and the register line then says `task: none`, so it sets the task again.
- The unregistered-agent reminder and the server instructions name `landline_status`, and a call to it before the
  process has registered gets the same reminder as the other tools.

**The task shows beside the project in the lists, and nowhere else.** `GET /v1/agents` returns `task` and `task_at` for
an agent that set one. `landline agents` prints `task: <text>` in its attribute text after `project`, and so does the
`landline_agents` tool, which already marks every attribute as the agents' own claim. The TUI gains a `TASK` column
after `PROJECT`, and its JSON export carries `task` and `task_at`. The menu bar item gains a `Task: <text>` line in an
agent's submenu, after `ID`. A delivered message's header does not show it, so a task is read by an agent that chooses
to list agents and never arrives as part of a message. None of the lists shows the age yet. A change to display it needs
a record of its own, because it makes the text depend on the time.

## Consequences

- Ids change shape from `landline-f0d` to `a3f9`, and `handle`, `name`, and `label` become `id` and `project` in the
  wire, the flags, the environment, the tools, the lists, and the screens. The README, the `/landline` skill, the server
  instructions, the fixtures and tests that use the old words or expect the folder in an id, and the acceptance
  scenarios that read an id's shape or an old key change. The agent list shows an id, a project, and a task, so a person
  reads who an agent is from the project and the task and not from the id.
- Anything that used the old words stops working until it changes: a script that sends to a project or a prefix, a
  config that sets `--label` or `LANDLINE_LABEL`, a reader of the JSON keys `name` or `label`. The Codex skill that
  approves a request on a matching label is outside this repository, and the owner changes it to match on `project`.
- A person or script that ran `landline send tutor`, or `landline send land`, now gets `404`, with a hint for a project,
  and uses `landline send <id>`. After a switchboard restart every agent has a new id, and no name stays stable. The
  README's advice to use a label for an address that stays is removed. This is the cost of a rule that does not change
  when a second agent starts. Removing prefixes goes beyond the owner's rule for labels. With 4 character ids a prefix
  saves little typing and brings back an ambiguous case.
- The `agent/registry` contract changes: `Resolve` is removed, the field check of an agent's text fields covers `Task`,
  and the words change from name and label to id and project. Its rules, fixtures, and tests change, including the tests
  that pin a label tie and a label or handle prefix, the limits on text fields, and the wiring test. Mutation runs on
  the new rules, and the accepted-survivor lists are checked. A person approves the contract change.
- The `message/router` code reads an agent's name today, and it reads the id after this record. Its tests change with
  the renamed field.
- The tests that send by label, by prefix, or to a grace-period agent by label change. They include the acceptance
  scenarios in `acceptance/landline_test.go` that send `--to` a label or a prefix, one of them after a switchboard
  restart, and unit tests in the switchboard's `server_test.go` that pin label and prefix resolution and a `409` for a
  label held only by a disconnected agent, which becomes `404`. Acceptance is a protected unit, so a person approves its
  change.
- The protocol document changes: the renamed keys, the new route, `project_peers` on the attach answer, `task` and
  `task_at` on the agents list, the `404` text and hint, and the paragraph on the order of resolution, which is replaced
  and not extended. `client/http`, the TUI client, and the menu bar client each change the keys and gain the fields, and
  the drift tests that pin them to the protocol document change.
- The domain model renames `Name` to `ID` and `Label` to `Project` on `Agent` and its wire types, and gains `Task` and
  `TaskAt` there, `ProjectPeers` on the attach answer, which is written even when it is `0`, and a status request and a
  status answer. The text of `AgentInfo.Attributes()` says `project:` and gains `task:`. `Render`, which writes the
  header of a delivered message, changes only for the renamed field. The MCP server's `Hub` interface and the HTTP
  client each gain a `Status` call. The model is protected, so a person approves the whole list.
- The MCP server gains one tool, `landline_status`, and new descriptions for `landline_register`, `landline_send`, the
  server instructions, and the unregistered reminder. The skill that lists the tools and says to send to "a handle, a
  label, or a prefix" is rewritten. A harness that has cached the tool list sees the change after a restart.
- A task is free text that every agent and every observer reads through the lists, so it is the likeliest place for a
  client name or a secret to leak, and it is a place an agent can write text another agent reads. The 64 byte limit, the
  sentence in the tool text, and the choice to keep it out of message headers lower the chance. They do not remove it.
  The log does not hold tasks, because they are not messages.
- A project that defaults to the repository's name puts that name into the agent list for every agent, where `folder`
  already puts a folder's name. The default also makes accidental matches likelier. Before, a label existed only when
  someone chose it, and now every agent has one, so `app` in two unrelated repositories matches. A project proves
  nothing, and any authorization that treats a match as consent is the decision of the skill that does so. That skill,
  which the owner uses for Codex, is not in this repository, so this record cannot change it. This is a security
  consequence, and a person approves it knowing that the default widens what a project match covers.
- Existing sessions that registered a label of their own keep running until they restart, and they register again under
  the new words.

## Alternatives considered

- **Rename only the screens and the tool text.** It is the smaller change. An agent that reads `label: landline` in the
  JSON or the tool output and sees PROJECT on a screen meets the same mismatch that caused this record, so the words
  change everywhere or nowhere.
- **Call the group `group`.** It is short. It suggests a team or a hand-made set that can cross projects, and the
  default is the repository's name, so `project` says what it is.
- **A separate `project` attribute, and keep the label as the remembered name.** It keeps the label as an address. It
  adds a second attribute that says nearly what `folder` says, and the owner's rule and the Codex skill already use the
  label for the project.
- **A project stays addressable when exactly one agent holds it.** It keeps a stable address across restarts for one
  agent. The same command fails the day a second agent joins, which is the behavior that confused sessions before.
- **Keep the id seeded from the folder name.** It reads well in a list. It repeats the project, and it makes a default
  project a prefix of the id, which blurs the id, the group, and the task into one name.
- **Keep unique-prefix matching of the id.** It restores the typing shortcut, as git has for short ids. With 4
  characters there is little to shorten, and it brings back the `409` for several matches, so the id stays exact.
- **Carry the task on `POST /v1/attach` or on `landline_register`.** It adds no route. Attach treats an empty claim as
  no change, so a task could not be cleared, and a tool named "register" is a poor place to change what you are doing
  every hour.
- **A task on every message.** It travels with the work. It repeats the text, puts it in the log, and has the sender
  write it each time.
- **A task in the message header.** The recipient would see what the sender is doing without listing agents. It would
  also put agent-written text in the part of a message that the switchboard otherwise writes, which is the part a
  recipient trusts for who sent it.
- **A task that agents update every few seconds.** It gives a live view. Agents forget, a stale line misleads, and the
  text reaches every viewer. Harness hooks give that signal as metadata, which is the activity direction.
- **Warn on a project that matches no one, and say nothing otherwise.** It would have caught the first session. A result
  that always says what the project means teaches the rule to the agent that got it right too.
