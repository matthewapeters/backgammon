// Package tui is the Bubble Tea front end for the backgammon game.
package tui

import (
	"context"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/MatthewAPeters/backgammon/internal/ai"
	"github.com/MatthewAPeters/backgammon/internal/game"
	"github.com/MatthewAPeters/backgammon/internal/history"
)

type phase int

const (
	phaseMenu       phase = iota
	phaseAbout            // reading about the game
	phasePersona          // choosing an opponent
	phaseNewPersona       // typing a description of a new opponent
	phaseGenerating       // waiting for the LLM to write the new opponent
	phaseOffer            // the opponent proposes stakes; choosing the match length
	phaseOpening
	phaseHumanRoll
	phaseHumanMove
	phaseAIThinking
	phaseAIMoving
	phaseGameOver
)

const (
	aiStepDelay = 700 * time.Millisecond

	// Each die tumbles for a random time in [rollMin, rollMax).
	rollMin   = 500 * time.Millisecond
	rollMax   = 2 * time.Second
	rollFrame = 70 * time.Millisecond
)

type logEntry struct {
	who  string // "" for narration
	text string
}

// Model is the root Bubble Tea model.
type Model struct {
	client  *ai.Client
	persona ai.Persona
	pick    int // persona menu cursor; len(roster) is "new opponent"

	// roster is the built-in personas followed by the player's own.
	roster   []ai.Persona
	store    *ai.PersonaStore // nil means custom personas are not saved
	input    textinput.Model  // new persona description
	genID    int              // ignores results from an abandoned generation
	menuNote string           // message shown on the menu screens

	// Main menu and about page.
	menu       int  // menu cursor
	confirmNew bool // asking whether to abandon the unfinished game
	about      struct{ tab, scroll int }

	// rec is the current game's record, or the unfinished game the menu
	// offers to resume. It is saved to games after every turn.
	rec      *history.Game
	games    *history.Store // nil means games are not saved
	saveNote string         // why the last save failed, if it did

	// match is the current match, nil for a single game. Between games it
	// is the unfinished match the menu offers to resume.
	match    *history.Match
	record   history.Record // results against recordVs
	recordVs string

	// The challenge: the persona's proposed stakes and the chosen length.
	offer struct {
		pick   int // index into matchLengths
		stakes ai.Stakes
		ready  bool // the stakes have been proposed
		id     int  // ignores stakes from an abandoned challenge
	}

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

	// Dice roll animation. While rolling, the dice show roll.faces; each die
	// lands on m.dice when its end time passes.
	roll struct {
		active  bool
		id      int // ignores ticks from a superseded roll
		faces   [2]int
		ends    [2]time.Time
		pending *ai.Decision // AI decision that arrived mid-roll
	}

	opening   [2]int // opening roll: human, AI
	winner    game.Player
	winKind   game.WinKind
	log       []logEntry
	status    string
	llmStatus string
	spinner   spinner.Model
	width     int
	height    int
}

// New returns the initial model, showing the main menu. Custom personas are
// loaded from store, and past games from games; either may be nil to not
// save them. The default opponent is whoever was played last.
func New(client *ai.Client, store *ai.PersonaStore, games *history.Store) Model {
	sp := spinner.New(spinner.WithSpinner(spinner.Dot))
	in := textinput.New()
	in.Placeholder = "e.g. Grumpy Cat, or a retired chess champion who thinks dice are beneath her"
	in.CharLimit = 300
	in.Width = 60
	styleInput(&in)
	m := Model{client: client, spinner: sp, selected: -1, aiLast: [2]int{-1, -1},
		roster: slices.Clone(ai.Personas), store: store, games: games, input: in}
	m.offer.pick = slices.Index(matchLengths, 3)
	if store != nil {
		custom, err := store.Load()
		if err != nil {
			m.menuNote = "Couldn't load your opponents: " + shortErr(err)
		}
		m.roster = append(m.roster, custom...)
	}
	m.persona = m.roster[0]
	if games != nil {
		saved, err := games.Load()
		if err != nil {
			m.menuNote = "Couldn't load your game history: " + shortErr(err)
		}
		matches, _ := games.LoadMatches()
		last, unfinished := history.Latest(saved)
		if last != nil {
			m.persona = last.Persona
		}
		m.rec = unfinished
		m.match = history.LatestMatch(matches)
		if unfinished != nil && (m.match == nil || unfinished.MatchID != m.match.ID) {
			m.match = nil // the unfinished game is a single game
		}
	}
	// Prefer the roster's copy, which may have been edited since.
	if i := slices.IndexFunc(m.roster, func(p ai.Persona) bool { return p.Name == m.persona.Name }); i >= 0 {
		m.pick = i
		m.persona = m.roster[i]
	}
	if m.canResume() {
		m.menu = menuResume
	}
	return m
}

