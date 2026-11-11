---
name: commit-message
description: Write a commit message that is a subject line by default. The diff already shows what changed and the ticket or docs already hold the reasoning, so the subject supplies the keywords someone will grep for, and a body appears only when it explains a why that is not in the diff, the ticket, or the docs the commit touches. Use when committing, amending a message, drafting one for review, or when asked to "make a commit" / "commit this" / "write the commit message".
---

# Commit message

Avoid repeating implementation details the diff already shows.

Use the message for search terms and any reasoning the diff does not explain:

1. **Keywords** so someone grepping `git log` a year from now finds this commit at all. That is the subject.
1. **Why** it happened, but only when that reason is recorded nowhere else. The ticket, an ADR, or the docs the commit
   touches are better homes, because they stay current and a commit body does not.

**Default to the subject line alone.** Add a body only when it passes the tests in the Body section. One short paragraph
is the ceiling.

If the repo's `AGENTS.md` or `CONTRIBUTING.md` states a commit convention, follow it over this skill.

This holds whichever way the repo merges, but the stakes differ. Under a merge strategy the body is the permanent record
in `main`. Under squash it is discarded and only shapes how a reviewer reads the branch, while the PR body becomes the
record. Check before assuming, since a repo can flip: `git log --format='%h %p | %s' origin/main | head -12` and look
for two parents.

## Shape

```text
<imperative subject, <=72 chars, no trailing period>

<optional, 1-3 sentences of why. What forced this choice over the one a
reader would expect, when no ticket or doc says so.>

Resolves <ticket>
```

Most commits are the subject line alone. When a body is present, the blank lines are not optional. When the commit
closes a ticket, put `Resolves <ticket>` on its own line at the bottom, in the form the repo's tracker uses
(`Resolves #123`, `Resolves ENG-123`). Only the ticket this commit closes. Skip the line when there is no ticket. Check
recent `git log` for the convention before inventing one.

## Subject

Name the change so someone searching the history can find it.

Imperative mood, as if completing "this commit will ...". Load it with the domain nouns someone would actually search:
the feature, the resource, the symbol, the user-facing concept. Prefer the words that appear in the ticket and the code
over a paraphrase.

- `Preserve image orientation when generating thumbnails`
- `Reject expired password reset links`
- `Clear leaked git env in the pre-commit hook so it runs from a linked worktree`

Not `Fix image bug`, `Update helpers`, `Address review feedback`. Those omit the feature or behavior someone would
search for.

A subject that needs "and" is usually two commits.

## Body

Write one only when the diff's intent is not self-evident **and** the reason is not already recorded in the ticket, an
ADR, or the docs the commit touches. A rename, a version bump, a formatting pass, or a commit that implements a decision
written up elsewhere needs no body. Do not copy reasoning out of a ticket or doc into the commit, because the copy goes
stale.

When you do write one, every sentence has to answer a question the diff raises and neither the diff nor the ticket can
settle:

- What was the reader's experience before, in one clause.
- Why this approach and not the obvious alternative.
- What constraint from outside the repo forced it (an upstream quirk, a client that would break, a policy).

Test each sentence: **could a careful reader derive this from the diff alone?** If yes, cut it. "Adds a field to X and
maps it in Y" is derivable. "Optional rather than required, to keep the published Python SDK reading responses from a
server that predates it" is not.

Name a decision and its reason in the same breath. Skip the design essay, skip the file tour, skip the test inventory.

## Where everything else goes

Three destinations, and the commit body is the smallest of them.

| Content                                                             | Home                                              |
| ------------------------------------------------------------------- | ------------------------------------------------- |
| Why, briefly, when no ticket or doc records it                      | commit body, rarely                               |
| Decisions and their reasoning                                       | the ticket, an ADR, or the doc the commit changes |
| Mechanics, review order, test plan, screenshots, out-of-scope notes | PR description                                    |
| Anything else that still seems worth recording                      | a comment on the ticket, when there is one        |

Use the ticket for context that should remain available across merge strategies. Investigation notes, rejected
alternatives, follow-up work, upstream links, sequencing that mattered at the time: all of it belongs there rather than
in `main`'s history.

Add it as a comment, never by editing the description. Editing the description clobbers the visible body of the ticket.
With no ticket, the PR description is the next best home.

## Voice

Match the committer's voice and repository conventions. The defaults below can be adapted to the author.

- First person, active. "I split it so ..." not "This change splits ...".
- Terse and plain. Short sentences, common words, no clause stacked on a clause.
- Lead with the finding or the action. No preamble, no restating the premise, no "Confirmed" or "As discussed".
- Proper grammar, capitalization and punctuation. The casual lowercase register is Slack only.
- No hedging on a claim. Not "this might", "I think", "arguably".
- No em dashes, and no `--` either. Periods, commas, parentheses, semicolons.
- No connective colons (`X: reason`). Colons label a list, nothing else.
- Never the word "pinning". Use shows, locks in, documents, proves.
- Never "load-bearing". Name the dependency.
- US English. No `behaviour`, no `-ise`.
- Do not adopt vocabulary from the branch's own code or an agent-written ticket just because it is there. Pick the
  plainest accurate word.

## Before committing

1. `git config user.email` must be the address you want in permanent history for this repo. A work repo that falls back
   to a personal global address publishes it. Check once per repo, and compare against recent `git log` authors.
1. Stage deliberately. Name the paths; do not `git add -A` over a tree with unrelated stray files.
1. Let the pre-commit hook run. Never reach for `--no-verify`.

Commit, then stop. Pushing is a separate ask and needs explicit confirmation every time, including to an existing PR
branch.

## Worked example

This example is fictional and uses generic identifiers and a placeholder ticket.

The diff: a new `orientation` field on `ImageMetadata`, a rotation step in `generate_thumbnail`, a renamed CSS class,
and new tests.

Too much, all derivable:

> Add orientation to ImageMetadata and rotate pixels in generate_thumbnail. Rename previewImage to thumbnailImage and
> add rotation tests.

What survives:

> Preserve image orientation when generating thumbnails
>
> The image library clears orientation metadata during export, so rotation must happen before that metadata is
> discarded.
>
> Resolves #123

The subject carries `image orientation` and `thumbnails`. The body explains an external library constraint that
motivated the ordering. The field names, the rotation step, the tests, and the CSS rename are all in the diff, so none
of them are here.

If the placeholder ticket #123 already explains the library constraint, or the decision is written up in an ADR, the
subject alone is enough (plus the closing reference when appropriate).
