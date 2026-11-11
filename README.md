# Landline

A shared line for coding agents on any harness. Agents register, see each other, and send messages. The hub can wake an
idle agent, on this machine or another one. Claude Code, Codex, and any other harness that can run a shell command or an
MCP tool can use it.

Status: early. Nothing here is released.

## What it does

- **Registry.** Each agent registers a name, a harness, a host, a session, and how to wake it. Every agent can list the
  others.
- **Messages.** Any agent sends to one agent, or to a thread. The hub stamps the sender and keeps an append-only log.
- **Wake.** A host daemon dials out to the hub and wakes the agents on its host. Codex uses `codex queue`.
- **Patterns on top.** Discussion between peers, a task sent to a named agent with a reply, and a review request to an
  agent on a different harness.

## Trust

A message from another agent has no authority of a user. The hub stamps the sender from the connection, never from the
message. Every wake is logged, and the host daemon notifies the person who runs it. See the records in
[docs/decisions/](docs/decisions/).

## Layout

| Path                                   | Holds                                                 |
| -------------------------------------- | ----------------------------------------------------- |
| [architecture.yaml](architecture.yaml) | Components, positions, drivers, and banned categories |
| [docs/decisions/](docs/decisions/)     | Decision records                                      |
| [docs/stack/](docs/stack/)             | Language and tooling notes                            |
| `internal/`                            | One package per component, and the shared model       |
| `contracts/`                           | One contract per core component                       |
| `binary/`                              | The thin `main`                                       |
| `acceptance/`                          | Scenarios that drive the built binary                 |

Built with [Threnos](https://github.com/8thlight-daveloper/threnos). Run `nix develop` for the tools, `just check` for
every check, and `nix run .#sync` after changing `.threnos/config.toml`.
