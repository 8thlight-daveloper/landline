---
category: ux
status: accepted
approval: person
affects: []
builds-on: ['0020', '0080', '0090', '0100', '0110']
note: Drafted on 2026-11-16 and approved by its owner the same day. It adds a fourth binary and a third-party dependency, and nothing is built from it yet.
---

# 0140. A macOS menu bar item shows agents, runs the switchboard, and sets up harnesses

Builds on [0020](0020-messages-carry-no-user-authority.md), [0080](0080-the-hub-keeps-only-connected-agents.md),
[0090](0090-observers-read-the-agent-list-and-the-tui-is-a-separate-binary.md),
[0100](0100-a-build-names-itself-by-version-commit-and-date.md), and
[0110](0110-three-binaries-landline-switchboard-and-landline-mcp.md).

## Context

The TUI shows the agent list, but only while a terminal is open and focused. A person who runs several sessions wants to
know that the switchboard is up and how many agents are connected without switching windows. A menu bar item on macOS is
always in view and costs no screen space.

The TUI direction is read-only, as [read-only TUI and hook events](../future/read-only-tui-and-hook-events.md) says.
[Writing from the TUI](../future/writing-from-the-tui.md) lists why sending waits. A menu bar item adds no reason to
send, so this record keeps to reading.

The same item is where a person would start, stop, and restart the switchboard and check which build is running. Today
that takes a terminal and `landline server`. After `just install` replaces the binaries, nothing says whether the
running switchboard is the new build. `landline version` reports the installed binary only, as
[0100](0100-a-build-names-itself-by-version-commit-and-date.md) says, and the switchboard has no route that reports its
own build.

Adding Landline to a harness is also a terminal job: `landline setup claude|codex` adds it, and nothing removes it. A
person who stops using Landline, or moves it, edits the harness's own configuration by hand.

0110 fixed three binaries. A menu bar item needs Cocoa, which the other binaries must not carry. The root module has no
third-party dependencies, and the `tui` module builds on Linux as well as macOS.

## Decision

**The menu bar item is its own binary, `landline-menubar`, in its own Go module under `menubar/`.** It is macOS only.
This adds a fourth binary to the three of 0110, and the other three are unchanged.

- It requires the root module with a `replace`, as the `tui` module does, and imports no `tui` package, because Go does
  not let one module import another module's `internal` packages. It has its own small client and view types, and a test
  pins them to `docs/reference/delivery-protocol.md`.
- It uses one systray library, with menus only and no web view. The stack document names the library and its version.
  Its dependencies stay inside `menubar/`. The root module and the `tui` module gain none.
- It runs with no Dock icon and no window. When the library cannot do that from a bare binary, the build wraps the
  binary in an app bundle that sets `LSUIElement`.

**It reads like the TUI and nothing more.** It finds the switchboard address and secret the way
[0090](0090-observers-read-the-agent-list-and-the-tui-is-a-separate-binary.md) and the TUI do: `LANDLINE_URL` and
`LANDLINE_SECRET` first, then `~/.config/landline/config.json`. A menu bar item that a person starts from Finder or at
login has no environment, so the file is the usual source.

- It registers as an observer with `kind` `menubar`, sends the observer token on every poll, and registers again on a
  `401`. The token may call `GET /v1/agents` and nothing else.
- It never shows, logs, or writes the secret or the token.
- It has no send or attach item. A later record adds those, and the observer token gains no right here. The server items
  below run commands and do not use the token.

**The icon is the Landline telephone jack, `assets/jack.svg`, with the count of connected agents beside it.**

| State                           | Icon                            |
| ------------------------------- | ------------------------------- |
| Switchboard unreachable         | The mark, dimmed, with no count |
| Reachable, no agent connected   | The mark, with no count         |
| Reachable, `N` agents connected | The mark and `N`                |

An agent that is listed but not connected, as [0080](0080-the-hub-keeps-only-connected-agents.md) allows, does not
count. The icon is a template image: the jack's shape in black on a transparent background, at 22 and 44 pixels, which
the grid of the drawing scales without blur. macOS tints it for light and dark menu bars, so the brand blue does not
show. The stack document says how the images are made from the SVG. The count is the title text next to the image.

**The menu lists agents in the order the TUI uses**: connected first, then label, then handle.

- A row shows the label, or the handle when there is no label, and a marker for the state: active, idle, or
  disconnected. A row shows no folder or branch, so a screen share of the open menu does not reveal them.
