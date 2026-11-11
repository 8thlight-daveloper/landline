---
name: comments-review
description: >-
  Review a file or diff for comments that should be dissolved into better names, types, structure, or tests. Applies SWEaSL (Stop Writing English as a Second Language) by checking whether names, types, structure, or tests can carry each comment's meaning. Covers inline, block, and doc comments. Use when the user asks to strip/replace comments, make code self-documenting, review through a "no comments" lens, or pre-flight a PR for comment smells.
---

# Comments review (SWEaSL)

SWEaSL means "Stop Writing English as a Second Language." Review each comment for information that names, types,
structure, or tests already express or could express. Prefer replacing the comment with code that carries its meaning.

Review only the requested diff, PR, or file. Review the whole repository only when explicitly asked. Check every comment
in scope and record one of two outcomes: a concrete replacement or a reason all seven transformations fail. Do not omit
comments from the report.

Delete a comment directly when it repeats an existing name, type, or structure. Otherwise, propose the code change that
would preserve its information before deleting it. Removing information the code does not yet express is a regression.

Keep a comment only when no transformation can carry its meaning. Identify the fact it preserves and explain why it
needs prose.

## When to invoke

- "Strip the comments" / "remove comments" / "SWEaSL this"
- "Make this self-documenting"
- "Comments are cluttering this up"
- "Review for comment-replacement opportunities"
- General PR-pre-flight cleanup

## Arguments

- **No argument**: review the current diff (`git diff` or staged changes)
- **A file path**: review just that file
- **A diff/PR identifier**: review one file at a time, emit the report for the first changed file, then ask which file
  to review next. Do not flood the user with findings across many files in one response.

## Scope

In scope: inline (`//`), block (`/* ... */`), and doc comments (`///`, `/** */`). Doc comments get the same scrutiny. A
`///` on an internal item that restates its name and signature is EASL exactly like an inline comment. The most common
offender is a doc block that narrates the function body clause by clause.

Judged more leniently, but still not exempt: doc comments on a genuinely external surface (published SDK, a trait third
parties implement) where rustdoc is consumed by people who can't read the body. Even there, a summary that only restates
the signature can be removed. Keep reasons the code cannot express.

Out of scope regardless of style: license headers, copyright notices, SPDX identifiers.

## The checklist

Try these transformations in order. Stop when one preserves the comment's meaning and makes it redundant. Continue only
when the current transformation cannot carry that meaning.

1. **Rename.** Does a better identifier (variable, function, field, type) say what the comment says? This often removes
   comments that restate identifiers.

1. **Intermediate binding.** A complex expression plus an explanatory comment becomes a named `let`. The name replaces
   the comment.

1. **Extract a function.** A section-marker comment above a block becomes a function whose name is the section. The call
   site reads as a table of contents.

1. **Encode in a type.** An invariant or a piece of state described in prose becomes an enum variant, a newtype, a
   `NonZero`, a builder that makes the bad state unrepresentable. Variant and type names carry the fact. (If a comment
   says "this is live / cached / validated / raw," ask whether a variant or wrapper already says it or could.)

1. **Encode a coupling as shared code.** A comment saying "keep in sync with X," "mirrors Y," or "same shape as Z" may
   be replaced by a shared function, constant, or type that both sites use. Extract the shared behavior so one change
   updates both sites.

