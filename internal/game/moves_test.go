package game

import (
	"math/rand/v2"
	"slices"
	"testing"
)

// mkBoard builds a position from per-player point->count maps (in each
// player's own numbering); unplaced checkers are treated as borne off.
func mkBoard(human, ai map[int]int) Board {
	var b Board
	for p, m := range []map[int]int{human, ai} {
		n := 0
		for r, c := range m {
			b.C[p][r] = c
			n += c
		}
		b.C[p][Off] += Checkers - n
	}
	return b
}

func playStrings(plays []Play) []string {
	var out []string
	for _, pl := range plays {
		out = append(out, pl.String())
	}
	return out
}

func TestStartingPosition(t *testing.T) {
	b := NewBoard()
	if err := b.Validate(); err != nil {
		t.Fatal(err)
	}
	if b.PipCount(Human) != 167 || b.PipCount(AI) != 167 {
		t.Fatalf("pip counts %d/%d, want 167", b.PipCount(Human), b.PipCount(AI))
	}
}

func TestOpening31IncludesMakingFivePoint(t *testing.T) {
	plays := DistinctPlays(NewBoard().LegalPlays(Human, Dice{3, 1}))
	found := false
	for _, pl := range plays {
		if err := pl.Result.Validate(); err != nil {
			t.Fatalf("%v: %v", pl, err)
		}
		if pl.Result.C[Human][5] == 2 && pl.Result.C[Human][8] == 2 && pl.Result.C[Human][6] == 4 {
			found = true
		}
	}
	if !found {
		t.Fatalf("8/5 6/5 not among %v", playStrings(plays))
	}
}

func TestHitSendsToBar(t *testing.T) {
	b := mkBoard(map[int]int{8: 1}, map[int]int{20: 1}) // AI 20 == human 5
	m, nb, ok := b.TryMove(Human, 8, 3)
	if !ok || !m.Hit {
		t.Fatalf("expected hit, got ok=%v move=%v", ok, m)
	}
	if nb.C[AI][Bar] != 1 || nb.C[AI][20] != 0 || nb.C[Human][5] != 1 {
		t.Fatalf("bad board after hit: %+v", nb.C)
	}
}

func TestBlockedPoint(t *testing.T) {
	b := mkBoard(map[int]int{8: 1}, map[int]int{20: 2})
	if _, _, ok := b.TryMove(Human, 8, 3); ok {
		t.Fatal("moved onto a made point")
	}
}

func TestMustEnterFromBar(t *testing.T) {
	b := mkBoard(map[int]int{Bar: 1, 13: 2}, map[int]int{6: 2})
	for _, pl := range b.LegalPlays(Human, Dice{6, 5}) {
		if pl.Moves[0].From != Bar {
			t.Fatalf("play %v does not enter first", pl)
		}
	}
}

func TestClosedBoardNoMoves(t *testing.T) {
	// The human enters on its points 19-24, which are AI points 1-6.
	ai := map[int]int{1: 2, 2: 2, 3: 2, 4: 2, 5: 2, 6: 2}
	b := mkBoard(map[int]int{Bar: 1, 13: 5}, ai)
	plays := b.LegalPlays(Human, Dice{4, 2})
	if len(plays) != 1 || len(plays[0].Moves) != 0 {
		t.Fatalf("expected a single empty play, got %v", playStrings(plays))
	}
}

func TestMustUseBothDiceWhenPossible(t *testing.T) {
	// One checker on 9 with 5-2 and human point 4 blocked: 9/4 is illegal,
	// but 9/7 then 7/2 uses both dice, so that is the only legal play.
	b := mkBoard(map[int]int{9: 1}, map[int]int{21: 2}) // AI 21 == human 4
	plays := b.LegalPlays(Human, Dice{5, 2})
	got := playStrings(plays)
	if !slices.Equal(got, []string{"9/7 7/2"}) {
		t.Fatalf("got %v, want [9/7 7/2]", got)
	}
}

func TestMustUseLargerDieWhenOnlyOneFits(t *testing.T) {
	// Checker on 10 with 6-2 and human point 2 blocked: 10/8 and 10/4 are
	// each legal, but neither can be followed by the other die (both land
	// on 2), so only one die can be used and it must be the 6.
	b := mkBoard(map[int]int{10: 1}, map[int]int{23: 2})
	plays := b.LegalPlays(Human, Dice{2, 6})
	got := playStrings(plays)
	if !slices.Equal(got, []string{"10/4"}) {
		t.Fatalf("got %v, want [10/4]", got)
	}
}

