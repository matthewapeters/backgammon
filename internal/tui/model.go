// Package tui is the Bubble Tea front end for the backgammon game.
package tui

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/MatthewAPeters/backgammon/internal/ai"
	"github.com/MatthewAPeters/backgammon/internal/game"
)

type phase int

const (
	phasePersona phase = iota
	phaseOpening
	phaseHumanRoll
	phaseHumanMove
	phaseAIThinking
	phaseAIMoving
	phaseGameOver
)

const aiStepDelay = 700 * time.Millisecond

type logEntry struct {
	who  string // "" for narration
	text string
}

// Model is the root Bubble Tea model.
type Model struct {
	client  *ai.Client
	persona ai.Persona
	pick    int // persona menu cursor

	phase phase
	board game.Board
	dice  game.Dice

	// Human turn state.
	plays     []game.Play
	made      []game.Move
	turnStart game.Board
	selected  int // selected source position, or -1
	cursor    int // index into the current source or destination list

	// AI turn state.
	aiPlay  game.Play
	aiQueue []game.Move
	aiLast  [2]int     // last AI move's from/to in the AI's numbering, -1 if none
	review  *ai.Review // grade of the human's last play, for the AI to react to

	opening   [2]int // opening roll: human, AI
	winner    game.Player
	winKind   game.WinKind
	score     [2]int
	log       []logEntry
	status    string
	llmStatus string
	spinner   spinner.Model
	width     int
}

// New returns the initial model.
func New(client *ai.Client) Model {
	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	return Model{client: client, spinner: sp, selected: -1, aiLast: [2]int{-1, -1}}
}

// Messages.
type (
	aiDecisionMsg struct{ dec ai.Decision }
	aiStepMsg     struct{}
	reactionMsg   struct{ text string }
	warmupMsg     struct{ err error }
)

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		return m.handleKey(msg.String())

	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil

	case spinner.TickMsg:
		if m.phase != phaseAIThinking {
			return m, nil
		}
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case warmupMsg:
		if msg.err != nil {
			m.llmStatus = "offline: " + shortErr(msg.err)
		} else {
			m.llmStatus = "ready"
		}
		return m, nil

	case aiDecisionMsg:
		return m.applyAIDecision(msg.dec)

	case aiStepMsg:
		return m.stepAI()

	case reactionMsg:
		m.say(m.persona.Name, msg.text)
		return m, nil
	}
	return m, nil
}

func (m Model) handleKey(k string) (tea.Model, tea.Cmd) {
	if k == "q" {
		return m, tea.Quit
	}
	switch m.phase {
	case phasePersona:
		return m.keyPersona(k)
	case phaseOpening:
		if isConfirm(k) {
			return m.finishOpening()
		}
	case phaseHumanRoll:
		if isConfirm(k) || k == "r" {
			m.rollForHuman()
		}
	case phaseHumanMove:
		return m.keyMove(k)
	case phaseGameOver:
		switch k {
		case "n", "enter", " ", "space":
			return m.newGame()
		case "p":
			m.phase = phasePersona
		}
	}
	return m, nil
}

func isConfirm(k string) bool { return k == "enter" || k == " " || k == "space" }

func (m Model) keyPersona(k string) (tea.Model, tea.Cmd) {
	n := len(ai.Personas)
	switch k {
	case "up", "k", "shift+tab":
		m.pick = (m.pick + n - 1) % n
	case "down", "j", "tab":
		m.pick = (m.pick + 1) % n
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		if i := int(k[0] - '1'); i < n {
			m.pick = i
		}
	case "enter", " ", "space":
		m.persona = ai.Personas[m.pick]
		m.score = [2]int{}
		m.log = nil
		m.say("", fmt.Sprintf("You sit down across from the %s.", m.persona.Name))
		if m.llmStatus != "ready" {
			m.llmStatus = "loading model…"
		}
		client := m.client
		warm := func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			return warmupMsg{client.Warmup(ctx)}
		}
		next, cmd := m.newGame()
		return next, tea.Batch(cmd, warm)
	}
	return m, nil
}

func (m Model) newGame() (tea.Model, tea.Cmd) {
	m.board = game.NewBoard()
	m.review = nil
	m.made = nil
	m.plays = nil
	m.selected = -1
	m.aiLast = [2]int{-1, -1}
	m.dice = game.Dice{}
	for {
		m.opening = [2]int{game.RollDie(), game.RollDie()}
		if m.opening[0] != m.opening[1] {
			break
		}
	}
	m.phase = phaseOpening
	first := "You go first"
	if m.opening[1] > m.opening[0] {
		first = m.persona.Name + " goes first"
	}
	m.status = fmt.Sprintf("Opening roll: you %d, %s %d. %s! (space)", m.opening[0], m.persona.Name, m.opening[1], first)
	return m, nil
}

