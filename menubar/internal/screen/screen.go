package screen

import (
	"reflect"
	"sync"
	"time"

	"fyne.io/systray"

	"github.com/8thlight-daveloper/landline/menubar/internal/about"
	"github.com/8thlight-daveloper/landline/menubar/internal/buildinfo"
	"github.com/8thlight-daveloper/landline/menubar/internal/clipboard/pasteboard"
	"github.com/8thlight-daveloper/landline/menubar/internal/confirm"
	"github.com/8thlight-daveloper/landline/menubar/internal/icon"
	"github.com/8thlight-daveloper/landline/menubar/internal/landline/runner"
	"github.com/8thlight-daveloper/landline/menubar/internal/menu/model"
	"github.com/8thlight-daveloper/landline/menubar/internal/menu/session"
	"github.com/8thlight-daveloper/landline/menubar/internal/pacing"
)

type Config struct {
	Source   session.Source
	Session  *session.Session
	Interval time.Duration
	Find     func() (string, bool)
	Path     string
	App      buildinfo.Build
}

type Screen struct {
	mu       sync.Mutex
	config   Config
	wake     chan struct{}
	lastIcon *model.Icon
	lastMenu []model.Item
	done     chan struct{}
}

func Run(config Config) {
	screen := &Screen{config: config, wake: make(chan struct{}, 1), done: make(chan struct{})}
	systray.Run(screen.ready, func() {})
}

func (s *Screen) ready() {
	s.poll()
	go s.loop()
}

func (s *Screen) loop() {
	for {
		started := time.Now()
		s.poll()
		select {
		case <-time.After(pacing.NextWait(s.config.Interval, time.Since(started))):
		case <-s.wake:
		}
	}
}

func (s *Screen) poll() {
	_, found := s.config.Find()
	s.config.Session.Apply(session.Observe(s.config.Source, found))
	s.render()
}

func (s *Screen) pollSoon() {
	select {
	case s.wake <- struct{}{}:
	default:
	}
}

func (s *Screen) render() {
	s.mu.Lock()
	defer s.mu.Unlock()
	menu := s.config.Session.Menu()
	if s.lastIcon == nil || *s.lastIcon != menu.Icon {
		s.showIcon(menu.Icon)
		s.lastIcon = &menu.Icon
	}
	if reflect.DeepEqual(s.lastMenu, menu.Items) {
		return
	}
	close(s.done)
	s.done = make(chan struct{})
	systray.ResetMenu()
	s.addItems(menu.Items, nil)
	s.lastMenu = menu.Items
}

func (s *Screen) showIcon(state model.Icon) {
	if state.Dimmed {
		systray.SetTemplateIcon(icon.Dimmed, icon.Dimmed)
	} else {
		systray.SetTemplateIcon(icon.Template, icon.Template)
	}
	systray.SetTitle(state.CountText())
}

func (s *Screen) addItems(items []model.Item, parent *systray.MenuItem) {
	for _, item := range items {
		if item.Separator {
			if parent == nil {
				systray.AddSeparator()
			} else {
				parent.AddSeparator()
			}
			continue
		}
		var entry *systray.MenuItem
		if parent == nil {
			entry = systray.AddMenuItem(item.Title, "")
		} else {
			entry = parent.AddSubMenuItem(item.Title, "")
		}
		if image := icon.ForState(item.State); image != nil {
			entry.SetIcon(image)
		}
		if item.Disabled {
			entry.Disable()
		}
		s.addItems(item.Children, entry)
		if item.Action.Kind != model.NoAction && !item.Disabled {
			go s.watch(entry.ClickedCh, item.Action, s.done)
		}
	}
}

func (s *Screen) watch(clicks <-chan struct{}, action model.Action, done <-chan struct{}) {
	for {
		select {
		case <-done:
			return
		case _, ok := <-clicks:
			if !ok {
				return
			}
			s.perform(action)
		}
	}
}

func (s *Screen) perform(action model.Action) {
	switch action.Kind {
	case model.Copy:
		pasteboard.Copy(pasteboard.Binary, action.Text)
	case model.Run:
		go s.confirmAndRun(action)
	case model.About:
		about.Show(about.ContentFor(s.config.App))
	case model.Refresh:
		s.pollSoon()
	case model.Quit:
		systray.Quit()
	}
}

func (s *Screen) confirmAndRun(action model.Action) {
	if action.Confirm != "" && !confirm.Ask(action.Confirm, action.ConfirmAccept) {
		return
	}
	s.run(action)
}

func (s *Screen) run(action model.Action) {
	if !s.config.Session.Start() {
		return
	}
	s.render()
	binary, _ := s.config.Find()
	failure, err := runner.New(binary, s.config.Path).Run(action.Args)
	switch {
	case err != nil:
	case failure != "":
		s.config.Session.Notify(failure)
	case action.Done != "":
		s.config.Session.Notify(action.Done)
	}
	s.config.Session.Finish()
	s.pollSoon()
}
