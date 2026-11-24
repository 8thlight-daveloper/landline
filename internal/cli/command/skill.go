package command

import (
	"bytes"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
)

//go:embed skills/landline
var skillFolder embed.FS

const skillRoot = "skills/landline"

const managedReferencesFolder = "references"

var skillFileNames = []string{"SKILL.md", "references/register.md", "references/agent.md"}

var earlierShippedSkills = map[string]bool{
	"83033d8e4d48a25f9c02423be42272c68ee4cdaff856f22c68b8258f4456a300": true,
	"5af404c4728f43b4495ff25d619186decfcf90af4ede03296009670912ccae21": true,
	"07bc155502da13e409e9480993ceb0749a82ca262cff63d5cc3f069908d1f3c6": true,
}

var managedLine = regexp.MustCompile(`(?m)^<!-- managed by landline setup, build [^ >]+ -->$`)

func skillFolderOf(harness string, env Env) string {
	if harness == "codex" {
		base := env.processEnvironment("CODEX_HOME")
		if base == "" {
			base = filepath.Join(env.Home, ".codex")
		}
		return filepath.Join(base, "skills", "landline")
	}
	return filepath.Join(env.Home, ".claude", "skills", "landline")
}

func renderedSkillFile(name, version string) ([]byte, error) {
	raw, err := skillFolder.ReadFile(skillRoot + "/" + name)
	if err != nil {
		return nil, err
	}
	if version == "" {
		version = "dev"
	}
	return bytes.ReplaceAll(raw, []byte("{{version}}"), []byte(version)), nil
}

func isManagedSkillFile(relative string, content []byte) bool {
	if managedLine.Match(content) {
		return true
	}
	if relative != "SKILL.md" {
		return false
	}
	sum := sha256.Sum256(content)
	return earlierShippedSkills[hex.EncodeToString(sum[:])]
}

func linkTarget(path string) (string, bool) {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink == 0 {
		return "", false
	}
	target, err := os.Readlink(path)
	if err != nil {
		return "", true
	}
	return target, true
}

func keptLink(out io.Writer, path, target string) {
	fmt.Fprintf(out, "kept %s, a link to %s, which is yours\n", printableLine(path), printableLine(target))
}

func installSkillFolder(harness string, env Env) (string, error) {
	dir := skillFolderOf(harness, env)
	if target, linked := linkTarget(dir); linked {
		keptLink(env.Stdout, dir, target)
		return dir, nil
	}
	for _, name := range skillFileNames {
		if err := installSkillFile(dir, name, env); err != nil {
			return dir, err
		}
	}
	return dir, nil
}

func installSkillFile(dir, name string, env Env) error {
	path := filepath.Join(dir, filepath.FromSlash(name))
	if folder := filepath.Dir(path); folder != dir {
		if target, linked := linkTarget(folder); linked {
			keptLink(env.Stdout, folder, target)
			return nil
		}
	}
	if target, linked := linkTarget(path); linked {
		keptLink(env.Stdout, path, target)
		return nil
	}
	if existing, err := os.ReadFile(path); err == nil {
		if !isManagedSkillFile(name, existing) {
			fmt.Fprintf(env.Stdout, "kept %s, which is not managed by landline setup\n", printableLine(path))
			return nil
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	content, err := renderedSkillFile(name, env.Version)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, content, 0o644)
}

func removeSkillFolder(harness string, env Env) error {
	dir := skillFolderOf(harness, env)
	info, err := os.Lstat(dir)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		target, _ := os.Readlink(dir)
		keptLink(env.Stdout, dir, target)
		return nil
	}
	err = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		switch {
		case entry.IsDir():
		case entry.Type()&fs.ModeSymlink != 0:
			target, _ := os.Readlink(path)
			keptLink(env.Stdout, path, target)
		default:
			relative, _ := filepath.Rel(dir, path)
			content, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			if isManagedSkillFile(filepath.ToSlash(relative), content) {
				return os.Remove(path)
			}
			if relative == "SKILL.md" {
				fmt.Fprintf(env.Stdout, "kept %s, which is not managed by landline setup\n", printableLine(path))
			} else {
				fmt.Fprintf(env.Stdout, "kept %s\n", printableLine(path))
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	removeEmptyDir(filepath.Join(dir, managedReferencesFolder))
	removeEmptyDir(dir)
	return nil
}

func removeEmptyDir(dir string) error {
	err := os.Remove(dir)
	if os.IsNotExist(err) || isDirectoryNotEmpty(err) {
		return nil
	}
	return err
}
