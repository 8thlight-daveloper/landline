# Menu bar menu text

The text that `landline-menubar menu --text` prints.
[0140](../decisions/0140-a-macos-menu-bar-item-shows-agents-runs-the-switchboard-and-sets-up-harnesses.md) adds the flag
so that a test can read the menu without a screen. The acceptance tests in `acceptance/menubar_test.go` pin this format
byte for byte. The screen and the text come from one menu model, so the screen shows the same items in the same order.

`landline-menubar menu --text` registers as an observer of kind `menubar`, polls once, prints the text to standard
output, and exits `0`. An unreachable switchboard is a state, so it exits `0` too. It prints nothing to standard error
when it succeeds, and never prints the secret or the observer token. Two runs against the same state print the same
bytes, because the text holds no time and no random value.

## Lines

| Line                    | Meaning                                                      |
| ----------------------- | ------------------------------------------------------------ |
| `icon: jack`            | Reachable, and no agent is connected                         |
| `icon: jack N`          | Reachable, and `N` agents are connected. `N` is at least 1   |
| `icon: jack dimmed`     | The switchboard is unreachable                               |
| `<title>`               | A menu item that is enabled                                  |
| `<title> (disabled)`    | A menu item that is disabled                                 |
| `---`                   | A separator                                                  |
| two spaces, then a line | An item of the submenu whose title line comes just before it |

The first line is always the icon line. Every other line is one menu item, in menu order. A submenu's title line is
enabled and has no suffix. Its items follow it, indented two spaces more than the title. A separator inside a submenu is
indented the same way. Text from an agent or from the switchboard goes through the sanitising of the TUI, which turns
each control character, line separator, paragraph separator, and invisible formatting character into one space, so no
value adds a line to the text.

## The whole menu

```text
icon: <state>
<agent rows, or the unreachable line>
---                                  (only when there is at least one line above it)
Switchboard
  <first line> (disabled)
  ---
  Start
  Stop
  Restart
  ---
  <switchboard build line> (disabled)
Harnesses
  Set up Claude Code
  Remove from Claude Code
  Set up Codex
  Remove from Codex
---
<menubar build line> (disabled)
About Landline
Refresh
---
Quit
```

`Start`, `Stop`, and `Restart` each end in ` (disabled)` when they are disabled.

## Agent rows

A reachable switchboard lists its agents in the order of the TUI: connected first, then the agents with a label before
those without, then by label, then by handle. A row is a submenu.

```text
<label, or the handle when there is no label> [<marker>]
  Handle: <handle> (disabled)
  Harness: <harness> (disabled)
  Host: <host> (disabled)
  Model: <model> (disabled)
  Effort: <effort> (disabled)
  Folder: <folder> (disabled)
  Branch: <branch> (disabled)
  Landline version: <landline version> (disabled)
  ---
  Copy handle
```

- The marker comes from the same state values as the TUI (`agentsview`): `disconnected` for an agent that is listed and
  not connected, and `active` for a connected agent. The state field of the TUI has no `idle` value today, so `idle`
  appears only when that field says so.
- A field the agent did not register is left out. The title line shows no folder and no branch.
- `Copy handle` is the only enabled item of the submenu.
- The count in the icon line is the number of connected agents. An agent that is not connected does not count.

When the switchboard is unreachable there are no rows. The line `Switchboard unreachable (disabled)` takes their place.

Any failure to get the agent list is the unreachable state: a connection error, a `401` after one registration again, a
`5xx`, a body that is not the JSON list, and a missing secret. The text is then the unreachable menu, with
`icon: jack dimmed`, exit `0`, nothing on standard error, and no secret or token anywhere in the output. The build line
is then `Switchboard (build unknown)`, and `Start`, `Stop`, and `Restart` follow the `Not running` rule.

## The Switchboard submenu

The first line is one of these, always disabled.

| Condition                                                                 | First line                             |
| ------------------------------------------------------------------------- | -------------------------------------- |
| The address is not a loopback address                                     | `Remote switchboard at <address>`      |
| Loopback, and `landline` is found neither next to the app nor on the path | `landline not found. Run just install` |
| Loopback, and the switchboard answered                                    | `Running at <address>`                 |
| Loopback, and the switchboard did not answer                              | `Not running`                          |

The address is `LANDLINE_URL`, or the `url` of `~/.config/landline/config.json`. A loopback address has a host of
`localhost`, or an IP address that is loopback, such as `127.0.0.1`, `127.0.0.2`, or `[::1]`. `landline` is looked for
next to the app's own binary, then on the path.

`Start` is enabled only when the first line is `Not running`. `Stop` and `Restart` are enabled only when it is
`Running at <address>`. In every other case all three are disabled.

The build line is one of these, always disabled.

| Condition                                                 | Build line                               |
| --------------------------------------------------------- | ---------------------------------------- |
| `GET /v1/build` answered with a version and a commit      | `Switchboard <version> (<short commit>)` |
| It answered with a version and an empty commit            | `Switchboard <version>`                  |
| `GET /v1/build` failed with a `404`, a `5xx`, or bad JSON | `Switchboard (build unknown)`            |
| The switchboard is unreachable                            | `Switchboard (build unknown)`            |

