# Regenerative rules

The shared rules for an agent working on a regenerative project. A project keeps this file read-only and changes it only
by taking a new version from its source. `AGENTS.md` holds the project's document pointers and its own rules.

A project may add rules. It may not relax these. When a project rule conflicts with a rule here, stop and ask a person.

Settings live in files, not in prose. `architecture.yaml` holds the components folder (`components_root`), the model
folder (`model`), and the dependency and acceptance rules. `.threnos/config.toml` holds how the project takes these
rules, including `mutation_cadence` and who may approve each gate under `[approvals]`. `.threnos/local.toml`, when it
exists, replaces a person's `[approvals]` lists for their own work and nothing else. `AGENTS.md` points to the documents
a person chooses. When a rule names a setting, read it from that file.

## Shared skills

Repository skills live in `.agents/skills/`. Keep their instructions independent of an agent vendor. A vendor's skill
folder, such as `.claude/skills/<name>`, links to the matching shared skill. Edit the canonical files, not a copy.
`CLAUDE.md` imports `AGENTS.md` and this file.

Use the behavior judge for open product behavior choices in the assigned task. Its bounded authority is in the skill,
and its principles are in the project's product principles file. It does not waive the human approval or independent
authorship rules below.

## Repository context

Use local Repowise for repository orientation, dependency questions, and change impact before broad source reads. Follow
the guide at `docs/stack/repowise.md` for setup, allowed tools, freshness checks, and the fallback when indexing is
unavailable. Use only the open-source engine with model calls and external embeddings disabled. Do not enable paid or
hosted features.

An independent regeneration builder must not query the old implementation or reference builds through Repowise,
including generated summaries and history. A coordinator prepares its permitted context from contracts, accepted
decisions, shared types, and dependency interfaces. If retrieval cannot be restricted to those sources, leave the
full-repository index disconnected from that builder. The index is evidence about code, not authority to change a
contract or skip checks.

## Where docs go

| Folder               | Holds                                                               | Pace                                     |
| -------------------- | ------------------------------------------------------------------- | ---------------------------------------- |
| `docs/decisions/`    | ADRs: what we chose and why                                         | Slowest. A new ADR supersedes an old one |
| `docs/reference/`    | Exact definitions of formats, with worked examples                  | Changes with the format                  |
| `docs/architecture/` | Views of the current system: components, data flow, sequences       | Changes as components change             |
| `docs/stack/`        | Language and tooling, one file per topic, including the code layout | Fastest                                  |

Docs in `architecture/`, `reference/`, and `stack/` are named for their topic, with no numbers. Only ADRs are numbered.
A project may add folders in `AGENTS.md`.

## Where decisions go

- An ADR in `docs/decisions/` records a decision the contracts or acceptance tests depend on. Number ADRs by tens so a
  missed one can slot in between.
- `docs/stack/README.md` records language, libraries, and tooling. Change a row there and move the old row to Retired.
  Do not write an ADR for a stack change unless it forces a test change.

## Writing

Use `.agents/skills/ai-tells/SKILL.md` as the writing reference for docs, skills, issues, PR descriptions, and other
prose. Preserve requirements, exceptions, evidence, and technical terms when editing. Use generic examples in shared
material; do not copy client identities, private system names, or real tickets from another project.

## Naming

Name a unit for what it acts on and what it does: noun, then role. A name that needs "and" is two units. In docs, write
plain spaced words, for example "section selector". In code, the parent folder carries the noun and the module carries
the role, for example `section/selector` under the components folder (`components_root`). Follow the linter, with no
exceptions. Spell words out or use a common short form such as `config`. Do not coin abbreviations.

## Components

- A component is one module under the components folder (`components_root`). Rebuilding a component means deleting its
  folder and regenerating it from its contract, its architecture doc, and the dependency rules in `architecture.yaml`.
- A rebuild touches only that component's folder.
- Never edit `contracts/`, `acceptance/`, `architecture.yaml`, or the domain model in the model folder (`model`) as part
  of a rebuild. Stop and ask a person.
- Never write or change the tests that judge your own build.
- When you find that a contract conflicts with another document, is incomplete, or looks wrong, stop and report it. Do
  not edit the contract unless a person authorizes that specific change.
