package game

import (
	"fmt"
	"strings"
)

// Move is a single checker movement in the mover's own numbering.
type Move struct {
	From, To int // 25 = bar, 0 = off
	Die      int
	Hit      bool
}

func (m Move) String() string {
	from, to := fmt.Sprint(m.From), fmt.Sprint(m.To)
	if m.From == Bar {
		from = "bar"
	}
	if m.To == Off {
		to = "off"
	}
	s := from + "/" + to
	if m.Hit {
		s += "*"
	}
	return s
}

// Play is a complete turn: a sequence of moves and the position it produces.
type Play struct {
	Moves  []Move
	Result Board
}

func (pl Play) String() string {
	if len(pl.Moves) == 0 {
		return "(no move)"
	}
	parts := make([]string, len(pl.Moves))
	for i, m := range pl.Moves {
		parts[i] = m.String()
	}
	return strings.Join(parts, " ")
}

// Dice is a roll of two dice.
type Dice [2]int

func (d Dice) IsDouble() bool { return d[0] == d[1] }

func (d Dice) String() string { return fmt.Sprintf("%d-%d", d[0], d[1]) }

// TryMove attempts to move one of p's checkers from `from` using die.
// It returns the resulting move and board, or ok=false if illegal.
// It does not enforce the "use as many dice as possible" rule; that is the
// job of LegalPlays.
func (b Board) TryMove(p Player, from, die int) (Move, Board, bool) {
	if from < 1 || from > Bar || b.C[p][from] == 0 {
		return Move{}, b, false
	}
	if b.C[p][Bar] > 0 && from != Bar {
		return Move{}, b, false // must enter from the bar first
	}
	to := from - die
	if to <= 0 {
		if !b.AllHome(p) {
			return Move{}, b, false
		}
		if to < 0 {
			// Overshooting is only allowed from the highest occupied point.
			for r := from + 1; r <= 6; r++ {
				if b.C[p][r] > 0 {
					return Move{}, b, false
				}
			}
		}
		to = Off
	}
	m := Move{From: from, To: to, Die: die}
	if to != Off {
		opp := b.OpponentAt(p, to)
		if opp >= 2 {
			return Move{}, b, false
		}
		if opp == 1 {
			m.Hit = true
			b.C[p.Opponent()][25-to] = 0
			b.C[p.Opponent()][Bar]++
		}
	}
	b.C[p][from]--
	b.C[p][to]++
	return m, b, true
}

// LegalPlays returns every legal way for p to play the roll, enforcing that
// as many dice as possible are used and, if only one die of a non-double can
// be used, that the larger one is used when possible. Plays reached by
// different move orders are all included (so a player's partial moves can be
// matched against them); use DistinctPlays to collapse them by result.
// If no move is possible it returns a single empty play.
func (b Board) LegalPlays(p Player, d Dice) []Play {
	var orders [][]int
	if d.IsDouble() {
		orders = [][]int{{d[0], d[0], d[0], d[0]}}
	} else {
		orders = [][]int{{d[0], d[1]}, {d[1], d[0]}}
	}

	var all []Play
	for _, dice := range orders {
		gen(b, p, dice, nil, &all)
	}

	maxLen := 0
	for _, pl := range all {
		maxLen = max(maxLen, len(pl.Moves))
	}
	if maxLen == 0 {
		return []Play{{Result: b}}
	}

	var plays []Play
	for _, pl := range all {
		if len(pl.Moves) == maxLen {
			plays = append(plays, pl)
		}
	}

	if maxLen == 1 && !d.IsDouble() {
		hi := max(d[0], d[1])
		var big []Play
		for _, pl := range plays {
			if pl.Moves[0].Die == hi {
				big = append(big, pl)
			}
		}
		if len(big) > 0 {
			plays = big
		}
	}

	// The same move sequence can come from both die orders only when it is
	// a single move; drop exact duplicates.
	seen := map[string]bool{}
	out := plays[:0]
	for _, pl := range plays {
		k := fmt.Sprint(pl.Moves)
		if !seen[k] {
			seen[k] = true
			out = append(out, pl)
		}
	}
	return out
}

func gen(b Board, p Player, dice []int, sofar []Move, out *[]Play) {
	if len(dice) == 0 {
		*out = append(*out, Play{Moves: sofar, Result: b})
		return
	}
	moved := false
	for from := Bar; from >= 1; from-- {
		if b.C[p][from] == 0 {
			continue
		}
		m, nb, ok := b.TryMove(p, from, dice[0])
		if !ok {
			continue
		}
		moved = true
		next := append(append([]Move(nil), sofar...), m)
		gen(nb, p, dice[1:], next, out)
	}
	if !moved {
		*out = append(*out, Play{Moves: sofar, Result: b})
	}
}

// DistinctPlays collapses plays that lead to the same position, keeping the
// first sequence found for each.
func DistinctPlays(plays []Play) []Play {
	seen := map[[2][26]int]bool{}
	var out []Play
	for _, pl := range plays {
		k := pl.Result.Key()
		if !seen[k] {
			seen[k] = true
			out = append(out, pl)
		}
	}
	return out
}

// NextMoves returns the single moves that can legally follow the moves
// already made this turn, given the turn's full list of legal plays.
// Moves are deduplicated by From/To; among duplicates the smallest die wins.
func NextMoves(plays []Play, made []Move) []Move {
	var out []Move
	idx := map[[2]int]int{}
	for _, pl := range plays {
		if len(pl.Moves) <= len(made) || !prefixMatches(pl.Moves, made) {
			continue
		}
		m := pl.Moves[len(made)]
		k := [2]int{m.From, m.To}
		if i, ok := idx[k]; ok {
			if m.Die < out[i].Die {
				out[i] = m
			}
			continue
		}
		idx[k] = len(out)
		out = append(out, m)
	}
	return out
}

func prefixMatches(moves, prefix []Move) bool {
	for i, m := range prefix {
		if moves[i].From != m.From || moves[i].To != m.To || moves[i].Die != m.Die {
			return false
		}
	}
	return true
}
