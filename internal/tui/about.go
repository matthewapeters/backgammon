package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// aboutSection is one tab of the about page. Text is in a tiny markup:
// paragraphs separated by blank lines, "# " headings, "- " bullets (one per
// line), "| a | b |" tables (one row per line, the first is the header; only
// the last column wraps), *bold* and `code`.
type aboutSection struct{ title, text string }

var aboutSections = []aboutSection{
	{"History", `# Five thousand years of racing games

Backgammon belongs to the oldest family of board games there is: race games, where dice decide how far your pieces run and the skill lies in choosing which pieces to move. The Royal Game of Ur, played in Mesopotamia around 2600 BCE, and Egypt's Senet are early relatives.

The Romans played *Tabula* on a board of 24 points with 15 pieces a side, which is essentially the modern game. A position from a game played by the emperor Zeno around 480 CE, in which a single unlucky roll wrecked his board, was recorded by the historian Agathias. It is often called the first written backgammon problem.

In Persia the game is *Nard*. Legend credits the sage Bozorgmehr with inventing it to answer a challenge posed by chess, which had just arrived from India: chess was a game of pure reason, nard a game of reason under fate.

Medieval Europe knew the game simply as *tables*. The name *backgammon* appears in English in the 1600s, probably from "back game", because checkers that get hit have to go back and start again. Edmond Hoyle published rules and strategy for it in 1743.

# The modern game

The *doubling cube* was invented in New York clubs in the 1920s and turned backgammon into a serious gambling game. A boom followed in the 1960s and 70s, when tournaments became glamorous social events; the World Championship has been held in Monte Carlo since 1979.

Computers changed the game in the 1990s. Gerald Tesauro's *TD-Gammon* at IBM taught itself by playing millions of games against itself, and reached the level of the best humans. It overturned long-held opening theory, and its successors, such as GNU Backgammon and eXtreme Gammon, are now how players study.

This game has no doubling cube: each game is worth 1, 2 or 3 points depending on how it ends.`},

	{"Terms", `# The board

- *Point*: one of the 24 triangles. You move from your 24-point down to your 1-point.
- *Home board*: your points 1 to 6. All 15 of your checkers must be there before you can bear off.
- *Outer board*: points 7 to 12. The *bar point* is your 7-point, next to the bar.
- *Bar*: the strip down the middle, where hit checkers wait.
- *Pip*: one point of movement. Your *pip count* is the total distance all your checkers still have to travel; the side panel shows both counts, and the lower one is ahead in the race.

# Moving

- *Opening roll*: each side rolls one die. The higher die goes first and plays both dice.
- *Doubles*: rolling two of the same number lets you move that number four times.
- You must use both dice if you can. If only one can be played, you must play the larger if possible.
- *Blot*: a single checker alone on a point. It can be hit.
- *Hit*: landing on an opponent's blot sends it to the bar.
- *Entering*: a checker on the bar must come back in, in the opponent's home board, before you move anything else.
- *Made point*: two or more of your checkers on a point. The opponent can't land there.
- *Anchor*: a point you have made in your opponent's home board, a safe landing spot.
- *Prime*: several made points in a row. Six in a row is a full prime, and nothing can get past it.
- *Builder*: a checker placed where it can help make a new point next turn.
- *Slot*: deliberately leaving a blot on a point you want to make, hoping to cover it next turn.

# Winning

- *Bearing off*: once all your checkers are home, you remove them from the board. First to bear off all 15 wins.
- *Single game*: the loser has borne off at least one checker. Worth 1 point.
- *Gammon*: the loser has borne off nothing. Worth 2.
- *Backgammon*: a gammon where the loser also still has a checker on the bar or in the winner's home board. Worth 3.
- *Match*: a series of games played to 3, 5, 7, 9 or 11 points. Each game's points add up, and the first to reach the target wins the match. Before a match your opponent names the stakes: something of theirs against something of yours. The stakes are just for fun.`},

	{"Engine", `# The rules engine

The game's rules live in ` + "`internal/game`" + `. For every roll it generates every legal play: each order of the dice, and for doubles all four moves. It keeps only the plays that use as many dice as possible, applies the "play the larger die" rule, and collapses plays that reach the same position by different routes.

# The heuristic

Each resulting position gets a score from a hand-tuned evaluation function: it adds up the race, made points, primes and the safety of every checker, and plays are ranked by the total. It is quick and opinionated rather than strong. The *Strategy* tab shows what it values and why.

# Grading your moves

When you finish a turn, the engine ranks your play against every alternative and grades it by how far it fell short of its favorite: *forced*, *best*, *solid*, *questionable* or *blunder*. It also notes what the play did, such as hitting, making a point, leaving blots or bearing off. Your opponent hears about that grade, which is where its taunts and compliments come from.

The heuristic is far from perfect. It will sometimes call a good move a blunder, much like a human opponent who is sure of themselves.`},

	{"Strategy", `# What the engine values

The engine scores every position it could reach and plays the highest. The units are arbitrary: what matters is how the terms compare. An average roll moves about 8 pips, which is worth 3.3.

# Points

A point you hold (two or more checkers) blocks your opponent. Its worth depends on where it is:

| Point | Worth | Why
| 1 (ace) | ▪ 0.5 | Checkers here are out of play for the rest of the game.
| 2 | ▪▪ 1 | Nearly as deep. Mostly good for closing out a hit checker.
| 3 | ▪▪▪▪ 2 | Useful, but builders do more good higher up.
| 4 | ▪▪▪▪▪▪▪ 3.5 | A golden point: blocks the opponent's back checkers where they need to run.
| 5 | ▪▪▪▪▪▪▪ 3.5 | The other golden point, and the one both sides fight for.
| 6 | ▪▪▪▪▪▪ 3 | Where the game starts you, and the anchor of most primes.
| 7 (bar) | ▪▪▪▪▪ 2.5 | Stops back checkers escaping with a 6.
| 8 to 24 | ▪▪ 1 | Safe landing spots. In the opponent's home board, an anchor.

While your opponent has a checker on the bar, every home point is worth at least 3, since each one blocks an entry and holding all six shuts them out.

# Strategies

| Strategy | The engine's rule
| Build primes | Three or more points in a row earn a bonus that grows with length: 2.4 for three, 14.4 for a full six-point prime.
| Don't stack | -0.3 for each checker beyond the third on a point.
| Don't bury | -1 for each checker beyond the second on your 1-, 2- or 3-point, unless the opponent is on the bar.
| Stay safe | Each blot costs its chance of being hit (counted over all 36 rolls) times the ground it would lose: about 6 for a back checker, up to 14 deep in your home board.
| Escape early | Each back checker costs the square of the opponent's points in the six in front of it: 1 for one point, 9 for three, 36 for a full prime.
| Hit | +4 for each opponent checker on the bar.
| Race | +0.4 for each pip you lead by.
| Bear off | +1.5 for each checker off.

# Saving the gammon

Once the two sides are past each other, points and blots stop mattering and the engine plays a pure race. If it hasn't borne off a checker, it estimates how many turns it needs to bring every checker home and bear one off, compares that with the turns the opponent needs to finish, and weighs the chance of losing a gammon (up to 20) and, if a checker is still in the opponent's home board, a backgammon (another 20).

Moving checkers within your own home board does nothing to save a gammon, so in a lost race the engine brings the stragglers home first.`},

	{"The AI", `# Who decides what

Your opponent is two things working together: the rules engine, which knows backgammon, and a language model running locally under llama.cpp, which plays the character.

The model never invents moves. Each turn, the engine works out the legal plays and ranks them, and the model receives:

- the board described from its own point of view,
- the engine's grade of your last play,
- its roll, and up to 10 of the engine's top plays as numbered options, best first, with notes such as "hits 1" or "leaves 2 blots". Only plays the engine grades as *solid*, within 3 of its favorite, are offered.

It replies in character with a reaction to your move, the number of the play it chooses, and a comment on its own move. The reply is constrained by a JSON schema, so it always comes back in a form the game can read.

The model is asked to choose the best play, but it has opinions. A trash-talking sailor may well prefer the option that hits, and that is part of the fun. Since every option is a solid one, its taste can cost it a little but never a blunder.

# When the model isn't there

If the llama.cpp server is unreachable, slow, or replies with something unusable, the engine plays its top-ranked play and the persona says one of its canned lines. The game never waits on the model, and it is fully playable offline. The side panel shows the model's status.

At the end of a game the model writes a win or loss reaction, with canned lines as a fallback.

# Your games

Every game is saved as you play, to ` + "`~/.config/backgammon/history.json`" + `: the opening roll, every turn's dice and moves, and everything said. Dice are saved as soon as they are rolled, so quitting and coming back can't change your luck. Choose *Resume* on the main menu to pick up your last unfinished game. Use ` + "`--history <file>`" + ` or ` + "`BACKGAMMON_HISTORY`" + ` to keep it somewhere else.`},

	{"Personas", `# Making your own opponent

Choose *Choose opponent* on the main menu, then *+ New opponent…* (or press ` + "`n`" + `). Describe who you want to play against. A name like "Grumpy Cat" is enough, and you can add as much detail as you like.

The model then writes the whole character:

- *name* and *tagline*, shown on the menus,
- *voice*: two to four sentences saying who the character is and how they talk. This becomes the model's instructions every turn, so it shapes everything the character says,
- *fallback*: three lines for when the model is unavailable,
- *win* and *lose*: canned reactions to the end of a game.

# Editing by hand

New opponents are saved to ` + "`~/.config/backgammon/personas.json`" + `. Edit the file to tweak a character, or add one entirely by hand; the game reads it at startup. Use ` + "`--personas <file>`" + ` or ` + "`BACKGAMMON_PERSONAS`" + ` to use a different file.

Tips for a good voice:

- Write it in the second person: "You are a retired chess champion who…"
- Say how they talk: vocabulary, catchphrases, attitude to winning and losing.
- Keep it short. A few vivid sentences work better than a page.

The six built-in personas live in the code and are never written to the file. The opponent you played last is the one waiting for you on the main menu.`},
}

