package command

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"

	client "github.com/8thlight-daveloper/landline/internal/client/http"
	"github.com/8thlight-daveloper/landline/internal/model"
)

type Env struct {
	Stdin     io.Reader
	Stdout    io.Writer
	Stderr    io.Writer
	Getenv    func(string) string
	ConfigDir string
	Host      string
	Banner    func() string
}

type session struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

const usage = `landline <command> [flags]

  server    up|run|down|status|install|join|log   run and find the hub
  setup     claude|codex   add the Landline MCP server to a harness, once
  claude    [--label L] [-- args]   start Claude Code with the channel loaded
  mcp       the MCP server a harness starts. It registers itself

  register  --name N --harness H [--host H] [--session S]   needs LANDLINE_URL and LANDLINE_SECRET.
            A hand-registered name holds no delivery stream, so it can send but cannot receive
            unless "landline mcp --as N --channel" runs for it
  agents    --as N   lists agents and whether each is connected
  send      --as N --to X [--thread T] [--kind K] [--reply-to ID] <body, or - for stdin>
            --to names one agent. The agent must be connected, or the send fails
  log       [--after SEQ]
  join      <hub address>   save a hub address and its secret, for a hub on another machine

--as defaults to LANDLINE_AS. The hub address and secret come from LANDLINE_URL and LANDLINE_SECRET, or from the file
that "landline join" and "landline server up" save, readable only by you.
`

func Run(args []string, env Env) int {
	env = withConfig(env)
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		if env.Banner != nil {
			if banner := env.Banner(); banner != "" {
				fmt.Fprint(env.Stderr, banner, "\n")
			}
		}
		fmt.Fprint(env.Stderr, usage)
		return 2
	}
	var err error
	switch args[0] {
	case "register":
		err = register(args[1:], env)
	case "agents":
		err = agents(args[1:], env)
	case "send":
		err = send(args[1:], env)
	case "log":
		err = logRead(args[1:], env)
	case "join":
		err = join(args[1:], env)
	default:
		fmt.Fprintf(env.Stderr, "landline: unknown command %q\n\n%s", args[0], usage)
		return 2
	}
	if err != nil {
		fmt.Fprintln(env.Stderr, "landline:", err)
		return 1
	}
	return 0
}

func flags(name string, env Env) *flag.FlagSet {
	set := flag.NewFlagSet(name, flag.ContinueOnError)
	set.SetOutput(env.Stderr)
	return set
}

func register(args []string, env Env) error {
	set := flags("register", env)
	name := set.String("name", env.Getenv("LANDLINE_AS"), "agent name")
	harness := set.String("harness", "", "harness, such as codex or claude-code")
	host := set.String("host", env.Host, "host name")
	sessionID := set.String("session", "", "harness session id")
	if err := set.Parse(args); err != nil {
		return err
	}
	url, secret := env.Getenv("LANDLINE_URL"), env.Getenv("LANDLINE_SECRET")
	if url == "" || secret == "" {
		return errors.New("set LANDLINE_URL and LANDLINE_SECRET")
	}
	out, err := client.New(url, secret).Register(model.RegisterRequest{
		Name: *name, Harness: *harness, Host: *host, Session: *sessionID,
	})
	if err != nil {
		return err
	}
	if out.Name != *name {
		return fmt.Errorf("the hub registered %q and not the name that was asked for. Nothing was saved", out.Name)
	}
	if err := saveSession(env, out.Name, session{URL: url, Token: out.Token}); err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "registered %s\n", out.Name)
	return nil
}

var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

func saveSession(env Env, name string, s session) error {
	if !namePattern.MatchString(name) {
		return errors.New("bad agent name")
	}
	if err := os.MkdirAll(env.ConfigDir, 0o700); err != nil {
		return err
	}
	raw, _ := json.Marshal(s)
	path := filepath.Join(env.ConfigDir, "session-"+name+".json")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		return err
	}
	return os.Chmod(path, 0o600)
}

type Direct struct{ *client.Client }

func (d Direct) Deliveries(ctx context.Context, handle func(model.Delivery)) error {
	return d.Stream(ctx, handle)
}

func Connect(as string, env Env) (Direct, error) {
	c, err := connect(as, env)
	return Direct{c}, err
}

