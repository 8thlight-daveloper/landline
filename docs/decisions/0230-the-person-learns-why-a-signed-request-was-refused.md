---
category: ux
status: accepted
approval: person
approver: {kind: person, name: dnawara}
note: Drafted on 2026-11-23 from the person's choices of a notification, a switchboard log line, and an Open Log item. It adds a macOS notification and an unauthenticated path to a log file, and it waits for the person's approval. Revised on 2026-11-24, so that 0200 defines the reason field and a refusal persists after the retries of 0200, and so that a menu action that fails for no credential notifies.
affects: [switchboard/server, menubar/viewer]
builds-on: ['0140', '0200']
---

# 0230. The person learns why a signed request was refused

Builds on [0140](0140-a-macos-menu-bar-item-shows-agents-runs-the-switchboard-and-sets-up-harnesses.md) and
[0200](0200-a-machine-joins-with-an-invite-code-and-a-keypair-that-outlasts-secret-rotation.md).

## Context

Under [0200](0200-a-machine-joins-with-an-invite-code-and-a-keypair-that-outlasts-secret-rotation.md) a machine signs
its requests with its key. A time outside the signature window gets `401` with a message that names the clock
difference. Every other refusal gets `401` and one text, `missing or wrong switchboard secret or machine signature`, so
that a caller with no credential learns nothing about which names exist, from the text or from the time the answer
takes.

Only the switchboard knows the real reason: a time in the start period of 0200, a name that is unknown, pending, or
revoked, a signature that does not verify, a signature it has already seen, or a header it cannot read. Today it tells
no one. It writes no line for a refused request. On the refused machine, the menu bar shows the refusal on the first
line of its `Switchboard` submenu, and only a person who opens the menu sees it. The clients tell the clock refusal from
the others by the `reason` field that 0200 defines, which has one value, `refused`, for every refusal inside the window.

A person whose machine is refused therefore has to guess. A clock that drifted, a revoke, and a restart a moment ago
look the same from the refused machine.

## Decision

The switchboard writes the real reason to its own output, for the person who runs it. The refused machine's menu bar
tells its person with a macOS notification once the refusal persists. The menu bar on the switchboard's machine opens
that output with one item. The answer to the caller does not change in what it reveals.

### The switchboard writes one line per refused signed request

A request whose `Authorization` header starts with `Landline-Machine` and that the switchboard refuses with `401`
produces one line on the switchboard's standard error. `landline server up` and the login unit of `server install` send
standard error to `switchboard.out` in the data folder, so the line lands there. A switchboard run in a terminal writes
it to that terminal.

- The line is the time, the name, and the reason:
  `2026-11-23T10:15:04Z refused a signed request from machine alpha: the signature does not match its key`. The time is
  the switchboard's clock in UTC, RFC 3339 to the second.
- The name is the one the request sent. It is written only when it follows the machine name rule of 0200: 1 to 32 lower
  case letters, digits, and hyphens, starting with a letter. Any other value, which an unauthenticated caller controls,
  is written as `(not a valid machine name)`, and a missing one as `(no name)`. So a line holds no control character, no
  space from the caller, and nothing longer than the rule allows.
- The reason is one of these texts:

| Check that failed                                     | Reason text                                                      |
| ----------------------------------------------------- | ---------------------------------------------------------------- |
| The header, time, nonce, or signature cannot be read  | `the signature header cannot be read`                            |
| The time is outside the signature window              | `its clock is N seconds ahead of the switchboard's`, or `behind` |
| The name is not an active machine                     | `no active machine has that name`                                |
| The time is in the start period of 0200, or before it | `it signed before the switchboard's start period ended`          |
| The signature does not verify                         | `the signature does not match its key`                           |
| The switchboard has already seen the signature        | `the signature was already used`                                 |

- The registry does not tell an unknown name from a pending or revoked one, and the line does not either.
- A line never holds the shared secret, a key, a signature, a nonce, a token, an invite code, or the request's body or
  path.
- The switchboard hands the refusal to the limiter below after the answer is written. The work before the answer stays
  the same for every refusal, as 0200 requires.
- A refused request with a bearer secret or an unknown agent or observer token is not a signed request, and this record
  writes no line for it.

### Lines are rate limited, and the rest are counted

An unauthenticated caller can send refused requests as fast as the switchboard answers them, and can change the name on
each. The switchboard bounds what it writes.

- Time is cut into minutes of the switchboard's clock. In each minute the first refusal of each pair of name and reason
  writes a line, up to 10 lines in that minute. Every other refusal in that minute is counted and writes nothing.