// aboutTextW is the width of the about page's text.
const aboutTextW = innerW - 12

// aboutHeight is how many lines of text the about page shows at once.
func (m Model) aboutHeight() int {
	if m.height <= 0 {
		return 20
	}
	return min(max(m.height-10, 6), 40)
}

func (m Model) keyAbout(k string) (tea.Model, tea.Cmd) {
	n := len(aboutSections)
	h := m.aboutHeight()
	maxScroll := max(0, len(aboutLines(aboutSections[m.about.tab].text, aboutTextW))-h)
	switch k {
	case "esc", "m", "backspace":
		return m.toMenu(), nil
	case "right", "l", "tab":
		m.about.tab, m.about.scroll = (m.about.tab+1)%n, 0
	case "left", "h", "shift+tab":
		m.about.tab, m.about.scroll = (m.about.tab+n-1)%n, 0
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		if i := int(k[0] - '1'); i < n {
			m.about.tab, m.about.scroll = i, 0
		}
	case "down", "j":
		m.about.scroll++
	case "up", "k":
		m.about.scroll--
	case "pgdown", " ", "space", "f":
		m.about.scroll += h - 1
	case "pgup", "b":
		m.about.scroll -= h - 1
	case "home", "g":
		m.about.scroll = 0
	case "end", "G":
		m.about.scroll = maxScroll
	}
	m.about.scroll = min(max(m.about.scroll, 0), maxScroll)
	return m, nil
}