- Each row opens a submenu with the fields the agent registered: handle, harness, host, model, effort, folder, branch,
  and landline version. Each field is a disabled item.
- The submenu has one action, `Copy handle`, which puts the handle on the clipboard so a person can paste it into
  `landline send`. It copies nothing else.
- After the rows come a `Switchboard` submenu, a `Harnesses` submenu, a disabled line with the menu bar app's own build,
  `About Landline`, `Refresh`, and `Quit`. When the switchboard is unreachable there are no rows, and the menu opens
  with a disabled line, `Switchboard unreachable`.
- Text from an agent goes through the same sanitising as the TUI before it reaches a menu item, so a handle or label
  cannot inject control characters or a long line.

**The `Switchboard` submenu runs the server and shows its build.**

- Its first line is disabled and reads `Running at <address>` or `Not running`.
- `Start`, `Stop`, and `Restart` run `landline server up`, `landline server down`, and `landline server restart`, with
  no flags. The menu bar app does not start or stop a process itself. `landline` finds the switchboard by its pid file
  and command name and owns the launchd and systemd units, as
  [0110](0110-three-binaries-landline-switchboard-and-landline-mcp.md) says, so the menu and the terminal act on the
  same process.
- `Start` is enabled when the switchboard is unreachable. `Stop` and `Restart` are enabled when it is reachable. Every
  item is disabled while one of them runs, and a command that runs 30 seconds is stopped and counted as failed.
- When the address is not a loopback address, the three items are disabled and the first line reads
  `Remote switchboard at <address>`, because `landline server up` starts a local process.
- The command's output is discarded. A non-zero exit shows `Failed: <first line of standard error>` in the first line
  until the next successful poll, with the line passed through the TUI's sanitising. After any command the app polls at
  once.
- It finds `landline` next to its own binary, then on the path. A login item has a short path, so the first place
  matters. When it finds none, the three items are disabled and the first line says to run `just install`.
- `Restart` has the effect that [README](../../README.md) states: every agent registers again and gets a new handle, and
  the old flags are not remembered. A person who runs the switchboard with its own flags starts it from a terminal.
- A disabled line shows the switchboard's build as `Switchboard 0.2.0 (abc1234)`. It reads `Switchboard dev` for an
  unstamped build, and `Switchboard (build unknown)` when the switchboard predates the route below. It adds
  ` (differs from this app)` when the commit differs from the menu bar app's own.

**The switchboard reports its own build.** `GET /v1/build` takes the secret or an observer token and answers
`{"version": "0.2.0", "commit": "<40 hex>", "date": "<RFC 3339>"}` from the values that
[0100](0100-a-build-names-itself-by-version-commit-and-date.md) stamps, with an empty string for a value the build
lacks.

- It is read-only and answers the same fields that an agent already sees in its own registration, so it adds no secret
  and no new kind of token. An agent token gets `403`. The protocol document gains the route.
- A switchboard from before this record answers `404`, and the menu bar app treats that as an unknown build.

**The menu bar app reports its own build the same way.** It shows `Menubar 0.2.0 (abc1234)` as a disabled line, with the
short commit of 0100: the first 7 characters, and `-dirty` when the value ends in it. Its `version` command prints one
line with the full commit, as the other binaries do. The one `VERSION` and the one stamp of 0110 cover it, so the four
binaries of one build show the same version and commit.

**`landline setup` gains `--remove`.** The harnesses are the two it supports, Claude Code (`claude`) and Codex
(`codex`). Adding is the `landline setup` that exists today, which runs `claude mcp add` or `codex mcp add`. Both
harnesses also have `mcp remove`, which `landline` does not call yet.

- `landline setup claude|codex --remove` runs `claude mcp remove --scope user landline` or `codex mcp remove landline`
  and exits with that command's status. For `claude` it then deletes `~/.claude/skills/landline/SKILL.md` only when the
  file is byte for byte the skill that `setup` installs, and removes the `landline` folder when that leaves it empty. A
  file that differs stays, and `setup` says so on standard output and still exits `0`. `--print` shows the harness
  command and changes nothing, as it does for the add.
- Any other argument exits `2` with the usage. `--remove` takes `--print`, and nothing else.
- Removing the registration changes new sessions only. A running session keeps its MCP process until it exits.
- Another harness is added to `setup` and `--remove` together when it has a delivery path or a send path, as
  [other harnesses](../future/other-harnesses.md) describes. This record adds none.

