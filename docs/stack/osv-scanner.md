# OSV-Scanner

Record the selected OSV-Scanner version and installation method in the stack index. This setup checks the project's
dependency lockfiles against the OSV vulnerability database. It is supporting development tooling, outside the shipped
program and its correctness tests.

## Operating constraints

OSV-Scanner reads lockfiles on your machine. By default it sends package names and versions to the OSV.dev API to look
up advisories. It does not send source code. Treat the dependency list of a client project as client information. Before
scanning one, confirm the project allows that lookup, or use offline mode.

Offline mode checks a local copy of the advisory database and makes no lookup calls. Download the database once, then
refresh it on a schedule. Flag names have changed between releases, so read the
[offline mode page](https://google.github.io/osv-scanner/usage/offline-mode) for the installed version. Commit-level
scanning does not work offline.

Do not substitute a hosted feed or a model-written summary for the advisory. Read the advisory itself on
[OSV.dev](https://osv.dev) when a finding needs a decision.

## Initial setup

Install OSV-Scanner with the project's tool manager, pinned to a version. Check `osv-scanner --version`. The
[installation guide](https://google.github.io/osv-scanner/installation/) lists the supported methods.

Scan the lockfiles the project commits:

```sh
osv-scanner scan source --recursive .
```

For a project that cannot send its dependency list, download the database and scan offline:

```sh
osv-scanner scan source --offline --download-offline-databases --recursive .
```

Check both commands against the [usage reference](https://google.github.io/osv-scanner/usage/) before you commit them to
a script. A finding makes the command exit nonzero. Keep that behavior so a check can fail on it.

## Use during ordinary development

Run the scan before merging a dependency change, and on a schedule. New advisories appear without any change to your
code, so a passing scan today says nothing about next week. A nightly CI job is the right cadence for most projects.

| Situation                     | Action                                                                                                                        |
| ----------------------------- | ----------------------------------------------------------------------------------------------------------------------------- |
| A finding has a fixed version | Update the dependency, run the project's checks, and review the change as any dependency update                               |
| A finding has no fix          | Read the advisory. Decide whether the vulnerable code path is reachable, and record the decision in an ADR or the stack index |
| A finding does not apply      | Ignore it with a reason and an expiry date in the tool's configuration. Never ignore without a reason                         |
| The scan cannot run           | Report the limitation. Do not report a clean result                                                                           |

A person approves every ignore entry. An agent proposes one and does not add it, in the same way it proposes an accepted
mutation survivor.

## Read the result with care

A clean report does not mean the project is safe. The scanner finds only known advisories that carry affected-version
data, in ecosystems it supports. It does not find flaws in your own code, flaws in a dependency that nobody has
reported, or a package that is malicious but not yet listed.

Check that every lockfile in the repository was scanned. A language without a committed lockfile is not covered by this
setup.

## Preserve independent regeneration

A dependency update is a change to the shipped program, so it follows the project's normal review. A finding does not
authorize an agent to change a contract, a fixture, or a dependency rule. If a fix needs one of those, stop and ask a
person.

## Verify the setup

Confirm the scan finds a known-vulnerable test dependency in a scratch copy, and that the exit code is nonzero. Confirm
the offline path, if used, makes no network calls. Record the version, the schedule, and any ecosystem the project uses
that the scanner does not cover.