func (m Model) finishOpening() (tea.Model, tea.Cmd) {
	if m.opening[0] > m.opening[1] {
		m.startHumanTurn(game.Dice{m.opening[0], m.opening[1]})
		return m, nil
	}
	return m.startAITurn(game.Dice{m.opening[1], m.opening[0]})
}

func (m *Model) rollForHuman() { m.startHumanTurn(game.RollDice()) }

func (m *Model) startHumanTurn(d game.Dice) {
	m.phase = phaseHumanMove
	m.dice = d
	m.turnStart = m.board
	m.plays = m.board.LegalPlays(game.Human, d)
	m.made = nil
	m.selected = -1
	m.cursor = 0
	if m.turnDone() {
		m.status = fmt.Sprintf("You rolled %s, but have no legal moves. Press enter to pass.", d)
	} else {
		m.status = fmt.Sprintf("You rolled %s.", d)
	}
}

// nextMoves are the single moves available now in the human's turn.
func (m Model) nextMoves() []game.Move { return game.NextMoves(m.plays, m.made) }

func (m Model) turnDone() bool { return len(m.nextMoves()) == 0 }

// sources are the positions the human may move a checker from, high to low.
func (m Model) sources() []int {
	var out []int
	for _, mv := range m.nextMoves() {
		if !slices.Contains(out, mv.From) {
			out = append(out, mv.From)
		}
	}
	slices.Sort(out)
	slices.Reverse(out)
	return out
}

// dests are the moves available from the selected source, high to low.
func (m Model) dests() []game.Move {
	var out []game.Move
	for _, mv := range m.nextMoves() {
		if mv.From == m.selected {
			out = append(out, mv)
		}
	}
	slices.SortFunc(out, func(a, b game.Move) int { return b.To - a.To })
	return out
}

func (m Model) keyMove(k string) (tea.Model, tea.Cmd) {
	switch k {
	case "u", "backspace":
		m.undo()
		return m, nil
	case "esc":
		m.selected, m.cursor = -1, 0
		return m, nil
	}
	if m.turnDone() {
		if isConfirm(k) {
			return m.commitHumanTurn()
		}
		return m, nil
	}

	n := len(m.sources())
	if m.selected >= 0 {
		n = len(m.dests())
	}
	switch k {
	case "right", "down", "l", "j", "tab":
		m.cursor = (m.cursor + 1) % n
	case "left", "up", "h", "k", "shift+tab":
		m.cursor = (m.cursor + n - 1) % n
	case "enter", " ", "space":
		if m.selected < 0 {
			m.selected = m.sources()[m.cursor]
			m.cursor = 0
			return m, nil
		}
		mv := m.dests()[m.cursor]
		_, nb, ok := m.board.TryMove(game.Human, mv.From, mv.Die)
		if !ok {
			m.status = "That move isn't legal." // shouldn't happen
			return m, nil
		}
		m.board = nb
		m.made = append(m.made, mv)
		m.selected, m.cursor = -1, 0
		if _, won := m.board.Winner(); won {
			return m.commitHumanTurn()
		}
		if m.turnDone() {
			m.status = "Turn complete. Press enter to finish, or u to undo."
		}
	}
	return m, nil
}

func (m *Model) undo() {
	if len(m.made) == 0 {
		m.selected, m.cursor = -1, 0
		return
	}
	m.made = m.made[:len(m.made)-1]
	m.board = m.turnStart
	for _, mv := range m.made {
		_, m.board, _ = m.board.TryMove(game.Human, mv.From, mv.Die)
	}
	m.selected, m.cursor = -1, 0
	m.status = fmt.Sprintf("You rolled %s.", m.dice)
}

func (m Model) commitHumanTurn() (tea.Model, tea.Cmd) {
	pl := game.Play{Moves: slices.Clone(m.made), Result: m.board}
	m.review = ai.ReviewHumanPlay(m.turnStart, m.dice, pl)
	if len(pl.Moves) == 0 {
		m.say("", "You pass.")
	} else {
		m.say("", "You play "+pl.String())
	}
	if w, won := m.board.Winner(); won {
		return m.endGame(w)
	}
	return m.startAITurn(game.RollDice())
}

