---
name: pr-review
description: Review someone else's open PR from a local worktree on their branch. Establishes ground truth first (worktree, branch, real PR number), fans review angles out to subagents, then refuses to emit any finding that is not either an honest question or proven by a test that was actually run. Produces paste-ready GitHub review comments. Use when the user asks to review a PR, review a branch, or says they are in a worktree on someone's PR.
---

# PR review

Review another author's open PR. Two rules govern everything below.

Each finding must be an answerable question about missing context or a claim supported by a test you wrote and ran.
Include assumptions in a question and the test and its output in a proven finding. Do not present suspicion as fact.

Name and check each assumption. Before a candidate becomes a finding, record what it depends on (a config default, an
upstream response, which enum variants exist, what a caller already guards) and go confirm each one in the tree. State
the ones you could not confirm inside the comment itself.

Comment style and accuracy are out of scope. Do not report comment problems or use stale comments as evidence. If a
comment describes behavior the code does not have, the finding is about the code. Deconstructing comments belongs to the
`comments-review` skill and is not part of a review of someone else's work.

Never push, never commit, never post. Produce the comments and stop.

## Step 1: Ground truth

Verify the worktree, branch, and PR identity before reading the diff.

```bash
git rev-parse --show-toplevel
git branch --show-current
git rev-parse --abbrev-ref --symbolic-full-name @{u}
git status --short
gh pr view --json number,title,author,headRefName,baseRefName,state,isDraft,url
git fetch origin
git rev-list --left-right --count @{u}...HEAD
git merge-base HEAD origin/<base>
git log --format='%h %an <%ae> %s' $(git merge-base HEAD origin/<base>)..HEAD
```

What each one settles:

- **Directory versus reality.** A path like `worktrees/pr-123` claims a PR number. If `gh pr view` returns a different
  number, or the branch has no PR at all, stop and ask. Reviewing the wrong branch wastes the author's time and yours.
- **Diff range.** Review `merge-base..HEAD`, never `origin/main..HEAD`. The latter mixes in everything `main` gained
  since the branch left, and findings against other people's merged work are noise.
- **Author.** Confirm from the commits, not the PR metadata alone. If the author is the user, review their own working
  diff using the repository's applicable review workflow.
- **Divergence.** If local HEAD is behind `@{u}`, you are reviewing stale code, so pull before reading. If it is ahead,
  someone has unpushed commits here and the reviewed code is not what the PR shows.
- **Starting cleanliness.** Record it. Later you will add test files, and you need to tell your own scratch work from
  the author's.

You may add uncommitted test files to the user's worktree for verification. Do not commit, amend, push, or force-push
the author's branch.

## Step 2: Read the claim before the code

The PR body and any linked issue are the claim. A finding is a gap between the claim and the code, so read the claim
first.

```bash
gh pr view --json body,title
git log $(git merge-base HEAD origin/<base>)..HEAD --format='%b'
```

Identify issue references using the repository's tracker conventions and read the ticket when one is linked through an
available authorized tool or URL. Report any access limitation rather than inventing ticket contents. Never turn the
description into a finding. Asking an author to document something in their PR body is not review feedback; either the
code raises a real question or there is nothing to say.

Do not adopt the PR's own vocabulary. Description text is often agent-generated and is not house terminology. Use the
plainest accurate word.

## Step 3: Fan out angles to subagents

Assign each selected angle to a separate subagent for independent review. Give every subagent the same contract:
**return candidates, not findings.** No fixes, no patches, no prose essays, no comment nits. Each candidate is one
falsifiable sentence plus `file:line`.

Angles worth separating, chosen per diff rather than run wholesale:

- **Behavior and edge cases** in the changed logic. Empty, absent, zero, error, and concurrent inputs.
- **State and status completeness.** Every variant of an enum or status union the change touches, plus what the fallback
  arm does with an unknown one.
- **Cross-layer agreement.** Compare the API schema, server types, client types, and any generated SDKs that actually
  exist in the repository. Check whether a contract change breaks existing consumers.
- **Supported configurations.** Read the project's supported environments, platforms, and tenancy model, when
  applicable. Check whether changed logic incorrectly assumes one configuration or conflates distinct concepts.
- **Coverage of the claim.** Does any test fail if the change is reverted?
- **Frontend states.** Loading, empty, and error. A choice to hide a control rather than disable it with a reason is a
  deliberate call worth a question.

Give each subagent the base SHA and tell it to read code at HEAD. Verify the returned candidates yourself.

## Step 4: Verify candidates

Check each candidate before including it in the review.

For each candidate:

1. **Re-read the cited code yourself.** Subagents cite lines that moved and describe code that is not there. Open the
   file at HEAD and confirm the quoted shape exists.
1. **Look for the guard elsewhere.** A caller, a type, a middleware, or an upstream validation often already handles it.
   Search before believing.
