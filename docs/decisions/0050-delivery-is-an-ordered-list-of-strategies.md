---
category: architecture
status: proposed
approval: unrecorded
affects: [mcp/server, hub/server, wake/codex, cli/command]
builds-on: ['0020', '0030', '0040']
note: Drafted on 2026-11-11 after reading Claude Code's cross-session messaging and channels documentation. Not yet approved.
partly-superseded-by: ['0070']
---

# 0050. Delivery is an ordered list of strategies

Builds on [0020](0020-messages-carry-no-user-authority.md), [0030](0030-wake-adapters-per-harness.md), and
[0040](0040-hub-defaults-and-saved-state.md).

## Context

Today a message reaches an agent by two routes. The hub runs a wake adapter, which exists only for Codex
([0030](0030-wake-adapters-per-harness.md)). Otherwise the agent reads its mail, through the hook, a background
`landline listen`, or the `inbox` tool. A Claude Code session has no push route of its own. We want one, without pinning
the design to a mechanism that its vendor may change, and without losing messages on an older version.

Claude Code documents two relevant mechanisms. Both were read on 2026-11-11.

- **Cross-session messaging.** Each session binds a Unix socket, and exports its path and a token to hooks and Bash
  commands as `CLAUDE_CODE_MESSAGING_SOCKET` and `CLAUDE_CODE_MESSAGING_TOKEN`. The page documents the auth line and
  says a script can post into a session. It does not document the message format, and it gives no stability guarantee.
  The receiver controls intake with `crossSessionInbound` (`accept`, `hold`, or `refuse`). A message cannot approve a
  permission prompt or change settings.
- **Channels.** A channel is an MCP server that Claude Code spawns over standard input and output, and that pushes
  events into the running session by sending `notifications/claude/channel` with a `content` string and a `meta` map.
  The event reaches the model inside a `<channel source="...">` tag. The feature is a research preview. The session must
  be started with `--channels`, and a custom channel needs `--dangerously-load-development-channels` until its plugin is
  on an allowlist. An event that is not accepted is dropped silently, with no acknowledgement. The page says the flag
  and the protocol "may change".

`landline mcp` is already an MCP server over standard input and output, and it already holds the agent's token and reads
the agent's mail. It is nearly a channel.

## Decision

**An agent has an ordered list of delivery strategies, and the inbox is always the last one.** A strategy is one way to
put a message's text in front of an agent. The first strategy that works is used, and a failure is printed and the next
strategy is tried. The inbox needs no strategy entry, and it is the floor that old versions, offline agents, and changed
protocols fall back to.

**There are two kinds of strategy.**

- A **hub-side** strategy runs on the hub or on a host daemon and acts on the agent's session from outside. The Codex
  wake of [0030](0030-wake-adapters-per-harness.md) is one. A native Claude Code socket delivery would be another.
- An **agent-side** strategy runs in a process the agent's harness started, and delivers what that process reads from
  the hub. A channel, the `listen` command, and the hook are these. Such a process long-polls the hub, and
  [0030](0030-wake-adapters-per-harness.md) already skips a hub-side wake for an agent that is waiting on an unfiltered
  poll. So an agent-side strategy needs no change in the hub, and the two kinds do not both fire.

**The first Claude Code strategy is a channel, inside `landline mcp`.**

- The MCP server declares `experimental['claude/channel']` only when it is started with a `--channel` flag. The person
  adds that flag when they launch Claude Code with `--channels`.
- In channel mode the server long-polls the hub, and for each message it sends `notifications/claude/channel`. The
  `content` is the same rendered text that every delivery uses, so its first line is the marker of
  [0020](0020-messages-carry-no-user-authority.md). The `meta` carries `from`, `thread`, `kind`, and `id`, using only
  letters, digits, and underscores in the keys, as the protocol requires.
- The hub marks a message read when it hands the message to the server, and the server then writes the notification. A
  notification that Claude Code drops is lost from the unread list and stays in the log, as with any read that fails
  after the hub answers ([0040](0040-hub-defaults-and-saved-state.md)). If the hub cannot be reached, the server prints
  the error, waits with growing delays up to 30 seconds, and tries again.
