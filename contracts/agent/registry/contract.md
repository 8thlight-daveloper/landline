# agent/registry

Status: draft, awaiting a person's approval.

Holds the list of registered agents as a value. Every call takes the list and returns a new one. Nothing is stored.

## Interface

`api.go` defines `Registry` with `Register`, `Online`, and `Find`.

## Rules

- `Register` adds an agent, or replaces the entry with the same name. It sets `LastSeen` to the `now` it was given.
- A name is 1 to 32 lowercase letters, digits, `-` or `_`, starts with a letter or digit, and is not `all`. Anything else
  is `model.ErrBadName`. An agent with no harness is `model.ErrNoHarness`.
- A name is checked as ASCII only, 32 characters at most.
- A harness that is empty or only spaces is `model.ErrNoHarness`.
- The harness, host, session, wake method, and wake target are each at most 64 bytes, counted in bytes. They hold no
  control character, no line or paragraph separator, and no invisible formatting character such as a right-to-left
  override, which are the Unicode categories Cc, Zl, Zp, and Cf. So none of them can break onto a new line or reorder
  the text around them. Anything else is `model.ErrBadField`. An empty host, session, wake method, or
  wake target is allowed.
- The returned list is sorted by name. The input list is never changed, including when `Register` replaces an entry.
- `Online` returns the agents whose `LastSeen` is no more than `window` before `now`, sorted by name.
- `Find` returns the named agent and true, or false.

## Side effects and data access

None. It reads no clock, file, or environment. It **returns a changed copy** of the list.