**The `Harnesses` submenu runs those two commands.**

- It has four fixed items: `Set up Claude Code`, `Remove from Claude Code`, `Set up Codex`, and `Remove from Codex`. The
  menu does not ask the harness whether Landline is registered. `claude mcp get` and `codex mcp get` start the server to
  check its health, and a started `landline-mcp` registers an agent with the switchboard, so a status check would add an
  agent to the list it displays.
- A set up item runs `landline setup claude` or `landline setup codex`, and a remove item adds `--remove`. They use the
  same lookup of `landline`, the same one-at-a-time rule, the same 30 second limit, and the same
  `Failed: <first line of standard error>` line as the `Switchboard` submenu. The line after a success reads
  `Claude Code set up. Start a new session.` or `Claude Code removed.` A harness that is not installed fails, and the
  line says so.
- A login item has a short path, so the app asks the person's login shell for its `PATH` once at start, with
  `$SHELL -l -c 'printf %s "$PATH"'`, and gives that path to every command it runs, the server commands included. The
  shell receives that one fixed string and no other input. When the shell fails, the app keeps its own path.

**`About Landline` opens the standard macOS About panel.**

- The panel shows the name `Landline`, the telephone jack in the brand blue `#0071CE` as the icon, and the version as
  `Version 0.2.0 (abc1234)`, with the same short commit as the menu line. An unstamped build shows `Version dev`.
- Its credits are two lines: `From 8th Light` and a link to the project's GitHub page. The panel's own link handling
  opens the page in the person's browser, and the app runs nothing for it.
- The brand blue is right here and wrong in the menu bar. The panel draws a full colour image, and the menu bar draws a
  template image, as above. The icon is a 440 pixel PNG made from `assets/jack.svg`, which is 40 times the drawing's
  grid. The stack document says how.
- The repository address is one constant in one file of `menubar/`. Moving the project to the 8th Light organization
  means editing that line and the links in the README, and the `Moving the repository` note in the stack document lists
  both.
- The panel needs a few lines of Objective-C through cgo, in the menu bar module, because the systray library has no
  About panel. They add no dependency. The app is brought to the front for the panel and goes back to no Dock icon when
  it closes.
- No other 8th Light mark is used. The brand rules place the 8\* mark and the logotype in set corners of a layout, and a
  panel with two lines of credits has no such layout.

**It polls every 5 seconds.** `--interval D` changes that, with the floor of 500 milliseconds and the slow-request
backoff that 0090 sets for the TUI. `Refresh` polls at once. The menu is rebuilt only when the list changed, so an open
menu does not flicker.

**It does not notify.** A connect or disconnect makes no banner and no sound in this version. A person who wants alerts
waits for the hook events in the future note, which carry what an agent is doing and not only whether it is there.

**Install and run.** `just build` builds `landline-menubar` on macOS and skips it elsewhere, and `just install` installs
it beside the others. The flake exposes the package for Darwin systems only, and `default` includes it there. A person
starts it by hand or adds it as a login item. No `landline` command starts it in this record, and the menu bar app
starts nothing but `landline server`.

## Consequences

- `architecture.yaml` gains a component for the menu bar screens, a driver that depends on nothing, as `agent/viewer`
  does. It is protected, so a person approves it. This record's `affects` then names that component, in the same commit.
- A new third-party dependency enters the project, in a module that only macOS builds. `just check` on Linux and the CI
  Linux jobs skip the module, and a macOS job runs its tests, `go vet`, and the dependency scan. The stack document
  gains a row and the flake gains a Darwin-only package and `vendorHash`.
- The binary needs cgo and the macOS SDK. A person cannot cross-build it from Linux.
- `switchboard/server` gains the `GET /v1/build` route, and `client/http` or the menu bar app's own client calls it. The
  protocol document and the acceptance scenarios change, and a person approves the scenarios. The `affects` line of this
  record names `switchboard/server` as well when the component exists for the menu bar screens.
- The menu bar app runs commands as the person, which any login item can do. It runs `landline server` with three fixed
  verbs and `landline setup` with two fixed harness names and one fixed flag, and no argument from the switchboard or an
  agent, so nothing a message says reaches a command line.
- `landline setup` changes the harness's user configuration, and `--remove` deletes a skill file, only when the file is
  unchanged from the installed one. A person who edited the skill keeps it. `setup` run again restores a removed
  registration. `cli/command` gains the flag, so its contract and the acceptance scenarios change, and a person approves
  them. The README and the usage text gain it.
