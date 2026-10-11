package tui

import (
	"fmt"
	"slices"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/MatthewAPeters/backgammon/internal/ai"
)

// Main menu entries, in order.
const (
	menuNew = iota
	menuResume
	menuPersona
	menuAbout
	menuQuit
	menuItems
)

const itemW = 40 // width of a menu entry

func (m Model) keyMenu(k string) (tea.Model, tea.Cmd) {
	if m.confirmNew {
		switch k {
		case "y", "Y":
			return m.startGame()
		case "n", "N", "esc":
			m.confirmNew = false
			m.menuNote = ""
		}
		return m, nil
	}
	step := 0
	switch k {
	case "up", "k", "shift+tab":
		step = menuItems - 1
	case "down", "j", "tab":
		step = 1
	case "enter", " ", "space":
		return m.chooseMenu(m.menu)
	case "n":
		return m.chooseMenu(menuNew)
	case "r":
		return m.chooseMenu(menuResume)
	case "o", "p":
		return m.chooseMenu(menuPersona)
	case "a", "?":
		return m.chooseMenu(menuAbout)
	}
	if step != 0 {
		m.menu = (m.menu + step) % menuItems
		if m.menu == menuResume && !m.canResume() {
			m.menu = (m.menu + step) % menuItems
		}
	}
	return m, nil
}

func (m Model) chooseMenu(item int) (tea.Model, tea.Cmd) {
	m.menu = item
	switch item {
	case menuNew:
		if m.canResume() {
			m.confirmNew = true
			m.menuNote = fmt.Sprintf("Your game against the %s isn't finished. Abandon it and start a new one? (y/n)", m.rec.Persona.Name)
			return m, nil
		}
		return m.startGame()
	case menuResume:
		if m.canResume() {
			return m.resume()
		}
	case menuPersona:
		m.phase = phasePersona
		m.menuNote = ""
		if i := slices.IndexFunc(m.roster, func(p ai.Persona) bool { return p.Name == m.persona.Name }); i >= 0 {
			m.pick = i
		}
	case menuAbout:
		m.phase = phaseAbout
		m.about.scroll = 0
	case menuQuit:
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) viewMenu() string {
	var ls []string
	ls = append(ls, pointRows(true)...)
	ls = append(ls, feltBlank())
	for _, r := range plaque("BACKGAMMON", 3) {
		ls = append(ls, feltCenter(r))
	}
	ls = append(ls, feltCenter(discs(7)), feltBlank())

	ls = append(ls,
		feltCenter(inkDim.Render(spaced("YOUR OPPONENT"))),
		feltCenter(inkBold.Render(m.persona.Name)),
		feltCenter(inkItalic.Render(m.persona.Tagline)),
		feltBlank())

	for i := range menuItems {
		ls = append(ls, feltCenter(m.menuEntry(i)))
	}

	note := m.menuNote
	if note == "" {
		note = m.saveNote
	}
	ls = append(ls, feltCenter(inkBold.Render(note)))
	ls = append(ls, pointRows(false)...)

	model := m.client.Model
	if model == "" {
		model = "auto"
	}
	help := "↑/↓ choose · enter select · n new · r resume · o opponent · a about · q quit"
	if m.confirmNew {
		help = "y abandon it and start a new game · n keep it"
	}
	return m.place(lipgloss.JoinVertical(lipgloss.Left,
		inlay(ls),
		footer(help),
		footer(fmt.Sprintf("llama.cpp: %s  model: %s", m.client.BaseURL, model)),
	))
}

func (m Model) menuEntry(i int) string {
	label := [menuItems]string{"New game", "Resume", "Choose opponent", "About the game", "Quit"}[i]
	disabled := false
	if i == menuResume {
		if m.canResume() {
			label = fmt.Sprintf("Resume · vs %s, turn %d", shortName(m.rec.Persona), len(m.rec.Turns)+1)
		} else {
			label = "Resume · no unfinished game"
			disabled = true
		}
	}
	switch {
	case i == m.menu:
		return chipStyle.Width(itemW).Render(" ▸ " + label)
	case disabled:
		return inkDim.Width(itemW).Render("   " + label)
	}
	return inkStyle.Width(itemW).Render("   " + label)
}

// viewPersona is the opponent picker.
func (m Model) viewPersona() string {
	entries := slices.Concat(m.roster, []ai.Persona{{Name: "+ New opponent…", Tagline: "Name anyone; the AI writes the character"}})

	// Show a window of entries around the cursor if they don't all fit.
	first, n := 0, len(entries)
	if m.height > 0 {
		n = max(3, (m.height-12)/2)
		first = min(max(0, m.pick-n/2), max(0, len(entries)-n))
		n = min(n, len(entries)-first)
	}

	ls := []string{feltBlank(), feltCenter(inkBold.Render(spaced("CHOOSE YOUR OPPONENT"))), feltBlank()}
	if first > 0 {
		ls = append(ls, feltLeft(inkDim.Render("   ▲ more"), 8))
	} else {
		ls = append(ls, feltBlank())
	}
	for i := first; i < first+n; i++ {
		p := entries[i]
		num := fmt.Sprintf("%d.", i+1)
		if i == len(m.roster) {
			num = "  "
		}
		name := inkStyle.Render(fmt.Sprintf("   %-3s %s", num, p.Name))
		if i == m.pick {
			name = chipStyle.Render(fmt.Sprintf(" ▸ %-3s %s ", num, p.Name))
		}
		ls = append(ls, feltLeft(name, 8), feltLeft(inkItalic.Render(p.Tagline), 15))
	}
	if first+n < len(entries) {
		ls = append(ls, feltLeft(inkDim.Render("   ▼ more"), 8))
	} else {
		ls = append(ls, feltBlank())
	}
	ls = append(ls, feltCenter(inkBold.Render(m.menuNote)), feltBlank())
	return m.place(lipgloss.JoinVertical(lipgloss.Left,
		inlay(ls),
		footer("↑/↓ choose · enter pick · n new opponent · esc menu · q quit"),
	))
}

func (m Model) viewNewPersona() string {
	ls := []string{
		feltBlank(),
		feltCenter(inkBold.Render(spaced("A NEW OPPONENT"))),
		feltBlank(),
		feltLeft(inkStyle.Render("Who do you want to play against?"), 8),
		feltLeft(inkItalic.Render("A name is enough. Add as much description as you like."), 8),
		feltBlank(),
		feltLeft(m.input.View(), 8),
		feltBlank(),
	}
	help := "enter create · esc back"
	switch {
	case m.phase == phaseGenerating:
		sp := m.spinner
		sp.Style = inkStyle
		ls = append(ls, feltLeft(sp.View()+inkDim.Render(" Writing the character…"), 8))
		help = "esc back · q quit"
	default:
		ls = append(ls, feltLeft(inkBold.Render(m.menuNote), 8))
	}
	ls = append(ls, feltBlank())
	return m.place(lipgloss.JoinVertical(lipgloss.Left, inlay(ls), footer(help)))
}

// styleInput puts the text input on the felt.
func styleInput(in *textinput.Model) {
	in.PromptStyle = inkBold
	in.TextStyle = inkStyle
	in.PlaceholderStyle = inkItalic
	in.Cursor.Style = lipgloss.NewStyle().Foreground(inlayInk)
	in.Cursor.TextStyle = inkStyle
	in.Prompt = "› "
}
