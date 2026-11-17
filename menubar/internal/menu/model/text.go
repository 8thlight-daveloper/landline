package model

import (
	"strings"

	"github.com/8thlight-daveloper/landline/menubar/internal/agentsview"
)

const (
	connectedShape    = "\u25cf"
	disconnectedShape = "\u25cb"
	separatorText     = "---"
	disabledText      = " (disabled)"
	indent            = "  "
)

func (m Menu) Text() string {
	lines := []string{"icon: " + m.Icon.text()}
	lines = appendItems(lines, m.Items, "")
	return strings.Join(lines, "\n") + "\n"
}

func (i Icon) text() string {
	switch {
	case i.Dimmed:
		return "jack dimmed"
	case i.Count > 0:
		return "jack " + i.CountText()
	}
	return "jack"
}

func appendItems(lines []string, items []Item, prefix string) []string {
	for _, item := range items {
		switch {
		case item.Separator:
			lines = append(lines, prefix+separatorText)
		case item.Disabled:
			lines = append(lines, prefix+itemTitle(item)+disabledText)
		default:
			lines = append(lines, prefix+itemTitle(item))
		}
		lines = appendItems(lines, item.Children, prefix+indent)
	}
	return lines
}

func itemTitle(item Item) string {
	switch item.State {
	case agentsview.Connected:
		return connectedShape + " " + item.Title
	case agentsview.Disconnected:
		return disconnectedShape + " " + item.Title
	}
	return item.Title
}