- When a minute ends with a count above zero, the switchboard writes one line at once, from a timer:
  `2026-11-23T10:16:00Z 37 more refused signed requests in the last minute were not logged`. A minute with a count of
  zero writes nothing.
- The limiter holds at most 10 pairs and one count, and clears them when a minute ends. Its memory does not grow with
  the number of names a caller tries.
- The switchboard reads its clock for the limiter through the `Now` function of its config, as 0200 says for every
  request, so a test sets the minute.
- At most 11 lines a minute reach the file. `landline server up` and `restart` start a new `switchboard.out`, and the
  login unit appends to it, as they do today.

### The lines are text for people

A refusal line, the notification text below, and the menu's notice line are written for people. By coding principle C1
in `docs/reference/coding-principles.md`, a program never depends on text written for people, so no program parses a
refusal line or compares a refusal's text.

A program that needs to know why a request was refused reads the `reason` field of the `401`, and `clock_difference` for
a clock refusal. [0200](0200-a-machine-joins-with-an-invite-code-and-a-keypair-that-outlasts-secret-rotation.md) defines
both fields and how every client reads them, and this record does not change them.

### The refused machine's menu bar notifies once the refusal persists

The menu bar app shows a macOS notification when this machine's key is refused.

- **When.** A refusal persists when it outlasts what the client already does about it under 0200. A `refused` answer
  persists when the retries of 0200 have run for 6 seconds and the last one is still refused, which is when the client
  gives the generic advice. By then a switchboard whose clock agrees with this Mac's is past its start period. A `clock`
  answer, which 0200 does not retry, persists when two polls in a row end with it. A poll with any other outcome between
  them starts the count again. `Refresh` is a poll. At the default interval, a notification comes about 6 seconds after
  the first `refused` answer and about 5 seconds after the first `clock` answer, and never for a refusal that the
  retries clear.

- **How often.** Once for each problem. A problem is a `reason` at one switchboard address. A clock difference that
  changes from 40 to 41 seconds is the same problem. The problem ends when a poll registers or reads, or when the
  address or the credential the app reads from `config.toml` changes. After it ends, a new refusal that persists
  notifies again. The app keeps this in memory, so a restart of the app notifies again if the refusal still persists.

- **Text.** The title is `Landline`. The body is one of:

  - `This Mac's clock is N seconds ahead of the switchboard's, so the switchboard refuses its machine key. Set the clock.`,
    or `behind`.
  - `The switchboard refuses this Mac's machine key. Its log on the switchboard's machine says why. If this Mac was revoked, join again with a new invite code.`

  The text holds no secret, key, token, signature, nonce, or invite code. The notification has no action, and clicking
  it does nothing more than macOS does for any notification.

- **Permission.** The app asks macOS for permission to notify when it first has a notification to show, and not at
  start, so a person who is never refused is never asked. macOS asks the person once. When the person declines, or macOS
  refuses for any reason, the app shows no notification and the notice line remains the signal.

- **Native code.** The notification uses the `UserNotifications` framework through a few lines of Objective-C and cgo in
  the menu bar module, as the About panel and the confirmation of
  [0170](0170-a-person-rotates-the-switchboard-secret-with-one-command-or-one-menu-item.md) use Cocoa. It links a system
  framework and adds no module dependency. The framework needs a bundle identifier, so only `Landline Menubar.app`
  notifies. The bare binary and `landline-menubar menu --text` never notify and keep the notice line.

- The notice line on the first line of the `Switchboard` submenu does not change.

- **No credential.** A menu action that fails while the menu bar finds no credential, as
  [0200](0200-a-machine-joins-with-an-invite-code-and-a-keypair-that-outlasts-secret-rotation.md) defines it, raises a
  notification with the full no-credential message of 0200. Only an action the person started from the menu bar
  notifies. The same command run from a terminal prints the message and never notifies, and the menu bar's own polling
  never notifies for it.

A shared-secret refusal is not a signed request, and it gets no notification in this record.

### `Open Log` opens `switchboard.out`

The `Switchboard` submenu gains `Open Log`, after `Rotate secret…`.

- It is in the menu only when the switchboard's address is this machine's, by the test of
  [0190](0190-one-toml-config-names-the-switchboard-by-host-and-port.md). For a remote switchboard the item is absent,
  because the file is on the other machine.
- It is enabled when `$HOME/.local/share/landline/switchboard.out` exists, and disabled otherwise. It stays enabled
  while a server action runs and while the switchboard is stopped, because the file then says why it stopped.