func (m Model) viewAbout() string {
	var tabs []string
	for i, s := range aboutSections {
		if i == m.about.tab {
			tabs = append(tabs, chipStyle.Render(" "+s.title+" "))
		} else {
			tabs = append(tabs, inkStyle.Render(" "+s.title+" "))
		}
	}
	lines := aboutLines(aboutSections[m.about.tab].text, aboutTextW)
	h := m.aboutHeight()
	first := min(m.about.scroll, max(0, len(lines)-h))

	ls := []string{feltBlank(), feltCenter(strings.Join(tabs, feltBg.Render("  "))), feltBlank()}
	for i := first; i < first+h; i++ {
		if i < len(lines) {
			ls = append(ls, feltLeft(lines[i], 6))
		} else {
			ls = append(ls, feltBlank())
		}
	}
	more := ""
	switch {
	case first > 0 && first+h < len(lines):
		more = "▲ ▼ more"
	case first > 0:
		more = "▲ more"
	case first+h < len(lines):
		more = "▼ more"
	}
	ls = append(ls, feltCenter(inkDim.Render(more)))
	return m.place(lipgloss.JoinVertical(lipgloss.Left,
		inlay(ls),
		footer("←/→ section · ↑/↓ scroll · space page · esc menu"),
	))
}

// aboutLines renders section text wrapped to width w, styled for the felt.
func aboutLines(text string, w int) []string {
	var out []string
	for i, para := range strings.Split(strings.TrimSpace(text), "\n\n") {
		if i > 0 {
			out = append(out, "")
		}
		switch {
		case strings.HasPrefix(para, "# "):
			out = append(out, onFelt(pointDark).Render("◆ ")+inkBold.Render(strings.ToUpper(para[2:])))
		case strings.HasPrefix(para, "|"):
			out = append(out, tableLines(para, w)...)
		case strings.HasPrefix(para, "- "):
			for _, item := range strings.Split(para, "\n") {
				for j, l := range styleWrapped(strings.TrimPrefix(item, "- "), w-2) {
					lead := onFelt(pointDark).Render("• ")
					if j > 0 {
						lead = feltBg.Render("  ")
					}
					out = append(out, lead+l)
				}
			}
		default:
			out = append(out, styleWrapped(para, w)...)
		}
	}
	return out
}

