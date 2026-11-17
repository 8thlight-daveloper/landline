---
category: behavior
status: accepted
approval: person
affects: [cli/command, menubar/viewer]
builds-on: ['0040', '0090', '0140']
note: Drafted on 2026-11-16 and approved by its owner on 2026-11-17. It adds a command and a menu item that replace the switchboard secret and restart the switchboard. It has security and destructive consequences.
---

# 0170. A person rotates the switchboard secret with one command or one menu item

Builds on [0040](0040-hub-defaults-and-saved-state.md),
[0090](0090-observers-read-the-agent-list-and-the-tui-is-a-separate-binary.md), and
[0140](0140-a-macos-menu-bar-item-shows-agents-runs-the-switchboard-and-sets-up-harnesses.md).

## Context

`landline server up` makes the switchboard secret once and saves it in `~/.config/landline/config.json`. Nothing makes a
new one afterward. `landline server restart` reads the saved value and keeps it, because `ensureLocalConfig` makes a
secret only when the file has none. A person whose secret was exposed, for example by printing the config file in a
shared terminal or a transcript, can replace it only by editing the file by hand and then restarting.

The secret opens the register, log, and observer routes of the switchboard. Agent tokens and observer tokens live in the
switchboard's memory, so a restart already ends every one of them and every agent registers again, as 0140 says.

## Decision

**`landline server rotate-secret` replaces the secret and restarts the switchboard.** It takes the flags that
`landline server restart` takes, and passes them to the new switchboard.

1. It checks that it can proceed, before it changes anything. It refuses, prints the reason on standard error, and exits
   `1` in each of these cases:
   - The saved address is not a loopback address. A switchboard on another machine is not this machine's to restart, and
     `landline server up` already refuses it.
   - `LANDLINE_SECRET` is set and not empty in the command's environment. The environment overrides the file for every
     `landline` command, so a new value in the file would change nothing for them and would leave the person believing
     the old secret was gone.
   - `landline-switchboard` cannot be found, as `landline server restart` already checks.
1. It stops the switchboard if one is running. It does not fail when none is.
1. It saves a new secret in `config.json` with mode `0600`, made the way the first one is, and keeps the saved address.
1. It starts the switchboard, as `landline server up` does.
1. It prints one line, `rotated the switchboard secret. Every agent must register again.`, and exits `0`. It never
   prints the old secret or the new one, and no message of this command includes either.

If step 3 fails, the switchboard stays stopped, the file keeps the old secret, and the message says so. If step 4 fails,
the file holds the new secret, the switchboard is stopped, and the message tells the person to run `landline server up`.
The order is stop, save, start, so that the running switchboard never holds a secret different from the file's.

**The old secret stops working at once. There is no period in which both work.** The switchboard keeps no secret but the
one it started with.

**A restart is the whole of what an agent has to do.** Every agent registers again and gets a new id, as it does after
any restart. The MCP process and the TUI register again on a `401` with the secret they read from the file. A session
whose own environment sets `LANDLINE_SECRET` keeps the old value and gets `401` until the person restarts that session.
This record adds nothing to force or announce it.

**The menu bar item has a `Rotate secret…` action in the `Switchboard` submenu, after `Restart`.**

- It is enabled and disabled under the same conditions as `Restart`, and is disabled while any server action runs.
- When the switchboard's address is not a loopback address, `Rotate secret…` is disabled, as `Start`, `Stop`, and
  `Restart` are under 0140, and the first line of the submenu reads `Remote switchboard at <address>`. Every server
  command item of the menu is disabled for a remote switchboard, and a later record that adds one keeps that rule.
- Choosing it opens a confirmation. Its text is
  `Rotate the switchboard secret? The switchboard restarts and every agent must register again.` Its buttons are
  `Rotate` and `Cancel`, and `Cancel` is the default. Nothing runs until the person chooses `Rotate`.
- It runs `landline server rotate-secret`, with no flags, as a child process under the rules that 0140 gives the other
  server actions: the output is discarded, a command that runs 30 seconds is stopped and counted as failed, and a
  non-zero exit shows `Failed: <first line of standard error>` in the first line of the submenu until the next
  successful poll. The app polls at once afterward.
- The app does not make, read for display, show, log, or write the secret, and it writes no file. It reads the config
  file again when it registers as an observer, so the new secret reaches it without a restart of the app.

## Consequences

- `cli/command` gains one subcommand, and the usage text, the README table of server commands, and the section on
  restarting name it. `menubar/viewer` gains one menu item, one confirmation, and the menu text in
  `docs/reference/menubar-menu-text.md`.
- The contracts and acceptance scenarios of both components change. A person authorizes those edits, and an agent does
  not make them.
- A person who exposed the secret has a one-step remedy. Every running agent session is cut off and has to register
  again, and that cost is chosen here.
- A test that runs the command must use a throwaway config directory, so that it never replaces a real secret.

## Alternatives considered

- **Edit the file by hand, as today.** It needs the person to know the key, the file, and the restart. The menu item
  exists so that an exposed secret is cheap to replace.
- **A route on the switchboard that rotates the secret in memory.** The switchboard would then own a file the CLI owns
  today, and an observer or MCP process that read the secret from the file would hold a stale one. The command that owns
  the file does the whole job instead.
- **The menu bar app writes the new secret itself.** 0140 says the app never shows, logs, or writes the secret, and the
  component description says it writes no files. It runs `landline`, as it does for every other server action.
- **Keep the old secret valid for a grace period.** It leaves an exposed secret working. Rotation is for the case where
  that is not acceptable.
- **Force every agent to register again by other means.** A restart already ends every token, so nothing more is needed.
- **Rotate a switchboard on another machine.** The restart is not this machine's to run. A person rotates it where it
  runs, and runs `landline server join` here again.
