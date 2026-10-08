package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/MatthewAPeters/backgammon/internal/game"
)

// fakeServer serves a model list and answers every chat completion with reply.
func fakeServer(t *testing.T, status int, reply string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/models":
			fmt.Fprint(w, `{"data":[{"id":"test-model"}]}`)
		case "/v1/chat/completions":
			var req chatRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				t.Errorf("bad request body: %v", err)
			}
			if req.Model != "test-model" {
				t.Errorf("model %q, want test-model", req.Model)
			}
			if status != http.StatusOK {
				http.Error(w, "boom", status)
				return
			}
			resp := map[string]any{"choices": []any{map[string]any{"message": map[string]any{"content": reply}}}}
			json.NewEncoder(w).Encode(resp)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestChooseMoveUsesLLMChoice(t *testing.T) {
	srv := fakeServer(t, http.StatusOK, `{"choice": 2, "comment": "Arr!"}`)
	c := NewClient(srv.URL, "")
	b := game.NewBoard()
	d := game.Dice{3, 1}
	dec := c.ChooseMove(context.Background(), Personas[3], b, d, nil)
	if dec.Err != nil {
		t.Fatal(dec.Err)
	}
	want := game.RankPlays(b, game.AI, d)[1]
	if dec.Play.Result != want.Result || dec.Comment != "Arr!" {
		t.Fatalf("got %v %q, want %v", dec.Play, dec.Comment, want)
	}
}

func TestChooseMoveFallsBack(t *testing.T) {
	tests := []struct {
		name   string
		status int
		reply  string
	}{
		{"server error", http.StatusInternalServerError, ""},
		{"garbage", http.StatusOK, "I like turtles"},
		{"out of range", http.StatusOK, `{"choice": 99, "comment": "hmm"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := fakeServer(t, tt.status, tt.reply)
			c := NewClient(srv.URL, "")
			b := game.NewBoard()
			d := game.Dice{6, 5}
			dec := c.ChooseMove(context.Background(), Personas[0], b, d, nil)
			if dec.Err == nil {
				t.Fatal("expected an error")
			}
			if dec.Play.Result != game.RankPlays(b, game.AI, d)[0].Result {
				t.Fatalf("fallback did not use best heuristic play: %v", dec.Play)
			}
			if dec.Comment == "" {
				t.Fatal("fallback comment empty")
			}
		})
	}
}

func TestDescribeOpponentPlayMirrors(t *testing.T) {
	pl := game.Play{Moves: []game.Move{{From: game.Bar, To: 20, Hit: true}, {From: 6, To: game.Off}}}
	got := describeOpponentPlay(pl)
	if !strings.HasPrefix(got, "bar/5* 19/off") {
		t.Fatalf("got %q", got)
	}
}

func TestExtractJSON(t *testing.T) {
	if got := extractJSON("sure! {\"a\":1} hope that helps"); got != `{"a":1}` {
		t.Fatalf("got %q", got)
	}
}
