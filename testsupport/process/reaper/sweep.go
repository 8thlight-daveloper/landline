package reaper

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

func Sweep(temp, prefix, own string) ([]Process, error) {
	all, err := list()
	if err != nil {
		return nil, err
	}
	running := map[int]bool{}
	for _, each := range all {
		running[each.PID] = true
	}
	ownName := filepath.Base(own)
	dead := map[string]bool{}
	var killed []Process
	for _, each := range all {
		if each.PID == os.Getpid() {
			continue
		}
		folder, name, found := runFolder(each.Command, temp, prefix)
		if !found || name == ownName {
			continue
		}
		if each.Parent != 1 && running[each.Parent] {
			continue
		}
		isDead, checked := dead[folder]
		if !checked {
			isDead = runIsGone(folder)
			dead[folder] = isDead
		}
		if isDead && syscall.Kill(each.PID, syscall.SIGKILL) == nil {
			killed = append(killed, each)
		}
	}
	return killed, nil
}

func runFolder(command, temp, prefix string) (folder, name string, found bool) {
	command = normalized(command)
	for _, form := range forms(temp) {
		start := form + "/" + prefix
		for offset := 0; ; {
			index := strings.Index(command[offset:], start)
			if index < 0 {
				break
			}
			index += offset
			offset = index + len(start)
			if index > 0 && !strings.ContainsRune(" =\"'", rune(command[index-1])) {
				continue
			}
			rest := command[index+len(form)+1:]
			end := strings.IndexAny(rest, "/ \"'")
			if end < 0 {
				end = len(rest)
			}
			name = rest[:end]
			if name == prefix {
				continue
			}
			return filepath.Join(form, name), name, true
		}
	}
	return "", "", false
}

func runIsGone(folder string) bool {
	if _, err := os.Stat(folder); errors.Is(err, fs.ErrNotExist) {
		return true
	}
	lock, err := os.OpenFile(filepath.Join(folder, lockName), os.O_RDWR, 0)
	if err != nil {
		return false
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return false
	}
	syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	return true
}
