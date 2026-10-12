package tui

import (
	"fmt"
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/MatthewAPeters/backgammon/internal/game"
)

// Board geometry, in terminal cells. Cells are about twice as tall as they
// are wide, so a checker 4 cells wide and 2 rows tall reads as round.
const (
	pointW    = 6  // width of one point
	halfH     = 11 // rows in each half: stackRows checkers × 2 rows, and a row clear of the top edge
	triLen    = 9  // rows a point's triangle spans, base to tip
	stackRows = 5  // checkers drawn per point before showing a count
	sideW     = 4  // bar and borne-off tray, as wide as a checker
	edgeW     = 2  // walnut at the board's left edge, and after each pair of boxes
	fieldW    = 1 + 6*pointW + 1 // half of the playing field with its inlay lining
	// The board inside its frame is a row of boxes on walnut: the left
	// field and the bar, then the right field and the tray. Each box casts
	// a shadow one cell wide past its right edge (and a row above it).
	boardInW = edgeW + (fieldW + 1) + (sideW + 1) + edgeW + (fieldW + 1) + (sideW + 1) + edgeW
	boardW   = boardInW + 6 // with the inlay frame round it, as on the menus
)

var (
	walnut     = lipgloss.Color("#6a4527") // frame and label rows
	labelFg    = lipgloss.Color("#f3e9d2")
	felt       = lipgloss.Color("#c9ab7f") // playing surface
	tray       = walnut                    // bar and off tray
	shadow     = lipgloss.Color("#3f2814") // dark walnut shadows cast by the boxes
	pointLight = lipgloss.Color("#f8f3e7")
	pointDark  = lipgloss.Color("#8b5530")

	humanCol = lipgloss.Color("#d32f2f")
	aiCol    = lipgloss.Color("#151515")
	countFg  = lipgloss.Color("#ffffff")
	diceInk  = lipgloss.Color("#151515")
	// Dice match the roller's checkers; used dice are a faded shade.
	humanDiceUsed = lipgloss.Color("#8f5a4a")
	aiDiceUsed    = lipgloss.Color("#6e6253")
	diceUsedInk   = lipgloss.Color("#d8cbb5")
	cursorCol     = lipgloss.Color("#f4b400")
	selectCol     = lipgloss.Color("#2e7d32")
	destCol       = lipgloss.Color("#7cc47f")
	aiMoveCol     = lipgloss.Color("#7d8fc4")
)

// mark is how a position is highlighted.
type mark int

const (
	markNone mark = iota
	markSource
	markCursor
	markSelected
	markDest
	markAIMove
)

// markFor returns the highlight for a human-numbered position (1-24, Bar, Off).
func (m Model) markFor(pos int) mark {
	if m.phase == phaseHumanMove && !m.turnDone() {
		if m.selected < 0 {
			srcs := m.sources()
			switch {
			case pos == srcs[m.cursor]:
				return markCursor
			case slices.Contains(srcs, pos):
				return markSource
			}
		} else {
			ds := m.dests()
			switch {
			case pos == ds[m.cursor].To:
				return markCursor
			case pos == m.selected:
				return markSelected
			case slices.ContainsFunc(ds, func(mv game.Move) bool { return mv.To == pos }):
				return markDest
			}
		}
	}
	// The AI's last move, mirrored into human numbering.
	if pos >= 1 && pos <= 24 && m.aiTouched(25-pos) {
		return markAIMove
	}
	return markNone
}

// aiTouched reports whether the AI's last move involved its position r.
func (m Model) aiTouched(r int) bool { return m.aiLast[0] == r || m.aiLast[1] == r }

// cell is one styled terminal cell.
type cell struct {
	ch     string
	fg, bg lipgloss.Color
	bold   bool
}

func (c cell) String() string {
	s := lipgloss.NewStyle().Background(c.bg)
	if c.fg != "" {
		s = s.Foreground(c.fg)
	}
	if c.bold {
		s = s.Bold(true)
	}
	return s.Render(c.ch)
}

// column is one half-height column, indexed [row from the board edge][x].
type column [halfH][]cell

