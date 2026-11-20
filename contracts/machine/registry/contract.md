# machine/registry

Status: draft, awaiting approval. Written for
[ADR 0200](../../../docs/decisions/0200-a-machine-joins-with-an-invite-code-and-a-keypair-that-outlasts-secret-rotation.md).
Component: `machine/registry`. Issue: none yet.

Holds the invites and the machines of a switchboard as one value, counts failed redeems, and checks a machine's signed
request against a time it is given. Every call takes the value and returns a new one. Nothing is stored.

## Interface

`api.go` defines `Registry`. The value is `model.Machines`, the machine is `model.Machine`, the invite is
`model.Invite`, a remembered signature is `model.SeenSignature`, and a signed request is `model.SignedRequest`. The
errors are in `internal/model/errors.go`.

| Method                                         | Does                                                               |
| ---------------------------------------------- | ------------------------------------------------------------------ |
| `Invite(state, name, random, now, lifetime)`   | Makes a pending invite and returns its code in the printed form    |
| `Redeem(state, code, publicKey, now)`          | Turns a pending invite into an active machine, or counts a failure |
| `Revoke(state, name)` and `RevokeAll(state)`   | Remove a machine or a pending invite, or every one                 |
| `Verify(state, request, now, started, window)` | Checks a signed request and remembers its signature                |
| `Fingerprint(publicKey)`                       | Names a key in 12 hex characters                                   |

## Rules

**Invite.**

- A name is 1 to 32 characters, lower case letters, digits, and hyphens, and starts with a letter. Anything else is
  `model.ErrBadMachineName`. `random` holds at least 8 bytes, so exactly 8 is enough, and fewer is
  `model.ErrNotEnoughRandom`.
- The code has 8 characters. Character `i` is `23456789ABCDEFGHJKMNPQRSTUVWXYZ[random[i] mod 31]`, so the first 8 bytes
  decide it and any further byte is ignored. The result is the returned code with a hyphen after the fourth character,
  and the stored `Invite.Code` without the hyphen. `Invite.Expires` is `now + lifetime`.
- A name is held when an active machine has it, or a pending invite has it and `Expires` is after `now`. A held name is
  `model.ErrMachineHeld`. A pending invite whose `Expires` is at or before `now` does not hold its name, and a new
  invite for that name replaces it.
- A new invite whose code equals the code of a pending invite for another name, which has not expired, is
  `model.ErrCodeInUse`. The value is returned unchanged, and the caller tries again with other random bytes.
- An invite leaves `Failures` unchanged. `Invites` stay sorted by name.

**Redeem.**

- The typed code is read by removing every hyphen (U+002D) and making its ASCII letters upper case. No other character
  is changed, so a Unicode letter that upper cases to an ASCII one is not in the alphabet, and an underscore, a space, a
  tab, a line break, or any other separator stays in the code and makes it invalid. It names a pending invite when the
  result equals the stored `Code` and `Expires` is after `now`. At `Expires` the invite is expired.
- A typed code that names no pending invite is `model.ErrInvalidCode`. That covers an empty code, a code of another
  length, a character outside the alphabet, a used code, an unknown code, and an expired code. The call returns a copy
  of the value with `Failures` one higher. When that makes `Failures` 5, the copy has no `Invites` and `Failures` 0.
- A code that names a pending invite with a `publicKey` that is not 32 bytes is `model.ErrBadPublicKey`. The value is
  returned unchanged, so the invite stays and `Failures` is not counted. A wrong code with a bad key is
  `model.ErrInvalidCode`.
- On success the invite is removed, a `Machine` with the invite's name, a copy of the key, and `Created` equal to `now`
  is added, `Failures` is 0, and the call returns the name. `Active` stays sorted by name.

**Revoke and RevokeAll.**

- `Revoke` removes the active machine with that name, or else the pending invite with that name, whether or not it has
  expired. An unknown name, matched exactly, is `model.ErrUnknownMachine` and the value is returned unchanged.
- `RevokeAll` removes every active machine and every pending invite. It leaves `Failures` and `Seen` as they are.

**Verify.**

