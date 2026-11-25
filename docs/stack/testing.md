# Testing

| Need               | Used                                                        | Notes                                                                                                           |
| ------------------ | ----------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------- |
| Test runner        | `go test -race -count=1 ./...`                              | Run a build's tests with `-count=3` too, to catch hidden state                                                  |
| TUI module tests   | `cd tui && go test -race -count=1 ./...` (`just test-tui`)  | The `tui/` module is separate, so `go test ./...` at the root skips it. `just lint-tui` runs gofmt and `go vet` |
| Dependency rules   | `tools/dependency-check/check.py`                           | Imports and clock calls against `architecture.yaml`                                                             |
| Acceptance imports | `tools/acceptance-imports/check.py`                         | Scenarios import no implementation package                                                                      |
| Mutation           | gomu, through `tools/mutation/` (`just mutate <component>`) | Runs against a core component's contract                                                                        |
| Decision records   | `nix flake check` (`threnos-records`)                       | Run by `just check`. Front matter, links, and approvals                                                         |
| Markdown format    | `mdformat --wrap 120`                                       | Run by `just check`                                                                                             |

## Time and ports in acceptance

A scenario builds the three binaries and drives them, so it cannot move a clock. The switchboard takes `--keepalive`,
`--delivery-timeout`, and `--grace` as Go durations. A scenario starts the switchboard with values of a few hundred
milliseconds and waits for the real interval to pass. The switchboard's own tests move the injected clock `Config.Now`
instead.

A scenario starts every switchboard on `--addr 127.0.0.1:0` or a free port it picked. The one check of the default
address sets `LANDLINE_PORT` to a free port, so it runs while a person's own switchboard holds 8787.

## Processes a test starts

A test package that starts a process calls `reaper.Main` from `testsupport/process/reaper` in its `TestMain`. The run
gets its own folder in the temporary directory, named with the package's prefix and random letters, and `TestMain` sets
`TMPDIR` inside that folder, so every `t.TempDir()`, built binary, and copied script lands in it.

- The reaper is the test binary started again in its own session. It holds a pipe that only the test process can write.
  When the test process ends in any way, including SIGKILL or a Ctrl-C to its process group, the reaper kills every
  process whose command line names a path in the run folder, then removes the folder.
- The leak check runs after the tests. A process from the run folder that is still running fails the run, even when
  every test passed, and the check prints its pid and command line. A run that cannot list processes fails too.
- The sweep runs before the tests, as a backup. It kills a process left by an earlier run of the same package only when
  that run's folder is gone or its reaper no longer holds the folder's lock, and the process's parent is pid 1 or gone.

Every process a test starts must name a path in the run folder on its command line, or exit by itself within a second. A
shell script that has to block uses a shell builtin, such as `read _ < fifo` on a FIFO under `t.TempDir()`, rather than
a `sleep` that runs as a child with no path.

A test that must wait for a wall-clock instant uses `sleeper.Until` from `testsupport/clock/sleeper` and never
`time.Sleep(time.Until(...))`, because `Sleep` runs on the monotonic clock and can wake before the wall clock reaches
the instant.
