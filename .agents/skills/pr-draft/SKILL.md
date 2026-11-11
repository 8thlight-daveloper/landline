---
name: pr-draft
description: Prepare a branch for review - audit the commit history (authorship, repository-appropriate emails, focused messages, issue linkage), then draft the PR description from the repo's template with honest merge checkboxes. Use when the user is opening a PR, asks to draft a PR description, wants the commit history reviewed before review, or says the branch is ready.
---

# PR draft

Audit the branch's committed history, then draft its PR description. Resolve history findings first when they affect the
description.

Never push and never open the PR without explicit confirmation. Produce the title and body, show them, and stop. If
asked to push, follow the user's and repository's history policies; a request to push does not by itself authorize a
force-push.

**Always open a PR as a draft.** `gh pr create --draft`, or the "Create draft pull request" option in the web UI. Never
open one review-ready, even when the branch looks finished and even when asked to "open the PR" without qualification. A
non-draft PR notifies reviewers and triggers CI/validation bots immediately, so opening ready-for-review is a claim that
the change is ready for someone else's time. The user decides when to make that claim by marking it ready themselves. If
they explicitly ask for a non-draft PR, say that draft is the default here and let them confirm.

## Squash merge changes where effort belongs

Check first:

```bash
git log --format='%s' origin/main | head -5
```

Subjects ending in `(#123)` with PR-template markdown in the body mean the repo squash-merges. When it does:

- The **PR title** becomes the permanent subject line in `main`.
- The **PR body** becomes the permanent commit body.
- Local commit messages are discarded at merge.

So the title gets the care a commit subject would otherwise get, and no rationale should be left only in a local commit
message. Say so if the user has put real reasoning there.

When subjects carry no `(#123)` and merge commits are present, the repo merges rather than squashes, so every local
commit message persists in `main` and has to stand on its own. Audit them against the `commit-message` skill, which
governs their length, register and the why-not-what rule. That skill also owns where overflow goes: mechanics into this
PR description, anything else into a note on the linked issue.

## Step 1: Authorship

```bash
git log --format='%h | %an <%ae> | %cn <%ce>' origin/main..HEAD
git log origin/main..HEAD --format='%b' | grep -i 'co-authored-by'
```

Check author, committer, and co-author addresses against the repository's identity policy and the user's intended
identity. Do not assume an employer, email domain, or address format. A personal address may be inappropriate for a work
repository, while a public project may use a privacy-preserving address.

When one is wrong, propose the fix based on scope. These commands are examples, not authorization to rewrite history:

```bash
git config user.email "<confirmed-address>"   # repo-local identity
git commit --amend --reset-author --no-edit    # single commit, resets author and committer
```

For more than one commit, hand the user `git rebase -i` or `git filter-branch`/`git-filter-repo` instructions rather
than running a history rewrite unprompted. Confirm the correct address against the repo's own history
(`git log --format='%ae' origin/main | sort -u`) rather than guessing a format.

## Step 2: Commit history

Judge the sequence as a reviewer would read it:

- **Legible progression.** Does each commit have a reason to exist? Under squash merge the sequence only matters for
  review, so the bar is "does this help someone read the change," not "is it bisectable."
- **No process noise.** `wip`, `fixup`, `address feedback`, `oops`, or a commit whose message is a file list. Offer to
  squash them.
- **Messages answer why.** A subject should give readers useful search terms. A body that narrates the implementation,
  or reproduces a design decision that belongs in the ticket, is worse than no body. Apply the user's commit style from
  their global instructions.
- **Voice.** Commit messages are external text written on the user's behalf, so match their voice rather than a fuller
  explanatory register: terse, first person and active, no hedging openers or scene-setting preambles, no citation-style
  file/line parentheticals unless essential. Proper grammar, capitalization and punctuation, this is not Slack. Say it
  as plainly and directly as the subject allows, so a reader carries as little complexity as possible: short sentences,
  common words, no clause stacked on a clause. Lead with the finding or action. Cut a trailing "why this matters" clause
  when the action already implies it. Prefer periods, commas and parentheses over em dashes or `--`. When referring to a
  tracked ticket, use that tracker's own status verb ("closed", "resolved") rather than a paraphrase. The same voice
  applies to the PR description in step 4.
