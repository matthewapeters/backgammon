// Package history saves each game as it is played, turn by turn, so an
// unfinished game can be resumed and finished games can be looked back on.
// Games played as part of a match also belong to a Match record.
package history

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/MatthewAPeters/backgammon/internal/ai"
	"github.com/MatthewAPeters/backgammon/internal/game"
)

// fileVersion is the current history file format. Version 2 added matches.
const fileVersion = 2

// Status is where a game stands.
type Status string

const (
	InProgress Status = "in_progress"
	Won        Status = "won"  // the human won
	Lost       Status = "lost" // the persona won
	Abandoned  Status = "abandoned"
)

// Side names who made a turn or rolled the dice.
type Side string

const (
	Human Side = "human"
	AI    Side = "ai"
)

// SideOf converts a game.Player to a Side.
func SideOf(p game.Player) Side {
	if p == game.AI {
		return AI
	}
	return Human
}

// Player converts s back to a game.Player.
func (s Side) Player() game.Player {
	if s == AI {
		return game.AI
	}
	return game.Human
}

// Move is one checker movement in the mover's own numbering
// (25 is the bar, 0 is off), as in game.Move.
type Move struct {
	From int  `json:"from"`
	To   int  `json:"to"`
	Die  int  `json:"die"`
	Hit  bool `json:"hit,omitempty"`
}

// Turn is one completed turn.
type Turn struct {
	Side  Side      `json:"side"`
	Dice  game.Dice `json:"dice"`
	Moves []Move    `json:"moves"`
}

// Roll is dice that were rolled for a turn that has not been finished yet,
// kept so that quitting and resuming cannot be used to roll again.
type Roll struct {
	Side Side      `json:"side"`
	Dice game.Dice `json:"dice"`
}

// Line is one entry in the game's chat log: narration when Who is empty,
// otherwise something the persona said.
type Line struct {
	Who  string `json:"who,omitempty"`
	Text string `json:"text"`
}

// Game is the record of one game against one persona.
type Game struct {
	ID      string     `json:"id"`
	Persona ai.Persona `json:"persona"`
	Started time.Time  `json:"started"`
	Updated time.Time  `json:"updated"`
	Status  Status     `json:"status"`
	// Opening is the opening roll: the human's die, then the persona's.
	// The higher die goes first and plays both dice as its first turn.
	Opening [2]int `json:"opening"`
	Turns   []Turn `json:"turns"`
	Pending *Roll  `json:"pending,omitempty"`
	// WinKind is 1 (single), 2 (gammon) or 3 (backgammon) once the game is won or lost.
	WinKind game.WinKind `json:"win_kind,omitempty"`
	Log     []Line       `json:"log"`
	// MatchID is the match this game is part of, if any.
	MatchID string `json:"match_id,omitempty"`
}

// NewGame starts a record for a game against p with the given opening roll.
func NewGame(p ai.Persona, opening [2]int, now time.Time) *Game {
	return &Game{
		ID:      now.UTC().Format("20060102T150405.000000000Z"),
		Persona: p,
		Started: now,
		Updated: now,
		Status:  InProgress,
		Opening: opening,
	}
}

// AddTurn records a finished turn and clears the pending roll.
func (g *Game) AddTurn(s Side, d game.Dice, moves []game.Move) {
	t := Turn{Side: s, Dice: d, Moves: []Move{}}
	for _, mv := range moves {
		t.Moves = append(t.Moves, Move{From: mv.From, To: mv.To, Die: mv.Die, Hit: mv.Hit})
	}
	g.Turns = append(g.Turns, t)
	g.Pending = nil
}

// Say appends a line to the chat log.
func (g *Game) Say(who, text string) { g.Log = append(g.Log, Line{who, text}) }

// Finish marks the game won or lost.
func (g *Game) Finish(winner game.Player, kind game.WinKind) {
	g.Status = Won
	if winner == game.AI {
		g.Status = Lost
	}
	g.WinKind = kind
	g.Pending = nil
}

// Next is the side to play the next turn.
func (g *Game) Next() Side {
	if len(g.Turns) == 0 {
		if g.Opening[1] > g.Opening[0] {
			return AI
		}
		return Human
	}
	if g.Turns[len(g.Turns)-1].Side == Human {
		return AI
	}
	return Human
}

// Replay plays the first n turns from the starting position, checking every
// move against the rules.
func (g *Game) Replay(n int) (game.Board, error) {
	b := game.NewBoard()
	for i, t := range g.Turns[:n] {
		p := t.Side.Player()
		for _, mv := range t.Moves {
			got, nb, ok := b.TryMove(p, mv.From, mv.Die)
			if !ok || got.To != mv.To {
				return b, fmt.Errorf("turn %d: %s can't move %d/%d with a %d", i+1, t.Side, mv.From, mv.To, mv.Die)
			}
			b = nb
		}
	}
	return b, nil
}

// Board is the position after every recorded turn.
func (g *Game) Board() (game.Board, error) { return g.Replay(len(g.Turns)) }

// Match is a series of games against one persona, played until one side
// reaches Length points. Each game is worth 1, 2 or 3 points by its WinKind.
type Match struct {
	ID      string     `json:"id"`
	Persona ai.Persona `json:"persona"`
	Started time.Time  `json:"started"`
	Updated time.Time  `json:"updated"`
	Status  Status     `json:"status"`
	Length  int        `json:"length"`
	Stakes  ai.Stakes  `json:"stakes"`
	// Score is the points won so far, indexed by game.Player.
	Score [2]int `json:"score"`
	// Games are the IDs of the match's games, in order.
	Games []string `json:"games"`
}

