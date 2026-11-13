package stamp

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

var (
	fortyHex = regexp.MustCompile(`^[0-9a-f]{40}$`)
	rfc3339Z = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}Z$`)
)

func scriptPath(t *testing.T) string {
	t.Helper()
	path, err := filepath.Abs("stamp.sh")
	if err != nil {
		t.Fatal(err)
	}
	return path
}

func gitEnv(home string) []string {
	return []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home,
		"GIT_CONFIG_GLOBAL=/dev/null",
		"GIT_CONFIG_SYSTEM=/dev/null",
		"GIT_CONFIG_NOSYSTEM=1",
		"GIT_TERMINAL_PROMPT=0",
		"TZ=America/New_York",
		"LC_ALL=C",
	}
}

func requireGit(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
}

func git(t *testing.T, dir string, extraEnv []string, args ...string) string {
	t.Helper()
	full := append([]string{"-c", "user.name=alpha", "-c", "user.email=alpha@example.invalid", "-c", "commit.gpgsign=false", "-c", "init.defaultBranch=main"}, args...)
	cmd := exec.Command("git", full...)
	cmd.Dir = dir
	cmd.Env = append(gitEnv(t.TempDir()), extraEnv...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func write(t *testing.T, path, text string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newRepo(t *testing.T, objectFormat string) string {
	t.Helper()
	requireGit(t)
	dir := t.TempDir()
	args := []string{"init", "-q"}
	if objectFormat != "" {
		args = append(args, "--object-format="+objectFormat)
	}
	cmd := exec.Command("git", append([]string{"-c", "init.defaultBranch=main"}, append(args, dir)...)...)
	cmd.Env = gitEnv(t.TempDir())
	if out, err := cmd.CombinedOutput(); err != nil {
		if objectFormat != "" {
			t.Skipf("this git cannot make a %s repository: %s", objectFormat, out)
		}
		t.Fatalf("git init: %v\n%s", err, out)
	}
	return dir
}

func commit(t *testing.T, dir, committerDate string) {
	t.Helper()
	git(t, dir, []string{
		"GIT_COMMITTER_DATE=" + committerDate,
		"GIT_AUTHOR_DATE=1999-12-31T23:59:59+0000",
	}, "add", "-A")
	git(t, dir, []string{
		"GIT_COMMITTER_DATE=" + committerDate,
		"GIT_AUTHOR_DATE=1999-12-31T23:59:59+0000",
	}, "commit", "-q", "-m", "fixture")
}

type stamped struct {
	commit, date, stdout string
	code                 int
}

func runStamp(t *testing.T, dir string, env []string) stamped {
	t.Helper()
	script := scriptPath(t)
	info, err := os.Stat(script)
	if err != nil {
		t.Fatalf("tools/stamp/stamp.sh does not exist: %v", err)
	}
	if info.Mode()&0o111 == 0 {
		t.Fatalf("%s is not executable", script)
	}
	cmd := exec.Command(script)
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.Output()
	result := stamped{stdout: string(out)}
	if exit, ok := err.(*exec.ExitError); ok {
		result.code = exit.ExitCode()
	} else if err != nil {
		t.Fatalf("running the script: %v", err)
	}
	lines := strings.Split(strings.TrimSuffix(result.stdout, "\n"), "\n")
	if len(lines) != 2 || !strings.HasPrefix(lines[0], "commit=") || !strings.HasPrefix(lines[1], "date=") || !strings.HasSuffix(result.stdout, "\n") {
		t.Fatalf("standard output must be exactly the lines commit=... and date=..., got %q", result.stdout)
	}
	result.commit = strings.TrimPrefix(lines[0], "commit=")
	result.date = strings.TrimPrefix(lines[1], "date=")
	return result
}

func stampIn(t *testing.T, dir string) stamped {
	t.Helper()
	return runStamp(t, dir, gitEnv(t.TempDir()))
}

func cleanRepo(t *testing.T, committerDate string) (string, string) {
	t.Helper()
	dir := newRepo(t, "")
	write(t, filepath.Join(dir, "tracked.txt"), "one\n")
	write(t, filepath.Join(dir, "sub", "deep.txt"), "two\n")
	write(t, filepath.Join(dir, ".gitignore"), "ignored.log\n")
	commit(t, dir, committerDate)
	return dir, git(t, dir, nil, "rev-parse", "HEAD")
}

func TestACleanCommitGivesTheFullHashAndTheCommitterDateInUTCWithZ(t *testing.T) {
	dir, head := cleanRepo(t, "2026-11-12T16:34:00+0000")
	got := stampIn(t, dir)
	if got.code != 0 || got.commit != head || !fortyHex.MatchString(got.commit) || got.date != "2026-11-12T16:34:00Z" {
		t.Fatalf("got %+v, want commit %s", got, head)
	}
}

func TestACommitterOffsetThatIsNotUTCIsConvertedToUTC(t *testing.T) {
	for date, want := range map[string]string{
		"2026-11-12T11:34:00-0500": "2026-11-12T16:34:00Z",
		"2001-02-03T04:05:06+0900": "2001-02-02T19:05:06Z",
		"2026-01-01T00:30:00+0100": "2025-12-31T23:30:00Z",
		"2026-06-30T23:59:59-0000": "2026-06-30T23:59:59Z",
		"2026-03-01T05:45:07+0545": "2026-03-01T00:00:07Z",
	} {
		dir, _ := cleanRepo(t, date)
		got := stampIn(t, dir)
		if got.date != want || !rfc3339Z.MatchString(got.date) {
			t.Errorf("committer date %s: got %q, want %q", date, got.date, want)
		}
	}
}

func TestTheDateIsTheCommitterDateAndNotTheAuthorDateOrTheTimeOfTheRun(t *testing.T) {
	dir, _ := cleanRepo(t, "2010-05-06T07:08:09+0000")
	got := stampIn(t, dir)
	if got.date != "2010-05-06T07:08:09Z" {
		t.Fatalf("got %+v", got)
	}
}

func TestTwoRunsOnTheSameCommitAreIdentical(t *testing.T) {
	dir, _ := cleanRepo(t, "2026-11-12T11:34:00-0500")
	first, second := stampIn(t, dir), stampIn(t, dir)
	if first != second {
		t.Fatalf("%+v then %+v", first, second)
	}
}

func TestAnyTrackedChangeGivesTheHashWithDirtyAndNoDate(t *testing.T) {
	changes := map[string]func(t *testing.T, dir string){
		"a modified file":             func(t *testing.T, dir string) { write(t, filepath.Join(dir, "tracked.txt"), "changed\n") },
		"a modified file in a folder": func(t *testing.T, dir string) { write(t, filepath.Join(dir, "sub", "deep.txt"), "changed\n") },
		"a deleted file":              func(t *testing.T, dir string) { os.Remove(filepath.Join(dir, "tracked.txt")) },
		"a staged change": func(t *testing.T, dir string) {
			write(t, filepath.Join(dir, "tracked.txt"), "staged\n")
			git(t, dir, nil, "add", "tracked.txt")
		},
		"a staged new file": func(t *testing.T, dir string) {
			write(t, filepath.Join(dir, "added.txt"), "new\n")
			git(t, dir, nil, "add", "added.txt")
		},
		"a changed file mode": func(t *testing.T, dir string) { os.Chmod(filepath.Join(dir, "tracked.txt"), 0o755) },
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			dir, head := cleanRepo(t, "2026-11-12T16:34:00+0000")
			git(t, dir, nil, "config", "core.fileMode", "true")
			change(t, dir)
			got := stampIn(t, dir)
			if got.code != 0 || got.commit != head+"-dirty" || got.date != "" {
				t.Fatalf("got %+v, want commit %s-dirty and no date", got, head)
			}
		})
	}
}

func TestAFileTouchedAfterTheCommitWithItsContentUnchangedLeavesTheCommitClean(t *testing.T) {
	dir, head := cleanRepo(t, "2026-11-12T16:34:00+0000")
	time.Sleep(1100 * time.Millisecond)
	now := time.Now()
	for _, name := range []string{"tracked.txt", filepath.Join("sub", "deep.txt")} {
		if err := os.Chtimes(filepath.Join(dir, name), now, now); err != nil {
			t.Fatal(err)
		}
	}
	got := stampIn(t, dir)
	if got.code != 0 || got.commit != head || got.date != "2026-11-12T16:34:00Z" {
		t.Fatalf("got %+v, want clean %s", got, head)
	}
}

func TestAFileRewrittenWithTheSameContentAfterTheCommitLeavesTheCommitClean(t *testing.T) {
	dir, head := cleanRepo(t, "2026-11-12T16:34:00+0000")
	time.Sleep(1100 * time.Millisecond)
	write(t, filepath.Join(dir, "tracked.txt"), "one\n")
	got := stampIn(t, dir)
	if got.code != 0 || got.commit != head || got.date != "2026-11-12T16:34:00Z" {
		t.Fatalf("got %+v, want clean %s", got, head)
	}
}

func TestOnlyUntrackedOrIgnoredFilesLeaveTheCommitCleanWithItsDate(t *testing.T) {
	dir, head := cleanRepo(t, "2026-11-12T16:34:00+0000")
	write(t, filepath.Join(dir, "untracked.txt"), "x\n")
	write(t, filepath.Join(dir, "newdir", "inside.txt"), "x\n")
	write(t, filepath.Join(dir, "ignored.log"), "x\n")
	got := stampIn(t, dir)
	if got.code != 0 || got.commit != head || got.date != "2026-11-12T16:34:00Z" {
		t.Fatalf("got %+v, want clean %s", got, head)
	}
}

func TestADirtyCommitIsStampedWithoutTouchingTheRepository(t *testing.T) {
	dir, _ := cleanRepo(t, "2026-11-12T16:34:00+0000")
	write(t, filepath.Join(dir, "tracked.txt"), "changed\n")
	before := git(t, dir, nil, "status", "--porcelain=v1")
	stampIn(t, dir)
	if after := git(t, dir, nil, "status", "--porcelain=v1"); after != before {
		t.Fatalf("status changed from %q to %q", before, after)
	}
}

func TestADirectoryThatIsNotARepositoryGivesEmptyValuesAndExitsZero(t *testing.T) {
	requireGit(t)
	dir := t.TempDir()
	env := append(gitEnv(t.TempDir()), "GIT_CEILING_DIRECTORIES="+filepath.Dir(dir))
	got := runStamp(t, dir, env)
	if got.code != 0 || got.commit != "" || got.date != "" {
		t.Fatalf("got %+v", got)
	}
}

func TestARepositoryWithNoCommitGivesEmptyValuesAndExitsZero(t *testing.T) {
	dir := newRepo(t, "")
	write(t, filepath.Join(dir, "tracked.txt"), "x\n")
	got := stampIn(t, dir)
	if got.code != 0 || got.commit != "" || got.date != "" {
		t.Fatalf("got %+v", got)
	}
}

func pathWithoutGit(t *testing.T) string {
	t.Helper()
	shims := t.TempDir()
	for _, dir := range []string{"/bin", "/usr/bin"} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if entry.Name() == "git" || strings.HasPrefix(entry.Name(), "git-") {
				continue
			}
			link := filepath.Join(shims, entry.Name())
			if _, err := os.Lstat(link); err == nil {
				continue
			}
			os.Symlink(filepath.Join(dir, entry.Name()), link)
		}
	}
	if _, err := exec.LookPath("git"); err == nil {
		probe := exec.Command("sh", "-c", "command -v git")
		probe.Env = []string{"PATH=" + shims}
		if out, _ := probe.Output(); strings.TrimSpace(string(out)) != "" {
			t.Skipf("git is still found on a path with no git: %s", out)
		}
	}
	return shims
}

func TestWhenGitIsMissingTheValuesAreEmptyAndTheScriptExitsZero(t *testing.T) {
	dir, _ := cleanRepo(t, "2026-11-12T16:34:00+0000")
	env := append(gitEnv(t.TempDir()), "PATH="+pathWithoutGit(t))
	got := runStamp(t, dir, env)
	if got.code != 0 || got.commit != "" || got.date != "" {
		t.Fatalf("got %+v", got)
	}
}

func TestAHashThatIsNotFortyHexCharactersGivesNoCommitAndNoDate(t *testing.T) {
	dir := newRepo(t, "sha256")
	write(t, filepath.Join(dir, "tracked.txt"), "x\n")
	commit(t, dir, "2026-11-12T16:34:00+0000")
	if head := git(t, dir, nil, "rev-parse", "HEAD"); len(head) == 40 {
		t.Skipf("git made a %d character hash", len(head))
	}
	clean := stampIn(t, dir)
	if clean.code != 0 || clean.commit != "" || clean.date != "" {
		t.Fatalf("clean: %+v", clean)
	}
	write(t, filepath.Join(dir, "tracked.txt"), "changed\n")
	dirty := stampIn(t, dir)
	if dirty.code != 0 || dirty.commit != "" || dirty.date != "" {
		t.Fatalf("dirty: %+v", dirty)
	}
}

func TestAValueTheScriptPrintsNeverExceedsFortySixBytes(t *testing.T) {
	dir, _ := cleanRepo(t, "2026-11-12T16:34:00+0000")
	write(t, filepath.Join(dir, "tracked.txt"), "changed\n")
	if got := stampIn(t, dir); len(got.commit) != 46 {
		t.Fatalf("got %+v", got)
	}
}
