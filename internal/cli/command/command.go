package command

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

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
}

type session struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

const usage = `landline <command> [flags]

  register  --name N --harness H [--host H] [--session S] [--wake codex --wake-target T]
            needs LANDLINE_URL and LANDLINE_SECRET
  agents    --as N
  send      --as N --to X [--thread T] [--kind K] [--reply-to ID] <body, or - for stdin>
  inbox     --as N [--wait SECONDS] [--thread T[,T]] [--after SEQ] [--json]
  log       [--after SEQ]   needs LANDLINE_URL and LANDLINE_SECRET

--as defaults to LANDLINE_AS. The hub secret comes from the environment only.
`

func Run(args []string, env Env) int {
	if len(args) == 0 || args[0] == "help" || args[0] == "-h" || args[0] == "--help" {
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
	case "inbox":
		err = inbox(args[1:], env)
	case "log":
		err = logRead(args[1:], env)
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
	wake := set.String("wake", "", "wake method")
	target := set.String("wake-target", "", "wake target, such as a codex thread id")
	if err := set.Parse(args); err != nil {
		return err
	}
	url, secret := env.Getenv("LANDLINE_URL"), env.Getenv("LANDLINE_SECRET")
	if url == "" || secret == "" {
		return errors.New("set LANDLINE_URL and LANDLINE_SECRET")
	}
	out, err := client.New(url, secret).Register(model.RegisterRequest{
		Name: *name, Harness: *harness, Host: *host, Session: *sessionID, WakeKind: *wake, WakeTarget: *target,
	})
	if err != nil {
		return err
	}
	if err := saveSession(env, out.Name, session{URL: url, Token: out.Token}); err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "registered %s\n", out.Name)
	return nil
}

func saveSession(env Env, name string, s session) error {
	if err := os.MkdirAll(env.ConfigDir, 0o700); err != nil {
		return err
	}
	raw, _ := json.Marshal(s)
	return os.WriteFile(filepath.Join(env.ConfigDir, "session-"+name+".json"), raw, 0o600)
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
		return err
	}
	for _, a := range list {
		wake := a.WakeKind
		if wake == "" {
			wake = "none"
		}
		fmt.Fprintf(env.Stdout, "%s\t%s\t%s\twake: %s\n", a.Name, a.Harness, a.Host, wake)
	}
	return nil
}

func send(args []string, env Env) error {
	set := flags("send", env)
	as := set.String("as", env.Getenv("LANDLINE_AS"), "your agent name")
	to := set.String("to", "", "recipient name, or all")
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
		return err
	}
	fmt.Fprintf(env.Stdout, "sent %s (seq %d) to %s", out.ID, out.Seq, strings.Join(out.Recipients, ", "))
	if len(out.Woken) > 0 {
		fmt.Fprintf(env.Stdout, "; woke %s", strings.Join(out.Woken, ", "))
	}
	fmt.Fprintln(env.Stdout)
	return nil
}

func inbox(args []string, env Env) error {
	set := flags("inbox", env)
	as := set.String("as", env.Getenv("LANDLINE_AS"), "your agent name")
	wait := set.Int("wait", 0, "seconds to wait for mail")
	threads := set.String("thread", "", "comma-separated threads")
	after := set.Int64("after", 0, "with --thread, only messages after this seq")
	asJSON := set.Bool("json", false, "print JSON")
	if err := set.Parse(args); err != nil {
		return err
	}
	c, err := connect(*as, env)
	if err != nil {
		return err
	}
	out, err := c.Inbox(*wait, *threads, *after)
	if err != nil {
		return err
	}
	if *asJSON {
		return json.NewEncoder(env.Stdout).Encode(out)
	}
	for _, m := range out.Messages {
		fmt.Fprintf(env.Stdout, "%s\n\n", m.Text)
	}
	if out.Remaining > 0 {
		fmt.Fprintf(env.Stderr, "%d more unread. Run inbox again.\n", out.Remaining)
	}
	return nil
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
		fmt.Fprintf(env.Stdout, "%d\t%s\t%s -> %s\t%s\t%s\n", e.Seq, e.Time.Format("2006-01-02T15:04:05Z07:00"), e.From, e.To, e.Thread, e.Kind)
	}
	return nil
}
