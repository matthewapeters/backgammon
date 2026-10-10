package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand/v2"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/MatthewAPeters/backgammon/internal/game"
)

// Client talks to a llama.cpp server's OpenAI-compatible API.
type Client struct {
	BaseURL string
	Model   string // empty means "first model the server lists"
	// MaxCandidates caps how many engine-ranked plays the model chooses from.
	MaxCandidates int
	HTTP          *http.Client
}

// NewClient returns a client for the llama.cpp server at baseURL.
func NewClient(baseURL, model string) *Client {
	return &Client{
		BaseURL:       strings.TrimRight(baseURL, "/"),
		Model:         model,
		MaxCandidates: 10,
		HTTP:          &http.Client{Timeout: 3 * time.Minute}, // first call may load the model
	}
}

// ResolveModel fills in Model from the server's model list if unset.
func (c *Client) ResolveModel(ctx context.Context) (string, error) {
	if c.Model != "" {
		return c.Model, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+"/v1/models", nil)
	if err != nil {
		return "", err
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	var list struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		return "", fmt.Errorf("decoding model list: %w", err)
	}
	if len(list.Data) == 0 {
		return "", fmt.Errorf("server at %s lists no models", c.BaseURL)
	}
	c.Model = list.Data[0].ID
	return c.Model, nil
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatRequest struct {
	Model              string         `json:"model"`
	Messages           []chatMessage  `json:"messages"`
	MaxTokens          int            `json:"max_tokens"`
	Temperature        float64        `json:"temperature"`
	ResponseFormat     map[string]any `json:"response_format,omitempty"`
	ChatTemplateKwargs map[string]any `json:"chat_template_kwargs,omitempty"`
}

var thinkRE = regexp.MustCompile(`(?s)<think>.*?</think>`)

// chat sends one system+user exchange and returns the assistant's text.
func (c *Client) chat(ctx context.Context, system, user string, schema map[string]any) (string, error) {
	return c.chatN(ctx, system, user, schema, 200)
}

// chatN is chat with a reply length limit of maxTokens.
func (c *Client) chatN(ctx context.Context, system, user string, schema map[string]any, maxTokens int) (string, error) {
	model, err := c.ResolveModel(ctx)
	if err != nil {
		return "", err
	}
	body := chatRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "system", Content: system},
			{Role: "user", Content: user},
		},
		MaxTokens:   maxTokens,
		Temperature: 0.8,
		// Reasoning models are far too slow per move with thinking on.
		ChatTemplateKwargs: map[string]any{"enable_thinking": false},
	}
	if schema != nil {
		body.ResponseFormat = map[string]any{
			"type":        "json_schema",
			"json_schema": map[string]any{"name": "reply", "schema": schema},
		}
	}
	buf, err := json.Marshal(body)
	if err != nil {
		return "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.BaseURL+"/v1/chat/completions", bytes.NewReader(buf))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
		return "", fmt.Errorf("llama.cpp returned %s: %s", resp.Status, bytes.TrimSpace(msg))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", fmt.Errorf("decoding completion: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("completion had no choices")
	}
	return strings.TrimSpace(thinkRE.ReplaceAllString(out.Choices[0].Message.Content, "")), nil
}

// Decision is the AI's chosen play for a turn.
type Decision struct {
	Play game.Play
	// Reaction is the persona's response to the human's last play, if any.
	Reaction string
	Comment  string
	// Err is set when the LLM could not be used and the heuristic chose instead.
	Err error
}

// ChooseMove picks the AI's play for roll d. review grades the human's
// previous play (nil if the AI is moving first) so the persona can react.
func (c *Client) ChooseMove(ctx context.Context, p Persona, b game.Board, d game.Dice, review *Review) Decision {
	plays := game.RankPlays(b, game.AI, d)
	cands := plays[:min(len(plays), c.MaxCandidates)]

	reply, err := c.pickWithLLM(ctx, p, b, d, review, cands)
	if err != nil {
		return Decision{Play: plays[0], Comment: pick(p.Fallback), Err: err}
	}
	return Decision{Play: cands[reply.Choice-1], Reaction: strings.TrimSpace(reply.Reaction), Comment: strings.TrimSpace(reply.Comment)}
}

type moveReply struct {
	Reaction string `json:"reaction"`
	Choice   int    `json:"choice"`
	Comment  string `json:"comment"`
}

