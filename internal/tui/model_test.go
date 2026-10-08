package tui

import (
	"testing"

	"github.com/MatthewAPeters/backgammon/internal/ai"
	"github.com/MatthewAPeters/backgammon/internal/game"
)

func key(m Model, k string) Model {
	next, _ := m.handleKey(k)
	return next.(Model)
}

func TestHumanBearsOffLastCheckerAndWins(t *testing.T) {
	m := New(ai.NewClient("http://127.0.0.1:0", "x"))
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
	m := New(ai.NewClient("http://127.0.0.1:0", "x"))
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
	m := New(ai.NewClient("http://127.0.0.1:0", "x"))
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