func newColumn(w int, bg lipgloss.Color) column {
	var col column
	for y := range col {
		col[y] = make([]cell, w)
		for x := range col[y] {
			col[y][x] = cell{ch: " ", bg: bg}
		}
	}
	return col
}

// triangle draws a point's triangle in half-cell steps, base at row 0.
func triangle(color lipgloss.Color) column {
	col := newColumn(pointW, felt)
	for y := range triLen {
		margin := (2*pointW*y + triLen) / (2 * triLen) // half-cells trimmed per side, rounded
		for x := range pointW {
			l, r := 2*x, 2*x+1
			in := func(h int) bool { return h >= margin && h < 2*pointW-margin }
			switch {
			case in(l) && in(r):
				col[y][x] = cell{ch: "█", fg: color, bg: felt}
			case in(l):
				col[y][x] = cell{ch: "▌", fg: color, bg: felt}
			case in(r):
				col[y][x] = cell{ch: "▐", fg: color, bg: felt}
			}
		}
	}
	return col
}

// under is the color behind a cell: the triangle if it covers any of it.
func under(c cell) lipgloss.Color {
	if c.ch == " " {
		return c.bg
	}
	return c.fg
}

// drawCheckers stacks n checkers of color from row 0 outward. Each checker
// is a 4×2-cell disc drawn at quadrant resolution, three half-rows tall with
// a half-row gap below so stacked checkers stay distinct:
//
//	▟██▙
//	▝▀▀▘
//
// top says which half of the board the column is in; rows count from the
// board edge, so in the bottom half the disc's upper row is the inner one.
// In the top half the stack starts a row in from the edge.
func drawCheckers(col *column, n int, color lipgloss.Color, top bool) {
	upper, lower := []string{"▟", "█", "█", "▙"}, []string{"▝", "▀", "▀", "▘"}
	x0 := (len(col[0]) - 4) / 2
	for k := range min(n, stackRows) {
		upperY, lowerY := 2*k+1, 2*k+2
		if !top {
			upperY, lowerY = 2*k+1, 2*k
		}
		for i := range 4 {
			c := &col[upperY][x0+i]
			*c = cell{ch: upper[i], fg: color, bg: under(*c)}
			c = &col[lowerY][x0+i]
			*c = cell{ch: lower[i], fg: color, bg: under(*c)}
		}
		if k == stackRows-1 && n > stackRows {
			for i, ch := range fmt.Sprintf("%2d", n) {
				col[upperY][x0+1+i] = cell{ch: string(ch), fg: countFg, bg: color, bold: true}
			}
		}
	}
}

func (m Model) pointColumn(r int, top bool) column {
	color := pointLight
	if r%2 == 0 {
		color = pointDark
	}
	col := triangle(color)
	if n := m.board.C[game.Human][r]; n > 0 {
		drawCheckers(&col, n, humanCol, top)
	} else if n := m.board.OpponentAt(game.Human, r); n > 0 {
		drawCheckers(&col, n, aiCol, top)
	}
	return col
}

// barColumn shows the AI's checkers on the bar in the top half and the
// human's in the bottom half.
func (m Model) barColumn(top bool) column {
	col := newColumn(sideW, tray)
	if top {
		drawCheckers(&col, m.board.C[game.AI][game.Bar], aiCol, true)
	} else {
		drawCheckers(&col, m.board.C[game.Human][game.Bar], humanCol, false)
	}
	return col
}

// offColumn shows borne-off checkers as stacked edge-on slabs.
func (m Model) offColumn(top bool) column {
	p, color, bg := game.Human, humanCol, tray
	if top {
		p, color = game.AI, aiCol
	}
	col := newColumn(sideW, bg)
	n := m.board.C[p][game.Off]
	first := 0 // in the top half, clear of the edge like the checkers
	if top {
		first = 1
	}
	slabs := min(n, halfH-1-first)
	for y := first; y < first+slabs; y++ {
		for x := range sideW {
			col[y][x] = cell{ch: "▀", fg: color, bg: bg}
		}
	}
	if n > 0 {
		for i, ch := range fmt.Sprintf("%2d", n) {
			col[first+slabs][(sideW-2)/2+i] = cell{ch: string(ch), fg: labelFg, bg: bg, bold: true}
		}
	}
	return col
}

