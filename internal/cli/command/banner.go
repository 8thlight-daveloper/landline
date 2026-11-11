package command

import "strings"

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
