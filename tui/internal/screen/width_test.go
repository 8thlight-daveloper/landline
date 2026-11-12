package screen

import "github.com/charmbracelet/x/ansi"

func lipglossWidth(s string) int { return ansi.StringWidth(s) }
