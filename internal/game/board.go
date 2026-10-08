// Package game implements the rules of backgammon: board state, dice,
// legal move generation and win detection.
package game

import "fmt"

// Player identifies one side of the board.
type Player int

const (
	Human Player = iota // moves "white" checkers, home board is points 1-6 in its own numbering
	AI
)

// Opponent returns the other player.
func (p Player) Opponent() Player { return 1 - p }

func (p Player) String() string {
	if p == Human {
		return "Human"
	}
	return "AI"
}

// Special positions in a player-relative position array.
const (
	Off       = 0  // borne off
	Bar       = 25 // on the bar
	Checkers  = 15 // checkers per player
	numPoints = 24
)

// Board holds the checkers of both players. Each player's array is indexed
// in that player's own numbering: index 1..24 are points (the player moves
// from high to low), 0 is borne off and 25 is the bar. A player's point r is
// the opponent's point 25-r.
type Board struct {
	C [2][26]int
}

// NewBoard returns the standard starting position.
func NewBoard() Board {
	var b Board
	for _, p := range []Player{Human, AI} {
		b.C[p][24] = 2
		b.C[p][13] = 5
		b.C[p][8] = 3
		b.C[p][6] = 5
	}
	return b
}

// OpponentAt returns how many of p's opponent's checkers sit on p's point r (1..24).
func (b *Board) OpponentAt(p Player, r int) int {
	return b.C[p.Opponent()][25-r]
}

// AllHome reports whether every checker p has left is in p's home board (points 1-6).
func (b *Board) AllHome(p Player) bool {
	for r := 7; r <= Bar; r++ {
		if b.C[p][r] > 0 {
			return false
		}
	}
	return true
}

// PipCount is the total distance p must travel to bear off every checker.
func (b *Board) PipCount(p Player) int {
	n := 0
	for r := 1; r <= Bar; r++ {
		n += r * b.C[p][r]
	}
	return n
}

// Winner returns the player who has borne off all checkers, if any.
func (b *Board) Winner() (Player, bool) {
	for _, p := range []Player{Human, AI} {
		if b.C[p][Off] == Checkers {
			return p, true
		}
	}
	return 0, false
}

// WinKind describes how big a win was.
type WinKind int

const (
	Single WinKind = iota + 1
	Gammon
	Backgammon
)

func (k WinKind) String() string {
	switch k {
	case Gammon:
		return "gammon"
	case Backgammon:
		return "backgammon"
	}
	return "single game"
}

// WinType classifies winner's victory: a gammon if the loser bore off nothing,
// a backgammon if the loser also still has a checker on the bar or in the
// winner's home board.
func (b *Board) WinType(winner Player) WinKind {
	loser := winner.Opponent()
	if b.C[loser][Off] > 0 {
		return Single
	}
	// Winner's home board (winner points 1-6) is loser points 19-24.
	for r := 19; r <= Bar; r++ {
		if b.C[loser][r] > 0 {
			return Backgammon
		}
	}
	return Gammon
}

// Key is a compact comparable representation of the position.
func (b *Board) Key() [2][26]int { return b.C }

// Validate checks the board invariants: 15 checkers each, and no point
// occupied by both players.
func (b *Board) Validate() error {
	for _, p := range []Player{Human, AI} {
		n := 0
		for r := 0; r <= Bar; r++ {
			if b.C[p][r] < 0 {
				return fmt.Errorf("%v has negative checkers at %d", p, r)
			}
			n += b.C[p][r]
		}
		if n != Checkers {
			return fmt.Errorf("%v has %d checkers, want %d", p, n, Checkers)
		}
	}
	for r := 1; r <= numPoints; r++ {
		if b.C[Human][r] > 0 && b.OpponentAt(Human, r) > 0 {
			return fmt.Errorf("point %d occupied by both players", r)
		}
	}
	return nil
}
