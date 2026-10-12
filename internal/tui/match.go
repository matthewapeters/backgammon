package tui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/MatthewAPeters/backgammon/internal/ai"
	"github.com/MatthewAPeters/backgammon/internal/game"
	"github.com/MatthewAPeters/backgammon/internal/history"
)

// matchLengths are the choices on the challenge screen; 1 is a single game
// with nothing at stake.
var matchLengths = []int{1, 3, 5, 7, 9, 11}

// stakesMsg is the bet the persona proposed for challenge id.
type stakesMsg struct {
	id     int
	stakes ai.Stakes
	err    error
}

// challenge has m.persona offer a match and propose the stakes. Nothing is
// abandoned until the player picks a length.
func (m Model) challenge() (tea.Model, tea.Cmd) {
	m.sitDown()
	m.phase = phaseOffer
	m.offer.id++
	m.offer.ready = false
	m.offer.stakes = ai.Stakes{}
	client, persona, id := m.client, m.persona, m.offer.id
	ask := func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		s, err := client.OfferStakes(ctx, persona)
		if err != nil {
			s = ai.FallbackStakes(persona)
		}
		return stakesMsg{id, s, err}
	}
	return m, tea.Batch(ask, m.spinner.Tick)
}

func (m Model) stakesProposed(msg stakesMsg) (tea.Model, tea.Cmd) {
	if msg.id != m.offer.id || m.phase != phaseOffer {
		return m, nil
	}
	m.offer.stakes, m.offer.ready = msg.stakes, true
	if msg.err != nil {
		m.llmStatus = "offline: " + shortErr(msg.err)
	} else {
		m.llmStatus = "ready"
	}
	return m, nil
}

func (m Model) keyOffer(k string) (tea.Model, tea.Cmd) {
	n := len(matchLengths)
	switch k {
	case "up", "k", "shift+tab":
		m.offer.pick = (m.offer.pick + n - 1) % n
	case "down", "j", "tab":
		m.offer.pick = (m.offer.pick + 1) % n
	case "enter", " ", "space":
		return m.play(matchLengths[m.offer.pick])
	case "p":
		return m.chooseMenu(menuPersona)
	case "esc", "m":
		m.offer.id++ // the stakes are no longer wanted
		return m.toMenu(), nil
	}
	return m, nil
}

// play starts a match to length points against m.persona, or a single game
// if length is 1, abandoning any unfinished game or match.
func (m Model) play(length int) (tea.Model, tea.Cmd) {
	m.abandon()
	m.match = nil
	if length > 1 {
		stakes := m.offer.stakes
		if !m.offer.ready {
			// Don't keep the player waiting on the LLM.
			stakes = ai.FallbackStakes(m.persona)
		}
		m.match = history.NewMatch(m.persona, length, stakes, time.Now())
	}
	m.offer.id++
	warm := m.warmup()
	next, cmd := m.newGame()
	return next, tea.Batch(cmd, warm)
}

// abandon gives up the unfinished game and match, if any.
func (m *Model) abandon() {
	if m.rec != nil && m.rec.Status == history.InProgress {
		m.rec.Status = history.Abandoned
		m.save()
	}
	if m.midMatch() {
		m.match.Status = history.Abandoned
		m.saveMatch()
	}
}

// midMatch reports whether a match is under way, even between games.
func (m Model) midMatch() bool { return m.match != nil && m.match.Status == history.InProgress }

// voiced is m.persona, told about the match if one is being played.
func (m Model) voiced() ai.Persona {
	if m.match == nil {
		return m.persona
	}
	return m.persona.InMatch(m.match.Length, m.match.Score[game.AI], m.match.Score[game.Human], m.match.Stakes)
}

// announceMatch narrates where the match stands as a game begins.
func (m *Model) announceMatch() {
	mt := m.match
	if len(mt.Games) == 1 {
		m.say("", fmt.Sprintf("A %d-point match. The %s stakes %s; you stake %s.",
			mt.Length, m.persona.Name, mt.Stakes.Theirs, mt.Stakes.Yours))
		if mt.Stakes.Offer != "" {
			m.say(m.persona.Name, mt.Stakes.Offer)
		}
		return
	}
	m.say("", fmt.Sprintf("Game %d. The score is you %d, %s %d, playing to %d.",
		len(mt.Games), mt.Score[game.Human], shortName(m.persona), mt.Score[game.AI], mt.Length))
}