- Choosing it asks macOS, through `NSWorkspace`, to open the file in Console, the macOS log viewer. The app runs no
  command for it, and reads and writes nothing in the file. When the open fails, the first line of the submenu reads
  `Failed: could not open the switchboard log` until the next successful poll.
- `landline-menubar menu --text` prints `Open Log` or `Open Log (disabled)` in its place, and nothing for a remote
  switchboard.

### What stays from 0140

[0140](0140-a-macos-menu-bar-item-shows-agents-runs-the-switchboard-and-sets-up-harnesses.md) says the menu bar app
never shows, logs, or writes the secret or the token. That holds: the notification and `Open Log` carry neither, and
`switchboard.out` is written by the switchboard, not by the app. 0140 says the app does not notify on a connect or a
disconnect, and that holds. This record adds one notification, for this machine's refused key. 0140 says the app starts
nothing but `landline`. `Open Log` asks macOS to open a file and starts no process of the app's own.

## Consequences

- `switchboard/server` gains the refusal lines and the limiter with its timer. Its contract and the acceptance scenarios
  change: a refused request of each kind writes its line, a name outside the rule is not written, the eleventh distinct
  pair in a minute is counted, the count line follows the minute, and no line holds the signature or the nonce. A person
  authorizes those edits, and an agent does not make them.
- `menubar/viewer` gains the notification, its rule for when and how often, and the `Open Log` item. It now checks
  whether `switchboard.out` exists, and still writes no file. The notification is tested as a value: a unit test feeds a
  sequence of poll outcomes to the rule and checks which notifications it returns and their text. No test needs a screen
  or macOS's permission. `docs/reference/menubar-menu-text.md` and `docs/architecture/components.md` gain the item and
  the file the component checks.
- The cgo flags of the menu bar module link `UserNotifications`. The stack document says so.
- A person who runs the switchboard can read why a machine was refused, and a person on the refused machine is told
  without opening the menu.
- An unauthenticated caller can make the switchboard write up to 11 lines a minute, about 16,000 a day, to a file that
  only a restart starts anew under the login unit. The lines hold only names that pass the rule, so a caller cannot
  inject a line or a control character.
- `switchboard.out` sits in the data folder that `landline` creates with mode `0700`, and `server up` writes it with
  mode `0600`. The lines tell which machine names are active to whoever can read the file. That is the person who runs
  the switchboard, who can already list them with `landline server machines`.
- `landline server up` and `rotate-secret` find the switchboard's startup line in `switchboard.out`. A refusal line
  starts with its time and holds only a name that passes the rule, so it cannot be taken for that line.

## Alternatives considered

- **The log line only.** It costs the least. The person on the refused machine, who is the one who has to act, still
  sees nothing until they open the menu, and the switchboard may be on another person's desk.
- **The notification only.** The refused machine can say that it was refused and, for the clock, by how much. Inside the
  window it cannot say why, because the answer does not say. Only the switchboard can, and without a line it says it to
  no one.
- **A separate structured log of refusals, such as JSON lines.** A program could read it. No program needs to: the
  person reads the reason, and a client already has the `reason` field. It would be a stored format with its own file,
  mode, and rotation, and a second place where an unauthenticated caller can make the switchboard write. Revisit when a
  program has a use for refused requests.
- **Tell the caller the real reason.** It is the simplest for the refused machine. It breaks the promise of 0200 that a
  caller with no credential learns nothing about which names exist, because `no active machine has that name` and
  `the signature does not match its key` would tell an unknown name from a known one.
- **A line for every refusal, with no limit.** It keeps every event. An unauthenticated caller could then fill the disk
  at the rate the switchboard answers.
- **A limit per name alone.** A caller that changes the name on each request would get a line for each. The cap of 10
  lines a minute bounds the file whatever the names, and the count keeps the total visible.
- **Notify on the first refused answer.** It is faster. The start period of 0200 refuses every machine for about 5
  seconds after every start, so a person would be told about a refusal that ends by itself.
- **Notify on every refused poll.** A persistent refusal would notify every 5 seconds, and a person would turn
  notifications off.
- **Ask for permission when the app starts.** macOS shows its prompt for a feature the person may never need, and a
  person who declines out of surprise never gets the notification.
- **`NSUserNotification`.** It needs no bundle identifier. Apple deprecated it, and `UserNotifications` is the framework
  that remains.
- **Open the log with `open` or in the person's default app.** `open` is a child process, and the app runs only
  `landline` as a child process. A `.out` file may have no default app, and then the open fails or asks the person to
  pick one.
- **Show `Open Log` for a remote switchboard, and disabled.** The other server items are disabled for a remote
  switchboard because the action is on the other machine. The log is too, and an item that can never work there is
  noise.
