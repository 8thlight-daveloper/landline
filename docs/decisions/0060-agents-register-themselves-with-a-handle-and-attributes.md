---
category: behavior
status: proposed
approval: unrecorded
affects: [agent/registry, hub/server, mcp/server, cli/command, client/http]
builds-on: ['0010', '0020', '0040', '0050']
partly-supersedes: ['0020']
note: Decided in conversation on 2026-11-11 and not yet approved. It replaces the part of 0020 that has the person who holds the hub secret choose agent names.
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

**A harness session registers itself.** `landline mcp` registers on start, so the only setup is one line of harness
configuration that names the server. No `register`, `agent`, or `wire` step is needed in the normal case.

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

**The hub is found, and started, in a fixed order.**

1. The `LANDLINE_URL` environment variable.
1. The hub address in `~/.config/landline/`.
1. The default `127.0.0.1:8787`. Only this case may start a hub, and the hub it starts is a background process that
   binds to localhost, writes its secret to a private file, and outlives the session that started it. A lock keeps two
   sessions from starting two hubs.

A machine whose configuration names a remote hub never starts one of its own.

**There is one hub, and everyone uses it.** Hubs do not find or join each other. A second machine joins the first
machine's hub with `landline server join <url>`, which stores the address and the secret in a private file. The hub
stays on localhost and the second machine reaches it through an SSH tunnel, or the hub listens on a private network
address such as a Tailscale one and prints the warning of [0040](0040-hub-defaults-and-saved-state.md). There is no
automatic discovery.

**The hub secret is still the root credential.** A registering process reads it from its private file, so any process
that runs as the same operating system user can read it too. The earlier advice to keep it out of an agent's environment
was a speed bump and not a boundary, and the README says so.

## Consequences

- Setup is one line of harness configuration. A second machine needs one `server join`.
- An agent can impersonate another by registering a handle that looks like it. The hub assigns handles, and the header
  shows the attributes, so a peer reading a message can see the sender's real handle and folder. That is acceptable on
  one person's machine, and a person who needs more has to separate the agents by operating system user.
- Handles accumulate until they age out. A label is the stable name a person remembers.
- The registry gains the attributes and the handle, and the router resolves a name against the live list. Those are
  contract changes that a person approves.
- A harness that does not expose its model or effort leaves those fields empty.

## Alternatives considered

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
| Session id                       | `CLAUDE_CODE_SESSION_ID`                                                                | not available                                                                                          |
| Folder                           | working directory, and `CLAUDE_PROJECT_DIR`                                             | working directory                                                                                      |
| Environment passed to the server | the whole environment of the session                                                    | almost none: `HOME`, `PATH`, `USER`, `SHELL`, `TERM`, `TMPDIR`, and what the server's own config names |

- **Model is not exposed by either harness.** It stays an attribute that a person supplies, and it is empty otherwise.
- **Effort is exposed by Claude Code only,** as of the time the session started.
- **Codex gives no session id.** A Codex agent cannot register the wake target that the Codex wake needs, so a Codex
  wake keeps the manual `--wake-target`, and a Codex agent otherwise uses the inbox or `listen`.
- **Claude Code also exports `CLAUDE_CODE_MESSAGING_SOCKET` and `CLAUDE_CODE_MESSAGING_TOKEN` to the server.** The
  socket delivery strategy of [0050](0050-delivery-is-an-ordered-list-of-strategies.md) could therefore be built from
  inside the MCP process without a hook. Its message format is still undocumented.

**An attribute comes from the first source that has it, in this order:** a flag in the server's own arguments, then a
`LANDLINE_*` environment variable, then what the harness reports. A launcher can set the environment variables for
Claude Code, which passes its environment through. Codex does not, so a Codex attribute must come from the server's
arguments or from the `env` entries in its MCP configuration. This closes the question of how `landline claude` and the
MCP server agree.
