package tui

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/MatthewAPeters/backgammon/internal/ai"
	"github.com/MatthewAPeters/backgammon/internal/game"
)

func key(m Model, k string) Model {
	next, _ := m.handleKey(k)
	return next.(Model)
}

func TestHumanBearsOffLastCheckerAndWins(t *testing.T) {
	m := New(ai.NewClient("http://127.0.0.1:0", "x"), nil)
	m.persona = ai.Personas[0]
	m.board = game.Board{}
	m.board.C[game.Human][1] = 1
	m.board.C[game.Human][game.Off] = 14
	m.board.C[game.AI][13] = 15
	m.startHumanTurn(game.Dice{1, 1})

	m = key(m, "enter") // select the checker on 1
	m = key(m, "enter") // bear it off
	if m.phase != phaseGameOver || m.winner != game.Human {
		t.Fatalf("phase %v winner %v, want game over with human winner", m.phase, m.winner)
	}
	if m.winKind != game.Gammon || m.score[game.Human] != 2 {
		t.Fatalf("win %v score %v, want gammon worth 2", m.winKind, m.score)
	}
}

func TestUndoRestoresBoard(t *testing.T) {
	m := New(ai.NewClient("http://127.0.0.1:0", "x"), nil)
	m.board = game.NewBoard()
	m.startHumanTurn(game.Dice{6, 1})
	start := m.board
	m = key(m, "enter")
	m = key(m, "enter")
	if m.board == start || len(m.made) != 1 {
		t.Fatal("move was not made")
	}
	m = key(m, "u")
	if m.board != start || len(m.made) != 0 {
		t.Fatal("undo did not restore the board")
	}
}

func TestAIAnimationReachesResult(t *testing.T) {
	m := New(ai.NewClient("http://127.0.0.1:0", "x"), nil)
	m.persona = ai.Personas[1]
	m.board = game.NewBoard()
	m.dice = game.Dice{4, 4}
	play := game.RankPlays(m.board, game.AI, m.dice)[0]
	next, _ := m.applyAIDecision(ai.Decision{Play: play, Comment: "Har!"})
	m = next.(Model)
	for range len(play.Moves) + 1 {
		next, _ = m.stepAI()
		m = next.(Model)
	}
	if m.board != play.Result || m.phase != phaseHumanRoll {
		t.Fatalf("board/phase after animation: phase=%v", m.phase)
	}
}

func TestRollAnimationBlocksKeysThenStartsTurn(t *testing.T) {
	m := New(ai.NewClient("http://127.0.0.1:0", "x"), nil)
	m.persona = ai.Personas[0]
	m.board = game.NewBoard()
	m.phase = phaseHumanRoll
	m = key(m, " ")
	if !m.roll.active || m.phase != phaseHumanRoll {
		t.Fatalf("active %v phase %v, want rolling before the turn starts", m.roll.active, m.phase)
	}
	if m = key(m, " "); m.phase != phaseHumanRoll {
		t.Fatalf("key during roll changed phase to %v", m.phase)
	}
	m.roll.ends = [2]time.Time{} // let both dice land
	next, _ := m.tickRoll(m.roll.id)
	m = next.(Model)
	if m.roll.active || m.phase != phaseHumanMove || len(m.plays) == 0 {
		t.Fatalf("active %v phase %v plays %d, want human move after roll", m.roll.active, m.phase, len(m.plays))
	}
}

func TestNewPersonaIsAddedAndSaved(t *testing.T) {
	store := &ai.PersonaStore{Path: filepath.Join(t.TempDir(), "personas.json")}
	m := New(ai.NewClient("http://127.0.0.1:0", "x"), store)
	n := len(ai.Personas)
	if len(m.roster) != n {
		t.Fatalf("roster has %d personas, want %d", len(m.roster), n)
	}

	m = key(m, "up") // wraps to "+ New opponent…"
	m = key(m, "enter")
	if m.phase != phaseNewPersona {
		t.Fatalf("phase %v, want new persona", m.phase)
	}
	// q is text here, not quit.
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	m = next.(Model)
	if m.input.Value() != "q" {
		t.Fatalf("input %q, want q", m.input.Value())
	}
	next, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = next.(Model)
	if m.phase != phaseGenerating {
		t.Fatalf("phase %v, want generating", m.phase)
	}

	p := ai.Persona{Name: "Queen", Voice: "You are a queen."}
	next, _ = m.Update(personaMsg{id: m.genID, p: p})
	m = next.(Model)
	if m.phase != phasePersona || m.pick != n || m.roster[n].Name != "Queen" {
		t.Fatalf("phase %v pick %d roster %v", m.phase, m.pick, m.roster)
	}
	saved, err := store.Load()
	if err != nil || len(saved) != 1 || saved[0].Name != "Queen" {
		t.Fatalf("saved %v, %v", saved, err)
	}
	if again := New(ai.NewClient("http://127.0.0.1:0", "x"), store); len(again.roster) != n+1 {
		t.Fatalf("reloaded roster has %d personas, want %d", len(again.roster), n+1)
	}
}

func TestNewPersonaFailureKeepsText(t *testing.T) {
	m := New(ai.NewClient("http://127.0.0.1:0", "x"), nil)
	m, _ = mustModel(m.startNewPersona())
	m.input.SetValue("a pirate")
	m, _ = mustModel(m.Update(tea.KeyMsg{Type: tea.KeyEnter}))
	m, _ = mustModel(m.Update(personaMsg{id: m.genID, err: errors.New("offline")}))
	if m.phase != phaseNewPersona || m.input.Value() != "a pirate" || m.menuNote == "" {
		t.Fatalf("phase %v input %q note %q", m.phase, m.input.Value(), m.menuNote)
	}
}

func mustModel(next tea.Model, cmd tea.Cmd) (Model, tea.Cmd) { return next.(Model), cmd }
