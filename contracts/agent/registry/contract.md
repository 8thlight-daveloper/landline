# agent/registry

Status: draft, awaiting approval. Component: `agent/registry`. Issue: none yet.

Holds the list of registered agents as a value. Every call takes the list and returns a new one. Nothing is stored.

## Interface

`api.go` defines `Registry` with `Register`, `Online`, and `Find`.

## Rules

- `Register` adds an agent, or replaces the entry with the same name. It sets `LastSeen` to the `now` it was given.
- A name is 1 to 32 lowercase letters, digits, `-` or `_`, starts with a letter or digit, and is not `all`. Anything
  else is `model.ErrBadName`. An agent with no harness is `model.ErrNoHarness`.
- A name is checked as ASCII only, 32 characters at most.
- A harness that is empty or only spaces is `model.ErrNoHarness`.
- The harness, host, session, wake method, and wake target are each at most 64 bytes, counted in bytes. They hold no
  control character, no line or paragraph separator, and no invisible formatting character such as a right-to-left
  override, which are the Unicode categories Cc, Zl, Zp, and Cf. So none of them can break onto a new line or reorder
  the text around them. Anything else is `model.ErrBadField`. An empty host, session, wake method, or wake target is
  allowed.
- The returned list is sorted by name. The input list is never changed, including when `Register` replaces an entry.
- `Online` returns the agents whose `LastSeen` is no more than `window` before `now`, sorted by name. An agent whose
  `LastSeen` is after `now` counts as heard from. The list passed in is never reordered.
- `Find` returns the named agent and true, or false. The name is matched exactly, with no case folding or trimming.

## Side effects and data access

None. It reads no clock, file, or environment. It **returns a changed copy** of the list.

## Decisions to know

| Decision                                                                                                  | Reversal |
| --------------------------------------------------------------------------------------------------------- | -------- |
| Registering an existing name replaces its entry, so a re-registering agent takes over the name            | cheap    |
| The wake fields are limited to 64 bytes and a printable character set, so they cannot forge a header line | hard     |
| An agent whose last-heard time is in the future counts as online                                          | cheap    |

## Open questions

None.

## Properties

- **No memory.** The same call gives the same result whatever calls came before, in any order.
- **Pure.** The list passed in is the same after every call, in content and order.
- **Declared data access.** A result differs from its inputs only in the fields the Rules section names.
- **Deterministic.** Two calls with the same arguments give identical results.
- **Sorted.** Every returned list is sorted by name.

## Not covered

- Duplicate names in the list passed in. The registry never creates one.
- What `Online` returns for an empty list. It is an empty list or nil, and a caller may not rely on which.

## Wrong implementations tried

| Wrong implementation                                       | Caught by                                                         |
| ---------------------------------------------------------- | ----------------------------------------------------------------- |
| Replace an entry inside the caller's list                  | `TestReplacingAnEntryLeavesTheCallersListUnchanged`               |
| Sort the caller's list in place in `Register` or `Online`  | `TestNeitherRegisterNorOnlineReordersTheCallersList`              |
| Accept a 33 character name or a leading `-`                | `TestANameOfExactlyThirtyTwoCharactersIsValidAndThirtyThreeIsNot` |
| Accept a line separator or a bidi override in a wake field | `TestControlCharacterBoundaries`                                  |
| Count characters and not bytes in the 64 byte limit        | `TestTheFieldLimitCountsBytesNotCharacters`                       |
| Match names ignoring case                                  | `TestFindMatchesTheNameExactly`                                   |
