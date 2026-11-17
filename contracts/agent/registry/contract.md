# agent/registry

Status: draft, awaiting approval. Changed for ADR 0160. Component: `agent/registry`. Issue: none yet.

Holds the list of registered agents as a value. Every call takes the list and returns a new one. Nothing is stored.

## Interface

`api.go` defines `Registry` with `Register` and `Find`. `Resolve` is removed by
[0160](../../../docs/decisions/0160-an-agent-has-an-id-a-project-and-a-task.md): a send takes an exact id, and `Find`
already looks one up.

## Rules

- `Register` adds an agent, or replaces the entry with the same `ID`. The new entry replaces the old one whole, so a
  field the new entry leaves empty, `Task` and `TaskAt` included, is empty afterwards. It sets `LastSeen` to the `now`
  it was given and changes no other field.

- An id is 1 to 32 lowercase letters, digits, `-` or `_`, starts with a letter or digit, and is not `all`. Anything else
  is `model.ErrBadName`, whose text says `agent id`. An id of 4 lowercase hexadecimal characters, such as `a3f9`, is
  valid. An agent with no harness is `model.ErrNoHarness`.

- An id is checked as ASCII only, 32 characters at most.

- Faults are checked in this order, and the first one found is the error returned: the id, then the harness, then the
  other fields.

- A harness that is empty after trimming Unicode white space (the Unicode White_Space property, so a zero-width space is
  not white space), such as a space, a tab, a no-break space, or an em space, is `model.ErrNoHarness`. That check comes
  before the field rules, so a harness of only a tab is `model.ErrNoHarness`. A harness of only a zero-width space or
  another invisible formatting character is not empty after trimming, so it is `model.ErrBadField`.

- The harness, host, session, folder, branch, worktree, harness version, landline version, landline commit, landline
  date, model, effort, project, and task are each at most 64 bytes, counted in bytes. They hold no control character, no
  line or paragraph separator, and no invisible formatting character such as a right-to-left override, which are the
  Unicode categories Cc, Zl, Zp, and Cf. So none of them can break onto a new line or reorder the text around them.
  Anything else is `model.ErrBadField`, the whole registration is refused, and the list passed in is unchanged. An empty
  host, session, project, or task is allowed.

- The task is stored as given, with its `TaskAt`, which the registry neither checks nor changes. A task of 64 bytes is
  accepted and one of 65 bytes is `model.ErrBadField`, so every registration of an entry validates its task.

- The landline commit and the landline date are plain free-text attributes that follow the rule above, with no other
  format check. Either is stored as given and found unchanged, and either may be empty. Registering an id again replaces
  both with the new entry's values, so an empty value clears the old one. `Find` never matches on either field.

- The returned list is sorted by id. The input list is never changed, including when `Register` replaces an entry.

- `Find(agents, id)` returns the agent whose `ID` equals `id` exactly, among every agent it is given, and true, or
  false. The id is matched with no case folding, trimming, or prefix, and an empty id finds nothing. It never matches on
  a project, a task, or any other field.

## Side effects and data access

None. It reads no clock, file, or environment. It **returns a changed copy** of the list.

## Decisions to know

| Decision                                                                                                            | Reversal |
| ------------------------------------------------------------------------------------------------------------------- | -------- |
| Registering an existing id replaces its entry whole, so a re-registering agent takes over the id and loses its task | cheap    |
| `Find` matches an exact id and nothing else, so a project or an id prefix never picks an agent                      | cheap    |
| The free-text fields are limited to 64 bytes and a printable character set, so they cannot forge a header line      | hard     |

## Open questions

None.

## Properties

- **No memory.** The same call gives the same result whatever calls came before, in any order.
- **Pure.** The list passed in is the same after every call, in content and order.
- **Declared data access.** A result differs from its inputs only in the fields the Rules section names.
- **Deterministic.** Two calls with the same arguments give identical results.
- **Sorted.** Every returned list is sorted by id.

## Not covered

- Whether the 64 byte limit on the harness applies before or after trimming. The stored value is never trimmed, and the
  limit is measured on it as stored.
- Duplicate ids in the list passed in. The registry never creates one.

## Wrong implementations tried

