---
category: architecture
status: accepted
approval: person
affects: [hub/server, cli/command]
builds-on: ['0020', '0070', '0080']
note: Drafted on 2026-11-12 after the registry was checked by hand. Sending from the TUI is left to a later record.
---

# 0090. Observers read the agent list, and the TUI is a separate binary

Builds on [0020](0020-messages-carry-no-user-authority.md),
[0070](0070-delivery-is-a-push-to-a-connected-agent-with-no-inbox.md), and
[0080](0080-the-hub-keeps-only-connected-agents.md).

## Context

The only ways to see who is registered are the agents' own tools and `landline agents --as <handle>`, which needs an
agent token. A person who runs several sessions has no single view of them. The wish is a terminal screen for the agent
list that a script can also read as JSON, and that can grow into a control surface.

`GET /v1/agents` takes an agent token. A viewer is not an agent. Giving it the hub secret would send the root credential
on every poll, and registering it as an agent would list the viewer among the agents it shows.

Landline has no third-party Go dependencies, and the nix build relies on that with `vendorHash = null`. A terminal
framework would be the first dependency.

## Decision

**An observer is a client that is not an agent.** `POST /v1/observers`, authorised by the hub secret, takes a `kind`
such as `tui` or `web`, a short text that follows the free-text field rules of the registry, and answers with an
observer token.

- An observer never appears in the agent list, holds no stream, and has no handle.
- An observer token is read-only. It may call `GET /v1/agents`, which answers with the same list and fields that an
  agent sees. Every other route refuses it with `403`.
- The hub holds observers in memory only, as it holds agents. An observer is forgotten after 2 minutes without a
  request, by the same lazy expiry, and its token then answers `401`. A client registers again on a `401`.
- A client uses the hub secret to register and to register again, and sends the observer token on every poll.

**The TUI is its own binary and its own Go module.** It lives in `tui/` with its own `go.mod` and builds `landline-tui`.

- It talks to the hub over HTTP and JSON as `docs/reference/delivery-protocol.md` describes, with its own small types,
  because one module cannot import the `internal` packages of another.
- It uses Bubble Tea. The `landline` module and its binary keep no third-party dependencies.
- `landline tui` runs `landline-tui` when it is on the path, and says how to install it when it is not. It finds the hub
  address and secret the way the other commands do.

**The first screen is the agent list.** It shows each agent's handle, label, harness, state, landline version, folder,
branch, model, and effort. `q` quits and `r` refreshes at once.

- It polls every 2 seconds. `--interval D` changes that, with a floor of 500 milliseconds. When a request takes longer
  than the interval, the next wait is twice that request's time, so a slow host or a remote hub does not pile up
  requests.

**Every screen has a JSON export.** `landline-tui agents --json` prints one snapshot of the screen as JSON and exits. It
is the same view model that the screen draws, so the two cannot differ. It prints no secret, and it exits non-zero with
a message on standard error when the hub cannot be reached. More screens later add more names.

**The first version only reads.** The TUI is meant to become a control surface, and sending a message from it is the
first control. A message from a person at a keyboard has a different sender than a message from an agent, and the marker
of [0020](0020-messages-carry-no-user-authority.md) says what a receiving agent may treat as an instruction. That needs
its own record, and the observer token gains no send right until then.

## Consequences

- The hub gains a second kind of token and a route that registers it. A leaked observer token shows the agent list, with
  handles, labels, folders, and branches, until it has been idle for 2 minutes. It cannot send, register, or read the
  log.
- The hub secret stays off the polling path. A viewer or a log that captured its requests holds only the observer token.
- A change to the shape of `GET /v1/agents` has to reach `tui/` as well. The protocol document is the contract, and a
  test of `landline-tui` against a real hub catches a drift.
- A second module needs more of the project's tooling. `just check` runs the module's tests, the flake builds it with
  its own vendor hash, the dependency scan covers its `go.mod`, the stack document gains a row, and `architecture.yaml`
  gains an entry for it. That file is protected, so a person approves the change.
- The acceptance scenarios gain observer registration, the `403` on every route but the read, the 2 minute expiry, and
  the JSON export against a real hub. A person approves them.
- A hub restart forgets observers. A running TUI gets a `401` on its next poll, registers again, and carries on.

## Alternatives considered

- **Accept the hub secret on `GET /v1/agents`.** No new route. The root credential would travel on every poll, and there
  would be no read-only role.
- **Register the viewer as an agent.** No new concept. The viewer would show in the list it displays, and its token
  could send.
- **A push stream for observers.** It saves the polling. It adds a second kind of stream to the hub before the first
  screen exists, so it can come when polling proves too slow.
- **A `landline watch` command in the main module, drawn with plain ANSI.** It keeps one module and no new tooling. It
  leaves resize, keys, and layout for Landline to write, and the TUI is expected to grow.
- **Export from a key inside the TUI.** It needs a terminal and a person, and gives scripts nothing.