// tableLines renders a markup table to width w. Every column but the last
// is as wide as its widest cell; the last takes the rest and wraps.
func tableLines(para string, w int) []string {
	const gap = 2
	var rows [][]string
	for _, line := range strings.Split(para, "\n") {
		cells := strings.Split(strings.Trim(strings.TrimSpace(line), "|"), "|")
		for i := range cells {
			cells[i] = strings.TrimSpace(cells[i])
		}
		rows = append(rows, cells)
	}
	cols := len(rows[0])
	widths := make([]int, cols)
	rest := w
	for c := 0; c < cols-1; c++ {
		for _, r := range rows {
			if c < len(r) {
				widths[c] = max(widths[c], lipgloss.Width(strings.NewReplacer("*", "", "`", "").Replace(r[c])))
			}
		}
		rest -= widths[c] + gap
	}
	widths[cols-1] = rest

	pad := func(s string, n int) string {
		return s + feltBg.Render(strings.Repeat(" ", max(0, n-lipgloss.Width(s))))
	}
	var out []string
	for i, r := range rows {
		cells := make([][]string, cols)
		height := 1
		for c := range cols {
			text := ""
			if c < len(r) {
				text = r[c]
			}
			if i == 0 && text != "" {
				text = "*" + text + "*"
			}
			cells[c] = styleWrapped(text, widths[c])
			height = max(height, len(cells[c]))
		}
		for l := range height {
			var b strings.Builder
			for c := range cols {
				cell := ""
				if l < len(cells[c]) {
					cell = cells[c][l]
				}
				if c < cols-1 {
					b.WriteString(pad(cell, widths[c]+gap))
				} else {
					b.WriteString(cell)
				}
			}
			out = append(out, b.String())
		}
		if i == 0 {
			out = append(out, onFelt(pointDark).Render(strings.Repeat("─", w)))
		}
	}
	return out
}

// styleWrapped word-wraps markup text to w visible cells and styles each line.
func styleWrapped(text string, w int) []string {
	visible := func(s string) int { return lipgloss.Width(strings.NewReplacer("*", "", "`", "").Replace(s)) }
	var raw []string
	line := ""
	for _, word := range strings.Fields(text) {
		switch {
		case line == "":
			line = word
		case visible(line)+1+visible(word) <= w:
			line += " " + word
		default:
			raw = append(raw, line)
			line = word
		}
	}
	if line != "" {
		raw = append(raw, line)
	}

	// Bold and code spans may cross line breaks, so carry them over.
	bold, code := false, false
	out := make([]string, len(raw))
	for i, l := range raw {
		var b strings.Builder
		run := ""
		flush := func() {
			if run == "" {
				return
			}
			st := inkStyle
			switch {
			case code:
				st = codeStyle
			case bold:
				st = inkBold
			}
			b.WriteString(st.Render(run))
			run = ""
		}
		for _, r := range l {
			switch r {
			case '*':
				flush()
				bold = !bold
			case '`':
				flush()
				code = !code
			default:
				run += string(r)
			}
		}
		flush()
		out[i] = b.String()
	}
	return out
}
