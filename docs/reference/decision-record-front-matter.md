# Decision record front matter

Each file in `docs/decisions/` starts with a YAML block that a machine can read. The block replaces a free-text
`Status:` line. The prose of the record is unchanged.

```yaml
---
category: ux
status: accepted
approval: person
note: Replaces the interaction style of 0340 and 0350. Their save rules stay.
affects: [report/store, shell/menu]
builds-on: ['0150']
partly-supersedes: ['0340', '0350']
---
```

## Fields

| Field                                       | Required                | Holds                                                                                                                                                                 |
| ------------------------------------------- | ----------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `category`                                  | Yes                     | One of the categories below                                                                                                                                           |
| `status`                                    | Yes                     | `proposed`, `accepted`, `superseded`, or `withdrawn`                                                                                                                  |
| `approval`                                  | Yes once accepted       | `person`, `agent`, or `unrecorded` for a record that never said. `agent` means an agent approved it, as the `decisions` gate allows. It needs `approver` and `author` |
| `approver`, `author`                        | With `approval: agent`  | `{kind: person, name}` or `{kind: agent, harness, session}`. The approver's kind matches `approval`. An agent approver's session must differ from the author's        |
| `note`                                      | No                      | One sentence that limits the status, such as "Accepted for experimental use."                                                                                         |
| `affects`                                   | Behavior and ux records | Component ids from `architecture.yaml`. Optional on `architecture`. Not allowed on process records, and never `all`                                                   |
| `principle`                                 | No                      | `none` only, on a `behavior` record, when no principle in `docs/reference/product-principles.md` should cite it. Without the file, the field is unused                |
| `builds-on`                                 | No                      | Record numbers this one depends on, as quoted four-digit strings                                                                                                      |
| `supersedes`, `superseded-by`               | No                      | Whole replacement, in both directions                                                                                                                                 |
| `partly-supersedes`, `partly-superseded-by` | No                      | Replacement of one part, in both directions                                                                                                                           |

Record numbers are quoted so that `0340` stays a string.

## Categories

| Category               | What it decides                                                                                                                                               |
| ---------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `architecture`         | The shape of the product and its code: components, layers, file formats. Its rules live in `architecture.yaml`, which cites the record                        |
| `behavior`             | What the tool does or refuses, whatever it looks like: rules, data meaning, safety, and protocols                                                             |
| `ux`                   | How a person meets it: screens, flows, messages, wording, and the command surface                                                                             |
| `regenerative-process` | The method of building from contracts: how contracts, tests, and agents work together. Reserved for Threnos's own method decisions. A project does not use it |
| `operations`           | How this project runs: its CI, releases, tooling, and one-time exceptions                                                                                     |

A record that does both picks the one that is its main point. A rule about what to refuse is `behavior`, and the message
that says so is `ux`. A project may add categories, and does not reuse `regenerative-process`.

## Where `affects` applies

The product categories say which components a decision changes. `behavior` and `ux` records must write the field. It is
`[]` only while the component the decision describes is not built. `architecture` records may name them, and their rules
bind through `architecture.yaml`. A process record binds through `AGENTS.md`, the workflows, and the stack files, so it
names no component. A decision that touches every component names none, and a contract that builds on it cites it
without being affected by it.

## What it leaves out

- **Implementation state.** "Implemented" and "pending" go stale. A check against the code says it better, so a record
  never holds it.
- **Counts, scores, and dates of checks.** They belong in an issue or a report.

## Checks

The `threnos-records` flake check runs over `docs/decisions/`, `architecture.yaml`, and `contracts/**/contract.md`.
`nix flake check` runs it with the rest. Its source is `tools/records/check.py` in Threnos, and a project that does not
use Nix can run it with Python and PyYAML. It reports:

- a missing field, an unknown field, or a value outside its list;
- a first heading that does not start with the record's number;
- an `affects` id that is not a component in `architecture.yaml`;
- a `builds-on` or replacement id that is not a record;
- a replacement that is not listed on both records;
- `affects` on a process record, or a missing `affects` on a `behavior` or `ux` record;
- a contract that cites a product record missing from its `affects`. A citation is `[0360]`, a path such as
  `decisions/0360-`, or `ADR 0360`. A bare four-digit number is not one, because file modes such as 0640 look the same.

A record whose status is `accepted` or `superseded` is also held to the `decisions` gate in `.threnos/config.toml`. The
default list is `["person"]`:

- an `approval` of `person` or `agent` that is not in `approvals.decisions`, which a person fixes by reading the record
  and setting `approval: person`;
- an `approval: agent` without `approver` and `author`, or whose approver is the author's session or, under
  `independence = "harness"`, the author's harness;
- an `approver` whose kind differs from `approval`, or a party missing its fields.

`unrecorded` passes and counts as no approval. The check prints how many accepted records are `unrecorded`. It reads the
project file and never `.threnos/local.toml`.

When the project has `docs/reference/product-principles.md`, the check also reads it. The file starts with `approval`
(`person` or `agent`) and `reviewed` (a date). Each `### P` heading ends with a `Records:` line of four-digit numbers,
or `none`. It reports:

- a cited record that is missing, superseded, or withdrawn;
- a principle with no `Records:` line;
- an accepted `behavior` record that no principle cites and that does not say `principle: none`;
- an `approval` that is not in the project's `approvals.principles` list in `.threnos/config.toml`, which defaults to
  `["person"]`. The check reads the project file and never `.threnos/local.toml`, so it judges the way CI does. A person
  ratifies an agent's edit by reading it and setting `approval: person`.

Every project needs an `architecture.yaml`. A project without one fails, and the message names the expected path, so a
misnamed or missing file cannot hide behind a skipped check. A project with no `docs/decisions/` folder passes the
record checks.

A check also reports, without failing, an `affects` component whose contract does not cite the record. Contracts hold
rules and fixtures and keep no history, so many never cite the decision that shaped them. Components with no contract,
such as the thin entry point, are skipped.

## Open points

- Whether a product record that spans many components lists them all or only the ones it changes most.