| Wrong implementation                                                                                      | Caught by                                                                                                                                                                                                                                                             |
| --------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Replace an entry inside the caller's list                                                                 | `TestReplacingAnEntryLeavesTheCallersListUnchanged`                                                                                                                                                                                                                   |
| Sort the caller's list in place in `Register`                                                             | `TestRegisterDoesNotReorderTheCallersList`                                                                                                                                                                                                                            |
| Accept a 33 character id or a leading `-`                                                                 | `TestAnIDOfExactlyThirtyTwoCharactersIsValidAndThirtyThreeIsNot`                                                                                                                                                                                                      |
| Accept a line separator or a bidi override in a field                                                     | `TestControlCharacterBoundaries`                                                                                                                                                                                                                                      |
| Count characters and not bytes in the 64 byte limit                                                       | `TestTheFieldLimitCountsBytesNotCharacters`                                                                                                                                                                                                                           |
| Skip the invisible-character check on the harness, or let it count as no harness                          | `TestAHarnessOfOnlyAnInvisibleCharacterThatIsNotWhiteSpaceIsABadFieldNotNoHarness`                                                                                                                                                                                    |
| Check only some Cf characters, or only some fields                                                        | `TestEveryFieldRefusesInvisibleFormattingCharacters`                                                                                                                                                                                                                  |
| Check every field except the landline version, or give it a different limit                               | `TestAnOverLongOrInvisibleLandlineVersionIsRefused`                                                                                                                                                                                                                   |
| Drop the landline version when storing, or keep the old one when it is empty                              | `TestALandlineVersionIsStoredAndFoundUnchanged`                                                                                                                                                                                                                       |
| Leave the commit or the date out of the field checks, or give either a limit other than 64 bytes          | `TestAnOverLongOrInvisibleCommitOrDateIsRefusedAndTheListIsUnchanged`, `TestEveryFieldAcceptsExactlySixtyFourBytesAndRefusesTheUnicodeLineBreaks`, `TestEveryFieldRefusesInvisibleFormattingCharacters`                                                               |
| Refuse a 46 byte commit that ends in `-dirty`, or a value that is not a 40 hex hash                       | `TestACommitOfFortySixBytesWithTheDirtyMarkIsAccepted`, `TestALandlineCommitAndDateAreStoredAndFoundUnchanged`                                                                                                                                                        |
| Drop the commit or the date when storing, or keep the old one when it is empty or on a re-registration    | `TestALandlineCommitAndDateAreStoredAndFoundUnchanged`, `TestRegisteringAgainReplacesTheCommitAndTheDateAndAnEmptyValueClearsThem`                                                                                                                                    |
| Store the commit in the date field or the reverse, or fold either into the version                        | `TestACommitAndADateAreSeparateFieldsThatDoNotLeakIntoEachOtherOrIntoTheVersion`                                                                                                                                                                                      |
| Refuse by returning a list with the partial entry, or change the caller's list on a refusal               | `TestAnOverLongOrInvisibleCommitOrDateIsRefusedAndTheListIsUnchanged`                                                                                                                                                                                                 |
| Report a bad commit before a bad id or a missing harness                                                  | `TestABadCommitDoesNotHideABadIDOrAMissingHarness`                                                                                                                                                                                                                    |
| Match ids ignoring case                                                                                   | `TestFindMatchesTheIDExactly`                                                                                                                                                                                                                                         |
| Match an id by prefix, or an empty id                                                                     | `TestFindMatchesOnlyAnExactIDNotAProjectAPrefixAnEmptyIDOrAnyOtherField`, `TestFindPicksTheRightAgentAmongSeveral`                                                                                                                                                    |
| Match a project, a task, a commit, or a date as an id                                                     | `TestFindMatchesOnlyAnExactIDNotAProjectAPrefixAnEmptyIDOrAnyOtherField`                                                                                                                                                                                              |
| Leave the task out of the field checks, or give it a limit other than 64 bytes                            | `TestATaskOfExactlySixtyFourBytesIsAcceptedAndASixtyFifthByteIsRefusedWhateverTheCharacters`, `TestATaskRefusesControlLineSeparatorAndInvisibleFormattingCharactersAndTheListIsUnchanged`, `TestEveryFieldAcceptsExactlySixtyFourBytesAndRefusesTheUnicodeLineBreaks` |
| Keep the old task or its time when an id registers again, or store a task time of the caller's list entry | `TestRegisteringAnIDAgainReplacesTheWholeEntryAndSoClearsTheTaskAndItsTime`                                                                                                                                                                                           |
| Drop the task or its time when storing                                                                    | `TestATaskAndItsTimeAreStoredAndFoundUnchanged`                                                                                                                                                                                                                       |
| Check the task before the id or the harness                                                               | `TestABadTaskDoesNotHideABadIDOrAMissingHarness`                                                                                                                                                                                                                      |
| Say `agent name` in the bad id error                                                                      | `TestTheErrorForABadIDSaysAgentIDAndNotAgentName`                                                                                                                                                                                                                     |
| Refuse a 4 hex character id, or accept an uppercase one                                                   | `TestAnIDOfFourLowercaseHexCharactersIsValid`                                                                                                                                                                                                                         |
| Add a second entry for an id that is already listed                                                       | `TestARegistrationWhoseIDIsAlreadyListedReplacesItAndDoesNotAddASecondEntry`                                                                                                                                                                                          |

## Fixture sources

- The ids `alpha`, `bravo`, `charlie`, `a3f9`, `b7c2`, and the project `landline` are invented for these tests. They are
  fictional agent names, as `AGENTS.md` requires. The 4 hex ids have the shape ADR 0160 gives an id.
- The task `ship ADR 0160` and the time 2026-11-16T09:30:15Z are invented. The 64 and 65 byte limits come from the
  registry's free-text rule, which the ADR extends to `Task`.
- The query forms for case and padding follow the rule that `Find` matches an id exactly. The Unicode White_Space
  characters used are the ones the harness rule names.
- The landline versions `0.2.0`, `0.2.0-rc 1`, and `dev` are invented values that look like a release, a pre-release,
  and a development build. They are not read from a real build.
- The invisible characters are Unicode category Cf code points from the Unicode Character Database (`UnicodeData.txt`,
  general category `Cf`): U+00AD, U+0600, U+061C, U+180E, U+200B to U+200F, U+202A, U+202D, U+202E, U+2060, U+2066,
  U+2069, U+FEFF, and the tag characters U+E0001 and U+E0041.
- The commit `0123456789abcdef0123456789abcdef01234567` and the date `2026-11-12T16:34:00Z` are invented, with the
  shapes that [0100](../../../docs/decisions/0100-a-build-names-itself-by-version-commit-and-date.md) gives: 40 hex
  characters, and RFC 3339 in UTC to the second. They are not read from a real commit. The registry checks only the
  free-text rules, so values of other shapes are stored too.