// renderHalf renders one half of the board; points are listed left to right.
func (m Model) renderHalf(points []int, top bool) []string {
	edge, lining, side := newColumn(edgeW, walnut), inlayColumn(), shadowColumn()
	cols := []column{edge, lining}
	for i, r := range points {
		if i == 6 {
			cols = append(cols, lining, side, m.barColumn(top), side, edge, lining)
		}
		cols = append(cols, m.pointColumn(r, top))
	}
	cols = append(cols, lining, side, m.offColumn(top), side, edge)

	lines := make([]string, halfH)
	for y := range halfH {
		var b strings.Builder
		for _, col := range cols {
			for _, c := range col[y] {
				b.WriteString(c.String())
			}
		}
		// Rows count outward from the board edge; the bottom half is drawn
		// from the middle down to its edge.
		if top {
			lines[y] = b.String()
		} else {
			lines[halfH-1-y] = b.String()
		}
	}
	return lines
}

// inlayColumn is a one-cell strip of the checkered inlay that lines each
// half of the playing field.
func inlayColumn() column {
	col := newColumn(1, pointDark)
	for y := range col {
		col[y][0] = cell{ch: "▚", fg: pointLight, bg: pointDark}
	}
	return col
}

// The boxes on the board and the side panel cast a shadow on the walnut
// half a cell past their top and right edges.
var shadowOnWood = lipgloss.NewStyle().Foreground(shadow).Background(walnut)

// shadowColumn is the shadow down the right side of a box.
func shadowColumn() column {
	col := newColumn(1, walnut)
	for y := range col {
		col[y][0] = cell{ch: "▐", fg: shadow, bg: walnut}
	}
	return col
}

// shadowSide is one cell of the shadow down the right side of a box.
func shadowSide() string { return shadowOnWood.Render("▐") }

// shadowTop is the shadow above a box w cells wide, with the corner where
// it meets the shadow down the box's right side.
func shadowTop(w int) string { return shadowOnWood.Render(strings.Repeat("▀", w) + "▜") }

func wood(n int) string { return walnutBg.Render(strings.Repeat(" ", n)) }

// shadowRow is the row of shadows above the board's boxes. The bar's and
// tray's start a cell in, to set them apart from the field's beside them.
func shadowRow() string {
	return wood(edgeW) + shadowTop(fieldW) + wood(1) + shadowTop(sideW-1) + wood(edgeW) +
		shadowTop(fieldW) + wood(1) + shadowTop(sideW-1) + wood(edgeW)
}

// liningRow is the inlay strip along the top or bottom of the playing
// field, between the points and the frame.
func liningRow() string {
	field := bandStyle.Render(strings.Repeat("▚", fieldW))
	trayRow := lipgloss.NewStyle().Background(tray).Render(strings.Repeat(" ", sideW))
	return wood(edgeW) + field + shadowSide() + trayRow + shadowSide() + wood(edgeW) +
		field + shadowSide() + trayRow + shadowSide() + wood(edgeW)
}