func (m Model) startAITurn(d game.Dice) (tea.Model, tea.Cmd) {
	m.phase = phaseAIThinking
	m.dice = d
	m.made = nil
	m.selected = -1
	m.status = fmt.Sprintf("%s rolled %s and is thinking…", m.persona.Name, d)
	client, persona, board, review := m.client, m.persona, m.board, m.review
	think := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		return aiDecisionMsg{client.ChooseMove(ctx, persona, board, d, review)}
	}
	return m, tea.Batch(think, m.spinner.Tick)
}

func (m Model) applyAIDecision(dec ai.Decision) (tea.Model, tea.Cmd) {
	if dec.Err != nil {
		m.llmStatus = "offline: " + shortErr(dec.Err)
	} else {
		m.llmStatus = "ready"
	}
	if dec.Reaction != "" {
		m.say(m.persona.Name, dec.Reaction)
	}
	if dec.Comment != "" {
		m.say(m.persona.Name, dec.Comment)
	}
	m.aiPlay = dec.Play
	m.aiQueue = slices.Clone(dec.Play.Moves)
	if len(m.aiQueue) == 0 {
		m.say("", m.persona.Name+" has no legal move and passes.")
	} else {
		m.say("", m.persona.Name+" plays "+playInHumanNumbering(dec.Play))
	}
	m.phase = phaseAIMoving
	m.status = fmt.Sprintf("%s rolled %s.", m.persona.Name, m.dice)
	return m, tea.Tick(aiStepDelay, func(time.Time) tea.Msg { return aiStepMsg{} })
}

func (m Model) stepAI() (tea.Model, tea.Cmd) {
	if len(m.aiQueue) > 0 {
		mv := m.aiQueue[0]
		m.aiQueue = m.aiQueue[1:]
		_, m.board, _ = m.board.TryMove(game.AI, mv.From, mv.Die)
		m.aiLast = [2]int{mv.From, mv.To}
		return m, tea.Tick(aiStepDelay, func(time.Time) tea.Msg { return aiStepMsg{} })
	}
	m.board = m.aiPlay.Result
	m.aiLast = [2]int{-1, -1}
	if w, won := m.board.Winner(); won {
		return m.endGame(w)
	}
	m.phase = phaseHumanRoll
	m.dice = game.Dice{}
	m.status = "Your turn. Press space to roll."
	return m, nil
}

func (m Model) endGame(w game.Player) (tea.Model, tea.Cmd) {
	m.phase = phaseGameOver
	m.winner = w
	m.winKind = m.board.WinType(w)
	m.score[w] += int(m.winKind)
	pts := int(m.winKind)
	if w == game.Human {
		m.status = fmt.Sprintf("You win a %s (%d pt)! n: new game · p: change opponent · q: quit", m.winKind, pts)
	} else {
		m.status = fmt.Sprintf("%s wins a %s (%d pt). n: new game · p: change opponent · q: quit", m.persona.Name, m.winKind, pts)
	}

	client, persona := m.client, m.persona
	aiWon := w == game.AI
	situation := fmt.Sprintf("The backgammon game just ended. You %s by a %s. React to the result.",
		map[bool]string{true: "WON", false: "LOST"}[aiWon], m.winKind)
	react := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		text, err := client.React(ctx, persona, situation)
		if err != nil || text == "" {
			text = ai.FallbackReaction(persona, aiWon)
		}
		return reactionMsg{text}
	}
	return m, react
}

func (m *Model) say(who, text string) {
	m.log = append(m.log, logEntry{who, text})
	if len(m.log) > 50 {
		m.log = m.log[len(m.log)-50:]
	}
}

// playInHumanNumbering renders an AI play using the human's point numbers.
func playInHumanNumbering(pl game.Play) string {
	s := ""
	for i, mv := range pl.Moves {
		if i > 0 {
			s += " "
		}
		s += posLabel(25-mv.From) + "/" + posLabel(25-mv.To)
		if mv.Hit {
			s += "*"
		}
	}
	return s
}

// posLabel names a human-numbered position; the AI's bar (25) maps to 0 and
// its off (0) maps to 25 after mirroring, so both ends are handled here.
func posLabel(r int) string {
	switch r {
	case 0, game.Bar:
		return map[int]string{0: "bar", game.Bar: "off"}[r]
	}
	return fmt.Sprint(r)
}

func shortErr(err error) string {
	s := err.Error()
	if len(s) > 60 {
		s = s[:57] + "…"
	}
	return s
}
