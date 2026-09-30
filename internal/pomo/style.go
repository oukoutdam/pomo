package pomo

import (
	"fmt"
	"io"
	"os"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
	"golang.org/x/term"
)

type styles struct{ heading, muted, focus, rest, warning lipgloss.Style }

func colorEnabled(w io.Writer) bool {
	f, ok := w.(*os.File)
	_, noColor := os.LookupEnv("NO_COLOR")
	return ok && term.IsTerminal(int(f.Fd())) && !noColor && os.Getenv("TERM") != "dumb"
}

func newStyles(color, dark bool) styles {
	if !color {
		return styles{}
	}
	green, blue, amber, gray := "#417451", "#386F9C", "#936515", "#686868"
	if dark {
		green, blue, amber, gray = "#94B99C", "#91B6D6", "#D9B56F", "#969696"
	}
	return styles{
		heading: lipgloss.NewStyle().Bold(true),
		muted:   lipgloss.NewStyle().Foreground(lipgloss.Color(gray)),
		focus:   lipgloss.NewStyle().Foreground(lipgloss.Color(green)),
		rest:    lipgloss.NewStyle().Foreground(lipgloss.Color(blue)),
		warning: lipgloss.NewStyle().Foreground(lipgloss.Color(amber)),
	}
}

// Static commands use the terminal's default foreground for readable text on
// either background, without querying a terminal or emitting escapes to pipes.
func staticStyles(w io.Writer) styles {
	if !colorEnabled(w) {
		return styles{}
	}
	return styles{heading: lipgloss.NewStyle().Bold(true)}
}
func outputWidth(w io.Writer) int {
	if f, ok := w.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		return terminalWidth(f)
	}
	return 80
}
func wrapWords(s string, width int) []string {
	return strings.Split(ansi.Wrap(s, max(1, width), ""), "\n")
}
func printWrapped(w io.Writer, s string) {
	fmt.Fprintln(w, strings.Join(wrapWords(s, outputWidth(w)), "\n"))
}
func printHelp(w io.Writer) {
	style := staticStyles(w)
	for _, line := range strings.Split(strings.TrimSuffix(usage, "\n"), "\n") {
		if strings.HasPrefix(line, "pomo -") || strings.HasSuffix(line, ":") {
			line = style.heading.Render(line)
		}
		printWrapped(w, line)
	}
}
func onOff(value bool) string {
	if value {
		return "on"
	}
	return "off"
}

func reportStyles(color bool) styles {
	if !color {
		return styles{}
	}
	return styles{
		heading: lipgloss.NewStyle().Bold(true),
		focus:   lipgloss.NewStyle().Foreground(lipgloss.Green),
		muted:   lipgloss.NewStyle().Faint(true),
	}
}
