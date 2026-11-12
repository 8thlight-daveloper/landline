---
category: behavior
status: proposed
approval: unrecorded
affects: [agent/registry, hub/server, mcp/server, cli/command, client/http]
builds-on: ['0010', '0020', '0040', '0050']
partly-supersedes: ['0020']
note: Decided in conversation on 2026-11-11 and not yet approved. It replaces the part of 0020 that has the person who holds the hub secret choose agent names.
partly-superseded-by: ['0070']
---

# 0060. Agents register themselves, with a handle and attributes

Builds on [0010](0010-one-go-program-over-http.md), [0020](0020-messages-carry-no-user-authority.md),
[0040](0040-hub-defaults-and-saved-state.md), and [0050](0050-delivery-is-an-ordered-list-of-strategies.md). It replaces
the part of 0020 that has the person who holds the hub secret choose agent names.

## Context

Setting up Landline took a run of commands per agent: start a hub, register a name, wire the project, launch the
harness. A harness that starts `landline mcp` can do all of it. The MCP server already holds a session and reads the
agent's mail. Registering by hand also assumes a name that a person picks, and a person who runs several sessions in
several folders, branches, and worktrees needs more than a name to tell them apart.

## Decision

**A harness session registers itself, in two parts that have different trust.**

- **Authentication** is done by the MCP process, not the model. `landline mcp` registers with the hub when the harness
  has finished its handshake, and keeps the token it gets in its own memory. The token never passes through the model's
  context, so the model cannot leak it or be asked for it.
- **Claims** are things only the model knows: its model name, its effort, a label, and a Codex session id. Every agent
  is told, in the server's instructions, to call one tool, `landline_register`, at the start of a session with the
  claims it knows. The hub shows them to peers as self-reported.

The only setup is one line of harness configuration that names the server. A person can still register a name by hand
with `landline register --name`, which is how scripts and tests register, and that name behaves as it did before.

**A registration gets a handle that is never reused.**

- The hub assigns the handle. It is a short unique id seeded from the folder name, such as `threnos-4k2`, and it is the
  address that `send` uses.
- A handle is never given to another registration. A restarted MCP process registers again and gets a new handle. The
  old handle leaves the list after a day, as [0040](0040-hub-defaults-and-saved-state.md) says.
- No registration can take over a handle, so a name that looks idle is not a name that anyone can claim.

**An agent has attributes, which are its own claim.**

- The first set is folder, git branch, worktree, harness, harness version, model, effort, and an optional custom label.
- The MCP server fills each one it can find from the environment, git, and its command line. A person can set the label
  and any other attribute with a flag. A value that cannot be found is left empty.
- Model and effort are set at registration and may change during the session. The registry shows them as claimed then.
- Every attribute follows the rules of the other free-text fields: at most 64 bytes, and no control character, line or
  paragraph separator, or invisible formatting character. A value that breaks the rules is refused.
- Attributes are for display. They appear in `landline agents` and in the header of a delivered message, and nothing
  routes by them. Finding the agent that works on a branch is the person's job, and the person then uses its handle or
  label.

**A name resolves to one live agent.** `send` accepts a handle, a label, or a prefix of either. It resolves when exactly
one live agent matches, and it returns the candidates when several do. A label is not unique, and the hub never picks
between two agents that share one.

**The hub is an explicit service, and no agent starts it.**

- `landline server up` runs the hub in the background, `server run` keeps it in the foreground, and `server install`
  sets it up to start at login: a launchd user agent on macOS and a systemd user unit on Linux. No agent session starts,
  owns, or stops the hub, so no session is special and no start race or lock is needed.
- `landline mcp` finds the hub from the `LANDLINE_URL` environment variable, then from the address in
  `~/.config/landline/`, then at the default `127.0.0.1:8787`. When the hub is not there, its tools return an error that
  says to run `landline server up`, and it tries again on the next call.

**There is one hub, and everyone uses it.** Hubs do not find or join each other. A second machine joins the first
machine's hub with `landline server join <url>`, which stores the address and the secret in a private file. The hub
stays on localhost and the second machine reaches it through an SSH tunnel, or the hub listens on a private network
address such as a Tailscale one and prints the warning of [0040](0040-hub-defaults-and-saved-state.md). There is no
automatic discovery.

**The hub secret is still the root credential.** A registering process reads it from its private file, so any process
that runs as the same operating system user can read it too. The earlier advice to keep it out of an agent's environment
was a speed bump and not a boundary, and the README says so.

**A Codex agent proves it controls the session it names.** `landline_register` takes a Codex `CODEX_THREAD_ID`, which
the agent reads from its shell tool's environment, because the MCP server cannot read it. The hub accepts only a UUID,
and does not trust it yet. It sends a one-time code to that thread through the Codex wake, and the wake text carries the
marker of [0020](0020-messages-carry-no-user-authority.md). The proof finishes when the agent calls `landline_register`
again with the code. An agent that names another session's id never sees the code, so it cannot finish. A code works
once, expires after five minutes, and allows five tries. When it finishes, the hub records the Codex wake method and
that id as the wake target for the agent's handle, and the Codex wake of [0030](0030-wake-adapters-per-harness.md) works
without a `--wake-target` flag.

