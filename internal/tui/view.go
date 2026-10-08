package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"github.com/MatthewAPeters/backgammon/internal/ai"
	"github.com/MatthewAPeters/backgammon/internal/game"
)

const (
	panelW   = 42
	logLines = 15
)

var (
	titleStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#e6c27a"))
	dimStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("#8a8a8a"))
	italicDim    = dimStyle.Italic(true)
	personaStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#e6c27a"))
	youStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#ef6b6b"))
	statusStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("#f3e9d2")).Bold(true)
	panelStyle   = lipgloss.NewStyle().Width(panelW).PaddingLeft(2)
)

func (m Model) View() string {
	if m.phase == phasePersona {
		return m.viewPersona()
	}
	board := m.renderBoard()
	var main string
	if m.width > 0 && m.width < boardW+panelW {
		main = lipgloss.JoinVertical(lipgloss.Left, board, m.renderPanel())
	} else {
		main = lipgloss.JoinHorizontal(lipgloss.Top, board, m.renderPanel())
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		main,
		statusStyle.Render(m.statusLine()),
		dimStyle.Render(m.helpLine()),
	)
}

func (m Model) viewPersona() string {
	var b strings.Builder
	b.WriteString(titleStyle.Render("♦ B A C K G A M M O N ♦") + "\n")
	b.WriteString(dimStyle.Render("Choose your opponent") + "\n\n")
	for i, p := range ai.Personas {
		cursor := "  "
		name := p.Name
		if i == m.pick {
			cursor = titleStyle.Render("▸ ")
			name = personaStyle.Render(name)
		}
		fmt.Fprintf(&b, "%s%d. %s\n     %s\n", cursor, i+1, name, italicDim.Render(p.Tagline))
	}
	model := m.client.Model
	if model == "" {
		model = "auto"
	}
	fmt.Fprintf(&b, "\n%s\n", dimStyle.Render(fmt.Sprintf("llama.cpp: %s  model: %s", m.client.BaseURL, model)))
	b.WriteString(dimStyle.Render("↑/↓ choose · enter play · q quit"))
	return lipgloss.NewStyle().Padding(1, 2).Render(b.String())
}

func (m Model) renderPanel() string {
	var b strings.Builder
	b.WriteString(personaStyle.Render(m.persona.Name) + "\n")
	b.WriteString(italicDim.Render(m.persona.Tagline) + "\n\n")
	fmt.Fprintf(&b, "%s %s %d  %s %d\n", dimStyle.Render("Score"),
		youStyle.Render("You"), m.score[game.Human], personaStyle.Render(shortName(m.persona)), m.score[game.AI])
	fmt.Fprintf(&b, "%s  %s %d  %s %d\n", dimStyle.Render("Pips"),
		youStyle.Render("You"), m.board.PipCount(game.Human), personaStyle.Render(shortName(m.persona)), m.board.PipCount(game.AI))
	llm := m.llmStatus
	if llm == "" {
		llm = "idle"
	}
	b.WriteString(dimStyle.Render("LLM   "+llm) + "\n")
	b.WriteString(dimStyle.Render(strings.Repeat("─", panelW-2)) + "\n")

	var logText []string
	textW := panelW - 2
	for _, e := range m.log {
		var line string
		if e.who == "" {
			line = dimStyle.Width(textW).Render("· " + e.text)
		} else {
			line = lipgloss.NewStyle().Width(textW).Render(personaStyle.Render(shortName(m.persona)+":") + " " + e.text)
		}
		logText = append(logText, strings.Split(line, "\n")...)
	}
	if m.phase == phaseAIThinking {
		logText = append(logText, m.spinner.View()+dimStyle.Render(" "+shortName(m.persona)+" is thinking…"))
	}
	if len(logText) > logLines {
		logText = logText[len(logText)-logLines:]
	}
	b.WriteString(strings.Join(logText, "\n"))
	return panelStyle.Render(b.String())
}

// shortName is the persona name without a leading adjective, for tight spots.
func shortName(p ai.Persona) string {
	f := strings.Fields(p.Name)
	return f[len(f)-1]
}

func (m Model) statusLine() string { return m.status }

func (m Model) helpLine() string {
	switch m.phase {
	case phaseHumanMove:
		if m.turnDone() {
			return "enter finish turn · u undo · q quit"
		}
		if m.selected >= 0 {
			return "←/→ choose destination · enter move · esc cancel · u undo · q quit"
		}
		return "←/→ choose checker · enter select · u undo · q quit"
	case phaseHumanRoll, phaseOpening:
		return "space roll/continue · q quit"
	case phaseGameOver:
		return "n new game · p change opponent · q quit"
	}
	return "q quit"
}