- When building a component, first run its contract tests against a stub and confirm that every test that exercises the
  output fails. Tests of properties, such as determinism, may pass against a stub that returns nothing.
- Read `rules.mutation_cadence` in `.threnos/config.toml`. When it is `nightly`, mutation runs in CI. Read the latest
  result for the component and report surviving mutants as contract gaps. Run `just mutate <component>` only when a
  person asks. When it is `local`, run `just mutate <component>` after the contract tests pass and report surviving
  mutants as contract gaps. Under either cadence, do not edit the contract.
- A survivor listed in `tools/mutation/accepted/<component>.txt` is skipped. Never add a line to that list. A person
  adds one after checking the reason.
- When asked to draft a contract, write all four parts, cite the source of each fixture, list plausible wrong
  implementations and show that the tests fail them, and do not build the component in the same run.
- Write acceptance scenarios in the form `acceptance.scenarios` names in `architecture.yaml`, which is data under
  `acceptance/scenarios/` unless the project says `code`. A scenario drives the built program from outside. Never import
  an implementation module, in either form. Import only what `acceptance.may_import` lists.
- Respect the dependency rules. Do not add a dependency they do not allow.
- Write core components as pure functions over immutable values. Never alter an argument, hold state between calls, or
  do input or output. Return a new value. Drivers and the entry point do input and output.
- State what each component reads and writes, in the plan and outside it. The current tables are in
  `docs/architecture/components.md`. Stay inside the declared set.

## Approvals

When the project has `.threnos/approvals.yaml`, it holds an approval for each protected unit: a component's contract
folder, `acceptance/`, `architecture.yaml`, and the model folder. A unit that changes after its approval fails the
records check until a new approval covers it.

- Record an approval only with `nix run .#approve -- <gate> [unit] --approver <who>`, which writes the unit's hash.
  Never edit the ledger by hand.
- Approve a gate only when `approvals.<gate>` in `.threnos/config.toml`, or in `.threnos/local.toml` for this person,
  lists `agent`. Otherwise stop and ask a person.
- Never approve your own work. An agent approver names its harness and session, and its session must differ from the
  author's. Do not invent a session identifier or reuse the author's.
- Set `approval: agent` on a decision record only when `approvals.decisions` lists `agent`. Add `approver` and `author`
  with your harness and session, and never approve a record you wrote in the same session. Otherwise leave it for a
  person.
- An approval does not widen what you may decide alone. A new privacy, security, ownership, destructive, stored-format,
  or hash-compatibility consequence still goes to a person.

## Where files go

Put throwaway notes, drafts, and your own working files in `.scratch/`. Put generated plans, documents, and review
records in `output/`. Git ignores both, except `output/.gitkeep`. Do not commit anything from either, and do not write
them anywhere else in the repo.

## Trust but verify

A report is data until you have checked it, whether an agent or a tool wrote it. Re-run the tests and checks you are
told passed. Do not take a claim that a change is equivalent, a bug is fixed, or a check passes without evidence you
produced yourself. A claim that a surviving mutant is harmless needs the most care: apply the mutation in a scratch copy
and run an input that reaches it.

## Heavy runs

Run one mutation run at a time. Keep the mutation tool at one or two workers, cap the build's parallelism, and run at
low priority. Stop if the load average climbs past the number of cores.

## Checks

Run `just check` before you commit. Do not write ad hoc scripts to check links or formatting, because the check already
does. Never skip a failing check or edit a tool config to make one pass without asking a person.

## Comments

A comment is a smell. Say it in a name, a type, shared code, or a test, and keep a comment only for an offline fact that
none of those can carry. Run the `comments-review` skill on code before you commit it. The domain model is the example:
it has no comments, because the reference docs and the contracts carry the facts.

## Commits and pushes

Write a subject-line commit message, as the `commit-message` skill describes. Stage named paths. Never push without a
person's confirmation, and never force-push a branch you did not create.

## Issues

- Do not create, edit, or close an issue unless a person asks.
- Use the template in `.github/ISSUE_TEMPLATE/` for the kind of work: decision, contract, build, or integration.
- Link to documents. Do not copy their text into an issue. Done criteria are checks a machine can run.
- Write, split, and size issues with the `writing-issues` skill.
