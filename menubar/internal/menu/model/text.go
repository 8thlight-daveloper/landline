package model

import "strings"

const (
	separatorText = "---"
	disabledText  = " (disabled)"
	indent        = "  "
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
			lines = append(lines, prefix+item.Title+disabledText)
		default:
			lines = append(lines, prefix+item.Title)
		}
		lines = appendItems(lines, item.Children, prefix+indent)
	}
	return lines
}
