package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/MatthewAPeters/backgammon/internal/ai"
	"github.com/MatthewAPeters/backgammon/internal/game"
)

const (
	paneW = 40 // the side panel, inlay lining included
	// The play screen inside its frame: the board (which ends in walnut),
	// the panel and its shadow, and a walnut edge.
	screenInW = boardInW + paneW + 1 + edgeW
	screenW   = screenInW + 6
)

var (
	dimStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#8a8a8a"))

	// The panel and status line sit on the board's wood and felt.
	plateName = lipgloss.NewStyle().Foreground(lipgloss.Color("#e6c27a")).Background(walnut).Bold(true)
	plateDim  = lipgloss.NewStyle().Foreground(felt).Background(walnut)
	plateText = lipgloss.NewStyle().Foreground(labelFg).Background(walnut).Bold(true)
	youOnFelt = onFelt(humanCol).Bold(true)
)

func (m Model) View() string {
	switch m.phase {
	case phaseMenu:
		return m.viewMenu()
	case phaseAbout:
		return m.viewAbout()
	case phasePersona:
		return m.viewPersona()
	case phaseNewPersona, phaseGenerating:
		return m.viewNewPersona()
	case phaseOffer:
		return m.viewOffer()
	}
	board := m.boardLines()
	w := screenInW
	var lines []string
	if m.width == 0 || m.width >= screenW {
		pane := m.paneLines(paneW, len(board))
		for i := range board {
			lines = append(lines, board[i]+pane[i]+wood(edgeW))
		}
	} else {
		// Too narrow for the panel beside the board: put it underneath.
		w = boardInW
		lines = append(board, walnutBg.Render(strings.Repeat(" ", w)))
		for _, l := range m.paneLines(w-2*edgeW-1, len(board)-4) {
			lines = append(lines, wood(edgeW)+l+wood(edgeW))
		}
	}
	lines = append(lines, padBg(walnutBg.Render("   ")+plateText.Render(m.statusLine()), w, walnutBg))
	return m.place(lipgloss.JoinVertical(lipgloss.Left,
		inlayW(lines, w),
		footerW(m.helpLine(), w+6),
	))
}

// paneLines renders the side panel as a felt pane lined with inlay, w cells
// wide (plus its shadow) and h tall: a walnut nameplate on top, the score,
// pips and the game log on the felt, and the LLM's status on a plate below.
// Its rows line up with the board's: nameplate, shadow, lining.
func (m Model) paneLines(w, h int) []string {
	tw := w - 4 // text width inside the lining and a cell of felt each side
	var body []string
	add := func(ls ...string) { body = append(body, ls...) }

	add(styled(m.persona.Tagline, tw, inkItalic)...)
	add("")
	add(m.scoreTable()...)
	if m.saveNote != "" {
		add(styled(m.saveNote, tw, inkDim)...)
	}
	add(inkDim.Render(strings.Repeat("─", tw)))

	var log []string
	for _, e := range m.log {
		if e.who == "" {
			log = append(log, styled("· "+e.text, tw, inkDim)...)
			continue
		}
		name := shortName(m.persona) + ":"
		ls := wrapWords(name+" "+e.text, tw)
		for i, l := range ls {
			if i == 0 && strings.HasPrefix(l, name) {
				log = append(log, inkBold.Render(name)+inkStyle.Render(strings.TrimPrefix(l, name)))
			} else {
				log = append(log, inkStyle.Render(l))
			}
		}
	}
	if m.phase == phaseAIThinking && !m.roll.active {
		sp := m.spinner
		sp.Style = inkStyle
		log = append(log, sp.View()+inkDim.Render(" "+shortName(m.persona)+" is thinking…"))
	}
	room := max(0, h-5-len(body))
	if len(log) > room {
		log = log[len(log)-room:]
	}
	add(log...)

	llm := m.llmStatus
	if llm == "" {
		llm = "idle"
	}
	lining := bandStyle.Render("▚")
	band := bandStyle.Render(strings.Repeat("▚", w))
	out := []string{
		plateName.Width(w).Align(lipgloss.Center).Render(m.persona.Name) + wood(1),
		shadowTop(w),
		band + shadowSide(),
	}
	for i := range h - 5 {
		l := ""
		if i < len(body) {
			l = body[i]
		}
		out = append(out, lining+padBg(feltBg.Render(" ")+l, w-2, feltBg)+lining+shadowSide())
	}
	return append(out,
		band+shadowSide(),
		plateDim.Width(w).Align(lipgloss.Center).Render("LLM "+llm)+wood(1),
	)
}

// scoreTable is the match score, the player's record against this
// opponent and the pip counts as a small table, with a column per player
// and the numbers right-aligned under the names.
func (m Model) scoreTable() []string {
	name := shortName(m.persona)
	youW, themW := 3, max(lipgloss.Width(name), 3)
	right := func(s string, w int) string { return strings.Repeat(" ", max(0, w-lipgloss.Width(s))) + s }
	label := func(s string) string { return inkDim.Render(fmt.Sprintf("%-8s", s)) }
	row := func(title string, you, them int) string {
		return label(title) + inkStyle.Render(right(fmt.Sprint(you), youW+2)+right(fmt.Sprint(them), themW+3))
	}
	out := []string{
		label("") + feltBg.Render("  ") + youOnFelt.Render("You") + feltBg.Render("   ") + inkBold.Render(right(name, themW)),
	}
	if m.match != nil {
		out = append(out, row("Match", m.match.Score[game.Human], m.match.Score[game.AI])+
			inkDim.Render(fmt.Sprintf("  to %d", m.match.Length)))
	}
	return append(out,
		row("Matches", m.record.Matches[game.Human], m.record.Matches[game.AI]),
		row("Games", m.record.Games[game.Human], m.record.Games[game.AI]),
		row("Pips", m.board.PipCount(game.Human), m.board.PipCount(game.AI)),
	)
}

// wrapWords word-wraps plain text to w cells.
func wrapWords(text string, w int) []string {
	var out []string
	line := ""
	for _, word := range strings.Fields(text) {
		switch {
		case line == "":
			line = word
		case lipgloss.Width(line)+1+lipgloss.Width(word) <= w:
			line += " " + word
		default:
			out = append(out, line)
			line = word
		}
	}
	if line != "" {
		out = append(out, line)
	}
	return out
}

// styled word-wraps plain text to w cells and renders each line in st.
func styled(text string, w int, st lipgloss.Style) []string {
	ls := wrapWords(text, w)
	for i, l := range ls {
		ls[i] = st.Render(l)
	}
	return ls
}

// shortName is the persona name without a leading adjective, for tight spots.
func shortName(p ai.Persona) string {
	f := strings.Fields(p.Name)
	if len(f) == 0 {
		return "Opponent"
	}
	return f[len(f)-1]
}

func (m Model) statusLine() string {
	if m.roll.active {
		if m.phase == phaseHumanRoll {
			return "Rolling…"
		}
		return m.persona.Name + " is rolling…"
	}
	return m.status
}

func (m Model) helpLine() string {
	switch m.phase {
	case phaseHumanMove:
		if m.turnDone() {
			return "enter finish turn · u undo · m menu · q quit"
		}
		if m.selected >= 0 {
			return "←/→ choose destination · enter move · esc cancel · u undo · m menu · q quit"
		}
		return "←/→ choose checker · enter select · u undo · m menu · q quit"
	case phaseHumanRoll, phaseOpening:
		return "space roll/continue · m menu · q quit"
	case phaseGameOver:
		if m.midMatch() {
			return "space next game · m menu · q quit"
		}
		return "n new game · p change opponent · m menu · q quit"
	}
	return "q quit"
}
