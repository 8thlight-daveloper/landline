# message/router

Status: draft, awaiting approval. Component: `message/router`. Issue: none yet.

Checks a send request and returns the stamped envelope and who receives it. The caller passes the registered agents, the
authenticated sender, a new id and sequence number, and the time.

## Interface

`api.go` defines `Router` with `Route`. The request and the decision are `model.SendRequest` and `model.Decision`.

## Rules

- The sender comes from the `from` argument, which the caller takes from the authenticated connection. A request has no
  sender field. `from` must be a registered agent, or `model.ErrUnknownSender`.
- A message has exactly one recipient. `To` is the exact name of a registered agent, or `model.ErrUnknownRecipient`. A
  message to oneself is `model.ErrSelfSend`. `all` has no meaning: it is an unknown recipient like any other name that
  no agent holds ([0070](../../../docs/decisions/0070-delivery-is-a-push-to-a-connected-agent-with-no-inbox.md)).
- Rules are checked in this order, and the first one broken is the error returned: sender, thread, kind, `in_reply_to`,
  body, then recipient. The sender is always registered, so a message to oneself is never also an unknown recipient.
- An empty thread is `main`. A thread is 1 to 64 lowercase letters, digits, `.`, `-` or `_`, starting with a letter or
  digit, or `model.ErrBadThread`.
- An empty kind is `chat`. Any other kind must be one of the model's kinds, or `model.ErrBadKind`.
- `in_reply_to` is empty or 1 to 64 ASCII letters, digits, `.`, `-` or `_`, or `model.ErrBadReplyTo`. It carries no
  space or line break, so it cannot forge a line in the delivered header.
- A body that is empty after trimming Unicode white space, such as a tab or a no-break space, is `model.ErrEmptyBody`.
  That check comes before the size check, so a 70 KiB body of spaces is `model.ErrEmptyBody`.
- A kind and a recipient are matched exactly, so `CHAT` is not a kind and `BRAVO` is not the agent `bravo`.
- `Recipients` is a list of exactly one name, the recipient, and is never nil.
- A body over 64 KiB, counted in bytes, is `model.ErrBodyTooLarge`. A body of exactly 64 KiB is accepted. The body is
  kept exactly as sent, spaces included.
- An empty recipient, and a recipient with spaces or a line break around it, is `model.ErrUnknownRecipient`. The list of
  agents passed in is never reordered or changed.
- A name that appears more than once in the agent list is still one recipient.
- A thread is ASCII only. Every one of the model's four kinds is accepted.
- The letters are exactly the ASCII ranges `a` to `z` and `A` to `Z`, and the digits `0` to `9`, so `z` and `Z` are
  accepted and the characters next to them in the ASCII table (`{` after `z`, `[` after `Z`, `` ` `` and `@` before `a`
  and `A`) are refused. Every character of `in_reply_to` is checked, the first included. Fixture source: the ASCII table
  (US-ASCII, code points `0x40` to `0x7B`).
- The envelope carries the given id, sequence number, and time, and the thread, kind, body, and `InReplyTo` as sent.

## Side effects and data access

None. It **creates** an envelope from inputs of another kind, and reads fields of the agent list.

## Decisions to know

| Decision                                                                                          | Reversal |
| ------------------------------------------------------------------------------------------------- | -------- |
| The router checks rules in a fixed order, so a request with two faults gets one predictable error | cheap    |
| `in_reply_to`, thread, and recipient are matched as ASCII tokens, so none can carry a line break  | hard     |
| The body is kept exactly as sent, spaces included                                                 | cheap    |
| A send names one recipient, and `all` is an unknown recipient, not a broadcast                    | cheap    |

## Open questions

None.

## Properties

- **No memory.** The same call gives the same result whatever calls came before, in any order.
- **Pure.** The agent list and the request are the same after the call.
- **Declared data access.** A result differs from its inputs only in the fields the Rules section names.
- **Deterministic.** Two calls with the same arguments give identical results.

## Not covered

- The sender listed more than once, or an agent with an empty name, in the list passed in. The registry never creates
  either, and a rebuild may treat them like any other name.
- Whether the sender is among the agents that were last heard from. The caller decides who is registered.
- An agent named `all` in the list passed in. The registry refuses that name, so a rebuild may treat it like any other
  name.

## Wrong implementations tried

| Wrong implementation                                                               | Caught by                                                                                               |
| ---------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------- |
| Trim the body                                                                      | `TestTheBodyIsKeptExactlyAsSent`                                                                        |
| Accept a 65 character thread or a leading `.`                                      | `TestThreadBoundaries`                                                                                  |
| Accept a newline in a thread or `in_reply_to`                                      | `TestAThreadWithALineBreakOrAUnicodeSeparatorIsRefused`, `TestInReplyToIsOneShortTokenWithNoLineBreaks` |
| Hard-code the id, sequence, or round the time                                      | `TestTheEnvelopeCarriesTheGivenIdSequenceAndExactTime`                                                  |
| Treat an empty recipient as a valid one                                            | `TestAnEmptyOrPaddedRecipientIsUnknown`                                                                 |
| Treat `all` as a broadcast to everyone else                                        | `TestTheNameAllIsNotARecipientAtAll`                                                                    |
| Treat `ALL` or `All` as special                                                    | `TestAllInAnyCaseIsAnUnknownRecipientToo`                                                               |
| Return more than one recipient, or none                                            | `TestADecisionNamesExactlyOneRecipientAndNeverNil`                                                      |
| Treat an unknown `all` as a self-send or success                                   | `TestAnUnknownRecipientNamedAllIsNeitherASelfSendNorASuccess`                                           |
| Check the `all` recipient before earlier rules                                     | `TestAnAllRecipientStillFollowsTheFixedOrderOfRules`                                                    |
| List a repeated agent name twice                                                   | `TestADuplicateNameInTheAgentListIsOneRecipient`                                                        |
| Sort the caller's list in place                                                    | `TestRoutingNeverReordersTheCallersAgentList`                                                           |
| Check self-send before the body or thread                                          | `TestASelfSendIsCheckedAfterEveryEarlierRule`                                                           |
| Skip the first character of `in_reply_to` when checking it                         | `TestEveryCharacterOfInReplyToIsCheckedIncludingTheFirst`                                               |
| Refuse `z` or `Z` in a thread or `in_reply_to`, or accept `{`, `[`, `` ` `` or `@` | `TestTokenAlphabetEdgesAtZAndUppercaseZ`                                                                |