1. **Check whether it predates the PR.** Compare against the merge-base version of the file. If the same shape is
   already on `main`, it is not this PR's problem. Drop it, or mention it as context with that fact stated plainly,
   never as something the author must fix here.
1. **Check whether the author already disclosed it.** A body that names the follow-up PR, or says a guard lands
   separately, has already answered the candidate. Drop it when that disclosure resolves the concern.
1. **Confirm each named assumption** in the tree: the actual config default, the actual variant list, the actual
   upstream field.
1. **Prefer the file's own convention** over your taste. A local pattern the author followed is not a finding.

Report how many candidates you dropped and why. Verify every candidate, including when all of them turn out to be valid.

## Step 5: Prove it with a test

A surviving candidate becomes a finding only after a test runs. The test targets the **desired** behavior, never a
description of the current behavior.

Look first for the same invariant already asserted somewhere in the suite. When the author has proven it for a
neighboring case, reuse that fixture and echo that test's wording for the case they missed. The comment then measures
the code against the author's own standard rather than importing yours, and the fixture is already known to be valid.

Write it where the repository puts that tier of test. Read its testing instructions, reuse existing fixtures, and follow
its framework and query conventions. Do not assume a language, directory layout, or test runner. A test written in the
wrong tier is not offerable to the author.

Run it. Both outcomes are useful:

- **Red** means the observed behavior differs from the expected behavior. Include the failing assertion and output in
  the comment.
- **Green** means your worry was wrong, but only after you have shown the assertion is capable of failing. Break it on
  purpose: assert the opposite, or aim it at the state that must trigger it, and watch it go red. Only then drop the
  finding.

**Prove your own test can fail before you trust it.** A test that queries for something never present passes no matter
what the code does, and a false green can cause you to discard a valid finding. For example, a role query using an
inappropriate accessible name can match nothing, letting a negative assertion pass without checking the intended
element. A `queryByText` whose string is split across elements and a `data-testid` that was never rendered fail the same
way. When a query returns nothing, first establish that it can ever return something.

For a coverage backfill, where the test passes today and simply protects behavior nothing currently covers, confirm that
the test detects the change. Check out the merge-base version of the changed file, confirm the test goes red, then
restore HEAD. Without that, do not claim the test adds coverage.

Environment failures are not evidence of an application failure. Check the repository's test prerequisites and the
actual error before classifying a failure as environmental. Report the command, limitation, and unverified behavior;
provide a reproduction command when the test needs another environment. Inspect unfamiliar setup or lint commands for
side effects before running them, especially when they may rewrite generated files.

Leave the test as an uncommitted diff. Offer it; do not commit it.

## Step 6: Write the comments

Two forms, chosen per finding. The examples below are fictional.

**Question form**, the default when context might be missing. Name the specific thing, ask something answerable, and
state the assumption you are working from so the author can correct it in one line. Avoid the fake question that is
really a verdict.

> `UploadStatus.tsx:42` treats an absent status as in-progress. Can the upload API return an absent status, or does the
> adapter always populate it? I could not find a default in the adapter.

**Proven form**, when a test ran. State the current behavior against the desired one, show the test, show the outcome,
and offer the test.

> Reverting to the merge-base version of this file leaves the suite green, so nothing currently covers the in-progress
> arm. This test goes red without the change and green with it, if it is useful:
>
> ```ts
> ...
> ```

Voice for both, since these are external text written on the user's behalf: terse, first person, active. No hedging
openers, no scene-setting, no citation-style parentheticals. Proper grammar and punctuation. Prefer periods, commas and
parentheses over em dashes. US English spelling. Avoid the words "load-bearing" and "pinning", and do not write
"projection" when you can name the table or row.

Never classify the work as a defect, bug, or regression. Frame it as current behavior against desired behavior.

When the finding is about tests, say what behavior a test should prove. Never write that existing tests assert the wrong
thing, and never say a change could be deleted with the suite still passing.

Anchor every code reference to a URL a reviewer can click, built from the reviewed SHA rather than the branch name, so
it keeps pointing at the code the author was asked about.

```bash
git rev-parse HEAD
git remote get-url origin
```

Shape: `https://<host>/<org>/<repo>/blob/<sha>/<path>#L<line>`.

## Output

1. Ground truth as a short table: worktree, branch, upstream, PR number and state, author, base, diff range. Call out
   any mismatch with the directory name.
1. What the PR claims, in one or two lines.
1. Angles run, and how many candidates each produced.
1. Candidates dropped, with the count and a one-line reason for the near misses.
1. Findings, blocking questions first, each with location link, form (question or proven), assumptions named including
   the unconfirmed ones, and the evidence.
1. Each finding's paste-ready comment in its own fenced block.
1. Any test written, as a diff, flagged as uncommitted and unoffered until the user says so, with a line on how you
   established it can fail.

Then stop. Do not post the review, comment on the PR, or touch the branch.
