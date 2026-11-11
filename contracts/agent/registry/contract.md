# agent/registry

Status: draft, awaiting a person's approval.

Holds the list of registered agents as a value. Every call takes the list and returns a new one. Nothing is stored.

## Interface

`api.go` defines `Registry` with `Register`, `Online`, and `Find`.

## Rules

- `Register` adds an agent, or replaces the entry with the same name. It sets `LastSeen` to the `now` it was given.
- A name is 1 to 32 lowercase letters, digits, `-` or `_`, starts with a letter or digit, and is not `all`. Anything else
  is `model.ErrBadName`. An agent with no harness is `model.ErrNoHarness`.
- The returned list is sorted by name. The input list is never changed.
- `Online` returns the agents whose `LastSeen` is no more than `window` before `now`, sorted by name.
- `Find` returns the named agent and true, or false.

## Side effects and data access

None. It reads no clock, file, or environment. It **returns a changed copy** of the list.
