package command

import (
	"fmt"
	"io"
	"net"
	"strconv"
	"time"
)

const rotatedMessage = "rotated the switchboard secret. Every agent must register again."

func serverRotateSecret(args []string, env Env, dataDir string) int {
	const name = "landline server rotate-secret:"
	if err := checkSwitchboardFlags(args); err != nil {
		fmt.Fprintln(env.Stderr, name, err)
		return 2
	}
	config, err := ReadConfig(env)
	if err != nil {
		fmt.Fprintln(env.Stderr, name, err)
		return 1
	}
	if config.Host != "" && !IsThisMachines(config.Host) {
		fmt.Fprintf(env.Stderr, "%s the saved switchboard host %s is not an address of this machine. Rotate the secret on the machine where that switchboard runs, then run landline server join here again\n", name, config.Host)
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
		err = SaveConfig(env, Config{Secret: secret})
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
	values, _ := FlagValues(args, SwitchboardFlags...)
	host, port, err := listenTarget(values["--addr"], config, env.Getenv)
	if err != nil {
		return "", err
	}
	return net.JoinHostPort(host, strconv.Itoa(port)), nil
}

func savedAddress(config Config) string {
	if config.Host == "" && config.Port == 0 {
		return ""
	}
	port := config.Port
	if port == 0 {
		port = DefaultPort
	}
	return net.JoinHostPort(Or(config.Host, loopback), strconv.Itoa(port))
}

func addressAnswers(address string) bool {
	connection, err := net.DialTimeout("tcp", address, 500*time.Millisecond)
	if err != nil {
		return false
	}
	connection.Close()
	return true
}
