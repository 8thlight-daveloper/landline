# agent/registry

Status: draft, awaiting approval. Component: `agent/registry`. Issue: none yet.

Holds the list of registered agents as a value. Every call takes the list and returns a new one. Nothing is stored.

## Interface

`api.go` defines `Registry` with `Register`, `Find`, and `Resolve`.

## Rules

- `Register` adds an agent, or replaces the entry with the same name. It sets `LastSeen` to the `now` it was given.

- A name is 1 to 32 lowercase letters, digits, `-` or `_`, starts with a letter or digit, and is not `all`. Anything
  else is `model.ErrBadName`. An agent with no harness is `model.ErrNoHarness`.

- A name is checked as ASCII only, 32 characters at most.

- Faults are checked in this order, and the first one found is the error returned: the name, then the harness, then the
  other fields.

- A harness that is empty after trimming Unicode white space (the Unicode White_Space property, so a zero-width space is
  not white space), such as a space, a tab, a no-break space, or an em space, is `model.ErrNoHarness`. That check comes
  before the field rules, so a harness of only a tab is `model.ErrNoHarness`. A harness of only a zero-width space or
  another invisible formatting character is not empty after trimming, so it is `model.ErrBadField`.

- The harness, host, session, folder, branch, worktree, harness version, model, effort, and label are each at most 64
  bytes, counted in bytes. They hold no control character, no line or paragraph separator, and no invisible formatting
  character such as a right-to-left override, which are the Unicode categories Cc, Zl, Zp, and Cf. So none of them can
  break onto a new line or reorder the text around them. Anything else is `model.ErrBadField`. An empty host or session
  is allowed.

- The returned list is sorted by name. The input list is never changed, including when `Register` replaces an entry.

- `Resolve(all, live, query)` turns what a person typed into one agent. It returns the agent and no candidates when it
  found exactly one, no agent and the candidates sorted by name when several matched, and no agent and no candidates
  when none did. The rules apply in this order, and the first that finds anything decides:

  1. A handle that equals the query exactly, among all agents, live or not.
  1. A label that equals the query exactly, among the live agents. An empty label never matches.
  1. A handle or a label that starts with the query, among the live agents. An agent whose handle and label both start
     with the query is one candidate. An empty query finds nothing.

  Matching is exact and case-sensitive. The query is not trimmed, so a query with a leading or trailing space or tab
  finds nothing. A query that is only a middle or end part of a handle or label finds nothing. Several matches at one
  step decide the result as candidates, and the later steps are not tried. Neither list is changed.

- `Find` returns the named agent and true, or false. The name is matched exactly, with no case folding or trimming.

## Side effects and data access

None. It reads no clock, file, or environment. It **returns a changed copy** of the list.

## Decisions to know

| Decision                                                                                                       | Reversal |
| -------------------------------------------------------------------------------------------------------------- | -------- |
| Registering an existing name replaces its entry, so a re-registering agent takes over the name                 | cheap    |
| The free-text fields are limited to 64 bytes and a printable character set, so they cannot forge a header line | hard     |

## Open questions

None.

## Properties

- **No memory.** The same call gives the same result whatever calls came before, in any order.
- **Pure.** The list passed in is the same after every call, in content and order.
- **Declared data access.** A result differs from its inputs only in the fields the Rules section names.
- **Deterministic.** Two calls with the same arguments give identical results.
- **Sorted.** Every returned list is sorted by name.

## Not covered

- Whether the 64 byte limit on the harness applies before or after trimming. The stored value is never trimmed, and the
  limit is measured on it as stored.
- Duplicate names in the list passed in. The registry never creates one.

## Wrong implementations tried

| Wrong implementation                                                             | Caught by                                                                          |
| -------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------- |
| Replace an entry inside the caller's list                                        | `TestReplacingAnEntryLeavesTheCallersListUnchanged`                                |
| Sort the caller's list in place in `Register`                                    | `TestRegisterDoesNotReorderTheCallersList`                                         |
| Accept a 33 character name or a leading `-`                                      | `TestANameOfExactlyThirtyTwoCharactersIsValidAndThirtyThreeIsNot`                  |
| Accept a line separator or a bidi override in a field                            | `TestControlCharacterBoundaries`                                                   |
| Count characters and not bytes in the 64 byte limit                              | `TestTheFieldLimitCountsBytesNotCharacters`                                        |
| Match names ignoring case                                                        | `TestFindMatchesTheNameExactly`                                                    |
| Match a query that is the middle or end of a handle or a label                   | `TestResolveDoesNotMatchTheMiddleOrEndOfAHandleOrALabel`                           |
| Trim the query before matching                                                   | `TestResolveDoesNotTrimTheQuery`                                                   |
| Fold case in a handle, a label, or a prefix                                      | `TestResolveMatchesHandlesAndLabelsOnlyInTheCaseTheyWereGiven`                     |
| Break a label tie by a prefix match, or by trying the next step                  | `TestResolveALabelTieIsNotBrokenByAPrefixMatch`                                    |
| List an agent twice when its handle and label both start with the query          | `TestResolveListsAnAgentOnceWhenItsHandleAndItsLabelBothStartWithTheQuery`         |
| Skip the invisible-character check on the harness, or let it count as no harness | `TestAHarnessOfOnlyAnInvisibleCharacterThatIsNotWhiteSpaceIsABadFieldNotNoHarness` |
| Check only some Cf characters, or only some fields                               | `TestEveryFieldRefusesInvisibleFormattingCharacters`                               |

## Fixture sources

- Handles `alpha`, `bravo`, `tutor-1`, and the labels `tutor`, `reviewer` are invented for these tests. They are
  fictional agent names, as `AGENTS.md` requires.
- The query forms for trimming and case come from the rules above: the contract says the query is matched exactly and
  not trimmed. The Unicode White_Space characters used are the ones the harness rule names.
- The invisible characters are Unicode category Cf code points from the Unicode Character Database (`UnicodeData.txt`,
  general category `Cf`): U+00AD, U+0600, U+061C, U+180E, U+200B to U+200F, U+202A, U+202D, U+202E, U+2060, U+2066,
  U+2069, U+FEFF, and the tag characters U+E0001 and U+E0041.
