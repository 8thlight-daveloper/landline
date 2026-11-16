package buildinfo

import (
	"strings"

	"github.com/8thlight-daveloper/landline/menubar/internal/clean"
)

const (
	shortLength = 7
	dirtySuffix = "-dirty"
	unstamped   = "dev"
)

type Build struct {
	Version string
	Commit  string
}

func Short(commit string) string {
	runes := []rune(commit)
	if len(runes) <= shortLength {
		return commit
	}
	short := string(runes[:shortLength])
	if strings.HasSuffix(commit, dirtySuffix) {
		short += dirtySuffix
	}
	return short
}

func Clean(build Build) Build {
	return Build{Version: clean.Text(build.Version), Commit: clean.Text(build.Commit)}
}

func (b Build) Label(name string) string {
	if b.Commit == "" {
		return name + " " + b.Version
	}
	return name + " " + b.Version + " (" + Short(b.Commit) + ")"
}

func (b Build) Display() string {
	if b.Commit == "" {
		return b.Version
	}
	return b.Version + " (" + Short(b.Commit) + ")"
}

func Stamped(version, commit string) Build {
	if version == "" {
		version = unstamped
	}
	return Build{Version: version, Commit: commit}
}
