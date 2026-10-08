package game

import (
	"math/rand/v2"
	"sort"
)

// shots[d] is the number of the 36 rolls that hit a blot at distance d.
var shots = [25]int{0, 11, 12, 14, 15, 15, 17, 6, 6, 5, 3, 2, 3, 0, 0, 1, 1, 0, 1, 0, 1, 0, 0, 0, 1}

// Evaluate scores a position from p's point of view; higher is better.
// It is a simple hand-tuned heuristic, not a strong player, used as the
// fallback opponent and to shortlist candidate plays for the LLM.
func Evaluate(b Board, p Player) float64 {
	o := p.Opponent()
	me, them := &b.C[p], &b.C[o]
	score := float64(b.PipCount(o)-b.PipCount(p)) * 0.4

	score += float64(me[Off]) * 1.5
	score += float64(them[Bar]) * 4

	contact := contactExists(b, p)
	run := 0
	for r := 1; r <= numPoints; r++ {
		n := me[r]
		switch {
		case n >= 2:
			w := 1.0
			switch {
			case r == 5 || r == 4:
				w = 3.5
			case r <= 6 || r == 7:
				w = 2.5
			}
			score += w
			score -= float64(max(0, n-3)) * 0.3
			run++
			if run >= 3 {
				score += float64(run) * 0.8
			}
		case n == 1 && contact:
			score -= blotRisk(b, p, r) * (6 + float64(25-r)/3)
			run = 0
		default:
			run = 0
		}
	}
	return score
}

// contactExists reports whether any of p's checkers could still be hit.
func contactExists(b Board, p Player) bool {
	// The opponent's rearmost checker, in p's numbering, is the lowest point
	// it occupies; p's frontmost checker is the highest point p occupies.
	o := p.Opponent()
	if b.C[o][Bar] > 0 {
		return true
	}
	oppBack := 25
	for r := 1; r <= numPoints; r++ {
		if b.OpponentAt(p, r) > 0 {
			oppBack = r
			break
		}
	}
	for r := Bar; r >= 1; r-- {
		if b.C[p][r] > 0 {
			return r > oppBack
		}
	}
	return false
}

// blotRisk estimates the probability that p's blot on point r gets hit.
// Opponent checkers move toward p's higher-numbered points, so attackers sit
// on lower points (or on their bar, at distance r).
func blotRisk(b Board, p Player, r int) float64 {
	hits := 0
	if b.C[p.Opponent()][Bar] > 0 && r < len(shots) {
		hits += shots[r]
	}
	for x := 1; x < r; x++ {
		if b.OpponentAt(p, x) > 0 && r-x < len(shots) {
			hits += shots[r-x]
		}
	}
	return float64(min(hits, 36)) / 36
}

// RankPlays returns the distinct plays sorted best-first by Evaluate.
func RankPlays(b Board, p Player, d Dice) []Play {
	plays := DistinctPlays(b.LegalPlays(p, d))
	type scored struct {
		play  Play
		score float64
	}
	s := make([]scored, len(plays))
	for i, pl := range plays {
		s[i] = scored{pl, Evaluate(pl.Result, p)}
	}
	sort.SliceStable(s, func(i, j int) bool { return s[i].score > s[j].score })
	for i := range s {
		plays[i] = s[i].play
	}
	return plays
}

// RollDice rolls two dice.
func RollDice() Dice { return Dice{rand.IntN(6) + 1, rand.IntN(6) + 1} }

// RollDie rolls one die.
func RollDie() int { return rand.IntN(6) + 1 }