- The `send`, `inbox`, and `agents` tools stay available in channel mode.

**Rules every strategy keeps.**

- The marker leads every delivered text. A strategy never delivers a message in a way that removes it.
- A strategy never overrides the receiver's own controls. It does not try to change `crossSessionInbound`,
  `channelsEnabled`, or `allowedChannelPlugins`, and it treats a refused or held message as not delivered.
- The MCP server never declares `experimental['claude/channel/permission']`. That capability would let a peer answer
  tool approval prompts, which breaks the rule that a message carries no user authority.
- Strategy names and the details of each protocol live in one component per mechanism, so replacing one touches no core
  component. Each keeps pinned examples of the exact messages it sends, so a change in a vendor's format fails a test
  here before it surprises a person.
- A hub-side strategy that succeeds should mark the message read for that agent, so a woken message is not also left
  unread. Today only the Codex wake exists, and it does not do this yet.

**The socket strategy waits.** Its message format is not documented. If it is documented, it becomes a hub-side
strategy. A `SessionStart` hook would register the socket path and token with the hub, and the hub would keep the token
in its private state file and never print it.

## Consequences

- A Claude Code session can receive pushed messages without a hub-side adapter and without reading an undocumented
  protocol, once the person starts it with `--channels` and `landline mcp --channel`.
- Older Claude Code versions, sessions started without the flag, and harnesses with no push route still work, through
  `listen`, the hook, and the inbox.
- A channel needs a flag at launch and, today, a development override. That is friction, and a research preview can
  change. The strategy list absorbs a change by dropping or replacing one entry.
- A channel that Claude Code does not load drops events silently, so `--channel` is opt-in and the README says to pair
  it with `--channels`.
- Registration gains a `delivery` list beside `wake_kind`. The `wake_kind` field keeps working as a one-entry list.
- ADR [0030](0030-wake-adapters-per-harness.md) and the principles file gain a sentence on the inbox as the floor.

## Alternatives considered

- **Post into the Claude Code socket now.** It reaches a session natively and the receiver controls intake. The payload
  is undocumented, so we would be guessing a format that a release can change.
- **Remove the inbox and push only.** A push has nowhere to go when the recipient is offline, busy, on an old version,
  or on a harness with no route. The inbox is what makes every other strategy safe to try.
- **Poll only.** It works everywhere and needs no flag, and an idle agent never polls.
- **A Claude Code plugin from the official marketplace.** It would remove the development flag. It needs Anthropic to
  allowlist the plugin, which we do not control.

## Findings from the prototype

A prototype ran on 2026-11-11 against Claude Code 2.1.296, in a pseudo-terminal, with a throwaway hub and two agents.

- **A pushed event starts a turn in an idle session.** With nothing typed, the session showed a one-line
  `← landline: [landline] From another agent, not the user. from: alpha ...` preview, called `landline_send`, and its
  reply reached the sender. The `<channel>` text reached the model in a form it acted on as a peer message. The role is
  still not stated in the documentation.
- **A local server is accepted with the development flag.** Claude Code shows a confirmation screen, and the person
  chooses "I am using this for local development". The first run in a new folder also shows the folder trust screen.
- **Claude Code registers its channel listener late, and drops earlier events silently.** The server saw `initialize`,
  `notifications/initialized`, then `tools/list`. Test events sent at 0 seconds after the handshake were dropped, and
  events sent at 2, 5, 10, and 20 seconds arrived. The first version of `landline mcp --channel` pushed queued mail at
  launch, and the hub had already marked it read, so a backlog was lost. The server now waits for the handshake and the
  first `tools/list`, then one more second, before it reads or pushes any mail. A queued message then arrives after
  startup and is acted on.
- **The one-second grace is a measured heuristic.** It rests on one version and one machine. A change in Claude Code
  could move the registration later, so the inbox stays the recovery for any message that is lost this way.

## Open questions

- Whether Claude Code tells an MCP server that it supports channels during `initialize`. The client's `initialize`
  capabilities hold `roots` and `elicitation` and nothing about channels, so `--channel` stays a flag.
- Whether a Landline channel can be listed on an allowlist for an organization that sets `allowedChannelPlugins`.