// Messages.
type (
	aiDecisionMsg struct{ dec ai.Decision }
	aiStepMsg     struct{}
	rollTickMsg   struct{ id int }
	reactionMsg   struct{ gameID, text string }
	warmupMsg     struct{ err error }
	personaMsg    struct {
		id  int
		p   ai.Persona
		err error
	}
)

func (m Model) Init() tea.Cmd { return nil }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			return m, tea.Quit
		}
		if m.phase == phaseNewPersona {
			return m.keyNewPersona(msg)
		}
		return m.handleKey(msg.String())

	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		return m, nil

	case spinner.TickMsg:
		if m.phase != phaseAIThinking && m.phase != phaseGenerating && !(m.phase == phaseOffer && !m.offer.ready) {
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

	case rollTickMsg:
		return m.tickRoll(msg.id)

	case aiDecisionMsg:
		if m.roll.active {
			m.roll.pending = &msg.dec
			return m, nil
		}
		return m.applyAIDecision(msg.dec)

	case aiStepMsg:
		return m.stepAI()

	case personaMsg:
		return m.personaCreated(msg)

	case stakesMsg:
		return m.stakesProposed(msg)

	case reactionMsg:
		// The player may have moved on to another game already.
		if m.rec != nil && m.rec.ID == msg.gameID {
			m.say(m.persona.Name, msg.text)
			m.save()
		}
		return m, nil
	}
	return m, nil
}

func (m Model) handleKey(k string) (tea.Model, tea.Cmd) {
	if k == "q" {
		return m, tea.Quit
	}
	if m.roll.active {
		return m, nil
	}
	switch m.phase {
	case phaseMenu:
		return m.keyMenu(k)
	case phaseAbout:
		return m.keyAbout(k)
	case phasePersona:
		return m.keyPersona(k)
	case phaseOffer:
		return m.keyOffer(k)
	case phaseGenerating:
		if k == "esc" {
			m.genID++
			m.phase = phaseNewPersona
			m.menuNote = ""
			return m, m.input.Focus()
		}
	case phaseOpening:
		switch {
		case isConfirm(k):
			return m.finishOpening()
		case k == "m":
			return m.toMenu(), nil
		}
	case phaseHumanRoll:
		switch {
		case isConfirm(k) || k == "r":
			cmd := m.startRoll(game.RollDice())
			m.setPending(history.Human)
			return m, cmd
		case k == "m":
			return m.toMenu(), nil
		}
	case phaseHumanMove:
		if k == "m" {
			// The roll is saved, so resuming restarts this turn with the same dice.
			return m.toMenu(), nil
		}
		return m.keyMove(k)
	case phaseGameOver:
		if m.midMatch() {
			switch k {
			case "n", "enter", " ", "space":
				return m.newGame()
			case "m":
				return m.toMenu(), nil
			}
			return m, nil
		}
		switch k {
		case "n", "enter", " ", "space":
			return m.challenge()
		case "p":
			m.phase = phasePersona
		case "m":
			return m.toMenu(), nil
		}
	}
	return m, nil
}

// toMenu shows the main menu. A game in progress stays saved for resuming.
func (m Model) toMenu() Model {
	m.phase = phaseMenu
	m.confirmNew = false
	m.menuNote = ""
	m.menu = menuNew
	if m.canResume() {
		m.menu = menuResume
	}
	return m
}

// canResume reports whether there is an unfinished game or match to go back to.
func (m Model) canResume() bool { return m.gameUnfinished() || m.midMatch() }

// gameUnfinished reports whether m.rec is a game still being played.
func (m Model) gameUnfinished() bool { return m.rec != nil && m.rec.Status == history.InProgress }

func isConfirm(k string) bool { return k == "enter" || k == " " || k == "space" }

