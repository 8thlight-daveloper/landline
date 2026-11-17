package screen

import (
	"strings"

	"github.com/8thlight-daveloper/landline/tui/internal/agentsview"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

const (
	columnGap = 2
	shapeGap  = 1
)

type column struct {
	title    string
	cell     func(agentsview.Row) string
	capWidth int
	minWidth int
	fromLeft bool
}

const (
	solidCircle  = "●"
	hollowCircle = "○"
)

func shapeOf(r agentsview.Row) string {
	if r.State == agentsview.Connected {
		return solidCircle
	}
	return hollowCircle
}

func legend() string {
	return connectedStyle.Render(solidCircle) + " connected  " + disconnectedStyle.Render(hollowCircle) + " disconnected"
}

var columns = []column{
	{"", shapeOf, 1, 1, false},
	{"ID", func(r agentsview.Row) string { return r.ID }, 0, 1, true},
	{"PROJECT", func(r agentsview.Row) string { return r.Project }, 16, 6, false},
	{"TASK", func(r agentsview.Row) string { return r.Task }, 24, 8, false},
	{"HARNESS", func(r agentsview.Row) string { return r.Harness }, 12, 6, false},
	{"LANDLINE", func(r agentsview.Row) string { return r.LandlineVersion }, 9, 5, false},
	{"FOLDER", func(r agentsview.Row) string { return r.Folder }, 28, 10, true},
	{"BRANCH", func(r agentsview.Row) string { return r.Branch }, 22, 8, false},
	{"MODEL", func(r agentsview.Row) string { return r.Model }, 18, 6, false},
	{"EFFORT", func(r agentsview.Row) string { return r.Effort }, 7, 5, false},
	{"COMMIT", func(r agentsview.Row) string { return r.ShortCommit() }, 13, 13, false},
}

var (
	headerStyle       = lipgloss.NewStyle().Bold(true)
	connectedStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	disconnectedStyle = lipgloss.NewStyle().Faint(true)
	errorStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("1"))
)

const (
	shapeIndex  = 0
	idIndex     = 1
	staleMarker = "(stale)"
)

var priority = []int{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10}

func naturalWidth(c column, rows []agentsview.Row) int {
	w := ansi.StringWidth(c.title)
	for _, row := range rows {
		if cw := ansi.StringWidth(c.cell(row)); cw > w {
			w = cw
		}
	}
	return w
}

func visibleColumns(rows []agentsview.Row, width int) ([]column, []int) {
	if width <= 0 {
		return nil, nil
	}
	idWidth := naturalWidth(columns[idIndex], rows)
	shapeWidth := columns[shapeIndex].capWidth
	if shapeWidth+shapeGap+idWidth > width {
		return []column{columns[shapeIndex], columns[idIndex]}, []int{min(shapeWidth, width), max(width-shapeWidth-shapeGap, 0)}
	}
	allowed := map[int]int{shapeIndex: shapeWidth, idIndex: idWidth}
	used := shapeWidth + shapeGap + idWidth
	for _, index := range priority[2:] {
		c := columns[index]
		remaining := width - used - columnGap
		if remaining >= c.capWidth {
			allowed[index] = c.capWidth
			used += columnGap + c.capWidth
			continue
		}
		if remaining >= c.minWidth {
			allowed[index] = remaining
		}
		break
	}
	shown := []column{}
	sizes := []int{}
	for index, c := range columns {
		limit, ok := allowed[index]
		if !ok {
			continue
		}
		size := limit
		if index != idIndex && index != shapeIndex {
			size = max(min(naturalWidth(c, rows), limit), c.minWidth)
		}
		shown = append(shown, c)
		sizes = append(sizes, size)
	}
	return shown, sizes
}

func fit(text string, width int, fromLeft bool) string {
	if width <= 0 {
		return ""
	}
	switch {
	case ansi.StringWidth(text) <= width:
	case fromLeft:
		cut := ansi.StringWidth(text) - (width - 1)
		tail := ansi.TruncateLeft(text, cut, "")
		for ansi.StringWidth(tail) > width-1 {
			cut++
			tail = ansi.TruncateLeft(text, cut, "")
		}
		text = "…" + tail
	default:
		text = ansi.Truncate(text, width, "…")
	}
	return text + strings.Repeat(" ", width-ansi.StringWidth(text))
}

func line(cells []string, columns []column, widths []int, styles []lipgloss.Style, width int) string {
	var out strings.Builder
	for i, cell := range cells {
		if i > 0 {
			out.WriteString(strings.Repeat(" ", gapAfter(i-1)))
		}
		out.WriteString(styles[i].Render(fit(cell, widths[i], columns[i].fromLeft)))
	}
	return ansi.Truncate(out.String(), width, "")
}

func gapAfter(position int) int {
	if position == shapeIndex {
		return shapeGap
	}
	return columnGap
}

func table(rows []agentsview.Row, width int, stale bool) []string {
	columns, widths := visibleColumns(rows, width)
	plain := lipgloss.NewStyle()
	headers := make([]string, len(columns))
	headerStyles := make([]lipgloss.Style, len(columns))
	for i, c := range columns {
		headers[i] = c.title
		headerStyles[i] = headerStyle
	}
	lines := []string{markStale(line(headers, columns, widths, headerStyles, width), width, stale)}
	for _, row := range rows {
		cells := make([]string, len(columns))
		styles := make([]lipgloss.Style, len(columns))
		for i, c := range columns {
			cells[i] = c.cell(row)
			styles[i] = plain
		}
		if len(styles) > shapeIndex {
			styles[shapeIndex] = disconnectedStyle
			if row.State == agentsview.Connected {
				styles[shapeIndex] = connectedStyle
			}
		}
		lines = append(lines, line(cells, columns, widths, styles, width))
	}
	return lines
}

func markStale(header string, width int, stale bool) string {
	if !stale {
		return header
	}
	marker := " " + staleMarker
	room := width - ansi.StringWidth(marker)
	if room < 1 {
		return ansi.Truncate(staleMarker, max(width, 0), "")
	}
	trimmed := strings.TrimRight(ansi.Strip(header), " ")
	if ansi.StringWidth(trimmed)+ansi.StringWidth(marker) <= width {
		return header + marker
	}
	return headerStyle.Render(ansi.Truncate(trimmed, room, "")) + marker
}
