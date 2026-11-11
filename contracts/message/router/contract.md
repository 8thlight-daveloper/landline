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
- Rules are checked in this order, and the first one broken is the error returned: sender, thread, kind,
  `in_reply_to`, body, then recipient (a message to oneself before an unknown recipient).
- An empty thread is `main`. A thread is 1 to 64 lowercase letters, digits, `.`, `-` or `_`, starting with a letter or
  digit, or `model.ErrBadThread`.
- An empty kind is `chat`. Any other kind must be one of the model's kinds, or `model.ErrBadKind`.
- `in_reply_to` is empty or 1 to 64 ASCII letters, digits, `.`, `-` or `_`, or `model.ErrBadReplyTo`. It carries no space
  or line break, so it cannot forge a line in the delivered header.
- A body that is empty after trimming is `model.ErrEmptyBody`. A body over 64 KiB, counted in bytes, is
  `model.ErrBodyTooLarge`. A body of exactly 64 KiB is accepted. The body is kept exactly as sent, spaces included.
- A direct message has one recipient. A broadcast has every registered agent except the sender, sorted by name whatever
  the order of the list it was given, and may have none.
- A thread is ASCII only. Every one of the model's four kinds is accepted.
- The envelope carries the given id, sequence number, and time, and the thread, kind, body, and `InReplyTo` as sent.

## Side effects and data access

None. It **creates** an envelope from inputs of another kind, and reads fields of the agent list.
