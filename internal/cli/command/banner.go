package command

import (
	"fmt"
	"os"
	"strings"
	"time"
	"unicode"
)

var jackRows = []string{
	"...###...",
	".#######.",
	"#########",
	"##WW#WW##",
	"##WP#WP##",
	"#########",
	"#########",
	"##.#.#.##",
	"##.#.#.##",
}

const (
	reset = "\x1b[0m"
	blue  = "\x1b[38;2;0;113;206m"
	white = "\x1b[38;2;255;255;255m"
	dark  = "\x1b[38;2;20;20;20m"
)

func cell(pixel byte, color bool) (string, string) {
	switch pixel {
	case '#':
		return blue, "██"
	case 'W':
		if color {
			return white, "██"
		}
		return "", "  "
	case 'P':
		if color {
			return dark, "██"
		}
		return "", "▒▒"
	}
	return "", "  "
}

func Banner(color bool) string {
	var out strings.Builder
	for _, row := range jackRows {
		active := ""
		for i := 0; i < len(row); i++ {
			code, glyphs := cell(row[i], color)
			if !color || code == "" {
				code = ""
			}
			if code != active {
				if active != "" {
					out.WriteString(reset)
				}
				out.WriteString(code)
				active = code
			}
			out.WriteString(glyphs)
		}
		if active != "" {
			out.WriteString(reset)
		}
		out.WriteString("\n")
	}
	return out.String()
}

func VersionLine(name, version, commit, date string) string {
	switch {
	case commit == "":
		return name + " " + version
	case strings.HasSuffix(commit, "-dirty") || date == "":
		return fmt.Sprintf("%s %s (commit %s)", name, version, commit)
	}
	return fmt.Sprintf("%s %s (commit %s, built %s)", name, version, commit, date)
}

func BannerFor(out *os.File) string {
	info, err := out.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return ""
	}
	color := os.Getenv("NO_COLOR") == "" && os.Getenv("TERM") != "dumb"
	return Banner(color)
}

func NotifyToStderr(line string) {
	plain := strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || unicode.In(r, unicode.Zl, unicode.Zp, unicode.Cf) {
			return ' '
		}
		return r
	}, line)
	fmt.Fprintln(os.Stderr, time.Now().Format(time.RFC3339), plain)
}
