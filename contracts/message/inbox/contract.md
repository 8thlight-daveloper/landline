# message/inbox

Status: draft, awaiting approval. Component: `message/inbox`. Issue: none yet.

Chooses the unread messages for one agent from the log. The caller keeps the agent's cursor.

## Interface

`api.go` defines `Inbox` with `Select`. The result is `model.Page`.

## Rules

- Names are matched whole. An agent named `alpha` gets nothing addressed to `alphabet` or `alph`, and sees a broadcast
  from `alpha2`.
- A message is for an agent when `To` is that agent's name, whoever sent it, or `To` is `all` and `From` is another
  agent.
- Only messages with `Seq` greater than `cursor` are considered. With a non-empty thread list, only messages whose
  thread equals one of those names exactly. The list applies to a broadcast too.
- Messages come back oldest first, whatever the order of the log. The log passed in is never reordered or changed.
  `limit` above zero caps how many. `Remaining` counts the matches left after the cap.
- `Next` is the `Seq` of the last message returned, or `cursor` when none was.

## Side effects and data access

None. It **reads** fields of the log and returns a new list. It never changes the log.

## Decisions to know

| Decision                                            | Reversal |
| --------------------------------------------------- | -------- |
| A thread list applies to a broadcast too            | cheap    |
| A message an agent sent to its own name is its mail | cheap    |
| Names and threads match whole and exactly           | hard     |

## Open questions

None.

## Properties

- **No memory.** The same call gives the same result whatever calls came before, in any order.
- **Pure.** The log is the same after the call, in content and order.
- **Declared data access.** A result differs from its inputs only in the fields the Rules section names.
- **Deterministic.** Two calls with the same arguments give identical results.
- **Ordered.** Messages come back oldest first whatever the order of the log.

## Not covered

- A log with two messages of the same `Seq`. The hub never writes one.

## Wrong implementations tried

| Wrong implementation                     | Caught by                                              |
| ---------------------------------------- | ------------------------------------------------------ |
| Sort the caller's log in place           | `TestSelectNeverReordersTheCallersLog`                 |
| Check only the first thread              | `TestEveryNamedThreadIsKept`                           |
| Match a thread by prefix                 | `TestAThreadIsMatchedWholeAndExactly`                  |
| Let a broadcast ignore the thread filter | `TestABroadcastInAThreadNobodyAskedForIsLeftOut`       |
| Match a name by prefix                   | `TestAnAgentNamedAsAPrefixOfAnotherSeesOnlyItsOwnMail` |
| Apply the limit before the filter        | `TestALimitReturnsTheOldestFirstAndCountsTheRest`      |
