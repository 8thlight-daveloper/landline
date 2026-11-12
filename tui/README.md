# landline-tui

A terminal screen for the agents registered with a Landline hub. It is its own Go module and its own binary, as
[decision 0090](../docs/decisions/0090-observers-read-the-agent-list-and-the-tui-is-a-separate-binary.md) describes. It
only reads.

## Usage

```
landline-tui [--interval D]
landline-tui agents --json
landline-tui -h | --help
```

- `landline-tui` opens the agent list and polls the hub. `q` or ctrl+c quits. `r` refreshes at once.
- `--interval D` is a Go duration. The default is `2s` and the floor is `500ms`. When a request takes longer than the
  interval, the next wait is twice that request's time.
- Flags may come before the screen name: `landline-tui --interval 1s agents --json`. `-h` and `--help` print the usage
  on standard output and exit `0`. A bad `--interval` exits `2` with `the interval must be a duration such as 2s`.
- `landline-tui agents --json` prints one snapshot and exits `0`. When the hub cannot be reached, it prints the reason on
  standard error and exits `1`. A usage error exits `2`.

The status line reads `error: <message> | hub <address> | refreshed <time>`, with the error first and only when the last
poll failed. `refreshing...` appears while a fetch runs, and `r` during a fetch says `already refreshing`. After a failed
poll the screen keeps the previous list and marks its header `(stale)` until a poll succeeds. The table drops whole
columns from the lowest priority first when the terminal is narrow (LANDLINE, FOLDER, BRANCH, MODEL, EFFORT, then
HARNESS and LABEL), and `+N more` marks rows that do not fit the height. When HANDLE and STATE cannot both fit, only the
handle shows, and its tail if it is still too wide.

Text that does not come from an agent field is cleaned before it reaches the terminal, the same way agent fields are:
control, line separator, paragraph separator, and format runes become a space, and invalid UTF-8 becomes U+FFFD. That
covers the hub address, every error on the status line, and every message on standard error. The cleaner is
`internal/clean`.

## JSON shape

```json
{
  "screen": "agents",
  "fetched_at": "2026-11-12T10:30:00Z",
  "hub": "http://127.0.0.1:8787",
  "agents": [
    {
      "handle": "alpha",
      "label": "lead",
      "harness": "claude",
      "state": "connected",
      "landline_version": "0.9.0",
      "harness_version": "2.1.0",
      "folder": "/work/app",
      "branch": "main",
      "worktree": "wt",
      "model": "opus",
      "effort": "high",
      "host": "mac"
    }
  ]
}
```

Connected agents come first, then labelled before unlabelled, then by label, then by handle. Empty optional fields are left out. The snapshot holds no
secret and no token.

## How it finds the hub

The same way `landline` does. `LANDLINE_URL` and `LANDLINE_SECRET` win. Otherwise it reads `url` and `secret` from
`$HOME/.config/landline/config.json`. With no address it uses `http://127.0.0.1:<port>`, where the port is
`LANDLINE_PORT` or 8787.

The address is checked as `landline server join` checks it: `http` or `https`, a host, no user information, no query,
and no path beyond `/`. It also refuses a fragment and any control or format rune, which the main module does not check.
A bad address exits `1` with `the hub address must look like http://host:port, with no credentials, query, or fragment`.
The message never repeats the address, so a password in it is never printed. The JSON `hub` field and the status line
never hold user information.

The client does not follow redirects. A `3xx` from the hub is the error `the hub answered with a redirect`, with no
`Location` text, and the bearer token is never sent to another host.

It registers as an observer with the secret, polls with the observer token, and registers again once on a `401`. The
secret and the token are never printed.

## Build and test

```
go build -o landline-tui .
go test -race -count=1 ./...
```