func TestBearOff(t *testing.T) {
	tests := []struct {
		name  string
		human map[int]int
		from  int
		die   int
		ok    bool
	}{
		{"exact", map[int]int{4: 1, 2: 1}, 4, 4, true},
		{"overshoot from highest", map[int]int{3: 1, 1: 1}, 3, 6, true},
		{"overshoot with higher checker", map[int]int{5: 1, 3: 1}, 3, 6, false},
		{"not all home", map[int]int{7: 1, 3: 1}, 3, 3, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := mkBoard(tt.human, map[int]int{24: 2})
			m, _, ok := b.TryMove(Human, tt.from, tt.die)
			if ok != tt.ok {
				t.Fatalf("ok=%v want %v", ok, tt.ok)
			}
			if ok && m.To != Off {
				t.Fatalf("move %v did not bear off", m)
			}
		})
	}
}

func TestDoublesUseFourMoves(t *testing.T) {
	plays := NewBoard().LegalPlays(Human, Dice{4, 4})
	for _, pl := range plays {
		if len(pl.Moves) != 4 {
			t.Fatalf("play %v has %d moves", pl, len(pl.Moves))
		}
	}
}

func TestNextMovesFollowsPrefix(t *testing.T) {
	b := mkBoard(map[int]int{9: 1}, map[int]int{21: 2})
	plays := b.LegalPlays(Human, Dice{5, 2})
	first := NextMoves(plays, nil)
	if len(first) != 1 || first[0].From != 9 || first[0].To != 7 {
		t.Fatalf("first moves %v", first)
	}
	second := NextMoves(plays, first)
	if len(second) != 1 || second[0].From != 7 || second[0].To != 2 {
		t.Fatalf("second moves %v", second)
	}
	if rest := NextMoves(plays, append(first, second...)); len(rest) != 0 {
		t.Fatalf("expected turn over, got %v", rest)
	}
}

func TestWinType(t *testing.T) {
	won := mkBoard(map[int]int{}, map[int]int{13: 15})
	if k := won.WinType(Human); k != Gammon {
		t.Fatalf("got %v want gammon", k)
	}
	won = mkBoard(map[int]int{}, map[int]int{13: 14, 20: 1})
	if k := won.WinType(Human); k != Backgammon {
		t.Fatalf("got %v want backgammon", k)
	}
	won = mkBoard(map[int]int{}, map[int]int{13: 14})
	if k := won.WinType(Human); k != Single {
		t.Fatalf("got %v want single", k)
	}
}

// TestRandomGames plays many games with random legal plays, checking board
// invariants after every turn and that games terminate.
func TestRandomGames(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for g := range 300 {
		b := NewBoard()
		p := Human
		for turn := 0; ; turn++ {
			if turn > 5000 {
				t.Fatalf("game %d did not finish", g)
			}
			d := Dice{r.IntN(6) + 1, r.IntN(6) + 1}
			plays := b.LegalPlays(p, d)
			pl := plays[r.IntN(len(plays))]
			// Replaying the moves one at a time must reach the same result.
			cur := b
			for _, m := range pl.Moves {
				var ok bool
				if _, cur, ok = cur.TryMove(p, m.From, m.Die); !ok {
					t.Fatalf("replay of %v failed", pl)
				}
			}
			if cur != pl.Result {
				t.Fatalf("replay mismatch for %v", pl)
			}
			b = pl.Result
			if err := b.Validate(); err != nil {
				t.Fatalf("game %d turn %d after %v: %v", g, turn, pl, err)
			}
			if _, done := b.Winner(); done {
				break
			}
			p = p.Opponent()
		}
	}
}

// TestHeuristicBeatsRandom sanity-checks that the evaluator plays better
// than chance.
func TestHeuristicBeatsRandom(t *testing.T) {
	r := rand.New(rand.NewPCG(3, 4))
	wins := 0
	const games = 200
	for g := range games {
		b := NewBoard()
		p := Player(g % 2)
		for {
			d := Dice{r.IntN(6) + 1, r.IntN(6) + 1}
			var pl Play
			if p == AI {
				pl = RankPlays(b, p, d)[0]
			} else {
				plays := b.LegalPlays(p, d)
				pl = plays[r.IntN(len(plays))]
			}
			b = pl.Result
			if w, done := b.Winner(); done {
				if w == AI {
					wins++
				}
				break
			}
			p = p.Opponent()
		}
	}
	if wins < games*8/10 {
		t.Fatalf("heuristic won only %d/%d vs random", wins, games)
	}
	t.Logf("heuristic won %d/%d vs random", wins, games)
}