func (m Model) keyPersona(k string) (tea.Model, tea.Cmd) {
	n := len(m.roster) + 1 // the last entry creates a new opponent
	switch k {
	case "up", "k", "shift+tab":
		m.pick = (m.pick + n - 1) % n
	case "down", "j", "tab":
		m.pick = (m.pick + 1) % n
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		if i := int(k[0] - '1'); i < n {
			m.pick = i
		}
	case "n", "+":
		return m.startNewPersona()
	case "enter", " ", "space":
		if m.pick == len(m.roster) {
			return m.startNewPersona()
		}
		m.persona = m.roster[m.pick]
		m = m.toMenu()
		m.menu = menuNew
	case "esc", "m":
		return m.toMenu(), nil
	}
	return m, nil
}

// warmup loads the model in the background so the first move is quick.
func (m *Model) warmup() tea.Cmd {
	if m.llmStatus != "ready" {
		m.llmStatus = "loading model…"
	}
	client := m.client
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		return warmupMsg{client.Warmup(ctx)}
	}
}

// sitDown prepares to play m.persona, loading the player's record against
// it. Without saved games the record only covers this session.
func (m *Model) sitDown() {
	if m.games != nil {
		games, _ := m.games.Load()
		matches, _ := m.games.LoadMatches()
		m.record = history.Tally(games, matches, m.persona.Name)
	} else if m.recordVs != m.persona.Name {
		m.record = history.Record{}
	}
	m.recordVs = m.persona.Name
	m.menuNote = ""
	m.confirmNew = false
}

// resume continues the unfinished game in m.rec where it left off, or
// starts the next game of an unfinished match.
func (m Model) resume() (tea.Model, tea.Cmd) {
	if !m.gameUnfinished() {
		m.persona = m.match.Persona
		m.sitDown()
		warm := m.warmup()
		next, cmd := m.newGame()
		return next, tea.Batch(cmd, warm)
	}
	rec := m.rec
	board, err := rec.Board()
	if err != nil {
		m.menuNote = "Couldn't resume that game: " + shortErr(err)
		return m, nil
	}
	m.persona = rec.Persona
	m.sitDown()
	m.resetTurnState()
	m.board = board
	m.opening = rec.Opening
	m.log = nil
	for _, l := range rec.Log[max(0, len(rec.Log)-maxLog):] {
		m.log = append(m.log, logEntry{l.Who, l.Text})
	}
	m.say("", fmt.Sprintf("You return to your game against the %s.", m.persona.Name))
	m.save()
	warm := m.warmup()

	// The persona reacts to the human's last play, so grade it again.
	if n := len(rec.Turns); n > 0 && rec.Turns[n-1].Side == history.Human {
		t := rec.Turns[n-1]
		if before, err := rec.Replay(n - 1); err == nil {
			m.review = ai.ReviewHumanPlay(before, t.Dice, game.Play{Moves: t.GameMoves(), Result: board})
		}
	}

	switch {
	case rec.Pending != nil && rec.Pending.Side == history.Human:
		m.startHumanTurn(rec.Pending.Dice)
		return m, warm
	case rec.Pending != nil:
		next, think := m.startAITurn(rec.Pending.Dice)
		return next, tea.Batch(think, warm)
	case len(rec.Turns) == 0:
		// The opening roll is the first turn's dice.
		next, cmd := m.finishOpening()
		return next, tea.Batch(cmd, warm)
	case rec.Next() == history.Human:
		m.phase = phaseHumanRoll
		m.dice = game.Dice{}
		m.status = "Your turn. Press space to roll."
		return m, warm
	}
	roll := m.startRoll(game.RollDice())
	m.setPending(history.AI)
	next, think := m.startAITurn(m.dice)
	return next, tea.Batch(think, roll, warm)
}

func (m Model) startNewPersona() (tea.Model, tea.Cmd) {
	m.pick = len(m.roster)
	m.phase = phaseNewPersona
	m.menuNote = ""
	m.input.Reset()
	return m, m.input.Focus()
}

