package game

import (
	"math"
	"math/rand/v2"
	"sort"
)

// shots[d] is the number of the 36 rolls that hit a blot at distance d.
var shots = [25]int{0, 11, 12, 14, 15, 15, 17, 6, 6, 5, 3, 2, 3, 0, 0, 1, 1, 0, 1, 0, 1, 0, 0, 0, 1}

// pointWeight is the value of holding each point (index 1..7) while the
// checkers are still in contact; points 8 and up are worth 1. Deep points
// are worth little early on: checkers there are out of play.
var pointWeight = [8]float64{0, 0.5, 1, 2, 3.5, 3.5, 3, 2.5}

// Weights of the race-mode gammon terms, in Evaluate units: roughly what a
// whole point of match score is worth next to the pip and point terms.
const (
	gammonWeight     = 20
	backgammonWeight = 20
)

// avgRoll is the average number of pips a roll moves, doubles included.
const avgRoll = 8.17

// Evaluate scores a position from p's point of view; higher is better.
// It is a simple hand-tuned heuristic, not a strong player, used as the
// fallback opponent and to shortlist candidate plays for the LLM.
func Evaluate(b Board, p Player) float64 {
	o := p.Opponent()
	me, them := &b.C[p], &b.C[o]
	score := float64(b.PipCount(o)-b.PipCount(p)) * 0.4

	score += float64(me[Off]) * 1.5
	score += float64(them[Bar]) * 4

	if !contactExists(b, p) {
		// A pure race: points and blots no longer matter, but the size of
		// the loss does.
		return score - raceRisk(b, p) + raceRisk(b, o)
	}

	run := 0
	for r := 1; r <= numPoints; r++ {
		n := me[r]
		switch {
		case n >= 2:
			w := 1.0
			if r < len(pointWeight) {
				w = pointWeight[r]
			}
			// With the opponent on the bar every home point blocks an
			// entry, so deep points are worth as much as any other.
			if r <= 6 && them[Bar] > 0 {
				w = max(w, 3)
			}
			score += w
			score -= float64(max(0, n-3)) * 0.3
			if r <= 3 && them[Bar] == 0 {
				score -= float64(n-2) * 1.0 // buried checkers
			}
			run++
			if run >= 3 {
				score += float64(run) * 0.8
			}
		case n == 1:
			score -= blotRisk(b, p, r) * (6 + float64(25-r)/3)
			run = 0
		default:
			run = 0
		}
	}
	score -= trapped(b, p)
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

// trapped penalizes p's back checkers (on the bar or in the opponent's home
// board) for the opponent points blocking the six points in front of them.
// The penalty grows with the square of the blockade, so a near-prime is far
// worse than a scattered point or two.
func trapped(b Board, p Player) float64 {
	pen := 0.0
	for r := 19; r <= Bar; r++ {
		n := b.C[p][r]
		if n == 0 {
			continue
		}
		blocks := 0
		for x := r - 6; x < r; x++ {
			if b.OpponentAt(p, x) >= 2 {
				blocks++
			}
		}
		pen += float64(n) * 1.0 * float64(blocks*blocks)
	}
	return pen
}

// raceRisk is the expected cost to p, in Evaluate units, of losing a gammon
// or backgammon in a no-contact race where the opponent is to roll next.
func raceRisk(b Board, p Player) float64 {
	me := &b.C[p]
	if me[Off] > 0 {
		return 0
	}
	o := p.Opponent()
	theirTurns := bearOffTurns(b, o)

	// To save the gammon p must bring every checker home and then bear one
	// off; to save the backgammon, get every checker out of the opponent's
	// home board.
	home, escape := 0, 0
	for r := 7; r <= Bar; r++ {
		home += (r - 6) * me[r]
		if r >= 19 {
			escape += (r - 18) * me[r]
		}
	}
	gammonTurns := float64(home)/avgRoll + 1
	risk := gammonWeight * lossChance(gammonTurns, theirTurns)
	if escape > 0 {
		risk += backgammonWeight * lossChance(float64(escape)/avgRoll, theirTurns)
	}
	return risk
}

// bearOffTurns estimates how many turns p needs to bear off everything.
func bearOffTurns(b Board, p Player) float64 {
	left := Checkers - b.C[p][Off]
	return max(float64(b.PipCount(p))/avgRoll+0.5, float64(left)/2)
}

// lossChance is the rough probability that a side needing need turns fails
// to finish before an opponent who needs have turns and rolls first.
func lossChance(need, have float64) float64 {
	// p rolls second, so it gets have-1 turns before the opponent finishes.
	return 1 / (1 + math.Exp(-2*(need-(have-1))))
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