An unstamped switchboard answers the version `dev`, so its line is `Switchboard dev`. The short commit is the first 7
characters of the commit after sanitising, followed by `-dirty` when the commit ends in it. A failure of `GET /v1/build`
alone changes nothing else in the menu. ` (differs from this app)` follows the closing parenthesis only when both the
switchboard's commit and the menu bar app's own commit are non-empty and differ, so the line reads
`Switchboard 0.2.0 (a1b2c3d) (differs from this app) (disabled)`. When either commit is empty, including a `dev`
switchboard against a stamped app, it is never shown.

## The build line of the app

`Menubar <version> (<short commit>)`, `Menubar <version>` when the build has no commit, and `Menubar dev` for a plain
`go build`. It is always disabled. It never shows a date.

## Flags

`--interval D` is accepted before and after `menu --text`. The floor of 500 milliseconds is enforced in text mode too,
so a smaller value exits `2` with the usage on standard error. A valid value changes nothing in the text.

## What the text cannot show

`menu --text` runs once and clicks nothing. The spawn of `landline server` and `landline setup` commands,
`About Landline`, `Copy handle`, the slow-request backoff, the template image, and the Dock behavior are not reachable
through it. The builder's own unit tests cover them.

## Where the app looks for the switchboard

`LANDLINE_URL` and `LANDLINE_SECRET` first, each on its own. When one is empty, the `url` or the `secret` of
`~/.config/landline/config.json` stands in for it, as it does for the TUI.

## Worked examples

A reachable switchboard on `127.0.0.1:8787`, both sides built from commit `a1b2c3d4e5f60718293a4b5c6d7e8f9012345678`,
two connected agents and one that is not, with `landline` next to the app:

```text
icon: jack 2
amy-docs [active]
  Handle: alpha (disabled)
  Harness: claude-code (disabled)
  Host: mac (disabled)
  Folder: /work/docs (disabled)
  Branch: main (disabled)
  ---
  Copy handle
charlie [active]
  Handle: charlie (disabled)
  Harness: claude-code (disabled)
  Host: mac (disabled)
  ---
  Copy handle
delta [disconnected]
  Handle: delta (disabled)
  Harness: claude-code (disabled)
  Host: mac (disabled)
  ---
  Copy handle
---
Switchboard
  Running at http://127.0.0.1:8787 (disabled)
  ---
  Start (disabled)
  Stop
  Restart
  ---
  Switchboard 0.2.0 (a1b2c3d) (disabled)
Harnesses
  Set up Claude Code
  Remove from Claude Code
  Set up Codex
  Remove from Codex
---
Menubar 0.2.0 (a1b2c3d) (disabled)
About Landline
Refresh
---
Quit
```

A reachable switchboard with no agents. No separator follows the icon line, because no line comes before it:

```text
icon: jack
Switchboard
  Running at http://127.0.0.1:8787 (disabled)
  ---
  Start (disabled)
  Stop
  Restart
  ---
  Switchboard dev (disabled)
Harnesses
  Set up Claude Code
  Remove from Claude Code
  Set up Codex
  Remove from Codex
---
Menubar dev (disabled)
About Landline
Refresh
---
Quit
```

Nothing listens on `127.0.0.1:8787`, with `landline` next to the app:

```text
icon: jack dimmed
Switchboard unreachable (disabled)
---
Switchboard
  Not running (disabled)
  ---
  Start
  Stop (disabled)
  Restart (disabled)
  ---
  Switchboard (build unknown) (disabled)
Harnesses
  Set up Claude Code
  Remove from Claude Code
  Set up Codex
  Remove from Codex
---
Menubar 0.2.0 (a1b2c3d) (disabled)
About Landline
Refresh
---
Quit
```

A switchboard on another machine, which does not answer. `Start`, `Stop`, and `Restart` are disabled whatever the
answer:

```text
icon: jack dimmed
Switchboard unreachable (disabled)
---
Switchboard
  Remote switchboard at http://192.0.2.10:8787 (disabled)
  ---
  Start (disabled)
  Stop (disabled)
  Restart (disabled)
  ---
  Switchboard (build unknown) (disabled)
Harnesses
  Set up Claude Code
  Remove from Claude Code
  Set up Codex
  Remove from Codex
---
Menubar 0.2.0 (a1b2c3d) (disabled)
About Landline
Refresh
---
Quit
```

A local switchboard that answers, with `landline` neither next to the app nor on the path:

```text
icon: jack
Switchboard
  landline not found. Run just install (disabled)
  ---
  Start (disabled)
  Stop (disabled)
  Restart (disabled)
  ---
  Switchboard 0.2.0 (a1b2c3d) (disabled)
Harnesses
  Set up Claude Code
  Remove from Claude Code
  Set up Codex
  Remove from Codex
---
Menubar 0.2.0 (a1b2c3d) (disabled)
About Landline
Refresh
---
Quit
```

An agent whose handle is `al<ESC>[31mpha<newline>br` shows as one line, with each control character as a space. Here
`<ESC>` is the escape character and `<newline>` is a line feed:

```text
al [31mpha br [active]
  Handle: al [31mpha br (disabled)
```
