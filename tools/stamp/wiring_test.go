package stamp

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func requireJust(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("just"); err != nil {
		t.Skip("just is not installed")
	}
}

func TestTheBuildAndInstallRecipesRunTheStampScriptAndPassTheThreeLinkerValues(t *testing.T) {
	requireJust(t)
	for _, recipe := range []string{"build", "install"} {
		cmd := exec.Command("just", "--dry-run", recipe)
		cmd.Dir = repoRoot(t)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("just --dry-run %s: %v\n%s", recipe, err, out)
		}
		for _, want := range []string{"tools/stamp/stamp.sh", "-X main.version", "-X main.commit", "-X main.date"} {
			if !strings.Contains(string(out), want) {
				t.Errorf("just %s does not contain %q:\n%s", recipe, want, out)
			}
		}
	}
}

func copyTree(t *testing.T, destination string) {
	t.Helper()
	root := repoRoot(t)
	list := exec.Command("git", "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	list.Dir = root
	out, err := list.Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range strings.Split(string(out), "\x00") {
		if name == "" {
			continue
		}
		source := filepath.Join(root, name)
		info, err := os.Lstat(source)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		raw, err := os.ReadFile(source)
		if err != nil {
			t.Fatal(err)
		}
		target := filepath.Join(destination, name)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(target, raw, info.Mode().Perm()); err != nil {
			t.Fatal(err)
		}
	}
}

func goEnv(t *testing.T, name string) string {
	t.Helper()
	out, err := exec.Command("go", "env", name).Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

func justBuildEnv(t *testing.T) []string {
	t.Helper()
	env := gitEnv(t.TempDir())
	for _, name := range []string{"GOCACHE", "GOMODCACHE", "GOPATH", "GOFLAGS", "GOPROXY"} {
		env = append(env, name+"="+goEnv(t, name))
	}
	return env
}

func justBuild(t *testing.T, dir string) string {
	t.Helper()
	cmd := exec.Command("just", "build")
	cmd.Dir = dir
	cmd.Env = append(justBuildEnv(t), "GIT_CEILING_DIRECTORIES="+filepath.Dir(dir))
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := cmd.Run(); err != nil {
		t.Fatalf("just build: %v\n%s", err, out.String())
	}
	version := exec.Command(filepath.Join(dir, "bin", "landline"), "version")
	version.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + t.TempDir()}
	printed, err := version.Output()
	if err != nil {
		t.Fatalf("landline version: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(printed)), "\n")
	return lines[len(lines)-1]
}

func releaseOf(t *testing.T, dir string) string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(dir, "VERSION"))
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(raw))
}

func TestJustBuildInACommittedCopyPrintsTheCommitAndTheCommitterDateThatTheScriptReports(t *testing.T) {
	requireJust(t)
	requireGit(t)
	dir := t.TempDir()
	copyTree(t, dir)
	git(t, dir, nil, "init", "-q")
	commit(t, dir, "2026-11-12T11:34:00-0500")
	head := git(t, dir, nil, "rev-parse", "HEAD")
	reported := stampIn(t, dir)
	if reported.commit != head || reported.date != "2026-11-12T16:34:00Z" {
		t.Fatalf("the script reports %+v", reported)
	}
	want := "landline " + releaseOf(t, dir) + " (commit " + head + ", built 2026-11-12T16:34:00Z)"
	if got := justBuild(t, dir); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestJustBuildInACopyWithATrackedChangePrintsTheDirtyCommitAndNoDate(t *testing.T) {
	requireJust(t)
	requireGit(t)
	dir := t.TempDir()
	copyTree(t, dir)
	git(t, dir, nil, "init", "-q")
	commit(t, dir, "2026-11-12T11:34:00-0500")
	head := git(t, dir, nil, "rev-parse", "HEAD")
	write(t, filepath.Join(dir, "README.md"), "changed after the commit\n")
	want := "landline " + releaseOf(t, dir) + " (commit " + head + "-dirty)"
	if got := justBuild(t, dir); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestJustBuildInACopyWithNoGitDirectoryStillBuildsAndPrintsNoCommit(t *testing.T) {
	requireJust(t)
	dir := t.TempDir()
	copyTree(t, dir)
	if got, want := justBuild(t, dir), "landline "+releaseOf(t, dir); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}