- The nonce is an opaque string. The component neither decodes nor checks it, and signs it as given. The signed text is
  these lines joined by a line feed with none at the end: `landline-machine-v1`, the name, the time in decimal, the
  nonce, the method, the path with its query, and the lower case hex SHA-256 of the body. The signature is an Ed25519
  signature of that text under the machine's key.
- The checks run in this order, and the first one that fails is the error. The difference `request.Time - now.Unix()`
  must be no more than `window` in whole seconds either way, or a `*model.ClockSkewError` whose `Seconds` is that signed
  difference and whose text names it and says `clock`. This check comes first and does not look at the name, so a time
  outside the window gets the same error for a known name, a pending name, a revoked name, and a name that never
  existed. The machine must then be active, or `model.ErrUnknownMachine`. `request.Time` must not be less than
  `started.Unix()`, or `model.ErrBeforeStart`. The signature must verify, and a missing or short one does not, or
  `model.ErrBadSignature`. No remembered signature that equals it may have an `Until` after `now`, or
  `model.ErrReplayed`.
- A window counts only its whole seconds, so a window of 90.5 seconds is 90 seconds. It is that count that bounds the
  difference and sets `Until`.
- On success the call returns the name and a copy of the value in which every `Seen` entry with an `Until` at or before
  `now` is dropped, and the signature is added with `Until` equal to the instant `request.Time + window + 1` seconds,
  where `window` counts whole seconds. That is the first moment a copy can no longer pass the window check, so a copy is
  refused as long as it could be accepted and never after. On any error it returns the value it was given.
- With a time inside the window, a pending machine, a revoked machine, and a name that never existed are all
  `model.ErrUnknownMachine`, and that error comes before `model.ErrBeforeStart`.
- A stored key that is not 32 bytes never verifies, so the result is `model.ErrBadSignature`. A time so far from `now`
  that the difference would not fit in an `int64` is a `*model.ClockSkewError`, whatever the signs of the two, and
  `now.Unix()` may be any `int64`. No input makes the call panic.

**Fingerprint.** It is the first 12 characters of the lower case hex SHA-256 of the key bytes themselves.

## Side effects and data access

None. It reads no clock, random source, file, or environment. It **returns a changed copy** of the value.

## Decisions to know

| Decision                                                                                                            | Reversal |
| ------------------------------------------------------------------------------------------------------------------- | -------- |
| The caller passes the time, the start time, the window, and the random bytes, so every rule can be tested           | cheap    |
| The start rule compares whole seconds, so a request in the second of the start works                                | cheap    |
| The window check comes first and ignores the name, so a caller with no credential cannot learn which names exist    | cheap    |
| An invalid code and a bad key with a valid code are different errors, and only the second leaves the value          | cheap    |
| A signature is remembered until one second after its time plus the window, so no copy gets in after the memory ends | cheap    |
| `Revoke` and `RevokeAll` leave the failure count alone, because a revoke is not a redeem                            | cheap    |
| The code takes each random byte modulo 31, which favors a few characters very slightly                              | cheap    |

## Open questions

None.

## Properties

- **No memory.** The same call gives the same result whatever calls came before.
- **Pure.** The value and the byte slices passed in are the same after every call.
- **Deterministic.** Two calls with the same arguments give identical results.
- **Sorted.** `Active` and `Invites` are sorted by name in every returned value.

## Not covered

- The length and the encoding of the nonce, which the switchboard checks before it calls the component.
- A lifetime or a window of zero or less.
- Duplicate names in a value passed in. The registry never creates one.
- A `random` slice, a key, or a signature that is nil and not empty, beyond the length rules above.
- The order of `Seen`, which a caller does not read.
- How the switchboard turns an error into a status code, which the acceptance tests cover.

## Wrong implementations tried

Each row was applied to a scratch build that passes the tests, and the tests failed it. The scratch build is not kept.
Two more changes, refusing a replay at the moment its memory ends and refusing a replay after it, cannot be told from a
correct build, because a copy is out of the window by then.