- `Stop` and `Restart` drop every delivery stream, and `Restart` gives every agent a new handle. The items are in a
  submenu so a click does not reach them by accident, and the app asks for no confirmation because a menu has no dialog.
  A person who wants one waits for a record that adds a dialog.
- The About panel is the first place where Landline names 8th Light and its repository to a person who did not write it.
  The wording, `From 8th Light`, and the address are the owner's to confirm, and a move to the organization changes
  them.
- The client and view types are written twice, here and in `tui/`. A change to `GET /v1/agents` reaches three places:
  the root client, `tui/`, and `menubar/`. The protocol document is the contract, and one test per module catches a
  drift.
- The acceptance suite gains a scenario that drives `landline-menubar` against a real switchboard through a flag that
  prints the menu as text and exits, so no test needs a screen. That flag is `landline-menubar menu --text`, and it
  prints the icon state and the menu items, one per line. A person approves the scenarios.
- An unsigned binary built on the owner's machine runs without Gatekeeper trouble. A binary downloaded from elsewhere
  would need signing and notarising, which this record does not take on. The project has no outside users.
- Any other person on the machine who can read `~/.config/landline/config.json` already holds the secret. The menu bar
  item adds no new exposure.

## Alternatives considered

- **A Swift app.** It is the native choice and allows rich menus, popovers, and notifications. It adds a second
  toolchain, `xcodebuild` does not fit the Nix sandbox cleanly, and the project would have two languages for one
  screen's worth of work. Revisit when the item needs a custom view or notifications.
- **A subcommand of `landline`, in the `tui` module.** One binary fewer. It puts Cocoa in the module that builds on
  Linux, so a build tag would split it, and the TUI would carry a dependency it never uses on a terminal.
- **Put the code in the root module.** The root has no third-party dependencies, and the other two binaries would build
  against a library they never import.
- **Share the TUI's client by moving it to a package that both modules import.** It removes the duplication. The screen
  packages under `tui/internal/` deliberately import no root package, and 0110 kept that boundary. A shared module is a
  larger change than one more small client.
- **Poll only while the menu is open.** It saves requests. The icon count would go stale, and the count is the point.
- **Notify on connect and disconnect.** It is cheap to add. Agents come and go all day, so banners would be noise, and a
  person turns them off. Hook events would give a better signal.
- **Show the folder and branch on each row.** It helps a person tell two sessions of one harness apart. The label does
  that, and the open menu is a common thing to show on a shared screen.
- **Start and stop the switchboard from the menu app's own code.** It would not need `landline` on the path. It would
  copy the pid file and launchd handling of `landline server`, and the two could disagree about which process is the
  switchboard.
- **Read the switchboard's build from `landline-switchboard version`.** It needs no new route. It reports the binary on
  disk, which is the stale one after an install, and not the process that runs.
- **Put the build on `GET /v1/agents`.** It needs no route. It mixes a fact about the switchboard into a list of agents
  and changes a shape that three clients parse.
- **Remove with `claude mcp remove` and no skill step.** It is simpler. It leaves a `/landline` skill that points at a
  server that is gone.
- **Delete the skill file whatever it holds.** It leaves nothing behind. A person who edited the skill would lose the
  edit with no warning.
- **Show `set up` or `remove` by the harness's own status.** The menu would match the state. `mcp get` starts the
  server, which registers an agent, and a config file has no stable format to read. Two fixed items need neither.
- **Make `setup` a toggle.** One verb is shorter. A script that means to add would remove after a second run.
- **Confirm before `Stop` and `Restart`.** A dialog would need a second library or Cocoa code. The submenu is the guard
  for now.
- **A custom About window with the 8th Light logo.** It would show the full wordmark. It needs a window and layout code
  that the standard panel gives for free, and a library or Cocoa views for a window the person opens once in a while.
- **A menu item that opens the GitHub page, with no panel.** It is the least code. It leaves the version and the
  branding out, and the menu already shows the version.
- **Stamp the repository address at build time.** It would let the flake supply the address after a move. It adds a
  fourth linker variable that 0100 does not list, for one string that changes once.
- **The brand blue icon in the menu bar.** It matches the mark. A coloured image cannot adapt to a dark menu bar, and a
  template image is what the other menu bar items on the machine use.
