package ai

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MatthewAPeters/backgammon/internal/game"
)

func TestReviewGradesBestAndWorst(t *testing.T) {
	b := game.NewBoard()
	d := game.Dice{3, 1}
	ranked := game.RankPlays(b, game.Human, d)

	if r := ReviewHumanPlay(b, d, ranked[0]); r.Verdict != VerdictBest || r.Rank != 1 {
		t.Fatalf("top play graded %v rank %d", r.Verdict, r.Rank)
	}
	worst := ranked[len(ranked)-1]
	r := ReviewHumanPlay(b, d, worst)
	if r.Rank != len(ranked) || r.Verdict == VerdictBest || r.Gap <= 0 {
		t.Fatalf("worst play graded %v rank %d/%d gap %.2f", r.Verdict, r.Rank, r.Total, r.Gap)
	}
}

func TestReviewForcedAndFacts(t *testing.T) {
	// A human checker on 9 hits an AI blot on human point 7 (AI point 18).
	var b game.Board
	b.C[game.Human][9] = 1
	b.C[game.Human][game.Off] = 14
	b.C[game.AI][18] = 1
	b.C[game.AI][game.Off] = 14
	_, nb, ok := b.TryMove(game.Human, 9, 2)
	if !ok {
		t.Fatal("setup move failed")
	}
	pl := game.Play{Moves: []game.Move{{From: 9, To: 7, Die: 2, Hit: true}}, Result: nb}
	r := ReviewHumanPlay(b, game.Dice{2, 2}, pl)
	if !strings.Contains(strings.Join(r.Facts, ";"), "hit 1") {
		t.Fatalf("facts %v missing hit", r.Facts)
	}

	// On the bar against a closed board there is no play at all.
	var closed game.Board
	closed.C[game.Human][game.Bar] = 1
	closed.C[game.Human][game.Off] = 14
	for r := 1; r <= 6; r++ {
		closed.C[game.AI][r] = 2
	}
	closed.C[game.AI][game.Off] = 3
	if r := ReviewHumanPlay(closed, game.Dice{6, 6}, game.Play{Result: closed}); r.Verdict != VerdictForced {
		t.Fatalf("closed-board pass graded %v", r.Verdict)
	}
	if !strings.Contains((&Review{Dice: game.Dice{6, 6}}).prompt(), "could not move") {
		t.Fatal("pass prompt should say they could not move")
	}
}

func TestChooseMoveReturnsReaction(t *testing.T) {
	var gotSchema json.RawMessage
	var gotUser string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" {
			w.Write([]byte(`{"data":[{"id":"m"}]}`))
			return
		}
		var req struct {
			Messages       []chatMessage `json:"messages"`
			ResponseFormat struct {
				JSONSchema struct {
					Schema json.RawMessage `json:"schema"`
				} `json:"json_schema"`
			} `json:"response_format"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		gotSchema = req.ResponseFormat.JSONSchema.Schema
		gotUser = req.Messages[1].Content
		reply := `{"reaction": "Ye left a blot, love!", "choice": 1, "comment": "Mine now."}`
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": reply}}}})
	}))
	defer srv.Close()

	b := game.NewBoard()
	d := game.Dice{6, 5}
	ranked := game.RankPlays(b, game.Human, d)
	worst := ranked[len(ranked)-1]
	review := ReviewHumanPlay(b, d, worst)

	dec := NewClient(srv.URL, "").ChooseMove(context.Background(), Personas[5], worst.Result, game.Dice{3, 1}, review)
	if dec.Err != nil {
		t.Fatal(dec.Err)
	}
	if dec.Reaction != "Ye left a blot, love!" || dec.Comment != "Mine now." {
		t.Fatalf("got reaction %q comment %q", dec.Reaction, dec.Comment)
	}
	s := string(gotSchema)
	if i, j := strings.Index(s, `"reaction"`), strings.Index(s, `"choice"`); i < 0 || i > j {
		t.Fatalf("reaction should precede choice in schema: %s", s)
	}
	if !strings.Contains(gotUser, "Verdict:") || !strings.Contains(gotUser, "6-5") {
		t.Fatalf("prompt missing review:\n%s", gotUser)
	}
}

func TestNoReviewOmitsReaction(t *testing.T) {
	srv := fakeServer(t, http.StatusOK, `{"choice": 1, "comment": "Arr!"}`)
	dec := NewClient(srv.URL, "").ChooseMove(context.Background(), Personas[3], game.NewBoard(), game.Dice{3, 1}, nil)
	if dec.Err != nil || dec.Reaction != "" {
		t.Fatalf("err %v reaction %q", dec.Err, dec.Reaction)
	}
}
