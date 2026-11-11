---
name: writing-issues
description: Write, split, and size GitHub issues for a regenerative software project, where contracts and acceptance scenarios are the lasting artifacts and components are rebuilt from them. Use when drafting a new issue, when an issue feels big, when splitting an epic, or when setting Size, Priority, or Status on the project board.
---

# Writing issues

A regenerative project's lasting artifacts are the ADRs, the domain model, the contracts, and the acceptance scenarios.
A component is disposable: it is rebuilt from its contract, and nobody reads its code to accept it. The shared rules say
so. Issues follow the same shape. An issue is one unit of work a person can review in one sitting and accept or reject,
and what the person reviews is a contract, a scenario, or a decision, not code.

Small issues get reviewed, finish, and unblock each other. Big ones sit in progress and hide their real size. The
mechanics of creating one, such as the templates and `gh` flags, are in the Issues section of the shared rules,
`.agents/rules/regenerative.md`.

## Rules

1. **One verb and one noun in the title.** The naming rule in the shared rules, `.agents/rules/regenerative.md`,
   applies: a name that needs "and" is two units. "Plan file: save, reopen, and assemble" is three issues. A list in a
   title is a list of children.
1. **Cut on a seam, not on size alone.** A seam is a place where one slice can be done, reviewed, and used without the
   next. Look for these, in order:
   - **Verb:** save, open, assemble.
   - **Component:** one issue per component, never two.
   - **Contract increment:** a contract can grow in steps, such as answer checks, then structure checks, then checks
     between sections. Rebuilding is cheap, so a component can follow each step. A person reviews the step, not the
     whole contract again.
   - **Stage:** a contract draft, its hardening pass (the `contract-hardening` skill), and the first build are separate
     issues.
   - **Scenario:** at most three acceptance scenarios in an integration issue.
   - **Document:** one doc, one decision, or one stack row per issue.
   - **Criterion:** more than five done criteria is a sign of more than one issue.
1. **Every leaf issue is XS, S, or M.** L and XL are not allowed on a leaf. L means "split me". XL means epic.
1. **An epic has children and no work of its own.** It names an outcome, lists its sub-issues, and closes when they all
   close. It gets no Size and no done criteria beyond "all children closed". Use the `epic` label.
1. **Done criteria are checks a machine can run.** Exceptions: "A person accepted it" on a decision or a contract, and a
   review by a named kind of person on content.
1. **Link, do not copy.** Link ADRs and docs. Do not paste their text into an issue.
1. **Say what is out of scope.** The next slice belongs to a sibling issue, and the out-of-scope line names it.
1. **A build is one issue per component, once.** A rebuild after a contract change is not an issue. It rides on the
   contract change, as a criterion on the contract-increment issue, done by an agent that did not write the new tests.
   Open a new build issue only for a component that has never been built.
1. **A change to the slowest rate of change gets the `slow-change` label.** That means an ADR, the domain model, or
   `architecture.yaml`. These need a person and an ADR whatever their size. A small edit to the domain model is still a
   big decision, so the label tells the board not to read XS or S as "quick".

## Sizes

Size is the review surface: what a person has to read to accept the work. It is not a time estimate and not a count of
code lines, because nobody reads the regenerated code. A build takes the size of the contract it builds. The calibration
column comes from onboard-kit, the first project built this way. If the project keeps its own examples in the issue
sizing doc that the pointers in `AGENTS.md` name, those replace this column.

| Size | A leaf issue of this size is                                                                                                                                                                            | Calibration                                                                                                                        |
| ---- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------- |
| XS   | Two files or fewer, no new behavior: a stack row, a doc fix, a config line                                                                                                                              | A pointer added to the testing stack doc                                                                                           |
| S    | One decision with a few options, or one contract draft or increment of 15 cases or fewer, or the build of one, or one doc under 100 lines, or an integration with one scenario                          | The encoder contract (14 cases, 7 fixture files) and its build                                                                     |
| M    | One contract draft or increment of 50 cases or fewer, or the build of one, or a hardening pass that adds 50 cases or fewer, or an integration with three scenarios or fewer, or one doc under 300 lines | The first draft of the decoder contract, and its build                                                                             |
| L    | Split it. Not a valid size for a leaf                                                                                                                                                                   | The decoder contract as it was filed, with 89 cases and 94 fixture files. It should have been a draft (M) and a hardening pass (M) |
| XL   | An epic                                                                                                                                                                                                 | The plan file: decode, encode, validate, and edit it                                                                               |

Two checks when the size is unclear:

- If a person could not finish reviewing it in one sitting, it is not M.
- If two reasonable people would size it differently by more than one step, the scope is vague. Narrow the scope until
  they agree.

## Order of a contract

A person approves a contract once, after it is hardened, so the approval covers what will bind regeneration. The order
for a component with a rich input format is:

1. A contract draft, written by an agent. A person reads and gives feedback.
1. A hardening pass, by agents, until a second independent build agrees with the first. The `contract-hardening` skill
   is the procedure.
1. The person approves the hardened contract. The approval is the last criterion of the hardening issue.
1. The first build, in its own issue.

A small component with a simple input can skip step 2, and its draft issue ends with the approval.

## Splitting an issue that already exists

1. Decide the seams first and write the children as a list.
1. Keep the original issue number for the slice that is already in progress or most advanced. Retitle it to that slice,
   and move its criteria.
1. Create the other slices. Make each one a sub-issue of the same parent (`--parent`), and set `--blocked-by` for each
   real dependency.
1. If nothing stays in the original, make it the epic. Add the `epic` label, list the children, and drop its own
   criteria and Size.
1. Leave a comment on the original saying what was split and where each criterion went, so nobody loses a criterion.

## The board

- **Status:** Ready means every blocker is closed and the issue is sized XS, S, or M. Backlog is everything else. An
  issue is In progress when someone is on it, and In review when its change is open for a person.
- **Work in review is limited.** Contracts can be drafted in parallel, because they do not depend on each other's code,
  but approvals are one person reading. Keep at most three contract issues in In review at once. Fill Ready only as fast
  as reviews finish.
- **Priority:** P0 is the critical path to the current milestone. P1 is needed for the milestone but not blocking the
  path. P2 is later. Set it on leaves and epics.
- **Size:** set on leaves only, by the table above.
- **Order inside Ready:** priority first, then the lowest position in the dependency order in `architecture.yaml`, then
  the issue that unblocks the most others.

## Before creating an issue

- [ ] The title has one verb and one noun and no "and"
- [ ] It is a leaf and XS, S, or M, or it is an epic with its children listed
- [ ] It names its template kind: decision, contract, build, integration
- [ ] If it touches an ADR, the domain model, or `architecture.yaml`, it has the `slow-change` label
- [ ] Each done criterion is a check, and there are five or fewer
- [ ] Inputs are links
- [ ] Out of scope names the sibling that takes the next slice
- [ ] `--parent` and `--blocked-by` are set, and Priority and Size are set on the board
