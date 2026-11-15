# Build stamp

[0100](../decisions/0100-a-build-names-itself-by-version-commit-and-date.md) stamps a build with a version, a commit,
and a date. `tools/stamp/stamp.sh` reads git and `just build` and `just install` both run it. Its tests are in
`tools/stamp/`.

## Interface of `tools/stamp/stamp.sh`

- It takes no arguments and reads the git repository in its working directory.

- It prints exactly two lines to standard output, in this order, and always exits 0:

  ```text
  commit=<value>
  date=<value>
  ```

- The commit is the 40 hex character hash of HEAD. When a tracked file differs from HEAD, the value ends in `-dirty`. An
  untracked file, an ignored file, and a file that was only touched do not count.

- The date is the committer date of HEAD in UTC, as RFC 3339 to the second with a trailing `Z`. It is empty when the
  commit ends in `-dirty`.

- When git is missing, the directory is not a repository, HEAD does not exist, or the hash is not 40 hex characters,
  both values are empty and both lines are still printed.

- It needs only git and POSIX tools, and prints nothing else to standard output.

- The version comes from `VERSION` and is not the script's business.

## Recipes

`just build` and `just install` build all three binaries. They call the script in the recipe body and pass
`-X main.version`, `-X main.commit`, and `-X main.date` to the linker for each of them. A tree without `.git` still
builds, with no commit and no date.

## TUI column

The agent list screen shows the short commit in a `COMMIT` column that comes last in the column order and has the lowest
priority. It is dropped first, is shown whole or not at all (13 columns wide, the width of a short commit with
`-dirty`), and so first appears at a terminal width of 162 columns. The date is not shown on the screen. The unit tests
in `tui/internal/screen` cover the commit cell and the column. An acceptance scenario does not, because acceptance
drives only `landline agents --json` for the TUI.
