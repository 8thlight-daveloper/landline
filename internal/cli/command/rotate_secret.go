package command

import (
	"fmt"
	"io"
	"net"
	"net/url"
	"time"
)

const rotatedMessage = "rotated the switchboard secret. Every agent must register again."

func serverRotateSecret(args []string, env Env, dataDir string) int {
	const name = "landline server rotate-secret:"
	if err := checkSwitchboardFlags(args); err != nil {
		fmt.Fprintln(env.Stderr, name, err)
		return 2
	}
	config := LoadConfig(env)
	if config.URL != "" && !isLocalURL(config.URL) {
		fmt.Fprintf(env.Stderr, "%s the saved switchboard address %s is not a loopback address. Rotate the secret on the machine where that switchboard runs, then run landline server join here again\n", name, config.URL)
		return 1
	}
	if env.processEnvironment("LANDLINE_SECRET") != "" {
		fmt.Fprintln(env.Stderr, name, "LANDLINE_SECRET is set in the environment, and it overrides the saved secret for every landline command. Unset it and run this again")
		return 1
	}
	if _, err := findBinary(SwitchboardBinaryName); err != nil {
		fmt.Fprintln(env.Stderr, name, err)
		return 1
	}
	address, err := rotationAddress(args, config, env)
	if err != nil {
		fmt.Fprintln(env.Stderr, name, err)
		return 2
	}
	if _, owned := switchboardPID(dataDir); !owned {
		for _, each := range []string{address, savedAddress(config)} {
			if each != "" && addressAnswers(each) {
				fmt.Fprintf(env.Stderr, "%s something answers at %s and landline server up did not start it, so this command cannot stop it. Stop it where it runs, then run this again\n", name, each)
				return 1
			}
		}
	}
	if values, _ := FlagValues(args, SwitchboardFlags...); values["--addr"] == "" {
		args = append([]string{"--addr", address}, args...)
	}
	quiet := env
	quiet.Stdout = io.Discard
	if code := serverDown(quiet, dataDir); code != 0 {
		return code
	}
	secret, err := newSecret()
	if err == nil {
		config.Secret = secret
		err = SaveConfig(env, config)
	}
	if err != nil {
		fmt.Fprintf(env.Stderr, "%s the new secret was not saved: %v. The switchboard is stopped and %s keeps the old secret. Run landline server up to start it again\n", name, err, configPath(env))
		return 1
	}
	if code := serverUp(args, quiet, dataDir); code != 0 {
		if serverDown(quiet, dataDir) != 0 {
			fmt.Fprintf(env.Stderr, "%s the new secret is saved and the new switchboard did not start and could not be stopped. Run landline server down, then landline server up\n", name)
			return 1
		}
		fmt.Fprintf(env.Stderr, "%s the new secret is saved and the switchboard is stopped. Run landline server up to start it\n", name)
		return 1
	}
	fmt.Fprintln(env.Stdout, rotatedMessage)
	return 0
}

func rotationAddress(args []string, config Config, env Env) (string, error) {
	if values, _ := FlagValues(args, SwitchboardFlags...); values["--addr"] != "" {
		return withPort(values["--addr"])
	}
	if saved := savedAddress(config); saved != "" {
		return withPort(saved)
	}
	return DefaultAddr(env.Getenv)
}

func withPort(address string) (string, error) {
	if _, _, err := net.SplitHostPort(address); err != nil {
		return "", fmt.Errorf("the address %q needs a port, as in 127.0.0.1:8787. Pass --addr host:port", address)
	}
	return address, nil
}

func savedAddress(config Config) string {
	if parsed, err := url.Parse(config.URL); err == nil {
		return parsed.Host
	}
	return ""
}

func addressAnswers(address string) bool {
	connection, err := net.DialTimeout("tcp", address, 500*time.Millisecond)
	if err != nil {
		return false
	}
	connection.Close()
	return true
}
