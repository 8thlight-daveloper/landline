package command

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	client "github.com/8thlight-daveloper/landline/internal/client/http"
)

const NoCredentialMessage = "there is no switchboard credential. Run landline server join <host:port> --code <code>, or set LANDLINE_SECRET"

func machineKeyPath(env Env) string { return filepath.Join(env.ConfigDir, "machine.key") }

func readMachineKey(env Env) (ed25519.PrivateKey, bool, error) {
	raw, err := os.ReadFile(machineKeyPath(env))
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("%s cannot be read: %v", machineKeyPath(env), err)
	}
	first, _, _ := strings.Cut(string(raw), "\n")
	seed, decodeErr := base64.StdEncoding.DecodeString(strings.TrimSpace(first))
	if decodeErr != nil || len(seed) != ed25519.SeedSize {
		return nil, false, fmt.Errorf("%s does not hold a 32 byte machine key in base64 on its first line", machineKeyPath(env))
	}
	return ed25519.NewKeyFromSeed(seed), true, nil
}

func writeMachineKey(env Env, key ed25519.PrivateKey) error {
	if err := os.MkdirAll(env.ConfigDir, 0o700); err != nil {
		return err
	}
	path := machineKeyPath(env)
	temporary := path + ".tmp"
	os.Remove(temporary)
	line := base64.StdEncoding.EncodeToString(key.Seed()) + "\n"
	if err := os.WriteFile(temporary, []byte(line), 0o600); err != nil {
		return err
	}
	if err := os.Chmod(temporary, 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

var machineNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,31}$`)

type savedFile struct {
	path    string
	content []byte
	mode    fs.FileMode
	existed bool
}

func snapshotJoinFiles(env Env) (func(changed []string) error, error) {
	paths := []string{machineKeyPath(env), configPath(env)}
	saved := make([]savedFile, len(paths))
	for i, path := range paths {
		saved[i].path = path
		info, statErr := os.Stat(path)
		raw, err := os.ReadFile(path)
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("%s cannot be read, so a failed join could not put it back: %v. Nothing was saved and the code was not used", path, err)
		}
		if statErr == nil && err == nil {
			saved[i].content, saved[i].mode, saved[i].existed = raw, info.Mode().Perm(), true
		}
	}
	return func(changed []string) error {
		var failures []error
		for _, file := range saved {
			os.Remove(file.path + ".tmp")
			if !slices.Contains(changed, file.path) {
				continue
			}
			if err := file.putBack(); err != nil {
				failures = append(failures, fmt.Errorf("%s could not be put back: %v", file.path, err))
			}
		}
		return errors.Join(failures...)
	}, nil
}

func (file savedFile) unchanged() bool {
	info, statErr := os.Stat(file.path)
	if !file.existed {
		return errors.Is(statErr, fs.ErrNotExist)
	}
	raw, readErr := os.ReadFile(file.path)
	return statErr == nil && readErr == nil && info.Mode().Perm() == file.mode && bytes.Equal(raw, file.content)
}

func (file savedFile) putBack() error {
	if file.unchanged() {
		return nil
	}
	if !file.existed {
		return os.Remove(file.path)
	}
	temporary, err := os.CreateTemp(filepath.Dir(file.path), filepath.Base(file.path)+".*.restoring")
	if err != nil {
		return err
	}
	_, writeErr := temporary.Write(file.content)
	closeErr := temporary.Close()
	err = errors.Join(writeErr, closeErr, os.Chmod(temporary.Name(), file.mode))
	if err == nil {
		err = os.Rename(temporary.Name(), file.path)
	}
	if err != nil {
		os.Remove(temporary.Name())
	}
	return err
}

func newMachineKey() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	return ed25519.GenerateKey(rand.Reader)
}

func savedSigner(env Env, saved Config) (*client.Signer, error) {
	key, found, err := readMachineKey(env)
	if err != nil || !found {
		return nil, err
	}
	if saved.Machine == "" {
		return nil, errors.New(machineKeyPath(env) + " is saved but config.toml names no machine. Run landline server join again")
	}
	return &client.Signer{Name: saved.Machine, Key: key}, nil
}

func credentialClient(env Env, url string) (*client.Client, error) {
	if env.signer != nil {
		return client.NewSigned(url, *env.signer), nil
	}
	if env.keyProblem != nil {
		return nil, env.keyProblem
	}
	secret := env.Getenv("LANDLINE_SECRET")
	if secret == "" {
		return nil, errors.New(NoCredentialMessage)
	}
	return client.New(url, secret), nil
}
