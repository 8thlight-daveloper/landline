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
	Home      string
	Host      string
	Banner    func() string

	processGetenv func(string) string
}

func (e Env) processEnvironment(key string) string {
	if e.processGetenv != nil {
		return e.processGetenv(key)
	}
	if e.Getenv == nil {
		return ""
	}
	return e.Getenv(key)
}

type session struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

const Usage = `usage: landline <command> [flags]

  Run with no command in a terminal, landline opens the agent screen. --interval D sets how often it asks for the list.

  agents    [--json] [--interval D]   list agents and whether each is connected. --json prints the screen's view
  setup     claude|codex [--remove]   add the Landline MCP server, landline-mcp, to a harness, once. --remove takes it out
  claude    [--project P] [-- args]   start Claude Code with the channel loaded
  server    up|down|restart|rotate-secret|status|install|join|log   run and find the switchboard

  register  --id N --harness H [--host H] [--session S] [--project P]   needs LANDLINE_URL and LANDLINE_SECRET.
            A hand-registered id holds no delivery stream, so it can send but cannot receive
            unless "landline-mcp --as N --channel" runs for it
  send      --as N --to X [--thread T] [--kind K] [--reply-to ID] <body, or - for stdin>
            --to takes the id of one agent, as "landline agents" lists it. The agent must be connected, or the send fails
  log       [--after SEQ]
  join      <switchboard address>   save a switchboard address and its secret, for a switchboard on another machine
  version   print the version and the build

--as defaults to LANDLINE_AS. The switchboard address and secret come from LANDLINE_URL and LANDLINE_SECRET, or from the file
that "landline join" and "landline server up" save, readable only by you.
`

func Run(args []string, env Env) int {
	env = withConfig(env)
	if len(args) == 0 {
		showUsage(env, env.Stderr)
		return 2
	}
	if args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
		showUsage(env, env.Stdout)
		return 0
	}
	var err error
	switch args[0] {
	case "register":
		err = register(args[1:], env)
	case "send":
		err = send(args[1:], env)
	case "log":
		err = logRead(args[1:], env)
	case "join":
		err = join(args[1:], env)
	case "setup":
		return runSetup(args[1:], env)
	case "claude":
		return runClaude(args[1:], env)
	case "server":
		return runServer(args[1:], env)
	default:
		fmt.Fprintf(env.Stderr, "landline: unknown command %q\n\n%s", args[0], Usage)
		return 2
	}
	if err != nil {
		fmt.Fprintln(env.Stderr, "landline:", err)
		return 1
	}
	return 0
}

func showUsage(env Env, out io.Writer) {
	if env.Banner != nil {
		if banner := env.Banner(); banner != "" {
			fmt.Fprint(env.Stderr, banner, "\n")
		}
	}
	fmt.Fprint(out, Usage)
}

func flags(name string, env Env) *flag.FlagSet {
	set := flag.NewFlagSet(name, flag.ContinueOnError)
	set.SetOutput(env.Stderr)
	return set
}

func register(args []string, env Env) error {
	set := flags("register", env)
	id := set.String("id", env.Getenv("LANDLINE_AS"), "agent id")
	harness := set.String("harness", "", "harness, such as codex or claude-code")
	host := set.String("host", env.Host, "host name")
	sessionID := set.String("session", "", "harness session id")
	project := set.String("project", "", "project the agent works on")
	if err := set.Parse(args); err != nil {
		return err
	}
	url, secret := env.Getenv("LANDLINE_URL"), env.Getenv("LANDLINE_SECRET")
	if url == "" || secret == "" {
		return errors.New("set LANDLINE_URL and LANDLINE_SECRET")
	}
	out, err := client.New(url, secret).Register(model.RegisterRequest{
		ID: *id, Harness: *harness, Host: *host, Session: *sessionID, Project: *project,
	})
	if err != nil {
		return err
	}
	if out.ID != *id {
		return fmt.Errorf("the switchboard registered %q and not the id that was asked for. Nothing was saved", out.ID)
	}
	if err := saveSession(env, out.ID, session{URL: url, Token: out.Token}); err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "registered %s\n", out.ID)
	return nil
}

var idPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

func saveSession(env Env, id string, s session) error {
	if !idPattern.MatchString(id) {
		return errors.New("bad agent id")
	}
	if err := os.MkdirAll(env.ConfigDir, 0o700); err != nil {
		return err
	}
	raw, _ := json.Marshal(s)
	path := filepath.Join(env.ConfigDir, "session-"+id+".json")
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
		return nil, errors.New("bad agent id")
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

const forgottenAgent = "the switchboard does not know this agent any more (it forgets an agent 2 minutes after it was last seen). Run landline register again"

func forgotten(err error) error {
	var status *client.StatusError
	if errors.As(err, &status) && status.Status == 401 {
		return errors.New(forgottenAgent)
	}
	return err
}

func send(args []string, env Env) error {
	set := flags("send", env)
	as := set.String("as", env.Getenv("LANDLINE_AS"), "your agent id")
	to := set.String("to", "", "recipient id")
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

func Printable(text string) string { return printable(text) }

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
