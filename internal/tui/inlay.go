package tui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// The menu screens are drawn as an inlaid wooden panel in the board's
// colors: a walnut frame, a band of light and dark checkered inlay, and a
// felt field with rows of points along the top and bottom.
const (
	frameW = boardW     // same width as the board
	innerW = frameW - 6 // felt field inside the frame and band
)

var (
	inlayInk  = lipgloss.Color("#3b2414") // text on felt
	inlayDim  = lipgloss.Color("#6f4e30")
	grainCol  = lipgloss.Color("#58391f") // grain lines in the walnut
	inlayCode = lipgloss.Color("#efe4cc") // background for `code` on felt

	feltBg     = lipgloss.NewStyle().Background(felt)
	walnutBg   = lipgloss.NewStyle().Background(walnut)
	inkStyle   = onFelt(inlayInk)
	inkBold    = onFelt(inlayInk).Bold(true)
	inkDim     = onFelt(inlayDim)
	inkItalic  = onFelt(inlayDim).Italic(true)
	chipStyle  = lipgloss.NewStyle().Foreground(labelFg).Background(walnut).Bold(true)
	codeStyle  = lipgloss.NewStyle().Foreground(inlayInk).Background(inlayCode)
	bandStyle  = lipgloss.NewStyle().Foreground(pointLight).Background(pointDark)
	plaqueText = lipgloss.NewStyle().Foreground(pointLight).Background(walnut)
)

func onFelt(fg lipgloss.Color) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(fg).Background(felt)
}

// Every segment drawn on the felt must carry the felt background itself,
// since each styled segment ends with a reset.

// feltCenter centers a rendered line in the felt field.
func feltCenter(s string) string {
	w := lipgloss.Width(s)
	if w >= innerW {
		return ansi.Truncate(s, innerW, "")
	}
	l := (innerW - w) / 2
	return feltBg.Render(strings.Repeat(" ", l)) + s + feltBg.Render(strings.Repeat(" ", innerW-w-l))
}

// feltLeft places a rendered line indent cells from the left of the felt field.
func feltLeft(s string, indent int) string {
	s = feltBg.Render(strings.Repeat(" ", indent)) + s
	w := lipgloss.Width(s)
	if w >= innerW {
		return ansi.Truncate(s, innerW, "")
	}
	return s + feltBg.Render(strings.Repeat(" ", innerW-w))
}

func feltBlank() string { return feltBg.Render(strings.Repeat(" ", innerW)) }

// grain is a walnut strip n cells wide with a few grain lines in it.
func grain(n, seed int) string {
	var b strings.Builder
	line := lipgloss.NewStyle().Foreground(grainCol).Background(walnut)
	run := ""
	flush := func(grained bool) {
		if run == "" {
			return
		}
		if grained {
			b.WriteString(line.Render(run))
		} else {
			b.WriteString(walnutBg.Render(run))
		}
		run = ""
	}
	prev := false
	for i := range n {
		h := (i*i*7 + i*3 + seed*31) % 23
		g := h < 6
		if g != prev {
			flush(prev)
			prev = g
		}
		if g {
			run += map[bool]string{true: "─", false: "╌"}[h < 4]
		} else {
			run += " "
		}
	}
	flush(prev)
	return b.String()
}

// inlay frames felt lines (each innerW wide) in walnut and inlay banding.
func inlay(lines []string) string { return inlayW(lines, innerW) }

// inlayW frames lines that are each w cells wide.
func inlayW(lines []string, w int) string {
	side := walnutBg.Render("  ")
	band := func(n int) string { return bandStyle.Render(strings.Repeat("▚", n)) }
	out := []string{grain(w+6, 1), side + band(w+2) + side}
	for _, l := range lines {
		out = append(out, side+band(1)+l+band(1)+side)
	}
	out = append(out, side+band(w+2)+side, grain(w+6, 2))
	return strings.Join(out, "\n")
}

// padBg pads a rendered line with cells of style bg to w cells, or
// truncates it to w.
func padBg(s string, w int, bg lipgloss.Style) string {
	n := lipgloss.Width(s)
	if n > w {
		return ansi.Truncate(s, w, "")
	}
	return s + bg.Render(strings.Repeat(" ", w-n))
}

