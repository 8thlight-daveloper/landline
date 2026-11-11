# message/inbox

Status: draft, awaiting a person's approval.

Chooses the unread messages for one agent from the log. The caller keeps the agent's cursor.

## Interface

`api.go` defines `Inbox` with `Select`, and the `Page` type.

## Rules

- A message is for an agent when `To` is that agent's name, or `To` is `all` and `From` is another agent.
- Only messages with `Seq` greater than `cursor` are considered. With a non-empty thread list, only those threads.
- Messages come back oldest first. `limit` above zero caps how many. `Remaining` counts the matches left after the cap.
- `Next` is the `Seq` of the last message returned, or `cursor` when none was.

## Side effects and data access

None. It **reads** fields of the log and returns a new list. It never changes the log.
