---
category: ux
status: accepted
approval: person
affects: [agent/viewer]
builds-on: ['0090', '0150']
note: Drafted on 2026-11-17 and approved by its owner the same day. It dims the whole row of a disconnected agent in the TUI. It changes no stored format.
---

# 0180. A disconnected agent row is dimmed in the TUI

Builds on [0090](0090-observers-read-the-agent-list-and-the-tui-is-a-separate-binary.md) and
[0150](0150-agent-state-shows-as-an-icon.md).

## Context

0150 gives each row a state shape in the first column: a green `●` for connected and a faint `○` for disconnected. The
rest of the row prints at normal intensity in both cases. A list with several disconnected agents therefore shows the
same text weight for an agent that can receive a message and one that cannot, and the eye has to find the shapes to tell
them apart. The shape cell is the only part of a disconnected row that is faint today (`disconnectedStyle` in
`tui/internal/screen/render.go`).

## Decision

**Every cell of a disconnected row is faint, and a connected row is unchanged.**

- The faint attribute is the one the shape and the legend already use. This record adds no color and no new style.
- The dimmed cells are the shape, the id, and every other column of the row, whichever columns the width allows.
- The header, the legend, the status line, and the key line keep their present styles. The legend's `○ disconnected`
  stays faint, as in 0150.
- A connected row keeps a green shape and normal text.
- Dimming changes no width. A cell is cut with `…` where it is cut today, and the `…` is faint with its cell.

**Dimming is never the only signal.** The shape still differs, `●` against `○`, and the legend still names both. A
terminal that ignores the faint attribute shows the rows at normal intensity and loses nothing but the emphasis. With
`NO_COLOR` set, the shapes carry the difference, as 0150 says, and this record does not rely on the faint attribute
there.

**The menu bar item is unchanged.** A menu item draws the shape as an image and has no per-row text weight to set.

## Consequences

- `tui/internal/screen` applies one style to every cell of a disconnected row. The tests that fix the styles of a row
  change, and a test fixes that a connected row's text cells carry no attribute and that a disconnected row's cells all
  carry the faint one.
- The acceptance scenarios for the TUI that check a row's escape sequences change. A person authorizes those edits, and
  an agent does not make them.
- Faint text has less contrast than normal text, and on some themes it is hard to read. A disconnected agent cannot be
  messaged, so low emphasis is the intent, and the shape and the legend remain for a person who cannot read the dimmed
  text. `landline agents` and `landline agents --json` still print every field in plain text.

## Alternatives considered

- **A gray color, such as bright black.** Its contrast differs by theme, and on some it is nearly invisible or matches
  the background. The faint attribute follows the person's own foreground color. It also leaves `NO_COLOR` alone.
- **Dim the id and project only, and leave the other columns.** It leaves a row half bright and half dim, and the eye
  reads the bright cells first.
- **Strike through the id of a disconnected agent.** It reads as removed, and the row is still a listed agent.
- **Hide disconnected rows, or sort them out of view.** 0150 shows them with their own shape, and a person looking for
  an agent that dropped needs to see it.
- **A setting for how a disconnected row looks.** It needs a file that the TUI reads, which is a stored format, and it
  waits for its own record, as 0150 says.
