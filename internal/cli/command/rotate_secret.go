package command

import (
	"errors"
	"fmt"
	"io"
	"net"
	nethttp "net/http"
	"strconv"
	"syscall"
	"time"

	client "github.com/8thlight-daveloper/landline/internal/client/http"
)

const (
	rotatedMessage     = "rotated the switchboard secret. Every agent must register again."
	reloadCheckPeriod  = 5 * time.Second
	reloadCheckPause   = 100 * time.Millisecond
	rotateSecretPrefix = "landline server rotate-secret:"
)

func serverRotateSecret(args []string, env Env, dataDir string) int {
	if len(args) > 0 {
		fmt.Fprintln(env.Stderr, rotateSecretPrefix, "takes no flags. To start the switchboard with other settings, run landline server restart with them")
		return 2
	}
	config, err := ReadConfig(env)
	if err != nil {
		fmt.Fprintln(env.Stderr, rotateSecretPrefix, printableLine(err.Error()))
		return 1
	}
	if _, err := readServerTable(env); err != nil {
		fmt.Fprintln(env.Stderr, rotateSecretPrefix, printableLine(err.Error()))
		return 1
	}
	if config.Host != "" && !IsThisMachines(config.Host) {
		fmt.Fprintf(env.Stderr, "%s the saved switchboard host %s is not an address of this machine. Rotate the secret on the machine where that switchboard runs, then run landline server join here again\n", rotateSecretPrefix, printableLine(config.Host))
		return 1
	}
	if env.processEnvironment("LANDLINE_SECRET") != "" {
		fmt.Fprintln(env.Stderr, rotateSecretPrefix, "LANDLINE_SECRET is set in the environment, and it overrides the saved secret for every landline command. Unset it and run this again")
		return 1
	}
	if _, err := findBinary(SwitchboardBinaryName); err != nil {
		fmt.Fprintln(env.Stderr, rotateSecretPrefix, printableLine(err.Error()))
		return 1
	}
	host, port, err := config.Listen(env.processEnvironment)
	if err != nil {
		fmt.Fprintln(env.Stderr, rotateSecretPrefix, printableLine(err.Error()))
		return 1
	}
	address := net.JoinHostPort(host, strconv.Itoa(port))
	secret, err := newSecret()
	if err == nil {
		err = SaveConfig(env, Config{Secret: secret})
	}
	if err != nil {
		fmt.Fprintf(env.Stderr, "%s the new secret was not saved: %s. %s keeps the old secret\n", rotateSecretPrefix, printableLine(err.Error()), configPath(env))
		return 1
	}
	if pid, running := switchboardPID(dataDir); running {
		if err := syscall.Kill(pid, syscall.SIGHUP); err != nil {
			fmt.Fprintf(env.Stderr, "%s the new secret is saved in %s, and the switchboard's process %d could not be told to read it: %s. Restart the switchboard where it runs\n", rotateSecretPrefix, configPath(env), pid, printableLine(err.Error()))
			return 1
		}
	} else {
		quiet := env
		quiet.Stdout = io.Discard
		if code := serverUp(nil, quiet, dataDir); code != 0 {
			fmt.Fprintf(env.Stderr, "%s the new secret is saved in %s, and the switchboard did not start. Run landline server up after fixing what it printed\n", rotateSecretPrefix, configPath(env))
			return 1
		}
	}
	if !switchboardUsesOnly(address, secret, config.Secret) {
		fmt.Fprintf(env.Stderr, "%s the new secret is saved in %s, and the switchboard at %s still uses the old one. Restart the switchboard where it runs\n", rotateSecretPrefix, configPath(env), printableLine(address))
		return 1
	}
	fmt.Fprintln(env.Stdout, rotatedMessage)
	return 0
}

func switchboardUsesOnly(address, fresh, old string) bool {
	url := "http://" + address
	deadline := time.Now().Add(reloadCheckPeriod)
	for {
		if acceptsSecret(url, fresh) && (old == "" || refusesSecret(url, old)) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(reloadCheckPause)
	}
}

func acceptsSecret(url, secret string) bool {
	_, err := client.New(url, secret).Machines()
	return err == nil
}

func refusesSecret(url, secret string) bool {
	_, err := client.New(url, secret).Machines()
	var refused *client.StatusError
	return errors.As(err, &refused) && refused.Status == nethttp.StatusUnauthorized
}
