---
category: ux
status: accepted
approval: person
affects: [agent/viewer, menubar/viewer]
builds-on: ['0090', '0140']
partly-supersedes: ['0140']
note: Drafted on 2026-11-16. It replaces the state marker words of 0140 and changes how the TUI shows state.
---

# 0150. Agent state shows as an icon

Builds on [0090](0090-observers-read-the-agent-list-and-the-tui-is-a-separate-binary.md) and
[0140](0140-a-macos-menu-bar-item-shows-agents-runs-the-switchboard-and-sets-up-harnesses.md). It partly supersedes
0140, where an agent row ends in a marker of `active`, `idle`, or `disconnected`.

## Context

The TUI shows an agent's state as the word `connected` in green or `disconnected` in faint text, in a `STATE` column.
The menu bar item shows `[active]` or `[disconnected]` after the agent's name. The two screens use different words for
one fact, because `active` and `idle` are markers that 0140 invented while the switchboard reports only `connected` and
`disconnected`. A person looking at the list has to read each word to learn who is alive.

A later activity feature, described in [the hook events note](../future/read-only-tui-and-hook-events.md), will add
states such as working and waiting for the person. This record fixes the two states that exist and leaves the shapes of
the others to that feature.

## Decision

**One word for one state.** The words are `connected` and `disconnected`, as the switchboard reports them and as
`landline agents --json` and the text of `landline agents` print them. Those two outputs do not change. The menu bar
item stops using `active` and `idle`. The marker of an agent row is the state word, and it has two values.

**A state shows as a shape and a color together.** Color is never the only signal.

| State        | Shape         | Color                            |
| ------------ | ------------- | -------------------------------- |
| connected    | solid circle  | green                            |
| disconnected | hollow circle | gray, readable on light and dark |

- The shapes are `●` for connected and `○` for disconnected where text is drawn. The menu bar item draws an image of the
  same shapes.
- The state colors are the terminal's and the system's own. Brand blue stays in the About panel and the app icon.

**The TUI shows the shape in a first column and has no STATE column.** A row is the shape, one space, and then the
handle and the other columns, which keep the screen's normal gap between them. The shape is `●` in green or `○` in faint
text.

- The shape column is one cell wide and has no heading, so its header cell is a blank of that width. It is the first
  column, it is never dropped, and it keeps its place when the terminal is too narrow for the rest. When only the handle
  fits beside it, the handle is cut, as it is today, and the shape stays. The other columns keep their order and their
  drop order, with `STATE` removed from both.
- A legend line below the table reads `● connected  ○ disconnected`. It is a footer line and not a row of the table, so
  it does not scroll and `+N more` does not count it. It sits in the footer directly above the status line and below the
  blank spacer. When the terminal is too short for the whole footer, the legend goes first, then the blank spacer, then
  the key line, and the status line is kept longest, as it is today. It always names both states, whichever are in the
  rows. It shows whenever the table shows rows, and not for the empty, loading, or error bodies. It is cut with the `…`
  that every line gets when the terminal is narrower than it.
- With `NO_COLOR` set, the shapes carry the difference, because `●` and `○` differ without color.
- The screen does not say the word `connected` or `disconnected` for a row. A screen reader that walks the rows hears
  the Unicode name of each shape, `black circle` or `white circle`, and hears the legend only when it reaches that line.
  This is accepted. `landline agents` and `landline agents --json` give the words in text, and a person who needs the
  words on the screen waits for the setting that the alternatives below describe.
- `●` and `○` are ambiguous in width. In a terminal that draws ambiguous characters two cells wide, the first column
  draws one cell too wide and the row shifts by one. The screen counts each as one cell, as `ansi.StringWidth` does, and
  this record accepts the shift in those terminals and offers no ASCII form. A test fixes the count of one so that a
  change is deliberate.
- The TUI has no setting for which columns or text to show in this record. A setting needs a file the program reads,
  which is a stored format, so it waits for its own record.

