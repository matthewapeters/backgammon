// Command backgammon is a terminal backgammon game against a local LLM.
package main

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"

	"github.com/MatthewAPeters/backgammon/internal/ai"
	"github.com/MatthewAPeters/backgammon/internal/tui"
)

func main() {
	url := flag.String("llm-url", envOr("BACKGAMMON_LLM_URL", "http://localhost:8888"), "llama.cpp server base URL")
	model := flag.String("model", os.Getenv("BACKGAMMON_MODEL"), "model name (default: first model the server lists)")
	personas := flag.String("personas", envOr("BACKGAMMON_PERSONAS", defaultPersonaPath()), "file holding the opponents you create")
	color := flag.String("color", envOr("BACKGAMMON_COLOR", "truecolor"), "color mode: truecolor, 256, 16, or auto (trust $TERM/$COLORTERM)")
	flag.Parse()

	// Many terminals that support 24-bit color still report TERM=xterm,
	// which makes auto-detection fall back to 16 colors and wreck the palette.
	switch *color {
	case "truecolor":
		lipgloss.SetColorProfile(termenv.TrueColor)
	case "256":
		lipgloss.SetColorProfile(termenv.ANSI256)
	case "16":
		lipgloss.SetColorProfile(termenv.ANSI)
	case "auto":
	default:
		fmt.Fprintf(os.Stderr, "unknown --color %q (want truecolor, 256, 16 or auto)\n", *color)
		os.Exit(2)
	}

	client := ai.NewClient(*url, *model)
	var store *ai.PersonaStore
	if *personas != "" {
		store = &ai.PersonaStore{Path: *personas}
	}
	if _, err := tea.NewProgram(tui.New(client, store), tea.WithAltScreen()).Run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

// defaultPersonaPath is "" (custom opponents not saved) if there is no config directory.
func defaultPersonaPath() string {
	p, err := ai.DefaultPersonaPath()
	if err != nil {
		return ""
	}
	return p
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