func (c *Client) pickWithLLM(ctx context.Context, p Persona, b game.Board, d game.Dice, review *Review, cands []game.Play) (moveReply, error) {
	var sys strings.Builder
	sys.WriteString(p.Voice + "\n\n" +
		"You are playing backgammon. A rules engine has already worked out the legal plays for your " +
		"roll; your job is to pick the best one and stay in character. ")
	if review != nil {
		sys.WriteString("Reply ONLY with JSON: {\"reaction\": \"<one short in-character sentence reacting to your " +
			"opponent's last move>\", \"choice\": <option number>, \"comment\": \"<one short in-character sentence " +
			"about your own move>\"}. The reaction and comment must say different things.")
	} else {
		sys.WriteString("Reply ONLY with JSON: {\"choice\": <option number>, \"comment\": \"<one or two short " +
			"sentences, in character>\"}.")
	}

	var u strings.Builder
	u.WriteString(DescribeBoard(b))
	if review != nil {
		u.WriteString("\n" + review.prompt())
		u.WriteString("For your reaction: " + review.reactionGuidance() + "\n")
	}
	fmt.Fprintf(&u, "\nYou rolled %s.\n", d)
	if len(cands) == 1 && len(cands[0].Moves) == 0 {
		u.WriteString("You have no legal moves and must pass.\n")
	}
	u.WriteString("\nOptions:\n")
	enum := make([]int, len(cands))
	for i, pl := range cands {
		enum[i] = i + 1
		fmt.Fprintf(&u, "%d. %s%s\n", i+1, pl, playNotes(pl))
	}
	u.WriteString("\nChoose one option.")

	// A struct, not a map, so the properties keep this order in the JSON
	// schema: llama.cpp's grammar emits them in order, and the reaction
	// should come before the model commits to its own move.
	type properties struct {
		Reaction any `json:"reaction,omitempty"`
		Choice   any `json:"choice"`
		Comment  any `json:"comment"`
	}
	text200 := map[string]any{"type": "string", "maxLength": 200}
	props := properties{Choice: map[string]any{"type": "integer", "enum": enum}, Comment: text200}
	required := []string{"choice", "comment"}
	if review != nil {
		props.Reaction = text200
		required = []string{"reaction", "choice", "comment"}
	}
	schema := map[string]any{"type": "object", "properties": props, "required": required}

	var reply moveReply
	text, err := c.chat(ctx, sys.String(), u.String(), schema)
	if err != nil {
		return reply, err
	}
	if err := json.Unmarshal([]byte(extractJSON(text)), &reply); err != nil {
		return reply, fmt.Errorf("unparseable reply %q: %w", text, err)
	}
	if reply.Choice < 1 || reply.Choice > len(cands) {
		return reply, fmt.Errorf("choice %d out of range 1-%d", reply.Choice, len(cands))
	}
	return reply, nil
}

// React asks the persona for a one-line reaction to a game event, such as
// the end of the game.
func (c *Client) React(ctx context.Context, p Persona, situation string) (string, error) {
	system := p.Voice + "\n\nReply with one or two short sentences, fully in character. No quotation marks."
	return c.chat(ctx, system, situation, nil)
}

// GeneratePersona has the LLM write a new opponent from the player's
// description, which may be just a name or a paragraph about the character.
func (c *Client) GeneratePersona(ctx context.Context, description string) (Persona, error) {
	example, err := json.Marshal(Personas[3])
	if err != nil {
		return Persona{}, err
	}
	system := "You create opponent characters for a backgammon game. The player describes a character; " +
		"it may be just a name or a longer description. Invent a vivid, playful character that fits it.\n" +
		"- name: a short display name, 1-4 words in Title Case. If the player gave a name, use it.\n" +
		"- tagline: one witty line, under 60 characters.\n" +
		"- voice: 2-4 sentences in the second person (\"You are ...\") saying who the character is and how " +
		"they talk while playing backgammon: attitude, vocabulary, catchphrases. These become the " +
		"character's instructions.\n" +
		"- fallback: three short in-character lines to say after making a move.\n" +
		"- win: one line for winning the game. lose: one line for losing it.\n" +
		"Keep it fun and PG-13: no slurs, hate or sexual content.\n\n" +
		"Example:\n" + string(example)

	str := func(n int) map[string]any { return map[string]any{"type": "string", "minLength": 1, "maxLength": n} }
	lines := func(n int) map[string]any {
		return map[string]any{"type": "array", "items": str(150), "minItems": n, "maxItems": n}
	}
	// A struct keeps the properties in a natural writing order.
	type properties struct {
		Name     any `json:"name"`
		Tagline  any `json:"tagline"`
		Voice    any `json:"voice"`
		Fallback any `json:"fallback"`
		Win      any `json:"win"`
		Lose     any `json:"lose"`
	}
	schema := map[string]any{
		"type":       "object",
		"properties": properties{str(40), str(80), str(700), lines(3), lines(1), lines(1)},
		"required":   []string{"name", "tagline", "voice", "fallback", "win", "lose"},
	}

	text, err := c.chatN(ctx, system, "Character: "+description, schema, 800)
	if err != nil {
		return Persona{}, err
	}
	var p Persona
	if err := json.Unmarshal([]byte(extractJSON(text)), &p); err != nil {
		return Persona{}, fmt.Errorf("unparseable persona %q: %w", text, err)
	}
	p.Name = strings.Join(strings.Fields(p.Name), " ")
	p.Tagline = strings.TrimSpace(p.Tagline)
	p.Voice = strings.TrimSpace(p.Voice)
	p.Fallback, p.Win, p.Lose = cleanLines(p.Fallback), cleanLines(p.Win), cleanLines(p.Lose)
	p.Prompt = description
	if p.Name == "" || p.Voice == "" {
		return Persona{}, fmt.Errorf("persona is missing a name or voice: %q", text)
	}
	return p, nil
}

