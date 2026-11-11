# Acceptance scenario format

This format is for data scenarios, the default. A project that writes scenarios as code sets
`acceptance.scenarios: code` in `architecture.yaml` and follows the boundary rules in `process/acceptance.md` in
Threnos.

Adjust this format to the project. It is protected: the runner and every scenario depend on it. `process/acceptance.md`
in Threnos explains why scenarios are data.

## Where they live

- A scenario is one file, `acceptance/scenarios/<name>.yaml`. A file ending in `.example` is a sample the runner
  ignores. Rename it to `.yaml` to use it.
- Files a scenario starts with, and golden outputs, live under `acceptance/testdata/`. A scenario may also point at an
  example under `docs/`.
- The runner lives in `acceptance/runner/`. It is the only language-specific part.

## A scenario

| Field         | Meaning                                                      |
| ------------- | ------------------------------------------------------------ |
| `name`        | Matches the file name                                        |
| `description` | One sentence, in plain words                                 |
| `source`      | Where the expectation comes from: a doc, an ADR, or an issue |
| `steps`       | One or more steps, run in order in one temporary folder      |

### A command step

| Field    | Meaning                                                                                                                        |
| -------- | ------------------------------------------------------------------------------------------------------------------------------ |
| `run`    | The command as an argument list. The first item is the program's name, which the runner replaces with the built program's path |
| `files`  | Files to place before the step. `path: text`, or `path: {from: <file>}` to copy one                                            |
| `stdin`  | Optional text for standard input                                                                                               |
| `expect` | What must be true after the run                                                                                                |

### A protocol step

For a program with a line-delimited JSON mode. The runner writes one request per line, reads one response per line, and
closes standard input when the list ends.

| Field    | Meaning                                                     |
| -------- | ----------------------------------------------------------- |
| `run`    | The command that starts the protocol mode                   |
| `talk`   | A list of `{send: <request>, expect: <response>}`           |
| `expect` | What must be true of the process once standard input closes |

## Expectations

| Field              | Meaning                                                                                                                     |
| ------------------ | --------------------------------------------------------------------------------------------------------------------------- |
| `exit`             | The exit code, exactly. Left out means not checked                                                                          |
| `stdout`, `stderr` | `{json: <value>}` compares as JSON by subset. `{contains: <text>}` checks for the text. `{equals: <text>}` compares exactly |
| `files`            | Per path: `{same-as: <file>}` compares bytes, `{json: <value>}` compares by subset, `{absent: true}` says it must not exist |
| `unchanged`        | Paths whose bytes must be the same as before the step                                                                       |

**Subset comparison.** An expected object matches when every key it names is present and matches. Extra keys and key
order do not matter. A list matches when it has the same items in the same order, each compared by subset. `null`
matches `null`. Numbers compare as 64-bit floats.

## Runner rules

- Each scenario gets its own temporary folder, removed afterward.
- Each step has a time limit, 30 seconds by default. A step that runs over is killed and fails with what was captured.
- A failure names the scenario, the step, and the expectation.
- Scenarios run in parallel and do not depend on each other.
- Paths stay inside their root: no `..`, no absolute paths.
- An unknown field is an error. `talk` and `stdin` do not mix in one step.