// labelRow renders the point numbers on the walnut frame. Highlights
// (the cursor, a selected checker, its destinations, the AI's last move)
// color the labels, never the points themselves; the bar and tray are
// labelled on the side whose checkers they hold.
func (m Model) labelRow(points []int, top bool) string {
	base := lipgloss.NewStyle().Background(walnut).Foreground(labelFg).Bold(true).Align(lipgloss.Center)
	styled := func(s lipgloss.Style, mk mark) lipgloss.Style {
		switch mk {
		case markCursor:
			return s.Background(cursorCol).Foreground(diceInk)
		case markSource:
			return s.Foreground(cursorCol).Underline(true)
		case markSelected:
			return s.Background(selectCol)
		case markDest:
			return s.Background(destCol).Foreground(diceInk)
		case markAIMove:
			return s.Background(aiMoveCol).Foreground(diceInk)
		}
		return s
	}
	// The AI's bar and tray are in the top half, the human's in the bottom.
	trayMark := func(pos int) mark {
		if top {
			if m.aiTouched(pos) {
				return markAIMove
			}
			return markNone
		}
		return m.markFor(pos)
	}
	// The top row always names the bar and tray; the bottom row only when
	// one of them is highlighted.
	trayLabel := func(name string, mk mark) string {
		if top || mk != markNone {
			return name
		}
		return ""
	}

	// Labels sit over their boxes: past the field's lining and shadow for
	// the bar and tray.
	var b strings.Builder
	b.WriteString(base.Width(edgeW + 1).Render(""))
	for i, r := range points {
		if i == 6 {
			mk := trayMark(game.Bar)
			b.WriteString(base.Width(2).Render(""))
			b.WriteString(styled(base.Width(sideW).Align(lipgloss.Right), mk).Render(trayLabel("BAR", mk)))
			b.WriteString(base.Width(1 + edgeW + 1).Render(""))
		}
		b.WriteString(styled(base.Width(pointW), m.markFor(r)).Render(fmt.Sprint(r)))
	}
	b.WriteString(base.Width(2).Render(""))
	mk := trayMark(game.Off)
	b.WriteString(styled(base.Width(sideW).Align(lipgloss.Right), mk).Render(trayLabel("OFF", mk)))
	b.WriteString(base.Width(1 + edgeW).Render(""))
	return b.String()
}

// middleRow is the gap between the halves; dice sit on the roller's side.
func (m Model) middleRow() string {
	feltStyle := lipgloss.NewStyle().Background(felt)
	halfW := 6 * pointW
	left, right := feltStyle.Width(halfW).Render(""), feltStyle.Width(halfW).Render("")
	if dice := m.renderDice(); dice != "" {
		placed := lipgloss.PlaceHorizontal(halfW, lipgloss.Center, dice,
			lipgloss.WithWhitespaceBackground(felt))
		if m.phase == phaseAIThinking || m.phase == phaseAIMoving {
			left = placed
		} else {
			right = placed
		}
	}
	lining := bandStyle.Render("▚")
	trayCell := shadowSide() + lipgloss.NewStyle().Background(tray).Width(sideW).Render("") + shadowSide()
	return wood(edgeW) + lining + left + lining + trayCell + wood(edgeW) + lining + right + lining + trayCell + wood(edgeW)
}

func (m Model) renderDice() string {
	if m.dice[0] == 0 {
		return ""
	}
	dice := []int{m.dieFace(0), m.dieFace(1)}
	if m.dice.IsDouble() && !m.roll.active {
		dice = []int{m.dice[0], m.dice[0], m.dice[0], m.dice[0]}
	}
	used := make([]bool, len(dice))
	if m.phase == phaseHumanMove {
		for _, mv := range m.made {
			for i, d := range dice {
				if !used[i] && d == mv.Die {
					used[i] = true
					break
				}
			}
		}
	}
	face, faceUsed := humanCol, humanDiceUsed
	if m.phase == phaseAIThinking || m.phase == phaseAIMoving {
		face, faceUsed = aiCol, aiDiceUsed
	}
	gap := lipgloss.NewStyle().Background(felt).Render(" ")
	var parts []string
	for i, d := range dice {
		s := lipgloss.NewStyle().Bold(true).Foreground(countFg).Background(face)
		if used[i] {
			s = s.Foreground(diceUsedInk).Background(faceUsed)
		}
		parts = append(parts, s.Render(fmt.Sprintf(" %d ", d)))
	}
	return strings.Join(parts, gap)
}

// boardLines renders the board, each line boardInW wide.
func (m Model) boardLines() []string {
	topPts := []int{13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24}
	botPts := []int{12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	lines := []string{m.labelRow(topPts, true), shadowRow(), liningRow()}
	lines = append(lines, m.renderHalf(topPts, true)...)
	lines = append(lines, m.middleRow())
	lines = append(lines, m.renderHalf(botPts, false)...)
	lines = append(lines, liningRow(), m.labelRow(botPts, false))
	return lines
}