func connect(as string, env Env) (*client.Client, error) {
	if as == "" {
		return nil, errors.New("say who you are with --as or LANDLINE_AS")
	}
	if strings.ContainsAny(as, "/\\") {
		return nil, errors.New("bad agent name")
	}
	raw, err := os.ReadFile(filepath.Join(env.ConfigDir, "session-"+as+".json"))
	if err != nil {
		return nil, fmt.Errorf("%s has no session here. Run landline register first", as)
	}
	var s session
	if err := json.Unmarshal(raw, &s); err != nil || s.URL == "" || s.Token == "" {
		return nil, fmt.Errorf("the session file for %s is damaged. Register again", as)
	}
	return client.New(s.URL, s.Token), nil
}

const forgottenAgent = "the hub does not know this agent any more (it forgets an agent 2 minutes after it was last seen). Run landline register again"

func forgotten(err error) error {
	var status *client.StatusError
	if errors.As(err, &status) && status.Status == 401 {
		return errors.New(forgottenAgent)
	}
	return err
}

func agents(args []string, env Env) error {
	set := flags("agents", env)
	as := set.String("as", env.Getenv("LANDLINE_AS"), "your agent name")
	if err := set.Parse(args); err != nil {
		return err
	}
	c, err := connect(*as, env)
	if err != nil {
		return err
	}
	list, err := c.Agents()
	if err != nil {
		return forgotten(err)
	}
	for _, a := range list {
		state := "disconnected"
		if a.Connected {
			state = "connected"
		}
		line := fmt.Sprintf("%s\t%s\t%s\t%s", printable(a.Name), printable(a.Harness), printable(a.Host), state)
		if attributes := a.Attributes(); attributes != "" {
			line += "\t" + printable(attributes)
		}
		fmt.Fprintln(env.Stdout, line)
	}
	return nil
}

func send(args []string, env Env) error {
	set := flags("send", env)
	as := set.String("as", env.Getenv("LANDLINE_AS"), "your agent name")
	to := set.String("to", "", "recipient name")
	thread := set.String("thread", "", "thread")
	kind := set.String("kind", "", "chat, task, reply, or review")
	reply := set.String("reply-to", "", "id of the message this answers")
	if err := set.Parse(args); err != nil {
		return err
	}
	body := strings.Join(set.Args(), " ")
	if body == "-" {
		raw, err := io.ReadAll(io.LimitReader(env.Stdin, model.MaxBodyBytes+1))
		if err != nil {
			return err
		}
		body = string(raw)
	}
	c, err := connect(*as, env)
	if err != nil {
		return err
	}
	out, err := c.Send(model.SendBody{To: *to, Thread: *thread, Kind: model.Kind(*kind), Body: body, InReplyTo: *reply})
	if err != nil {
		return forgotten(err)
	}
	fmt.Fprintf(env.Stdout, "sent %s (seq %d) to %s", printable(out.ID), out.Seq, printable(strings.Join(out.Recipients, ", ")))
	fmt.Fprintln(env.Stdout)
	return nil
}

func printable(text string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '\n' || r == '\t' || r == 0x200c || r == 0x200d || r == 0xad:
			return r
		case unicode.IsControl(r) || unicode.In(r, unicode.Zl, unicode.Zp, unicode.Cf):
			return ' '
		}
		return r
	}, text)
}

func logRead(args []string, env Env) error {
	set := flags("log", env)
	after := set.Int64("after", 0, "only messages after this seq")
	if err := set.Parse(args); err != nil {
		return err
	}
	url, secret := env.Getenv("LANDLINE_URL"), env.Getenv("LANDLINE_SECRET")
	if url == "" || secret == "" {
		return errors.New("set LANDLINE_URL and LANDLINE_SECRET")
	}
	entries, err := client.New(url, secret).Log(*after)
	if err != nil {
		return err
	}
	for _, e := range entries {
		line := fmt.Sprintf("%d\t%s\t%s -> %s\t%s\t%s", e.Seq, e.Time.Format("2006-01-02T15:04:05Z07:00"), printable(string(e.From)), printable(e.To), printable(e.Thread), printable(string(e.Kind)))
		if e.Result != "" {
			line += "\t" + printable(e.Result)
		}
		fmt.Fprintln(env.Stdout, line)
		for _, line := range strings.Split(printable(e.Body), "\n") {
			fmt.Fprintf(env.Stdout, "    %s\n", line)
		}
	}
	return nil
}