// matchResult narrates a finished match and says what the persona should
// react to.
func (m *Model) matchResult(w game.Player) (situation string) {
	mt := m.match
	mine, theirs := mt.Score[game.AI], mt.Score[game.Human]
	if w == game.Human {
		m.say("", fmt.Sprintf("You win the match %d–%d and collect the %s's stake: %s.",
			theirs, mine, m.persona.Name, mt.Stakes.Theirs))
		return fmt.Sprintf("You just LOST the %d-point backgammon match, %d to %d. You must now hand over %s "+
			"to the player, and you don't get %s. Say how much it pains you to lose it.",
			mt.Length, mine, theirs, mt.Stakes.Theirs, mt.Stakes.Yours)
	}
	m.say("", fmt.Sprintf("The %s wins the match %d–%d and collects your stake: %s.",
		m.persona.Name, mine, theirs, mt.Stakes.Yours))
	return fmt.Sprintf("You just WON the %d-point backgammon match, %d to %d. The player must now hand over %s, "+
		"and you keep %s. Gloat gleefully about collecting your winnings.",
		mt.Length, mine, theirs, mt.Stakes.Yours, mt.Stakes.Theirs)
}

// saveMatch writes the match record, like save does the game's.
func (m *Model) saveMatch() {
	if m.games == nil || m.match == nil {
		return
	}
	m.match.Updated = time.Now()
	if err := m.games.SaveMatch(m.match); err != nil {
		m.saveNote = "Not saving games: " + shortErr(err)
	}
}

// viewOffer is the challenge: the persona's bet and the choice of match.
func (m Model) viewOffer() string {
	ls := []string{feltBlank(), feltCenter(inkBold.Render(spaced("THE CHALLENGE"))), feltBlank(),
		feltCenter(inkBold.Render(m.persona.Name)),
		feltCenter(inkItalic.Render(m.persona.Tagline)),
		feltBlank(),
	}
	const textW = innerW - 16
	if !m.offer.ready {
		sp := m.spinner
		sp.Style = inkStyle
		ls = append(ls, feltCenter(sp.View()+inkDim.Render(" The "+m.persona.Name+" is thinking up the stakes…")))
		ls = append(ls, feltBlank(), feltBlank(), feltBlank())
	} else {
		s := m.offer.stakes
		if s.Offer != "" {
			for _, l := range wrapWords("“"+s.Offer+"”", textW) {
				ls = append(ls, feltCenter(inkStyle.Render(l)))
			}
			ls = append(ls, feltBlank())
		}
		label := func(t string) string { return inkDim.Render(fmt.Sprintf("%-14s", t)) }
		ls = append(ls,
			feltLeft(label(shortName(m.persona)+" stakes")+inkBold.Render(truncate(s.Theirs, textW-14)), 8),
			feltLeft(label("You stake")+inkBold.Render(truncate(s.Yours, textW-14)), 8))
	}
	ls = append(ls, feltBlank())
	for i, n := range matchLengths {
		label := fmt.Sprintf("%d-point match", n)
		if n == 1 {
			label = "Just a game · no stakes"
		}
		if i == m.offer.pick {
			ls = append(ls, feltCenter(chipStyle.Width(itemW).Render(" ▸ "+label)))
		} else {
			ls = append(ls, feltCenter(inkStyle.Width(itemW).Render("   "+label)))
		}
	}
	ls = append(ls, feltBlank())
	return m.place(lipgloss.JoinVertical(lipgloss.Left,
		inlay(ls),
		footer("↑/↓ choose · enter play · p change opponent · esc menu · q quit"),
	))
}

// truncate shortens plain text to w cells.
func truncate(s string, w int) string {
	if lipgloss.Width(s) <= w {
		return s
	}
	r := []rune(s)
	for lipgloss.Width(string(r))+1 > w {
		r = r[:len(r)-1]
	}
	return string(r) + "…"
}