// NewMatch starts a match to length points against p, for stakes s.
func NewMatch(p ai.Persona, length int, s ai.Stakes, now time.Time) *Match {
	return &Match{
		ID:      "m" + now.UTC().Format("20060102T150405.000000000Z"),
		Persona: p,
		Started: now,
		Updated: now,
		Status:  InProgress,
		Length:  length,
		Stakes:  s,
		Games:   []string{},
	}
}

// Add makes g the match's next game.
func (m *Match) Add(g *Game) {
	g.MatchID = m.ID
	m.Games = append(m.Games, g.ID)
}

// Record adds the points for a finished game and reports whether that won
// the match.
func (m *Match) Record(g *Game) (over bool) {
	if g.Status != Won && g.Status != Lost {
		return false
	}
	w := game.Human
	if g.Status == Lost {
		w = game.AI
	}
	m.Score[w] += int(g.WinKind)
	if m.Score[w] < m.Length {
		return false
	}
	m.Status = map[game.Player]Status{game.Human: Won, game.AI: Lost}[w]
	return true
}

// Record is the player's results against one persona.
type Record struct {
	// Games and Matches count wins, indexed by game.Player.
	Games, Matches [2]int
}

// Tally counts the finished games and matches against the persona named name.
func Tally(games []*Game, matches []*Match, name string) Record {
	var r Record
	for _, g := range games {
		if g.Persona.Name != name {
			continue
		}
		switch g.Status {
		case Won:
			r.Games[game.Human]++
		case Lost:
			r.Games[game.AI]++
		}
	}
	for _, m := range matches {
		if m.Persona.Name != name {
			continue
		}
		switch m.Status {
		case Won:
			r.Matches[game.Human]++
		case Lost:
			r.Matches[game.AI]++
		}
	}
	return r
}

type file struct {
	Version int      `json:"version"`
	Games   []*Game  `json:"games"`
	Matches []*Match `json:"matches,omitempty"`
}

// Store keeps every game in one JSON file, oldest first.
type Store struct {
	Path string
}

// DefaultPath is history.json in the user's config directory.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "backgammon", "history.json"), nil
}

// Load returns every saved game, or none if the file does not exist yet.
func (s *Store) Load() ([]*Game, error) {
	f, err := s.read()
	return f.Games, err
}

// LoadMatches returns every saved match.
func (s *Store) LoadMatches() ([]*Match, error) {
	f, err := s.read()
	return f.Matches, err
}

func (s *Store) read() (file, error) {
	data, err := os.ReadFile(s.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return file{}, nil
	}
	if err != nil {
		return file{}, err
	}
	var f file
	if err := json.Unmarshal(data, &f); err != nil {
		return file{}, fmt.Errorf("reading %s: %w", s.Path, err)
	}
	if f.Version > fileVersion {
		return file{}, fmt.Errorf("%s is format version %d; this build reads up to %d", s.Path, f.Version, fileVersion)
	}
	return f, nil
}

// Save writes g, replacing the saved game with the same ID. It rereads the
// file first so it never clobbers a history it could not parse.
func (s *Store) Save(g *Game) error {
	f, err := s.read()
	if err != nil {
		return err
	}
	f.Games = replace(f.Games, g, func(x *Game) bool { return x.ID == g.ID })
	return s.write(f)
}

// SaveMatch writes m, replacing the saved match with the same ID.
func (s *Store) SaveMatch(m *Match) error {
	f, err := s.read()
	if err != nil {
		return err
	}
	f.Matches = replace(f.Matches, m, func(x *Match) bool { return x.ID == m.ID })
	return s.write(f)
}

// replace swaps v in for the element matching same, or appends it.
func replace[T any](xs []T, v T, same func(T) bool) []T {
	if i := slices.IndexFunc(xs, same); i >= 0 {
		xs[i] = v
		return xs
	}
	return append(xs, v)
}

func (s *Store) write(f file) error {
	if f.Games == nil {
		f.Games = []*Game{}
	}
	f.Version = fileVersion
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return err
	}
	// Write and rename so a crash never leaves a half-written history.
	tmp := s.Path + ".tmp"
	if err := os.WriteFile(tmp, append(data, '\n'), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.Path)
}

// Latest returns the most recently updated game, and the most recently
// updated game still in progress. Either may be nil.
func Latest(games []*Game) (last, unfinished *Game) {
	for _, g := range games {
		if last == nil || !g.Updated.Before(last.Updated) {
			last = g
		}
		if g.Status == InProgress && (unfinished == nil || !g.Updated.Before(unfinished.Updated)) {
			unfinished = g
		}
	}
	return last, unfinished
}

// LatestMatch returns the most recently updated match still in progress, or nil.
func LatestMatch(matches []*Match) *Match {
	var out *Match
	for _, m := range matches {
		if m.Status == InProgress && (out == nil || !m.Updated.Before(out.Updated)) {
			out = m
		}
	}
	return out
}

// GameMoves converts t's moves back to game.Moves.
func (t Turn) GameMoves() []game.Move {
	out := make([]game.Move, len(t.Moves))
	for i, mv := range t.Moves {
		out[i] = game.Move{From: mv.From, To: mv.To, Die: mv.Die, Hit: mv.Hit}
	}
	return out
}