**The menu bar item draws the same two shapes as an image on each agent row**, in the system green and a mid gray, as
non-template images because the colors matter. A menu's text does not change with the image, so the rule of 0140 that
the text output stays meaningful on its own still holds.

- A row's title in the text output starts with the shape and a space, `● alpha` or `○ bravo`, and has no bracketed
  state. The shape is the state's one signal in the title. On the screen the shape is the image alone, in the item's
  image slot, and the title is the name without a second text shape, so a row shows one shape and not two.
- An agent's submenu gains a first disabled item, `State: connected` or `State: disconnected`, so a screen reader that
  cannot read a menu image still reads the state.
- The two images are made from a procedure in `menubar/tools/icons`, as the jack's images are, at 16 points with a 2x
  version, and the stack document names them. The menu model's agent row carries the state word, and the screen chooses
  the image from it. The text output ignores the image. A unit test fixes which image each state gets, because the text
  output cannot show an image.
- The icon of the status item itself, the jack with the count, does not change.

## Consequences

- The pinned menu text of 0140 changes: the row marker word, which loses `active` and `idle`, and the new `State:` line
  in each submenu. `docs/reference/menubar-menu-text.md` and the acceptance tests of 0140 change with it, and a person
  approves them.
- `menubar/internal/agentsview` loses its `Marker` and `Active` values, and the code and tests that use them change,
  including `Row.Connected()`, which no longer compares against a marker.
- The TUI's column breakpoints move, because the shape column reserves one cell where `STATE` reserved the width of
  `disconnected`. The expected column sets at each width in `columns_test.go` are recomputed, and so are the height
  tests, since the legend adds a footer line. A fixture for widths 1 to 4 pins that the shape stays and the handle is
  cut to nothing.
- The TUI's tests change. Its column and header expectations lose `STATE` and gain the blank first cell. A test that
  asserts the words `connected` and `disconnected` over the whole view passes for the wrong reason once the legend holds
  both, so the tests assert on row lines, and a separate test asserts the legend. An acceptance scenario for the first
  column and its `NO_COLOR` form is added if none covers it.
- 0090 lists `state` among the columns of its first screen. The first column now carries it, and 0090 stays as written.
- The record of [0160](0160-an-agent-has-an-id-a-project-and-a-task.md) adds a `TASK` column after the project column.
  Whichever record is built second places it in the drop order after the project column and before the others.
- `menubar/` gains two small colored images, and the dependency list does not change. The tool in `menubar/tools/icons`
  draws only straight-edged shapes today, so it gains code to draw a circle and a ring on the pixel grid, byte for byte
  the same on every run. The app loads the 32 pixel file, which the library shows at 16 points, because the library
  takes one image per item. The menu's `Item` gains a field that carries the state word to the screen.
- The activity feature adds its states and their shapes in a record of its own. It does not change the two that exist.

## Alternatives considered

- **Colored emoji such as a green circle.** They need no images. Their size and look vary by font and system, a screen
  reader reads each one as a long description, and they do not honor `NO_COLOR`. The plain shapes avoid all three.
- **Keep a STATE column with the word and the shape.** It is the safest change, and it keeps the word for screen
  readers. It spends a column to say what a shape and a legend say, and the word is still in `--json` and in
  `landline agents`.
- **A setting to show or hide columns and the status text.** It suits a person who wants the word back. It needs a
  config file that the TUI reads, which is a new stored format, and a column set that will change when the activity
  feature adds columns. It waits until that feature shows what a person wants to hide.
- **Color alone.** It is the lightest change. A person with color vision loss cannot tell the rows apart, and a
  monochrome terminal loses the signal.
- **Keep `active` in the menu.** It anticipates the activity states. It names a state that does not exist, and the two
  screens would keep saying different things.
- **An `Open Landline in Terminal` menu item.** It would start the TUI from the menu through a temporary script. It may
  open in a terminal the person did not choose, it adds a new kind of process start, and a native window may replace it,
  so it waits for the events work.
- **A native window now.** It is the larger step. It waits until the events work shows how a person interacts with an
  agent's activity, because the layout depends on that.
