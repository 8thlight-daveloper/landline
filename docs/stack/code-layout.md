# Code layout

How the neutral terms in [architecture.yaml](../../architecture.yaml) map to Go folders.

| Term             | In Go                                                                                            |
| ---------------- | ------------------------------------------------------------------------------------------------ |
| Component        | One package under `internal/<noun>/<role>`. The directory carries the noun, the package the role |
| Domain model     | `internal/model`                                                                                 |
| Entry point      | `binary/`, a thin `main`                                                                         |
| Interface file   | `api.go` in the contract package, with the interface and its data types                          |
| Contract tests   | `<role>_test.go` in the contract package                                                         |
| Wiring test      | `wire_test.go`, the only file in a contract package that imports its implementation              |
| Fixtures         | `testdata/` folders, with a `cases.json` manifest                                                |
| Shared test code | `contracts/harness/`                                                                             |

```text
internal/
  model/                 shared types, imports nothing from the project
  <noun>/<role>/         one component per package
contracts/
  harness/               shared test code
  <noun>/<role>/         api.go, contract.md, <role>_test.go, wire_test.go, testdata/
acceptance/              scenarios as Go code, and their runner
binary/                  thin main
tools/mutation/accepted/<noun>/<role>.txt
```
