---
name: wall-of-code-review
description: Review a function (or set of functions) for structural decomposition opportunities. Identifies phases within long functions and proposes extractions that let the calling function read as a table of contents. Use this when a function exceeds ~40 lines, contains multiple obvious phases, has indentation depth above 3 levels, or when the user references a "wall of code." Works with or without existing comments; phase identification falls back to structural signals (blank-line clusters, variable lifetimes, indentation breaks, loop boundaries) when comments aren't present.
---

# Wall-of-code review

A function that runs many lines, mixes phases, or buries logic in deep indentation is hard to read regardless of how
well its individual lines are written. Extract phases into named methods so the calling function reads as a table of
contents.

Comments often *help* identify the phase boundaries (especially section markers like "// ---- Setup ----"), but they
aren't required. A long uncommented function still has identifiable phases; finding them just takes more inference. This
skill operates on function shape, not comment presence.

## When to invoke

- The user references a "wall of code" or "wall of X"
- A function exceeds ~40 lines (private) or ~20 lines (public; see asymmetry below)
- A function has indentation depth > 3 levels
- The user asks to decompose, restructure, or refactor a function for readability
- The user says "this is hard to read" about a specific function

## Arguments

- **No argument**: scan the current diff for wall-of-code candidates
- **A file path**: scan just that file
- **A function name** (with file path): focus on that one function

## Procedure

1. **Identify wall-of-X functions.** A function qualifies if any of:

   - Length > ~40 lines (private) / ~20 lines (public)
   - Indentation depth > 3 levels
   - Contains multiple obvious sub-phases (see phase identification below)
   - Contains 2+ section-marker comments
   - Contains 2+ loops or 3+ conditional branches at the top level

1. **Identify phases within each wall.** Use the strongest signal available:

   - **Section-marker comments** (when present): the author has already mentally decomposed the function. Use the marker
     text as the seed for the extracted method name.
   - **Blank-line clustering**: code separated by blank lines is the author's hint about logical groups.
   - **Variable lifetime regions**: a block where `let x = ...` is introduced and `x` is used and then unused often
     delimits a phase.
   - **Mutability shifts**: blocks of pure reads followed by blocks of writes (or vice versa) often mark phase
     boundaries.
   - **Loop / conditional boundaries**: each loop or top-level conditional branch is often its own phase.
   - **Function-call density / shape**: long sequences of similar-shape calls (`let x = self.do_thing(...)`) often
     collapse to one named operation.

1. **Propose decomposition.** For each identified phase:

   - Generate a candidate method name that describes *what* the phase accomplishes, not *how* (e.g.,
     `drain_pending_tasks`, not `await_join_set_with_timeout`)
   - Apply the extraction rubric (below)
   - Reject candidates that fail the rubric

1. **Verify the result passes the table-of-contents test.** After all accepted extractions, the calling function should
   read as a sequence of named operations with no body-reading required to know what each does. If it doesn't, propose
   additional cuts or revise method names.

## Public vs private asymmetry

Public methods (`pub fn`, exported APIs, equivalent) are read more often than they're modified. Their bodies serve as
behavioral documentation to callers. Apply the rubric more aggressively for public methods:

- Length threshold drops to ~20 lines
- Even single-caller helper extractions are encouraged if they make the public method read as a TOC
- The veto conditions still apply, but the bar for invoking them is higher; when in doubt, extract

Private methods serve module authors. They tolerate more inline complexity because the reader is already in the
implementation. Keep ~40 lines as the threshold.

## Extraction rubric

For each "extract this block" suggestion, apply these tests in order:

1. **Replacement test**: would the extraction replace a comment?

   - Yes → strong signal to extract
   - No → continue to next test

1. **Naming test**: can the block be named in ≤5 words that don't appear verbatim in the code already?

   - Yes → extract
   - No → the abstraction isn't valid; rethink the cut

1. **TOC test**: would the calling function read more clearly as a sequence of named operations?

   - Yes → extract
   - No → don't extract for its own sake

### Veto conditions (override the above)

- **5+ parameters would flow through**: the cut line is wrong. Find a different cut, group params into a struct, or
  surface as a data-flow smell (see "use as forcing function" below).
- **Name would be longer than body**: the abstraction is forced. Use a `let`-binding instead.
- **Body is a single expression**: use a `let`-binding, not a function.

### Use as a forcing function

If extraction surfaces a data-flow smell (e.g., the extracted code needs mutable access to 3 caller-scoped variables),
**do not leave the code inline as a workaround**. The smell is real; extraction surfaced it. The correct response is to
rethink the data flow: group state into a struct, move state into the caller's argument list, or reshape the call graph.

Use the extraction attempt to identify which state or dependencies need reorganizing.

## Output format

For each wall-of-X candidate found, emit:

- **Location**: `file:function-name` (with line range)
- **Wall signal**: which heuristic flagged it (length / depth / phases / markers / etc.)
- **Visibility**: public / private (affects threshold)
- **Proposed decomposition**: list of phases with proposed method names and approximate line ranges
- **Resulting TOC**: what the calling function would look like after extraction (a sketch, not full code)
- **Confidence**: High / Medium / Low

Sort by confidence descending, then by file location.

For each proposed extraction, the rubric outcome should be visible: which tests it passed, whether any veto conditions
were considered.

Do not apply the transformations. The output is a review document the user can apply selectively.

## Tone

Propose concrete extractions. State the expected readability benefit and record uncertainty in the confidence rating.
