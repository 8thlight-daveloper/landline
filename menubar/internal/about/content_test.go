package about

import (
	"os"
	"strings"
	"testing"

	"github.com/8thlight-daveloper/landline/menubar/internal/buildinfo"
)

func TestTheContentNamesLandlineItsBuildAndTheCredits(t *testing.T) {
	got := ContentFor(buildinfo.Build{Version: "0.2.0", Commit: "a1b2c3d4e5f60718293a4b5c6d7e8f9012345678"})
	want := Content{Name: "Landline", Version: "0.2.0 (a1b2c3d)", CreditsLine: "From 8th Light", Link: Repository}
	if got != want {
		t.Fatalf("%+v", got)
	}
	if got := ContentFor(buildinfo.Build{Version: "dev"}).Version; got != "dev" {
		t.Fatalf("an unstamped build shows %q", got)
	}
}

func TestTheRepositoryAddressIsOneHTTPSConstantInOneFile(t *testing.T) {
	if !strings.HasPrefix(Repository, "https://github.com/") {
		t.Fatal(Repository)
	}
	entries, _ := os.ReadDir(".")
	for _, entry := range entries {
		raw, _ := os.ReadFile(entry.Name())
		if entry.Name() != "repository.go" && !strings.HasSuffix(entry.Name(), "_test.go") && strings.Contains(string(raw), Repository) {
			t.Errorf("%s repeats the address", entry.Name())
		}
	}
}