- **Subject form.** Imperative, under ~72 characters, no trailing period.

Report each problem with the commit hash and a concrete replacement message. Do not rewrite history to fix wording
unless asked.

## Step 3: Issue linkage

Read the repository's contribution rules to learn whether an issue is required and how it should be linked. Inspect the
branch name, commit messages, and any existing PR body for references in the project's tracker format.

When a ticket is linked, read it through an available authorized tool or its URL and confirm it describes the work
actually on the branch. If the implementation diverged from its scope or acceptance criteria, identify what needs
updating. Do not modify the ticket without authorization.

If the tracker is unavailable, report that its state could not be verified. Do not infer that an issue is missing from
an authentication error or empty response. Ask for a missing reference only when the repository requires one; otherwise
draft the PR without inventing a ticket.

## Step 4: Draft the description

Read the repo's template (`.github/PULL_REQUEST_TEMPLATE.md`) at draft time rather than working from memory; it changes.
Then:

- Fill every non-optional section. Handle optional sections and instructional comments according to the template. If no
  template exists, use a concise summary and validation section.
- **Do not hard-wrap prose.** Write each paragraph as one unbroken line and let the service soft-wrap it. Manual
  mid-paragraph line breaks render as ragged text on the web and reflow badly on later edits. Reserve line breaks for
  real structure: list items, headings, code blocks.
- **Summary** answers what and why, and states the user-visible impact. Lead with the problem, not the mechanism.
- **Details** exists for reviewers, so use it for review order on a wide diff, and for anything non-obvious a reader
  would otherwise flag. Skip it on a small change.
- **Design**, when the template requests it, links the design document or discussion that preceded the work. Check any
  required design process in the repository rather than assuming a particular document or approval exists.
- **Test Plan** states what was run and what passed. Name anything that could not be verified and why. UI changes need
  screenshots; ask the user for them rather than describing what they would show.

### Link files instead of naming them

Whenever the description names a file, function, or line, make it a link into the branch's own code so a reviewer lands
on it in one click:

```bash
git rev-parse HEAD          # the SHA to pin to
git remote get-url origin   # the host and org/repo
```

Shape: `https://<host>/<org>/<repo>/blob/<sha>/<path>#L<line>`, or `#L120-L134` for a range. Always absolute, never
repo-relative, since repo-relative links only resolve when rendered on the web and are useless anywhere else.

Pin the URL to the commit SHA, not a branch name. A branch link silently moves to different code as the branch advances,
while a SHA link keeps pointing at what the reviewer was asked to look at. This is the one place a SHA belongs: in
prose, still say "the branch's HEAD commit" rather than a bare hash, because prose about a SHA goes stale across amends
and rebases.

Regenerate the links if the branch is amended or rebased before the PR opens, and use the SHA that was actually pushed.

## Step 5: Verify the merge checklist

Investigate each checkbox and record the evidence before marking it complete. An unchecked compatibility claim can lead
to a deployment failure.

Use the repository's actual checklist. Common compatibility questions include:

- **Configuration compatibility.** Does the code read new config? Is there a safe default at startup, or does a missing
  key fail the boot?
- **Production database schema.** Does the branch add a migration? Confirm the previous code revision still serves
  during a rolling deploy: new columns nullable or defaulted, no in-place renames, nothing dropped that the running code
  selects.
- **Upstream services.** Are the APIs and capabilities the branch calls available in the environments where it will run?

Leave a box unticked with a one-line reason when the answer is no or unknown. Check a box only when the evidence
supports it.

## Output

1. Authorship result: pass, or the exact fix.
1. Commit history findings, each with hash and suggested message.
1. Issue linkage, when applicable: ticket, scope match, and verification gaps.
1. The proposed PR title.
1. The full PR body, in a fenced block ready to paste.
1. Checkbox answers with evidence, and anything left unverified.

Then stop and wait for confirmation before pushing or opening anything.
