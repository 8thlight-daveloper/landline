package screen

import (
	"fmt"
	"strings"
	"time"

	"github.com/8thlight-daveloper/landline/tui/internal/agentsview"
	"github.com/8thlight-daveloper/landline/tui/internal/clean"
	"github.com/8thlight-daveloper/landline/tui/internal/hubclient"
	"github.com/8thlight-daveloper/landline/tui/internal/pacing"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

const (
	defaultWidth            = 80
	refreshingMarker        = "refreshing..."
	noticeAlreadyRefreshing = "already refreshing"
)

type Fetch func() ([]hubclient.Agent, error)

type snapshotMsg struct {
	agents []hubclient.Agent
	err    error
	at     time.Time
	took   time.Duration
}

type tickMsg struct{ generation int }

type Model struct {
	hub        string
	interval   time.Duration
	fetch      Fetch
	now        func() time.Time
	view       *agentsview.View
	err        error
	refreshed  time.Time
	fetching   bool
	notice     string
	generation int
	nextWait   time.Duration
	width      int
	height     int
}

func New(hub string, interval time.Duration, fetch Fetch, now func() time.Time) Model {
	return Model{hub: hub, interval: interval, fetch: fetch, now: now, fetching: true, nextWait: interval, width: defaultWidth}
}

func (m Model) Init() tea.Cmd { return m.fetchCmd() }

func (m Model) fetchCmd() tea.Cmd {
	return func() tea.Msg {
		var agents []hubclient.Agent
		var err error
		took := pacing.Measure(m.now, func() { agents, err = m.fetch() })
		return snapshotMsg{agents: agents, err: err, at: m.now(), took: took}
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "r":
			if m.fetching {
				m.notice = noticeAlreadyRefreshing
				return m, nil
			}
			m.fetching = true
			m.generation++
			return m, m.fetchCmd()
		}
	case snapshotMsg:
		m.fetching = false
		m.notice = ""
		m.generation++
		if msg.err != nil {
			m.err = msg.err
		} else {
			m.err = nil
			view := agentsview.Build(m.hub, msg.at, msg.agents)
			m.view = &view
			m.refreshed = msg.at
		}
		m.nextWait = pacing.NextWait(m.interval, msg.took)
		generation := m.generation
		return m, tea.Tick(m.nextWait, func(time.Time) tea.Msg { return tickMsg{generation} })
	case tickMsg:
		if msg.generation == m.generation && !m.fetching {
			m.fetching = true
			return m, m.fetchCmd()
		}
	}
	return m, nil
}

func (m Model) View() string {
	width := m.width
	body := m.body(width)
	footer := []string{"", m.status(width), "q quit  r refresh"}
	if m.height > 0 {
		body, footer = fitHeight(body, footer, m.height)
	}
	lines := append(body, footer...)
	for i, l := range lines {
		lines[i] = ansi.Truncate(l, max(width, 0), "…")
	}
	return strings.Join(lines, "\n")
}

func (m Model) body(width int) []string {
	switch {
	case m.view == nil && m.err == nil:
		return []string{"Loading the agent list..."}
	case m.view == nil:
		return []string{"No agent list yet."}
	case len(m.view.Agents) == 0:
		return []string{"No agents are registered with this hub."}
	}
	return table(m.view.Agents, width, m.err != nil)
}

func fitHeight(body, footer []string, height int) ([]string, []string) {
	status := footer[1:2]
	var room int
	chosen := status
	for _, option := range [][]string{footer, footer[1:], status} {
		room = height - len(option)
		chosen = option
		if len(body) <= room || room >= 3 {
			break
		}
	}
	footer = chosen
	if len(body) <= room {
		return body, footer
	}
	switch {
	case room <= 0:
		return nil, footer
	case room == 1:
		return body[:1], footer
	}
	shown := room - 2
	hidden := len(body) - 1 - shown
	return append(append([]string{}, body[:1+shown]...), fmt.Sprintf("+%d more", hidden)), footer
}

func (m Model) status(width int) string {
	refreshed := "never"
	if !m.refreshed.IsZero() {
		refreshed = m.refreshed.Format("15:04:05")
	}
	hub := "hub " + clean.Text(m.hub)
	lead := []string{}
	plainLead := []string{}
	if m.err != nil {
		text := "error: " + clean.Text(m.err.Error())
		lead = append(lead, errorStyle.Render(text))
		plainLead = append(plainLead, text)
	}
	switch {
	case m.notice != "":
		lead = append(lead, m.notice)
		plainLead = append(plainLead, m.notice)
	case m.fetching:
		lead = append(lead, refreshingMarker)
		plainLead = append(plainLead, refreshingMarker)
	}
	tiers := [][]string{
		{hub, "refreshed " + refreshed},
		{hub, refreshed},
		{refreshed},
		{},
	}
	for i, tier := range tiers {
		plain := strings.Join(append(append([]string{}, plainLead...), tier...), " | ")
		if ansi.StringWidth(plain) <= width || i == len(tiers)-1 {
			return strings.Join(append(append([]string{}, lead...), tier...), " | ")
		}
	}
	return ""
}