func (m Model) keyNewPersona(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.input.Blur()
		m.phase = phasePersona
		m.menuNote = ""
		return m, nil
	case "enter":
		desc := strings.TrimSpace(m.input.Value())
		if desc == "" {
			return m, nil
		}
		m.input.Blur()
		m.phase = phaseGenerating
		m.menuNote = ""
		m.genID++
		client, id := m.client, m.genID
		gen := func() tea.Msg {
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
			defer cancel()
			p, err := client.GeneratePersona(ctx, desc)
			return personaMsg{id, p, err}
		}
		return m, tea.Batch(gen, m.spinner.Tick)
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

// personaCreated adds a freshly generated persona to the roster and saves it.
func (m Model) personaCreated(msg personaMsg) (tea.Model, tea.Cmd) {
	if msg.id != m.genID || m.phase != phaseGenerating {
		return m, nil
	}
	if msg.err != nil {
		// Back to the prompt with the text intact so the player can retry.
		m.phase = phaseNewPersona
		m.menuNote = "Couldn't create that opponent: " + shortErr(msg.err)
		return m, m.input.Focus()
	}
	m.roster = append(m.roster, msg.p)
	m.pick = len(m.roster) - 1
	m.phase = phasePersona
	m.menuNote = "New opponent: " + msg.p.Name
	if m.store != nil {
		if err := m.store.Add(msg.p); err != nil {
			m.menuNote = "New opponent for this session only; couldn't save: " + shortErr(err)
		}
	}
	return m, nil
}

func (m *Model) resetTurnState() {
	m.roll.active = false
	m.review = nil
	m.made = nil
	m.plays = nil
	m.selected = -1
	m.aiLast = [2]int{-1, -1}
	m.dice = game.Dice{}
}

func (m Model) newGame() (tea.Model, tea.Cmd) {
	m.board = game.NewBoard()
	m.resetTurnState()
	for {
		m.opening = [2]int{game.RollDie(), game.RollDie()}
		if m.opening[0] != m.opening[1] {
			break
		}
	}
	m.rec = history.NewGame(m.persona, m.opening, time.Now())
	if m.match != nil {
		m.match.Add(m.rec)
		m.saveMatch()
	}
	m.log = nil
	m.say("", fmt.Sprintf("You sit down across from the %s.", m.persona.Name))
	if m.match != nil {
		m.announceMatch()
	}
	m.save()
	m.phase = phaseOpening
	m.status = m.openingStatus()
	return m, nil
}

func (m Model) openingStatus() string {
	first := "You go first"
	if m.opening[1] > m.opening[0] {
		first = m.persona.Name + " goes first"
	}
	return fmt.Sprintf("Opening roll: you %d, %s %d. %s! (space)", m.opening[0], m.persona.Name, m.opening[1], first)
}

func (m Model) finishOpening() (tea.Model, tea.Cmd) {
	if m.opening[0] > m.opening[1] {
		m.startHumanTurn(game.Dice{m.opening[0], m.opening[1]})
		return m, nil
	}
	return m.startAITurn(game.Dice{m.opening[1], m.opening[0]})
}

// startRoll sets m.dice to d and starts tumbling the dice towards it.
func (m *Model) startRoll(d game.Dice) tea.Cmd {
	m.dice = d
	now := time.Now()
	for i := range m.roll.ends {
		m.roll.faces[i] = game.RollDie()
		m.roll.ends[i] = now.Add(rollMin + rand.N(rollMax-rollMin))
	}
	m.roll.active = true
	m.roll.pending = nil
	m.roll.id++
	return rollTick(m.roll.id)
}

func rollTick(id int) tea.Cmd {
	return tea.Tick(rollFrame, func(time.Time) tea.Msg { return rollTickMsg{id} })
}

// tickRoll advances the roll animation and, once both dice have landed,
// continues the turn the roll was for.
func (m Model) tickRoll(id int) (tea.Model, tea.Cmd) {
	if !m.roll.active || id != m.roll.id {
		return m, nil
	}
	now := time.Now()
	settled := true
	for i, end := range m.roll.ends {
		if now.Before(end) {
			settled = false
			// Always change face so the tumble is visible.
			m.roll.faces[i] = (m.roll.faces[i]+rand.IntN(5))%6 + 1
		}
	}
	if !settled {
		return m, rollTick(id)
	}
	m.roll.active = false
	switch {
	case m.phase == phaseHumanRoll:
		m.startHumanTurn(m.dice)
	case m.roll.pending != nil:
		dec := *m.roll.pending
		m.roll.pending = nil
		return m.applyAIDecision(dec)
	}
	return m, nil
}

// dieFace is what die i currently shows, mid-roll or not.
func (m Model) dieFace(i int) int {
	if m.roll.active && time.Now().Before(m.roll.ends[i]) {
		return m.roll.faces[i]
	}
	return m.dice[i]
}

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
	m.recordTurn(history.Human, pl.Moves)
	if w, won := m.board.Winner(); won {
		return m.endGame(w)
	}
	roll := m.startRoll(game.RollDice())
	m.setPending(history.AI)
	next, think := m.startAITurn(m.dice)
	return next, tea.Batch(think, roll)
}