| Wrong implementation                                                             | Caught by                                                                                                                                                                                                                                         |
| -------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Take the code from the bytes modulo 30, or from other bytes                      | `TestInviteMakesAnEightCharacterCodeFromTheFirstEightRandomBytesModuloThirtyOne`                                                                                                                                                                  |
| Need more than 8 random bytes                                                    | `TestInviteWithExactlyEightRandomBytesIsAcceptedAndMakesTheCodeFromThem`                                                                                                                                                                          |
| Need 16 random bytes                                                             | `TestInviteMakesAnEightCharacterCodeFromTheFirstEightRandomBytesModuloThirtyOne`, `TestInviteWithExactlyEightRandomBytesIsAcceptedAndMakesTheCodeFromThem`                                                                                        |
| Allow upper case letters in a name                                               | `TestInviteAcceptsAMachineNameOfOneToThirtyTwoLowercaseLettersDigitsAndHyphensStartingWithALetter`                                                                                                                                                |
| Let an invite take a name that an active machine holds                           | `TestInviteIsRefusedForANameAnActiveMachineHolds`                                                                                                                                                                                                 |
| Let an invite take a name that a pending invite holds                            | `TestInviteIsRefusedForANameAPendingInviteHoldsUntilItExpires`                                                                                                                                                                                    |
| Keep a name held after its invite expired                                        | `TestInviteIsRefusedForANameAPendingInviteHoldsUntilItExpires`                                                                                                                                                                                    |
| Ignore the lifetime given and use five minutes                                   | `TestInviteUsesTheLifetimeItIsGiven`                                                                                                                                                                                                              |
| Give two pending names the same code                                             | `TestInviteRefusesACodeThatAnotherPendingInviteAlreadyHoldsAndKeepsTheFirstInvite`                                                                                                                                                                |
| Refuse a code that an expired invite once had                                    | `TestInviteRefusesACodeThatAnotherPendingInviteAlreadyHoldsAndKeepsTheFirstInvite`, `TestRedeemTakesTheLiveInviteWhenAnExpiredOneHasTheSameCode`                                                                                                  |
| Accept a code at the moment it expires                                           | `TestInviteUsesTheLifetimeItIsGiven`, `TestRedeemOfAnExpiredCodeIsInvalidAtTheExpiryAndValidOneNanosecondBefore`, `TestRedeemOfAnExpiredCodeWithAMalformedKeyIsAnInvalidCode`                                                                     |
| Cancel the invites at the sixth failure                                          | `TestAfterACancelTheNextInviteSurvivesFourFailures`, `TestAnInviteMadeBetweenFailuresDoesNotRestartTheCount`, `TestEachFunctionChangesOnlyWhatItsRulesNameAndLeavesTheRestOfTheValueAlone` and 2 more                                             |
| Cancel the invites at the fourth failure                                         | `TestAGoodRedeemStartsTheFailureCountAgainAtZero`, `TestAfterACancelTheNextInviteSurvivesFourFailures`, `TestFailedRedeemsAreCountedWhenNoInviteIsWaiting` and 1 more                                                                             |
| Wipe the active machines when the invites are cancelled                          | `TestEachFunctionChangesOnlyWhatItsRulesNameAndLeavesTheRestOfTheValueAlone`                                                                                                                                                                      |
| Wipe the remembered signatures when the invites are cancelled                    | `TestEachFunctionChangesOnlyWhatItsRulesNameAndLeavesTheRestOfTheValueAlone`                                                                                                                                                                      |
| Clear the other waiting invites after a good redeem                              | `TestEachFunctionChangesOnlyWhatItsRulesNameAndLeavesTheRestOfTheValueAlone`                                                                                                                                                                      |
| Keep the failure count after a good redeem                                       | `TestAGoodRedeemStartsTheFailureCountAgainAtZero`, `TestEachFunctionChangesOnlyWhatItsRulesNameAndLeavesTheRestOfTheValueAlone`                                                                                                                   |
| Keep the failure count after a cancel                                            | `TestAfterACancelTheNextInviteSurvivesFourFailures`, `TestEachFunctionChangesOnlyWhatItsRulesNameAndLeavesTheRestOfTheValueAlone`, `TestFailedRedeemsAreCountedWhenNoInviteIsWaiting` and 1 more                                                  |
| Count a bad key as a failed redeem                                               | `TestRedeemWithAValidCodeAndAMalformedKeyIsRefusedWithoutUsingUpTheCodeOrCountingAFailure`                                                                                                                                                        |
| Check the key before the code                                                    | `TestRedeemOfAnExpiredCodeWithAMalformedKeyIsAnInvalidCode`, `TestRedeemWithAWrongCodeAndAMalformedKeyIsAnInvalidCodeAndNotABadKey`                                                                                                               |
| Store the caller's key slice and not a copy                                      | `TestARedeemedKeyIsACopyOfTheBytesTheCallerPassed`                                                                                                                                                                                                |
| Read a code in upper case only                                                   | `TestRedeemReadsTheCodeInAnyCaseWithOrWithoutTheHyphen`                                                                                                                                                                                           |
| Leave the hyphens in a typed code                                                | `TestAFailedVerifyReturnsTheStateItWasGivenUnchanged`, `TestAGoodRedeemStartsTheFailureCountAgainAtZero`, `TestARedeemedKeyIsACopyOfTheBytesTheCallerPassed` and 40 more                                                                          |
| Strip only the first hyphen of a typed code                                      | `TestRedeemReadsEveryHyphenAndOnlyASCIILettersAsUpperCase`                                                                                                                                                                                        |
| Strip an underscore or a tab from a typed code                                   | `TestRedeemRefusesACodeTypedWithAnySeparatorOtherThanAHyphenSuchAsAnUnderscoreOrATab`                                                                                                                                                             |
| Strip spaces from a typed code                                                   | `TestRedeemRefusesACodeTypedWithAnySeparatorOtherThanAHyphenSuchAsAnUnderscoreOrATab`, `TestRedeemWithAnyBadCodeIsInvalidCountsAsOneFailureAndChangesNothingElse`                                                                                 |
| Strip every ASCII separator from a typed code                                    | `TestRedeemRefusesACodeTypedWithAnySeparatorOtherThanAHyphenSuchAsAnUnderscoreOrATab`, `TestRedeemWithAnyBadCodeIsInvalidCountsAsOneFailureAndChangesNothingElse`                                                                                 |
| Trim white space around a typed code                                             | `TestRedeemRefusesACodeTypedWithAnySeparatorOtherThanAHyphenSuchAsAnUnderscoreOrATab`, `TestRedeemWithAnyBadCodeIsInvalidCountsAsOneFailureAndChangesNothingElse`                                                                                 |
| Strip a non-breaking space or another Unicode space from a typed code            | `TestRedeemRefusesACodeTypedWithAnySeparatorOtherThanAHyphenSuchAsAnUnderscoreOrATab`                                                                                                                                                             |
| Require the hyphen in a typed code                                               | `TestRedeemReadsTheCodeInAnyCaseWithOrWithoutTheHyphen`                                                                                                                                                                                           |
| Make Unicode letters upper case in a typed code                                  | `TestRedeemReadsEveryHyphenAndOnlyASCIILettersAsUpperCase`                                                                                                                                                                                        |
| Revoke only an active machine and not a pending invite                           | `TestEachFunctionChangesOnlyWhatItsRulesNameAndLeavesTheRestOfTheValueAlone`, `TestRevokeRemovesAPendingInviteSoItsCodeStopsWorkingAndItsNameIsFree`                                                                                              |
| Clear the remembered signatures on a revoke                                      | `TestEachFunctionChangesOnlyWhatItsRulesNameAndLeavesTheRestOfTheValueAlone`                                                                                                                                                                      |
| Clear the failure count on a revoke                                              | `TestEachFunctionChangesOnlyWhatItsRulesNameAndLeavesTheRestOfTheValueAlone`                                                                                                                                                                      |
| Revoke every machine and keep the pending invites                                | `TestRevokeAllRemovesEveryMachineAndEveryPendingInviteAndLeavesTheFailureCountAndTheSeenSignatures`                                                                                                                                               |
| Skip the clock window                                                            | `TestAFailedVerifyReturnsTheStateItWasGivenUnchanged`, `TestVerifyAcceptsATimeExactlyTheWindowFromNowInEitherDirectionAndRefusesOneSecondMore`, `TestVerifyChecksTheWindowBeforeTheStartRule` and 8 more                                          |
| Refuse a time exactly the window away                                            | `TestVerifyAcceptsATimeExactlyTheWindowFromNowInEitherDirectionAndRefusesOneSecondMore`, `TestVerifyCountsOnlyTheWholeSecondsOfAWindowThatHasAFraction`, `TestVerifyCountsTheWindowInWholeSecondsOfNowWhateverTheFraction` and 4 more             |
| Use 60 seconds and ignore the window given                                       | `TestVerifyCountsOnlyTheWholeSecondsOfAWindowThatHasAFraction`, `TestVerifyRefusesACopyUntilItsMemoryRunsOutAndNeverAcceptsItAfter`, `TestVerifyUsesTheWindowItIsGiven`                                                                           |
| Round the current time up to the next second for the window                      | `TestVerifyCountsTheWindowInWholeSecondsOfNowWhateverTheFraction`, `TestVerifyRefusesACopyUntilItsMemoryRunsOutAndNeverAcceptsItAfter`                                                                                                            |
| Round a window that has a fraction to the nearest second                         | `TestVerifyCountsOnlyTheWholeSecondsOfAWindowThatHasAFraction`                                                                                                                                                                                    |
| Round a window that has a fraction up                                            | `TestVerifyCountsOnlyTheWholeSecondsOfAWindowThatHasAFraction`                                                                                                                                                                                    |
| Remember a signature until the unrounded window passes                           | `TestVerifyCountsOnlyTheWholeSecondsOfAWindowThatHasAFraction`                                                                                                                                                                                    |
| Bound the difference and set `Until` with the nearest whole second of the window | `TestVerifyCountsOnlyTheWholeSecondsOfAWindowThatHasAFraction`                                                                                                                                                                                    |
| Subtract the times with no guard against overflow                                | `TestVerifyRefusesATimeWhoseDistanceFromNowDoesNotFitAnInt64WithoutPanickingOrWrapping`                                                                                                                                                           |
| Leave out the overflow guard for a positive `now`                                | `TestVerifyRefusesATimeWhoseDistanceFromNowDoesNotFitAnInt64WithoutPanickingOrWrapping`                                                                                                                                                           |
| Leave out the overflow guard for a negative `now`                                | `TestVerifyRefusesATimeWhoseDistanceFromNowDoesNotFitAnInt64WithoutPanickingOrWrapping`                                                                                                                                                           |
| Guard the overflow for a negative `now` only                                     | `TestVerifyRefusesATimeWhoseDistanceFromNowDoesNotFitAnInt64WithoutPanickingOrWrapping`                                                                                                                                                           |
| Treat a difference that overflows as no difference                               | `TestVerifyRefusesATimeAtTheLimitsOfTheIntegerRangeAsOutOfTheWindowWithoutOverflowing`, `TestVerifyRefusesATimeWhoseDistanceFromNowDoesNotFitAnInt64WithoutPanickingOrWrapping`                                                                   |
| Take the absolute value of the difference                                        | `TestVerifyRefusesATimeAtTheLimitsOfTheIntegerRangeAsOutOfTheWindowWithoutOverflowing`, `TestVerifyRefusesATimeWhoseDistanceFromNowDoesNotFitAnInt64WithoutPanickingOrWrapping`                                                                   |
| Check the machine before the clock window                                        | `TestVerifyGivesTheSameClockErrorWhetherOrNotTheNameBelongsToAMachine`, `TestVerifyReportsAClockErrorForAnyNameWhenTheTimeIsOutsideTheWindow`                                                                                                     |
| Check the start rule before the clock window                                     | `TestVerifyChecksTheWindowBeforeTheStartRule`, `TestVerifyRefusesATimeAtTheLimitsOfTheIntegerRangeAsOutOfTheWindowWithoutOverflowing`, `TestVerifyReportsAnUnknownMachineBeforeATimeBeforeTheStart`                                               |
| Check the machine after the start rule                                           | `TestVerifyReportsAnUnknownMachineBeforeATimeBeforeTheStart`                                                                                                                                                                                      |
| Check the machine after the signature                                            | `TestVerifyRefusesAMachineThatIsNotActive`, `TestVerifyReportsAnUnknownMachineBeforeATimeBeforeTheStart`, `TestVerifyReportsAnUnknownMachineForAnyNameThatIsNotActiveWhenTheTimeIsInsideTheWindow`                                                |
| Compare the start rule below one second                                          | `TestVerifyRefusesATimeBeforeTheSecondTheSwitchboardStartedAndAcceptsATimeInThatSecond`                                                                                                                                                           |
| Skip the start rule                                                              | `TestVerifyRefusesATimeBeforeTheSecondTheSwitchboardStartedAndAcceptsATimeInThatSecond`, `TestVerifyReportsATimeBeforeTheStartBeforeABadSignature`                                                                                                |
| Check the start rule after the signature                                         | `TestVerifyReportsATimeBeforeTheStartBeforeABadSignature`                                                                                                                                                                                         |
| Skip the replay check                                                            | `TestAFailedVerifyReturnsTheStateItWasGivenUnchanged`, `TestARememberedSignatureIsACopyOfTheBytesTheCallerPassed`, `TestVerifyDoesNotTreatASignatureRememberedUntilExactlyNowAsAReplay` and 3 more                                                |
| Forget a signature one second before its last moment                             | `TestVerifyCountsOnlyTheWholeSecondsOfAWindowThatHasAFraction`, `TestVerifyDropsARememberedSignatureWhoseMemoryEndsExactlyNowAndKeepsOneThatEndsANanosecondLater`, `TestVerifyRefusesACopyUntilItsMemoryRunsOutAndNeverAcceptsItAfter` and 1 more |
| Remember a signature twice the window from the moment it was accepted            | `TestVerifyRefusesACopyUntilItsMemoryRunsOutAndNeverAcceptsItAfter`, `TestVerifyRemembersASignatureUntilOneSecondAfterItsTimePlusTheWindow`                                                                                                       |
| Remember a signature for one window from the moment it was accepted              | `TestVerifyRefusesACopyUntilItsMemoryRunsOutAndNeverAcceptsItAfter`, `TestVerifyRemembersASignatureUntilOneSecondAfterItsTimePlusTheWindow`                                                                                                       |
| Never drop a remembered signature                                                | `TestVerifyDropsARememberedSignatureWhoseMemoryEndsExactlyNowAndKeepsOneThatEndsANanosecondLater`, `TestVerifyDropsRememberedSignaturesThatHaveRunOutWhenItAcceptsAnother`                                                                        |
| Keep a remembered signature whose `Until` equals `now`                           | `TestVerifyDropsARememberedSignatureWhoseMemoryEndsExactlyNowAndKeepsOneThatEndsANanosecondLater`                                                                                                                                                 |
| Drop a remembered signature whose `Until` is a nanosecond after `now`            | `TestVerifyDropsARememberedSignatureWhoseMemoryEndsExactlyNowAndKeepsOneThatEndsANanosecondLater`                                                                                                                                                 |
| Refuse a copy whose remembered `Until` equals `now`                              | `TestVerifyDoesNotTreatASignatureRememberedUntilExactlyNowAsAReplay`                                                                                                                                                                              |
| Remember only the latest signature                                               | `TestEachFunctionChangesOnlyWhatItsRulesNameAndLeavesTheRestOfTheValueAlone`, `TestTwoResultsBuiltFromOneStateWithSpareCapacityDoNotShareStorage`, `TestVerifyRemembersEverySignatureItAcceptsForTheirWholeMemory`                                |
| Drop expired signatures in the caller's list                                     | `TestNoFunctionChangesTheStateOrTheBytesItWasGiven`, `TestTwoResultsBuiltFromOneStateWithSpareCapacityDoNotShareStorage`                                                                                                                          |
| Reset the failure count when a request verifies                                  | `TestEachFunctionChangesOnlyWhatItsRulesNameAndLeavesTheRestOfTheValueAlone`                                                                                                                                                                      |
| Clear the waiting invites when a request verifies                                | `TestEachFunctionChangesOnlyWhatItsRulesNameAndLeavesTheRestOfTheValueAlone`                                                                                                                                                                      |
| Leave the nonce out of the signed text                                           | `TestARememberedSignatureIsACopyOfTheBytesTheCallerPassed`, `TestEachFunctionChangesOnlyWhatItsRulesNameAndLeavesTheRestOfTheValueAlone`, `TestTwoResultsBuiltFromOneStateWithSpareCapacityDoNotShareStorage` and 14 more                         |
| Sign the path without its query                                                  | `TestVerifyRefusesASignatureThatCoversAnyOtherNameTimeNonceMethodPathOrBodyOrCameFromAnotherKey`, `TestVerifySignsTheQueryAsPartOfThePath`                                                                                                        |
| Leave the body out of the signed text                                            | `TestARememberedSignatureIsACopyOfTheBytesTheCallerPassed`, `TestEachFunctionChangesOnlyWhatItsRulesNameAndLeavesTheRestOfTheValueAlone`, `TestTwoResultsBuiltFromOneStateWithSpareCapacityDoNotShareStorage` and 13 more                         |
| Leave the name out of the signed text                                            | `TestARememberedSignatureIsACopyOfTheBytesTheCallerPassed`, `TestEachFunctionChangesOnlyWhatItsRulesNameAndLeavesTheRestOfTheValueAlone`, `TestTwoResultsBuiltFromOneStateWithSpareCapacityDoNotShareStorage` and 14 more                         |
| Remember a signature that failed to verify                                       | `TestAFailedVerifyReturnsTheStateItWasGivenUnchanged`                                                                                                                                                                                             |
| Hash the hex text of the key for the fingerprint                                 | `TestFingerprintIsTheFirstTwelveHexCharactersOfTheSHA256OfTheRawKeyBytes`                                                                                                                                                                         |
| Report the clock difference with the wrong sign                                  | `TestVerifyAcceptsATimeExactlyTheWindowFromNowInEitherDirectionAndRefusesOneSecondMore`, `TestVerifyCountsOnlyTheWholeSecondsOfAWindowThatHasAFraction`, `TestVerifyReportsAClockErrorForAnyNameWhenTheTimeIsOutsideTheWindow` and 1 more         |
| Let a pending name sign a request                                                | `TestVerifyRefusesAMachineThatIsNotActive`                                                                                                                                                                                                        |
| Take the first invite with a code and then check its expiry                      | `TestRedeemTakesTheLiveInviteWhenAnExpiredOneHasTheSameCode`                                                                                                                                                                                      |
| Check a replay before the signature                                              | `TestVerifyRefusesAChangedRequestWithARememberedSignatureAsABadSignatureAndNotAReplay`                                                                                                                                                            |
| Count a failed redeem only while an invite is waiting                            | `TestFailedRedeemsAreCountedWhenNoInviteIsWaiting`                                                                                                                                                                                                |
| Panic on a stored key that is not 32 bytes                                       | `TestVerifyNeverVerifiesAgainstAStoredKeyThatIsNotThirtyTwoBytes`                                                                                                                                                                                 |
| Store the caller's signature slice and not a copy                                | `TestARememberedSignatureIsACopyOfTheBytesTheCallerPassed`                                                                                                                                                                                        |
| Verify every machine with the first machine's key                                | `TestVerifyChecksEachMachineWithItsOwnKey`                                                                                                                                                                                                        |
| Refuse a nonce that is not 24 characters                                         | `TestVerifyTakesTheNonceAsAnOpaqueStringAndSignsItAsGiven`                                                                                                                                                                                        |
| Build a redeem result on the caller's machine slice                              | `TestTwoResultsBuiltFromOneStateWithSpareCapacityDoNotShareStorage`                                                                                                                                                                               |

## Fixture sources

- The names `linux-box`, `pending-box`, `alpha`, `bravo`, and `charlie` are invented. They are fictional, as `AGENTS.md`
  requires.
- The keys come from `ed25519.NewKeyFromSeed` on a seed of 32 equal bytes. They belong to no machine.
- The time 2026-11-19T12:00:00Z is invented. The 5 minute lifetime and the 60 second window are the defaults in
  [ADR 0200](../../../docs/decisions/0200-a-machine-joins-with-an-invite-code-and-a-keypair-that-outlasts-secret-rotation.md).
- The code vectors come from the rule `alphabet[byte mod 31]` of this contract. For example the bytes 0 to 7 give
  `2345-6789`, and the bytes 255 down to 248 give `9876-5432`.
- The fingerprints `66687aadf862` and `630dcd2966c4` are the first 12 hex characters of the SHA-256 of 32 zero bytes and
  of the bytes 0 to 31. They were computed with Python's `hashlib`, not with the implementation.
- The signed text follows ADR 0200, "Proving it on a request".
