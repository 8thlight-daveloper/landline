# message/router

Status: draft, awaiting a person's approval.

Checks a send request and returns the stamped envelope and who receives it. The caller passes the registered agents, the
authenticated sender, a new id and sequence number, and the time.

## Interface

`api.go` defines `Router` with `Route`, and the `Request` and `Decision` types.

## Rules

- The sender comes from the `from` argument, which the caller takes from the authenticated connection. A request has no
  sender field. `from` must be a registered agent, or `model.ErrUnknownSender`.
- `To` is `all` or a registered agent, or `model.ErrUnknownRecipient`. A message to oneself is `model.ErrSelfSend`.
- An empty thread is `main`. A thread is 1 to 64 lowercase letters, digits, `.`, `-` or `_`, starting with a letter or
  digit, or `model.ErrBadThread`.
- An empty kind is `chat`. Any other kind must be one of the model's kinds, or `model.ErrBadKind`.
- A body that is empty after trimming is `model.ErrEmptyBody`. A body over 64 KiB is `model.ErrBodyTooLarge`. A body of
  exactly 64 KiB is accepted.
- A direct message has one recipient. A broadcast has every registered agent except the sender, sorted by name, and may
  have none.
- The envelope carries the given id, sequence number, and time, and the thread, kind, body, and `InReplyTo` as sent.

## Side effects and data access

None. It **creates** an envelope from inputs of another kind, and reads fields of the agent list.