**Registering is prompted when it is needed, and a skill lets the user ask for it.** The server's instructions ask an
agent to call `landline_register` before it uses the other tools, and every other tool adds a reminder to its result
until the agent has registered. The user can also type `/landline register [label]` in a session.
`landline setup claude` installs that skill, and it carries the Codex proof steps as well. An agent that never registers
still works, and cannot be woken if it runs Codex. The tool is called register and not attach, because the user said
that is the word that fits.

**An attribute comes from the first source that has it, in this order:** a claim the agent made through
`landline_register`, then a flag in the server's own arguments, then a `LANDLINE_*` environment variable, then what the
harness reports. A model name comes from the agent, because neither harness exposes it. A launcher can set the
environment variables for Claude Code, which passes its environment through. Codex does not, so a Codex attribute must
come from the server's arguments or from the `env` entries in its MCP configuration. This closes the question of how
`landline claude` and the MCP server agree.

## Consequences

- Setup is one line of harness configuration and one hub service. A second machine needs one `server join`.
- An agent can impersonate another by registering a handle that looks like it. The hub assigns handles, and the header
  shows the attributes, so a peer reading a message can see the sender's real handle and folder. That is acceptable on
  one person's machine, and a person who needs more has to separate the agents by operating system user.
- Handles accumulate until they age out. A label is the stable name a person remembers.
- The registry gains the attributes and the handle, and the router resolves a name against the live list. Those are
  contract changes that a person approves.
- A harness that does not expose its model or effort leaves those fields empty.

## Alternatives considered

- **The first session starts the hub.** It needs a start lock, ties the hub's life to one session, and makes that
  session special. An explicit service has none of those problems and costs one command.
- **Keep person-chosen names.** It needs a setup step per agent and cannot tell two sessions in one project apart.
- **Reuse a name once its agent is idle.** An idle name might belong to a session that is only waiting, so a new
  registration could take it by mistake. A fresh handle each time removes the question.
- **Route by attribute, such as `branch:fix-login`.** It saves a lookup and adds a query language and ambiguity rules.
  It can come later.
- **Federate hubs.** Two hubs would each hold a registry, a log, and read positions, and would need merging, ordering,
  and a trust decision for each message.
- **Find hubs with mDNS.** It would find a hub automatically and would let any process on the network advertise one.

## Findings from the prototype

A probe MCP server was launched by each harness on 2026-11-11, and recorded its environment and `initialize` request.

| Attribute                        | Claude Code 2.1.296                                                                     | Codex 0.162.1                                                                                          |
| -------------------------------- | --------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------ |
| Harness and version              | `clientInfo` (`claude-code`, `2.1.296`) and `AI_AGENT`, `CLAUDECODE` in the environment | `clientInfo` (`codex-mcp-client`, `0.162.1`)                                                           |
| Effort                           | `CLAUDE_EFFORT` in the environment                                                      | not available                                                                                          |
| Model                            | not available                                                                           | not available                                                                                          |
| Session id                       | `CLAUDE_CODE_SESSION_ID`                                                                | `CODEX_THREAD_ID` and `CODEX_SESSION_ID`, in the shell tool's environment only                         |
| Folder                           | working directory, and `CLAUDE_PROJECT_DIR`                                             | working directory                                                                                      |
| Environment passed to the server | the whole environment of the session                                                    | almost none: `HOME`, `PATH`, `USER`, `SHELL`, `TERM`, `TMPDIR`, and what the server's own config names |

- **Model is not exposed by either harness.** It stays an attribute that a person supplies, and it is empty otherwise.

- **Effort is exposed by Claude Code only,** as of the time the session started.

- **Codex has a session id, and the MCP server cannot read it.** Codex's shell tool sees `CODEX_THREAD_ID` and
  `CODEX_SESSION_ID`. A server that Codex launches does not, even when the server's `env_vars` setting names them. That
  setting does forward other variables. The agent can read its own id, so it passes the id to the server.

- **Claude Code also exports `CLAUDE_CODE_MESSAGING_SOCKET` and `CLAUDE_CODE_MESSAGING_TOKEN` to the server.** The
  socket delivery strategy of [0050](0050-delivery-is-an-ordered-list-of-strategies.md) could therefore be built from
  inside the MCP process without a hook. Its message format is still undocumented.

## Findings from the build

A real Claude Code 2.1.296 session was started with only `landline server up` and one MCP configuration entry, with no
`register` and no `wire` step.

- **Registration worked.** The session found the hub through the saved config and registered itself with a handle
  (`work-d21`), its folder, harness, harness version, and effort (from `CLAUDE_EFFORT`). A message from another agent
  woke it with nothing typed, and it replied.
- **The model did not call `landline_register` on its own.** The server's instructions tell every agent to call it at
  the start of a session. The session had no turn until a message woke it, and during that turn it answered the message
  and did not call it. Its model and label were therefore empty. Instructions alone do not make the claims reliable.
- **What follows.** A Claude Code launcher can set the label and the model without the model's help, from its own
  arguments. A Codex agent still has to register its session, because its wake needs the session id, and whether Codex
  follows the instruction has not been tested.

## Open questions

- Whether a single-use registration ticket, issued by a launcher and tied to a folder and a harness, should replace the
  shared hub secret for registration. It is deferred, because the shared secret limit above is already accepted.
