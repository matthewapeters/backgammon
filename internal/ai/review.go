package ai

import (
	"fmt"
	"strings"

	"github.com/MatthewAPeters/backgammon/internal/game"
)

// Verdict is the engine's grade for a human play.
type Verdict string

const (
	VerdictForced       Verdict = "forced"       // only one legal play (or none)
	VerdictBest         Verdict = "best"         // the engine's top choice
	VerdictSolid        Verdict = "solid"        // close to the best
	VerdictQuestionable Verdict = "questionable" // noticeably worse than the best
	VerdictBlunder      Verdict = "blunder"      // far worse than the best
)

// Score gaps (in Evaluate units) separating the verdicts, calibrated on
// simulated games: the second-best play trails the best by under 4.2 three
// times in four, while a random legal play trails by about 7 at the median.
const (
	solidGap   = 3.0
	blunderGap = 8.0
)

// Review is the engine's assessment of the human's last play, given to the
// AI so its persona can taunt or fret. The heuristic is far from perfect,
// so the persona will sometimes misjudge a move; that is part of the fun.
type Review struct {
	Dice    game.Dice
	Play    game.Play // as made, in the human's numbering
	Best    game.Play // the engine's preferred play
	Rank    int       // 1-based rank of Play among distinct plays
	Total   int       // number of distinct plays
	Gap     float64   // Evaluate(best) - Evaluate(play), from the human's side
	Verdict Verdict
	Facts   []string // plain-language notes, from the AI's point of view
}

// ReviewHumanPlay grades the human's play pl, made with dice d from before.
func ReviewHumanPlay(before game.Board, d game.Dice, pl game.Play) *Review {
	ranked := game.RankPlays(before, game.Human, d)
	r := &Review{Dice: d, Play: pl, Best: ranked[0], Rank: 1, Total: len(ranked)}
	for i, cand := range ranked {
		if cand.Result == pl.Result {
			r.Rank = i + 1
			break
		}
	}
	r.Gap = game.Evaluate(ranked[0].Result, game.Human) - game.Evaluate(pl.Result, game.Human)
	switch {
	case r.Total <= 1:
		r.Verdict = VerdictForced
	case r.Rank == 1:
		r.Verdict = VerdictBest
	case r.Gap < solidGap:
		r.Verdict = VerdictSolid
	case r.Gap < blunderGap:
		r.Verdict = VerdictQuestionable
	default:
		r.Verdict = VerdictBlunder
	}
	r.Facts = playFacts(before, pl)
	return r
}

// playFacts describes notable features of the human's play, addressed to the AI.
func playFacts(before game.Board, pl game.Play) []string {
	var facts []string
	after := pl.Result
	if hits := after.C[game.AI][game.Bar] - before.C[game.AI][game.Bar]; hits > 0 {
		facts = append(facts, fmt.Sprintf("they hit %d of your checkers, sending them to the bar", hits))
	}
	var made []string
	for r := 1; r <= 24; r++ {
		if before.C[game.Human][r] < 2 && after.C[game.Human][r] >= 2 {
			made = append(made, fmt.Sprintf("their %d-point (your %d)", r, 25-r))
		}
	}
	if len(made) > 0 {
		facts = append(facts, "they made "+strings.Join(made, " and "))
	}
	blots := 0
	for r := 1; r <= 24; r++ {
		if after.C[game.Human][r] == 1 {
			blots++
		}
	}
	if blots > 0 {
		facts = append(facts, fmt.Sprintf("they now have %d blot(s) you might hit", blots))
	}
	if off := after.C[game.Human][game.Off] - before.C[game.Human][game.Off]; off > 0 {
		facts = append(facts, fmt.Sprintf("they bore off %d", off))
	}
	return facts
}

// prompt renders the review for the AI's user message.
func (r *Review) prompt() string {
	var b strings.Builder
	if len(r.Play.Moves) == 0 {
		fmt.Fprintf(&b, "Your opponent rolled %s and could not move at all.\n", r.Dice)
		return b.String()
	}
	fmt.Fprintf(&b, "Your opponent rolled %s and played: %s (in your numbering).\n", r.Dice, describeOpponentPlay(r.Play))
	switch r.Verdict {
	case VerdictForced:
		b.WriteString("Their move was forced; they had no real choice.\n")
	case VerdictBest:
		fmt.Fprintf(&b, "Your gut says that was the best of their %d options, exactly what you would have played. A strong move.\n", r.Total)
	default:
		fmt.Fprintf(&b, "Your gut ranks it #%d of their %d options; you would have played %s instead. Verdict: %s.\n",
			r.Rank, r.Total, describeOpponentPlay(r.Best), r.Verdict)
	}
	for _, f := range r.Facts {
		b.WriteString("- " + f + "\n")
	}
	return b.String()
}

// reactionGuidance tells the persona how to pitch its reaction.
func (r *Review) reactionGuidance() string {
	switch r.Verdict {
	case VerdictBlunder, VerdictQuestionable:
		return "They made a mistake (as far as you can tell): gloat, taunt or tease them about it."
	case VerdictBest:
		return "They played well: show worry, grudging respect, or bluster to hide your nerves."
	case VerdictForced:
		return "Their move was forced: comment on their luck or the dice, good or bad."
	}
	return "They played a reasonable move: react however suits your character."
}