// pointRows draws a row of twelve points, six each side of a walnut bar,
// pointing down from the top edge (top) or up from the bottom edge.
func pointRows(top bool) []string {
	shape := []string{"██████", " ████ ", "  ██  "}
	if !top {
		shape = []string{shape[2], shape[1], shape[0]}
	}
	barW := 9 + (innerW-12*pointW-9)%2 // keeps the margins even
	margin := (innerW - 12*pointW - barW) / 2
	rows := make([]string, len(shape))
	for y, s := range shape {
		var b strings.Builder
		b.WriteString(feltBg.Render(strings.Repeat(" ", margin)))
		for i := range 12 {
			if i == 6 {
				b.WriteString(walnutBg.Render(strings.Repeat(" ", barW)))
			}
			col := pointLight
			if (i%2 == 0) == top {
				col = pointDark
			}
			b.WriteString(onFelt(col).Render(s))
		}
		b.WriteString(feltBg.Render(strings.Repeat(" ", innerW-12*pointW-barW-margin)))
		rows[y] = b.String()
	}
	return rows
}

// glyphs are 5-pixel-high letters for the title, drawn with half blocks.
var glyphs = map[rune][5]string{
	'A': {".##.", "#..#", "####", "#..#", "#..#"},
	'B': {"###.", "#..#", "###.", "#..#", "###."},
	'C': {".###", "#...", "#...", "#...", ".###"},
	'G': {".###", "#...", "#.##", "#..#", ".###"},
	'K': {"#..#", "#.#.", "##..", "#.#.", "#..#"},
	'M': {"#...#", "##.##", "#.#.#", "#...#", "#...#"},
	'N': {"#..#", "##.#", "#.##", "#..#", "#..#"},
	'O': {".##.", "#..#", "#..#", "#..#", ".##."},
}

// plaque renders text in large ivory letters inlaid in a walnut plaque.
// The letters sit half a row below the plaque's top edge, and the last row
// is the plaque's lower half-row margin.
func plaque(text string, margin int) []string {
	// Pixel rows: a blank margin row, then the five glyph rows.
	px := make([]string, 6)
	for i, r := range text {
		g := glyphs[r]
		for y := range 5 {
			if i > 0 {
				px[y+1] += "."
			}
			px[y+1] += g[y]
		}
	}
	w := len(px[1])
	px[0] = strings.Repeat(".", w)
	pad := walnutBg.Render(strings.Repeat(" ", margin))
	var rows []string
	for y := 0; y < 6; y += 2 {
		var b strings.Builder
		for x := range w {
			hi, lo := px[y][x] == '#', px[y+1][x] == '#'
			b.WriteString(map[[2]bool]string{{false, false}: " ", {true, false}: "▀", {false, true}: "▄", {true, true}: "█"}[[2]bool{hi, lo}])
		}
		rows = append(rows, pad+plaqueText.Render(b.String())+pad)
	}
	edge := lipgloss.NewStyle().Foreground(walnut).Background(felt)
	rows = append(rows, edge.Render(strings.Repeat("▀", w+2*margin)))
	return rows
}

// discs is a row of alternating red and black checkers.
func discs(n int) string {
	var parts []string
	for i := range n {
		col := humanCol
		if i%2 == 1 {
			col = aiCol
		}
		parts = append(parts, onFelt(col).Render("●"))
	}
	return strings.Join(parts, feltBg.Render("   "))
}

// spaced letter-spaces s, for small headings.
func spaced(s string) string { return strings.Join(strings.Split(s, ""), " ") }

// footer is a line of help text centered under the frame.
func footer(s string) string { return footerW(s, frameW) }

// footerW is a line of help text centered under a frame w cells wide.
func footerW(s string, w int) string {
	return lipgloss.NewStyle().Width(w).Align(lipgloss.Center).Render(dimStyle.Render(s))
}

// place centers a framed screen in the terminal.
func (m Model) place(s string) string {
	if m.width > frameW {
		return lipgloss.PlaceHorizontal(m.width, lipgloss.Center, s)
	}
	return s
}