func (m Model) startAITurn(d game.Dice) (tea.Model, tea.Cmd) {
	m.phase = phaseAIThinking
	m.dice = d
	m.made = nil
	m.selected = -1
	m.status = fmt.Sprintf("%s rolled %s and is thinking…", m.persona.Name, d)
	client, persona, board, review := m.client, m.voiced(), m.board, m.review
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
	m.recordTurn(history.AI, dec.Play.Moves)
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
	m.record.Games[w]++
	if m.rec != nil {
		m.rec.Finish(w, m.winKind)
		m.save()
	}
	matchOver := false
	if m.match != nil && m.rec != nil {
		matchOver = m.match.Record(m.rec)
		if matchOver {
			m.record.Matches[w]++
		}
		m.saveMatch()
	}

	pts := int(m.winKind)
	aiWon := w == game.AI
	result := fmt.Sprintf("You win a %s (%d pt)!", m.winKind, pts)
	if aiWon {
		result = fmt.Sprintf("%s wins a %s (%d pt).", m.persona.Name, m.winKind, pts)
	}
	situation := fmt.Sprintf("The backgammon game just ended. You %s by a %s. React to the result.",
		map[bool]string{true: "WON", false: "LOST"}[aiWon], m.winKind)
	switch {
	case matchOver:
		situation = m.matchResult(w)
		result = fmt.Sprintf("You win the match %d–%d!", m.match.Score[game.Human], m.match.Score[game.AI])
		if aiWon {
			result = fmt.Sprintf("%s wins the match %d–%d.", m.persona.Name, m.match.Score[game.AI], m.match.Score[game.Human])
		}
		m.status = result + " n: new game · p: change opponent · m: menu"
	case m.midMatch():
		situation = fmt.Sprintf("You just %s a game of your %d-point backgammon match by a %s, worth %d. "+
			"The match score is now you %d, the player %d. React, and needle the player about the stakes.",
			map[bool]string{true: "WON", false: "LOST"}[aiWon], m.match.Length, m.winKind, pts,
			m.match.Score[game.AI], m.match.Score[game.Human])
		m.status = fmt.Sprintf("%s You %d, %s %d, to %d. space: next game · m: menu",
			result, m.match.Score[game.Human], shortName(m.persona), m.match.Score[game.AI], m.match.Length)
	default:
		m.status = result + " n: new game · p: change opponent · m: menu"
	}

	client, persona := m.client, m.persona
	var id string
	if m.rec != nil {
		id = m.rec.ID
	}
	react := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		text, err := client.React(ctx, persona, situation)
		if err != nil || text == "" {
			text = ai.FallbackReaction(persona, aiWon)
		}
		return reactionMsg{id, text}
	}
	return m, react
}

// maxLog is how much of the chat log is kept on screen; the record keeps it all.
const maxLog = 50

func (m *Model) say(who, text string) {
	m.log = append(m.log, logEntry{who, text})
	if len(m.log) > maxLog {
		m.log = m.log[len(m.log)-maxLog:]
	}
	if m.rec != nil {
		m.rec.Say(who, text)
	}
}

// recordTurn saves the turn just played with m.dice.
func (m *Model) recordTurn(s history.Side, moves []game.Move) {
	if m.rec != nil {
		m.rec.AddTurn(s, m.dice, moves)
		m.save()
	}
}

// setPending saves m.dice as rolled for s's turn, so quitting and resuming
// can't reroll them.
func (m *Model) setPending(s history.Side) {
	if m.rec != nil {
		m.rec.Pending = &history.Roll{Side: s, Dice: m.dice}
		m.save()
	}
}

// save writes the game record, noting any failure on screen rather than
// interrupting play.
func (m *Model) save() {
	if m.games == nil || m.rec == nil {
		return
	}
	m.rec.Updated = time.Now()
	m.saveNote = ""
	if err := m.games.Save(m.rec); err != nil {
		m.saveNote = "Not saving games: " + shortErr(err)
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
