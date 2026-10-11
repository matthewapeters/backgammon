package tui

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/MatthewAPeters/backgammon/internal/ai"
	"github.com/MatthewAPeters/backgammon/internal/game"
	"github.com/MatthewAPeters/backgammon/internal/history"
)

func newWithHistory(t *testing.T, games *history.Store) Model {
	t.Helper()
	return New(ai.NewClient("http://127.0.0.1:0", "x"), nil, games)
}

// playToHumanMove starts a game from the menu and plays until the human is
// mid-turn, with the AI's turns decided by the engine.
func playToHumanMove(t *testing.T, m Model) Model {
	t.Helper()
	m = key(m, "n")
	m = key(m, " ") // opening roll
	for range 10 {
		switch {
		case m.phase == phaseHumanMove && !m.turnDone():
			return m
		case m.phase == phaseHumanMove:
			m, _ = mustModel(m.commitHumanTurn())
		case m.phase == phaseAIThinking:
			m.roll.active = false
			play := game.RankPlays(m.board, game.AI, m.dice)[0]
			m, _ = mustModel(m.applyAIDecision(ai.Decision{Play: play, Comment: "Arr!"}))
			for m.phase == phaseAIMoving {
				m, _ = mustModel(m.stepAI())
			}
		case m.phase == phaseHumanRoll:
			m = key(m, " ")
			m.roll.active = false
			m.startHumanTurn(m.dice)
		default:
			t.Fatalf("unexpected phase %v", m.phase)
		}
	}
	t.Fatal("never reached a human move")
	return m
}

func TestResumeRestoresGameWithSameDice(t *testing.T) {
	games := &history.Store{Path: filepath.Join(t.TempDir(), "history.json")}
	m := newWithHistory(t, games)
	if m.phase != phaseMenu || m.canResume() {
		t.Fatalf("phase %v resumable %v, want a fresh menu", m.phase, m.canResume())
	}
	m = key(m, "down")
	if m.menu != menuPersona {
		t.Fatalf("cursor on %d, want Resume skipped", m.menu)
	}
	m = key(m, "up")

	m = playToHumanMove(t, m)
	board, dice, persona := m.board, m.dice, m.persona
	m = key(m, "enter") // select a checker, then leave mid-turn

	// Start the program again.
	m = newWithHistory(t, games)
	if !m.canResume() || m.menu != menuResume || m.persona.Name != persona.Name {
		t.Fatalf("resumable %v menu %d persona %q", m.canResume(), m.menu, m.persona.Name)
	}
	m = key(m, "enter")
	if m.phase != phaseHumanMove || m.board != board || m.dice != dice || m.selected != -1 {
		t.Fatalf("resumed phase %v dice %v (want %v), board equal %v", m.phase, m.dice, dice, m.board == board)
	}
	if len(m.log) < 2 || m.log[0].text != "You sit down across from the "+persona.Name+"." {
		t.Fatalf("log not restored: %+v", m.log)
	}
}

func TestNewGameAsksBeforeAbandoning(t *testing.T) {
	games := &history.Store{Path: filepath.Join(t.TempDir(), "history.json")}
	m := playToHumanMove(t, newWithHistory(t, games))
	old := m.rec.ID
	m = key(m, "m")
	m = key(m, "n")
	if !m.confirmNew || m.phase != phaseMenu {
		t.Fatal("didn't ask before abandoning the unfinished game")
	}
	m = key(m, "n")
	if m.confirmNew || !m.canResume() || m.rec.ID != old {
		t.Fatal("declining should keep the game")
	}
	m = key(m, "n")
	m = key(m, "y")
	if m.phase != phaseOpening || m.rec.ID == old {
		t.Fatalf("phase %v, want a new game", m.phase)
	}
	saved, err := games.Load()
	if err != nil || len(saved) != 2 || saved[0].Status != history.Abandoned {
		t.Fatalf("saved %d games, first %v, err %v", len(saved), saved[0].Status, err)
	}
}

func TestFinishedGameIsSavedWithReaction(t *testing.T) {
	games := &history.Store{Path: filepath.Join(t.TempDir(), "history.json")}
	m := newWithHistory(t, games)
	m = key(m, "n")
	m.board = game.Board{}
	m.board.C[game.Human][1] = 1
	m.board.C[game.Human][game.Off] = 14
	m.board.C[game.AI][13] = 15
	m.startHumanTurn(game.Dice{1, 1})
	m = key(m, "enter")
	m = key(m, "enter")
	if m.phase != phaseGameOver {
		t.Fatalf("phase %v, want game over", m.phase)
	}
	m, _ = mustModel(m.Update(reactionMsg{"stale", "from another game"}))
	m, _ = mustModel(m.Update(reactionMsg{m.rec.ID, "Well played."}))

	saved, _ := games.Load()
	g := saved[0]
	if g.Status != history.Won || g.WinKind != game.Gammon || g.Log[len(g.Log)-1].Text != "Well played." {
		t.Fatalf("status %v kind %v log %+v", g.Status, g.WinKind, g.Log)
	}
	for _, l := range g.Log {
		if l.Text == "from another game" {
			t.Fatal("a stale reaction was recorded")
		}
	}
	if m = newWithHistory(t, games); m.canResume() {
		t.Fatal("a finished game is offered for resuming")
	}
}

func TestChoosingOpponentReturnsToMenu(t *testing.T) {
	m := newWithHistory(t, nil)
	m = key(m, "o")
	m = key(m, "down")
	m = key(m, "enter")
	if m.phase != phaseMenu || m.persona.Name != ai.Personas[1].Name {
		t.Fatalf("phase %v persona %q", m.phase, m.persona.Name)
	}
}

func TestScreensFitTheFrame(t *testing.T) {
	m := newWithHistory(t, nil)
	m.width, m.height = frameW, 40
	views := map[string]string{"menu": m.viewMenu(), "picker": m.viewPersona(), "new": m.viewNewPersona()}
	for i := range aboutSections {
		m.about.tab = i
		views[aboutSections[i].title] = m.viewAbout()
	}
	for name, v := range views {
		for i, l := range strings.Split(v, "\n") {
			if w := ansi.StringWidth(l); w != frameW {
				t.Errorf("%s line %d is %d wide, want %d: %q", name, i, w, frameW, ansi.Strip(l))
			}
		}
	}
}
