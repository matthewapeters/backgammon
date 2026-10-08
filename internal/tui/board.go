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
	halfH     = 10 // rows in each half of the board (stackRows checkers × 2 rows)
	triLen    = 9  // rows a point's triangle spans, base to tip
	stackRows = 5  // checkers drawn per point before showing a count
	sideW     = 6  // bar and borne-off tray
	boardW    = 12*pointW + sideW + 1 + sideW + 2
)

var (
	walnut     = lipgloss.Color("#6a4527") // frame and label rows
	labelFg    = lipgloss.Color("#f3e9d2")
	felt       = lipgloss.Color("#c9ab7f") // playing surface
	tray       = walnut                    // bar and off tray
	pointLight = lipgloss.Color("#f8f3e7")
	pointDark  = lipgloss.Color("#8b5530")

	humanCol  = lipgloss.Color("#d32f2f")
	aiCol     = lipgloss.Color("#151515")
	countFg   = lipgloss.Color("#ffffff")
	diceFace  = lipgloss.Color("#fbf8f0")
	diceUsed  = lipgloss.Color("#9c8a6e")
	diceInk   = lipgloss.Color("#151515")
	cursorCol = lipgloss.Color("#f4b400")
	selectCol = lipgloss.Color("#2e7d32")
	destCol   = lipgloss.Color("#7cc47f")
	aiMoveCol = lipgloss.Color("#7d8fc4")
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
func drawCheckers(col *column, n int, color lipgloss.Color, top bool) {
	upper, lower := []string{"▟", "█", "█", "▙"}, []string{"▝", "▀", "▀", "▘"}
	x0 := (len(col[0]) - 4) / 2
	for k := range min(n, stackRows) {
		upperY, lowerY := 2*k, 2*k+1
		if !top {
			upperY, lowerY = lowerY, upperY
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

func (m Model) pointColor(r int) lipgloss.Color {
	switch m.markFor(r) {
	case markCursor:
		return cursorCol
	case markSelected:
		return selectCol
	case markDest:
		return destCol
	case markAIMove:
		return aiMoveCol
	}
	if r%2 == 0 {
		return pointDark
	}
	return pointLight
}

func (m Model) pointColumn(r int, top bool) column {
	col := triangle(m.pointColor(r))
	if n := m.board.C[game.Human][r]; n > 0 {
		drawCheckers(&col, n, humanCol, top)
	} else if n := m.board.OpponentAt(game.Human, r); n > 0 {
		drawCheckers(&col, n, aiCol, top)
	}
	return col
}

func trayColor(mk mark) lipgloss.Color {
	switch mk {
	case markCursor:
		return cursorCol
	case markSelected:
		return selectCol
	case markDest:
		return destCol
	}
	return tray
}

// barColumn shows the AI's checkers on the bar in the top half and the
// human's in the bottom half.
func (m Model) barColumn(top bool) column {
	if top {
		bg := tray
		if m.aiTouched(game.Bar) {
			bg = aiMoveCol
		}
		col := newColumn(sideW, bg)
		drawCheckers(&col, m.board.C[game.AI][game.Bar], aiCol, true)
		return col
	}
	col := newColumn(sideW, trayColor(m.markFor(game.Bar)))
	drawCheckers(&col, m.board.C[game.Human][game.Bar], humanCol, false)
	return col
}

// offColumn shows borne-off checkers as stacked edge-on slabs.
func (m Model) offColumn(top bool) column {
	p, color, bg := game.Human, humanCol, trayColor(m.markFor(game.Off))
	if top {
		p, color, bg = game.AI, aiCol, tray
		if m.aiTouched(game.Off) {
			bg = aiMoveCol
		}
	}
	col := newColumn(sideW, bg)
	n := m.board.C[p][game.Off]
	slabs := min(n, halfH-1)
	for y := range slabs {
		for x := 1; x < sideW-1; x++ {
			col[y][x] = cell{ch: "▀", fg: color, bg: bg}
		}
	}
	if n > 0 {
		label := fmt.Sprintf("%2d", n)
		y := min(n, halfH-1)
		for i, ch := range label {
			col[y][2+i] = cell{ch: string(ch), fg: labelFg, bg: bg, bold: true}
		}
	}
	return col
}

// renderHalf renders one half of the board; points are listed left to right.
func (m Model) renderHalf(points []int, top bool) []string {
	cols := make([]column, 0, 15)
	for i, r := range points {
		if i == 6 {
			cols = append(cols, m.barColumn(top))
		}
		cols = append(cols, m.pointColumn(r, top))
	}
	divider := newColumn(1, walnut)
	cols = append(cols, divider, m.offColumn(top))

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

// labelRow renders the point numbers on the walnut frame.
func (m Model) labelRow(points []int, bar, off string) string {
	base := lipgloss.NewStyle().Background(walnut).Foreground(labelFg).Bold(true).Align(lipgloss.Center)
	var b strings.Builder
	for i, r := range points {
		if i == 6 {
			b.WriteString(base.Width(sideW).Render(bar))
		}
		s := base.Width(pointW)
		switch m.markFor(r) {
		case markCursor:
			s = s.Background(cursorCol).Foreground(diceInk)
		case markSource:
			s = s.Foreground(cursorCol).Underline(true)
		case markSelected:
			s = s.Background(selectCol)
		case markDest:
			s = s.Background(destCol).Foreground(diceInk)
		case markAIMove:
			s = s.Background(aiMoveCol).Foreground(diceInk)
		}
		b.WriteString(s.Render(fmt.Sprint(r)))
	}
	b.WriteString(base.Width(1).Render(""))
	b.WriteString(base.Width(sideW).Render(off))
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
	return left + lipgloss.NewStyle().Background(tray).Width(sideW).Render("") +
		right + lipgloss.NewStyle().Background(walnut).Width(1).Render("") +
		lipgloss.NewStyle().Background(tray).Width(sideW).Render("")
}

func (m Model) renderDice() string {
	if m.dice[0] == 0 {
		return ""
	}
	dice := []int{m.dice[0], m.dice[1]}
	if m.dice.IsDouble() {
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
	gap := lipgloss.NewStyle().Background(felt).Render(" ")
	var parts []string
	for i, d := range dice {
		face := diceFace
		if used[i] {
			face = diceUsed
		}
		parts = append(parts, lipgloss.NewStyle().Bold(true).Foreground(diceInk).Background(face).Render(fmt.Sprintf(" %d ", d)))
	}
	return strings.Join(parts, gap)
}

func (m Model) renderBoard() string {
	topPts := []int{13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24}
	botPts := []int{12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}
	lines := []string{m.labelRow(topPts, "BAR", "OFF")}
	lines = append(lines, m.renderHalf(topPts, true)...)
	lines = append(lines, m.middleRow())
	lines = append(lines, m.renderHalf(botPts, false)...)
	lines = append(lines, m.labelRow(botPts, "", ""))
	return lipgloss.NewStyle().
		Border(lipgloss.OuterHalfBlockBorder()).
		BorderForeground(walnut).
		Render(strings.Join(lines, "\n"))
}
