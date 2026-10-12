package history

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/MatthewAPeters/backgammon/internal/ai"
	"github.com/MatthewAPeters/backgammon/internal/game"
)

// playGame plays n turns with the engine's top choices, recording them in g.
func playGame(t *testing.T, g *Game, n int) game.Board {
	t.Helper()
	b := game.NewBoard()
	side := g.Next()
	for range n {
		p := side.Player()
		d := game.RollDice()
		pl := game.RankPlays(b, p, d)[0]
		g.AddTurn(side, d, pl.Moves)
		b = pl.Result
		if _, won := b.Winner(); won {
			break
		}
		side = g.Next()
	}
	return b
}

func TestReplayRebuildsBoard(t *testing.T) {
	for range 20 {
		g := NewGame(ai.Personas[0], [2]int{3, 5}, time.Now())
		want := playGame(t, g, 60)
		got, err := g.Board()
		if err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Fatalf("replayed board differs from the played one")
		}
	}
}

func TestReplayRejectsIllegalMove(t *testing.T) {
	g := NewGame(ai.Personas[0], [2]int{5, 3}, time.Now())
	g.Turns = []Turn{{Side: Human, Dice: game.Dice{5, 3}, Moves: []Move{{From: 3, To: 0, Die: 3}}}}
	if _, err := g.Board(); err == nil {
		t.Fatal("bearing off from the start position was accepted")
	}
}

func TestNextFollowsOpeningThenAlternates(t *testing.T) {
	g := NewGame(ai.Personas[0], [2]int{2, 6}, time.Now())
	if g.Next() != AI {
		t.Fatal("persona rolled higher but doesn't go first")
	}
	g.AddTurn(AI, game.Dice{6, 2}, nil)
	if g.Next() != Human {
		t.Fatal("turns don't alternate")
	}
}

func TestStoreRoundTripAndLatest(t *testing.T) {
	s := &Store{Path: filepath.Join(t.TempDir(), "sub", "history.json")}
	if games, err := s.Load(); err != nil || games != nil {
		t.Fatalf("missing file: %v, %v", games, err)
	}
	t0 := time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC)
	a := NewGame(ai.Personas[0], [2]int{4, 1}, t0)
	b := NewGame(ai.Personas[3], [2]int{1, 4}, t0.Add(time.Minute))
	playGame(t, a, 6)
	a.Say("", "You sit down.")
	a.Say(a.Persona.Name, "So sorry, boss.")
	for _, g := range []*Game{a, b} {
		if err := s.Save(g); err != nil {
			t.Fatal(err)
		}
	}
	b.Finish(game.Human, game.Gammon)
	b.Updated = t0.Add(2 * time.Minute)
	if err := s.Save(b); err != nil {
		t.Fatal(err)
	}

	games, err := s.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(games) != 2 {
		t.Fatalf("got %d games, want 2 (saving again must replace)", len(games))
	}
	last, unfinished := Latest(games)
	if last.ID != b.ID || unfinished == nil || unfinished.ID != a.ID {
		t.Fatalf("latest %v unfinished %v", last.ID, unfinished)
	}
	if len(unfinished.Log) != 2 || unfinished.Log[1].Who != a.Persona.Name {
		t.Fatalf("log not saved: %+v", unfinished.Log)
	}
	want, _ := a.Board()
	if got, err := unfinished.Board(); err != nil || got != want {
		t.Fatalf("loaded game replays differently: %v", err)
	}
	if last.Status != Won || last.WinKind != game.Gammon {
		t.Fatalf("finished game: %v %v", last.Status, last.WinKind)
	}
}

func TestSaveRefusesToClobberUnreadableFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "history.json")
	if err := os.WriteFile(path, []byte("not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := &Store{Path: path}
	if err := s.Save(NewGame(ai.Personas[0], [2]int{2, 1}, time.Now())); err == nil {
		t.Fatal("saved over a file it couldn't read")
	}
	if data, _ := os.ReadFile(path); string(data) != "not json" {
		t.Fatal("file was changed")
	}
}

func TestMatchScoresGamesUntilLength(t *testing.T) {
	t0 := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	m := NewMatch(ai.Personas[1], 3, ai.Stakes{Theirs: "rum", Yours: "boots"}, t0)
	g1 := NewGame(m.Persona, [2]int{3, 1}, t0)
	m.Add(g1)
	g1.Finish(game.AI, game.Single)
	if m.Record(g1) || m.Score != [2]int{0, 1} || g1.MatchID != m.ID {
		t.Fatalf("after one game: score %v status %v", m.Score, m.Status)
	}
	g2 := NewGame(m.Persona, [2]int{1, 3}, t0.Add(time.Minute))
	m.Add(g2)
	if m.Record(g2) {
		t.Fatal("an unfinished game ended the match")
	}
	g2.Finish(game.Human, game.Backgammon)
	if !m.Record(g2) || m.Status != Won || m.Score != [2]int{3, 1} {
		t.Fatalf("score %v status %v, want the human to win 3-1", m.Score, m.Status)
	}

	s := &Store{Path: filepath.Join(t.TempDir(), "history.json")}
	for _, g := range []*Game{g1, g2} {
		if err := s.Save(g); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SaveMatch(m); err != nil {
		t.Fatal(err)
	}
	open := NewMatch(ai.Personas[1], 5, ai.Stakes{}, t0.Add(time.Hour))
	if err := s.SaveMatch(open); err != nil {
		t.Fatal(err)
	}
	// Saving a game again must keep the matches.
	if err := s.Save(g1); err != nil {
		t.Fatal(err)
	}
	games, _ := s.Load()
	matches, err := s.LoadMatches()
	if err != nil || len(matches) != 2 || matches[0].Games[1] != g2.ID || matches[0].Stakes.Yours != "boots" {
		t.Fatalf("matches %+v, err %v", matches, err)
	}
	if got := LatestMatch(matches); got == nil || got.ID != open.ID {
		t.Fatalf("latest open match %v", got)
	}
	r := Tally(games, matches, ai.Personas[1].Name)
	if r.Games != [2]int{1, 1} || r.Matches != [2]int{1, 0} {
		t.Fatalf("record %+v", r)
	}
	if r := Tally(games, matches, "Nobody"); r != (Record{}) {
		t.Fatalf("record against nobody %+v", r)
	}
}
