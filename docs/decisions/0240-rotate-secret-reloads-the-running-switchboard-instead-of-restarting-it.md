---
category: behavior
status: accepted
approval: person
approver: {kind: person, name: dnawara}
note: Drafted on 2026-11-24 after the tenth review of 0200, from the person's choice of a reload over a restart. It changes how a rotation reaches the running switchboard, adds a signal the switchboard answers, and has security consequences. The person approved it on 2026-11-24. Amended on 2026-11-24 after the twelfth review of 0200, so that a signal that cannot be sent and a start that fails exit at once. The person approved it on 2026-11-24. Renumbered from 0260 to 0240 on 2026-11-25, so the records on main stay in order before the harness adapter records. The person approved the renumbering on 2026-11-25.
affects: [cli/command, switchboard/server]
builds-on: ['0170', '0200', '0220']
partly-supersedes: ['0170', '0220']
---

# 0240. `rotate-secret` reloads the running switchboard instead of restarting it

## Context

[0170](0170-a-person-rotates-the-switchboard-secret-with-one-command-or-one-menu-item.md) rotates the shared secret by
stopping the switchboard, saving a new secret, and starting it again.
[0220](0220-the-switchboard-reads-its-settings-from-the-config-and-keeps-machines-json-in-the-data-directory.md) keeps
that order.

A switchboard can run under two supervisors: `landline server up`, which starts it once, and the launchd agent or
systemd unit that `landline server install` writes, which starts it again whenever it stops. The tenth review of 0200
showed what the restart does under the second one. `rotate-secret` stops the switchboard, the agent starts it again at
once with the old secret, `rotate-secret` saves the new secret, sees a switchboard running, and reports success. The old
secret keeps working and the new one is refused. A person who rotated because a secret leaked is told it is done.

Each earlier fix to the restart added a check of who started the switchboard, and each check opened the next case. A
restart also starts the start period of 0200, which refuses every signed request for about 5 seconds.

## Decision

**When a switchboard is running, `rotate-secret` saves the new secret and tells the switchboard to read it again. It
never stops or starts the switchboard.**

1. It refuses, changes nothing, and exits `1` in the cases 0170 lists, and in a home that joined a switchboard, as 0200
   says.
1. It finds the running switchboard by the process id in `switchboard.lock`, and only while the lock is held, as 0220
   says.
1. It saves a new secret in `config.toml`, made the way the first one is, with mode `0600`, and keeps every other line.
1. When a switchboard is running, it sends that process `SIGHUP`. When none is running, it starts one as
   `landline server up` does, and the new switchboard reads the new secret.
1. It waits up to 5 seconds until the switchboard accepts the new secret and refuses the old one, by asking
   `GET /v1/machines` with each. Then it prints `rotated the switchboard secret. Every agent must register again.` and
   exits `0`.
1. When the signal cannot be sent, it exits `1` at once. The message says that the new secret is saved in `config.toml`,
   names the process id it could not signal, and says to restart the switchboard where it runs. When no switchboard was
   running and the start fails, it exits `1` at once after the failure that `up` prints, and says that the new secret is
   saved in `config.toml`. Neither case waits for step 5.
1. When the switchboard still accepts the old secret, or never accepts the new one, it exits `1`. The message says that
   the new secret is saved in `config.toml`, that the switchboard at `<host:port>` still uses the old one, and to
   restart the switchboard where it runs. It never prints either secret.

**On `SIGHUP`, the switchboard reads the top-level `secret` of `config.toml` again, and nothing else.** When the file
holds a secret it can read and it differs from the one in use, the switchboard replaces the one in use, ends every agent
token and every observer token, and closes their open streams, as a restart did. Machines, their keys, the signatures it
remembers, the start time, and every setting stay as they are, so a rotation starts no start period. When the file
cannot be read or holds no secret, the switchboard keeps the secret in use and prints one line that says so. A
switchboard whose secret came from `LANDLINE_SECRET` in its own environment keeps it, and `rotate-secret` reports that
through step 7.

**`rotate-secret` takes no flags.** It no longer starts a switchboard with new settings, and `landline server restart`
does that. A flag given to it is an error, as for any unknown flag.

The signal is a format between programs, by C1. A change to what the switchboard reads on `SIGHUP` needs a new record.

The menu bar's `Rotate secret…` runs `landline server rotate-secret` as before. Its confirmation text changes to
`Rotate the switchboard secret? Every agent must register again.`, because the switchboard no longer restarts.

## Consequences

- The rotation works the same whoever supervises the switchboard. `rotate-secret` no longer asks who started it, and the
  "did not start it" refusal of a rotation goes away.
- There is a short time after the save and before the reload in which `config.toml` holds the new secret and the
  switchboard the old one. A client that reads the file in that time is refused and registers again, as after any
  rotation.
- `switchboard/server` gains a signal handler and a lock around the secret in use. `cli/command` loses the stop and the
  start in `rotate-secret`, and gains the check of step 5.
- The rotation tests that drove a stub switchboard through a stop and a start are replaced by tests of the reload and of
  steps 6 and 7.

## Alternatives considered

- **Keep the restart, and stop only a switchboard that `up` started.** It refuses under an installed agent, which is the
  usual setup, so the person could not rotate from the menu bar. It also keeps the start period after every rotation.
- **Keep the restart, and check afterward that the new secret works.** It catches the agent's restart but still races
  it, and still adds the start period.
- **A route on the switchboard that rotates in memory.** 0170 rejected it because the switchboard would own a file that
  the CLI owns. With a reload, the CLI still writes the file and the switchboard only reads it.
