package reaper

import (
	"bufio"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

const (
	folderVariable = "LANDLINE_REAPER_FOLDER"
	pipeDescriptor = 3
	lockName       = "reaper.lock"
	pidName        = "reaper.pid"
	scratchName    = "tmp"
	readyLine      = "ready"
	readyTimeout   = 20 * time.Second
	exitTimeout    = 10 * time.Second
	nameLetters    = "bcdfgjkmqvwxz"
	nameLength     = 12
	nameAttempts   = 100
)

type Runner interface{ Run() int }

type Run struct {
	folder  string
	pipe    *os.File
	reaper  *exec.Cmd
	exited  chan struct{}
	stopped bool
}

var current *Run

func Current() *Run { return current }

func (r *Run) Folder() string { return r.folder }

func (r *Run) ReaperPID() int { return r.reaper.Process.Pid }

func Main(m Runner, prefix string) {
	if folder := os.Getenv(folderVariable); folder != "" {
		os.Exit(serve(folder))
	}
	run, err := Start(prefix)
	if err != nil {
		fmt.Fprintln(os.Stderr, "reaper:", err)
		os.Exit(1)
	}
	current = run
	os.Exit(run.Finish(m.Run()))
}

func Start(prefix string) (*Run, error) {
	temp := os.TempDir()
	folder, err := makeRunFolder(temp, prefix)
	if err != nil {
		return nil, err
	}
	scratch := filepath.Join(folder, scratchName)
	if err := os.Mkdir(scratch, 0o700); err != nil {
		return nil, err
	}
	run, err := startReaper(folder)
	if err != nil {
		os.RemoveAll(folder)
		return nil, err
	}
	if err := keepTemporaryFilesInside(scratch); err != nil {
		run.stop()
		return nil, err
	}
	if killed, err := Sweep(temp, prefix, folder); err != nil {
		fmt.Fprintln(os.Stderr, "reaper: the sweep of earlier runs cannot list processes:", err)
	} else {
		for _, each := range killed {
			fmt.Fprintf(os.Stderr, "reaper: killed pid %d, left by a run that is gone: %s\n", each.PID, each.Command)
		}
	}
	return run, nil
}

func makeRunFolder(temp, prefix string) (string, error) {
	for attempt := 0; attempt < nameAttempts; attempt++ {
		folder := filepath.Join(temp, prefix+lettersOnlyName())
		err := os.Mkdir(folder, 0o700)
		if err == nil {
			return folder, nil
		}
		if !errors.Is(err, os.ErrExist) {
			return "", err
		}
	}
	return "", errors.New("every run folder name tried already exists")
}

func lettersOnlyName() string {
	random := make([]byte, nameLength)
	rand.Read(random)
	for i, each := range random {
		random[i] = nameLetters[int(each)%len(nameLetters)]
	}
	return string(random)
}

func keepTemporaryFilesInside(folder string) error { return os.Setenv("TMPDIR", folder) }

func startReaper(folder string) (*Run, error) {
	read, write, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	readyRead, readyWrite, err := os.Pipe()
	if err != nil {
		read.Close()
		write.Close()
		return nil, err
	}
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), folderVariable+"="+folder)
	cmd.ExtraFiles = []*os.File{read}
	cmd.Stdout = readyWrite
	cmd.Stderr = os.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	started := cmd.Start()
	read.Close()
	readyWrite.Close()
	if started != nil {
		write.Close()
		readyRead.Close()
		return nil, started
	}
	run := &Run{folder: folder, pipe: write, reaper: cmd, exited: make(chan struct{})}
	go func() { cmd.Wait(); close(run.exited) }()
	ready := make(chan error, 1)
	go func() {
		line, err := bufio.NewReader(readyRead).ReadString('\n')
		readyRead.Close()
		if err == nil && strings.TrimSpace(line) != readyLine {
			err = fmt.Errorf("the reaper said %q", line)
		}
		ready <- err
	}()
	select {
	case err := <-ready:
		if err != nil {
			run.stop()
			return nil, fmt.Errorf("the reaper did not start: %w", err)
		}
	case <-time.After(readyTimeout):
		run.stop()
		return nil, errors.New("the reaper did not start in time")
	}
	return run, nil
}

func (r *Run) stop() {
	if r.stopped {
		return
	}
	r.stopped = true
	r.pipe.Close()
	select {
	case <-r.exited:
	case <-time.After(exitTimeout):
		fmt.Fprintln(os.Stderr, "reaper: the reaper did not exit after the run ended")
	}
}

func serve(folder string) int {
	signal.Ignore(syscall.SIGPIPE)
	syscall.CloseOnExec(pipeDescriptor)
	pipe := os.NewFile(pipeDescriptor, "reaper pipe")
	lock, err := os.OpenFile(filepath.Join(folder, lockName), os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return 1
	}
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return 1
	}
	if err := os.WriteFile(filepath.Join(folder, pidName), []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		return 1
	}
	if _, err := fmt.Fprintln(os.Stdout, readyLine); err != nil {
		return 1
	}
	os.Stdout.Close()
	io.Copy(io.Discard, pipe)
	killed := killAll(folder, map[int]bool{os.Getpid(): true})
	os.RemoveAll(folder)
	if killed != nil {
		fmt.Fprintln(os.Stderr, "reaper: the processes of the ended run may still be running:", killed)
		return 1
	}
	return 0
}