1. **Write a test.** An invariant about observable behavior ("stays in this state," "writes no event," "retries
   indefinitely") becomes a test that fails when violated. The test outlasts the comment across refactors, and test
   names and assertion messages explain the expected behavior.

1. **Assert it.** A runtime invariant the type system can't hold can sometimes become a `debug_assert!`/`assert!` with a
   message, which both documents and enforces.

Keep a comment only after checking all seven transformations. It may carry knowledge outside the code, such as an
upstream quirk, an external ticket or RFC, or a historical failure that can no longer be reproduced. Explain why the
information cannot be preserved by a transformation.

## Recognition aids

Use these categories to find comments, then check each for a replacement.

- **Restating**: describes what the code does. Check steps 1-3. `// iterate users and count active ones`
- **Section marker**: demarcates a phase. Check step 3. `// ---- setup ----`
- **Invariant**: a constraint the code "can't" express. Try steps 4, 6, 7 before believing that.
- **Design narration**: explains why a choice was made. Try steps 4, 5 first; a variant name or a shared function
  usually carries it. Surviving narration is one sentence on the *decision*, never the *mechanics*.

## Worked example: a doc comment that fully dissolves

The examples below are fictional and use generic identifiers.

Before:

```rust
/// Uses the current plugin registry rather than a startup snapshot, so plugins
/// registered later are available, matching the background refresh task.
fn request_context(&self) -> RequestContext {
    // ... PluginRegistry::Shared(...), build_settings(...)
}
```

Checklist:

- "current plugin registry rather than a startup snapshot" is already in the `PluginRegistry::Shared(...)` variant (step
  4, already done).
- "plugins registered later are available" is a consequence of reading through the shared registry; verify that behavior
  in a test (step 6).
- "matching the background refresh task" describes a coupling. Extract a shared `RequestContext` assembler both call
  (step 5). Both callers then use the same construction logic.

Result: delete the comment after the shared assembler and test preserve its meaning. Shortening it alone would leave the
duplication.

## Worked example: a behavior fact that moves into a test

Before:

```rust
/// Converts parser errors into display text. Kept separate so it can be tested
/// without opening a file. An unexpected end of input asks the user to check
/// for a missing closing delimiter.
fn format_parse_error(error: ParseError) -> String { /* ... */ }
```

Checklist:

- "Converts parser errors into display text" restates the function name and signature (step 1).
- "Kept separate so it can be tested" is evident from the pure function and its unit tests (step 6).
- The real behavior is the missing-delimiter hint. Add a test named
  `unexpected_end_of_input_suggests_checking_closing_delimiter` that asserts the hint appears (step 6).

Result: comment deleted, the behavior is now checked by a test whose name explains its purpose.

## Worked example: a comment that needs to stay

A fact about an external system can explain a workaround whose original cause cannot be reproduced locally.

In this fictional example, a vendor bulletin documents an old device defect:

```rust
// Older devices reject requests unless this otherwise unused field is present.
// The vendor bulletin linked in the compatibility guide documents the defect;
// affected hardware is no longer available for local reproduction.
```

Checklist:

- Rename, intermediate binding, or extraction (steps 1-3) can name the compatibility behavior, but cannot explain its
  external cause.
- A type or shared helper (steps 4-5) can require the field, but cannot explain why an otherwise unused field is
  required by those devices.
- A test or assertion (steps 6-7) can enforce field presence, but cannot verify the unavailable device's defect or
  replace its provenance.

Result: retain a concise explanation and an actual source link when applying this pattern in a real repository. Do not
claim that a test is impossible merely because reproducing a failure is inconvenient.

## Output format

Enumerate every comment in scope, including obvious cases. Split the report into two lists: **Removed** (or dissolved)
and **Survives**.

For each removed/dissolved comment:

- **Location**: `file:line` or function name
- **Current state**: brief quote of the comment + surrounding code
- **Checklist step that dissolves it**: which transformation (1-7) applies
- **Suggested transformation**: concrete rewrite, rename, extraction, shared function, type, or test sketch; or, for a
  pure restatement, plain deletion naming which existing identifier already says it
- **Confidence**: High / Medium / Low

For a survivor, instead emit **Survives** plus the specific reason every checklist step fails (which offline fact it
carries), so the user can sanity check the classification. Survivors should be rare; if most of your findings survive,
recheck whether a transformation could preserve their meaning.

Sort by confidence descending, then by file location. Do not apply the transformations; the output is a review the user
applies selectively.

## Tone

Give concrete transformations and explain their effect. State uncertainty in the confidence rating and identify any
missing facts. Prefer replacing a comment's information with code over merely shortening it.