func cleanLines(in []string) []string {
	var out []string
	for _, s := range in {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

// Warmup asks the server to load the model so the first real move is fast.
func (c *Client) Warmup(ctx context.Context) error {
	_, err := c.chat(ctx, "Reply with the word ready.", "ready?", nil)
	return err
}

// DescribeBoard renders the position as text from the AI's point of view.
func DescribeBoard(b game.Board) string {
	var s strings.Builder
	s.WriteString("Point numbers are in YOUR direction of travel: you move your checkers from 24 down to 1 " +
		"and bear off from your home board (points 1-6). Your opponent moves the opposite way, " +
		"and their home board is your points 19-24.\n")
	var mine, theirs []string
	for r := 24; r >= 1; r-- {
		if n := b.C[game.AI][r]; n > 0 {
			mine = append(mine, fmt.Sprintf("%d:%d", r, n))
		}
		if n := b.OpponentAt(game.AI, r); n > 0 {
			theirs = append(theirs, fmt.Sprintf("%d:%d", r, n))
		}
	}
	fmt.Fprintf(&s, "Your checkers (point:count): %s\n", strings.Join(mine, " "))
	fmt.Fprintf(&s, "Opponent checkers (point:count, your numbering): %s\n", strings.Join(theirs, " "))
	fmt.Fprintf(&s, "On the bar: you %d, opponent %d. Borne off: you %d, opponent %d.\n",
		b.C[game.AI][game.Bar], b.C[game.Human][game.Bar], b.C[game.AI][game.Off], b.C[game.Human][game.Off])
	fmt.Fprintf(&s, "Pip count: you %d, opponent %d (lower is ahead in the race).\n",
		b.PipCount(game.AI), b.PipCount(game.Human))
	return s.String()
}

// describeOpponentPlay converts a human play to the AI's numbering.
func describeOpponentPlay(pl game.Play) string {
	if len(pl.Moves) == 0 {
		return "they had no legal move and passed"
	}
	label := func(r int) string {
		switch r {
		case game.Bar:
			return "bar"
		case game.Off:
			return "off"
		}
		return fmt.Sprint(25 - r)
	}
	var parts []string
	hit := false
	for _, m := range pl.Moves {
		s := label(m.From) + "/" + label(m.To)
		if m.Hit {
			s += "*"
			hit = true
		}
		parts = append(parts, s)
	}
	out := strings.Join(parts, " ")
	if hit {
		out += " (they hit your checker!)"
	}
	return out
}

// playNotes adds plain-language hints the model can riff on.
func playNotes(pl game.Play) string {
	var notes []string
	hits, off := 0, 0
	for _, m := range pl.Moves {
		if m.Hit {
			hits++
		}
		if m.To == game.Off {
			off++
		}
	}
	if hits > 0 {
		notes = append(notes, fmt.Sprintf("hits %d", hits))
	}
	if off > 0 {
		notes = append(notes, fmt.Sprintf("bears off %d", off))
	}
	blots := 0
	for r := 1; r <= 24; r++ {
		if pl.Result.C[game.AI][r] == 1 {
			blots++
		}
	}
	if blots > 0 {
		notes = append(notes, fmt.Sprintf("leaves %d blot(s)", blots))
	}
	if len(notes) == 0 {
		return ""
	}
	return "  (" + strings.Join(notes, ", ") + ")"
}

// extractJSON trims any prose around a JSON object.
func extractJSON(s string) string {
	i, j := strings.Index(s, "{"), strings.LastIndex(s, "}")
	if i >= 0 && j > i {
		return s[i : j+1]
	}
	return s
}

func pick(lines []string) string {
	if len(lines) == 0 {
		return ""
	}
	return lines[rand.IntN(len(lines))]
}

// FallbackReaction returns a canned end-of-game line.
func FallbackReaction(p Persona, aiWon bool) string {
	if aiWon {
		return pick(p.Win)
	}
	return pick(p.Lose)
}
