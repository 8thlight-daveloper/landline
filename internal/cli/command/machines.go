package command

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	client "github.com/8thlight-daveloper/landline/internal/client/http"
)

const adminName = "landline server"

func adminClient(command string, env Env) (*client.Client, Config, int) {
	config, err := ReadConfig(env)
	if err != nil {
		fmt.Fprintf(env.Stderr, "%s %s: %v\n", adminName, command, err)
		return nil, config, 1
	}
	if config.Host != "" && !IsThisMachines(config.Host) {
		fmt.Fprintf(env.Stderr, "%s %s: the saved switchboard host %s is not an address of this machine. Run it on the machine where that switchboard runs\n", adminName, command, printableLine(config.Host))
		return nil, config, 1
	}
	if address, parseErr := url.Parse(HubURL(env)); parseErr == nil && address.Hostname() != "" && !IsThisMachines(address.Hostname()) {
		fmt.Fprintf(env.Stderr, "%s %s: the switchboard address %s is not an address of this machine. Run it on the machine where that switchboard runs\n", adminName, command, printableLine(address.Host))
		return nil, config, 1
	}
	secret := env.processEnvironment("LANDLINE_SECRET")
	if secret == "" {
		secret = config.Secret
	}
	if secret == "" {
		fmt.Fprintf(env.Stderr, "%s %s: %s\n", adminName, command, NoCredentialMessage)
		return nil, config, 1
	}
	return client.New(HubURL(env), secret), config, 0
}

func refusalText(err error) string {
	var refused *client.StatusError
	if errors.As(err, &refused) && refused.Message != "" {
		return printableLine(refused.Message)
	}
	return printableLine(err.Error())
}

func serverInvite(args []string, env Env) int {
	if len(args) != 2 || args[0] != "--name" {
		fmt.Fprintln(env.Stderr, "usage: landline server invite --name <name>")
		return 2
	}
	hub, config, code := adminClient("invite", env)
	if code != 0 {
		return code
	}
	made, err := hub.Invite(args[1])
	if err != nil {
		var refused *client.StatusError
		if errors.As(err, &refused) && refused.Status == 409 {
			fmt.Fprintf(env.Stderr, "%s invite: a machine or a pending invite already holds the name %s. Run: landline server revoke %s\n", adminName, args[1], args[1])
			return 1
		}
		fmt.Fprintf(env.Stderr, "%s invite: %s\n", adminName, refusalText(err))
		return 1
	}
	host, port, _ := config.Listen(env.processEnvironment)
	address := net.JoinHostPort(host, strconv.Itoa(port))
	if contacted, parseErr := url.Parse(HubURL(env)); parseErr == nil && contacted.Host != "" {
		address = contacted.Host
	}
	fmt.Fprintf(env.Stdout, "landline server join %s --code %s\n", address, printableLine(made.Code))
	return 0
}

func serverMachines(args []string, env Env) int {
	if len(args) != 0 {
		fmt.Fprintln(env.Stderr, "usage: landline server machines")
		return 2
	}
	hub, _, code := adminClient("machines", env)
	if code != 0 {
		return code
	}
	listed, err := hub.Machines()
	if err != nil {
		fmt.Fprintf(env.Stderr, "%s machines: %s\n", adminName, refusalText(err))
		return 1
	}
	for _, machine := range listed {
		fmt.Fprintf(env.Stdout, "%s\t%s\t%s\n", printableLine(machine.Name), printableLine(machine.Fingerprint), printableLine(machine.Created))
	}
	return 0
}

func serverRevoke(args []string, env Env) int {
	if len(args) != 1 || strings.HasPrefix(args[0], "-") && args[0] != "--all" {
		fmt.Fprintln(env.Stderr, "usage: landline server revoke <name> | landline server revoke --all")
		return 2
	}
	hub, _, code := adminClient("revoke", env)
	if code != 0 {
		return code
	}
	var err error
	if args[0] == "--all" {
		err = hub.RevokeAll()
	} else {
		err = hub.Revoke(args[0])
	}
	if err != nil {
		fmt.Fprintf(env.Stderr, "%s revoke: %s\n", adminName, refusalText(err))
		return 1
	}
	if args[0] == "--all" {
		fmt.Fprintln(env.Stdout, "revoked every machine")
	} else {
		fmt.Fprintf(env.Stdout, "revoked %s\n", args[0])
	}
	return 0
}
